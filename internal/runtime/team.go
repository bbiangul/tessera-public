package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Who may answer a task (RFC-0006, backoffice-001). A queue with roles is
// answered only by Team members holding one of them; its tasks, once
// escalated or forwarded, also by the members of the queues they reached.
// A queue without roles is answered by whoever the channel trusts, as
// before.

// ErrNotAllowed means the person may not answer or forward this task.
var ErrNotAllowed = errors.New("not allowed to answer this task")

// team is a namespace's members and how they prove who they are.
type team struct {
	members map[string]agents.Member
	verify  *agents.Verify
}

func (p *Program) compileTeams(docs []*agents.Team, queues []*agents.HumanQueue) {
	p.teams = map[string]*team{}
	p.queueRoles = map[string][]string{}
	p.queueChannels = map[string][]string{}
	for _, d := range docs {
		t := p.teams[d.Metadata.Namespace]
		if t == nil {
			t = &team{members: map[string]agents.Member{}}
			p.teams[d.Metadata.Namespace] = t
		}
		for _, m := range d.Spec.Members {
			t.members[m.ID] = m
		}
		if d.Spec.Verify != nil {
			t.verify = d.Spec.Verify
		}
	}
	for _, q := range queues {
		p.queueRoles[q.Metadata.Namespace+"/"+q.Metadata.Name] = q.Spec.Roles
		p.queueChannels[q.Metadata.Namespace+"/"+q.Metadata.Name] = q.Spec.Channels
	}
}

// QueueChannels are where a queue's tasks reach its members (whatsapp).
func (r *Runtime) QueueChannels(ns, queue string) []string {
	return r.Program.queueChannels[ns+"/"+queue]
}

// MemberByWhatsApp is the member with this WhatsApp number, if any.
func (r *Runtime) MemberByWhatsApp(ns, number string) (agents.Member, bool) {
	if t := r.Program.teams[ns]; t != nil && number != "" {
		for _, m := range t.members {
			if m.WhatsApp == number {
				return m, true
			}
		}
	}
	return agents.Member{}, false
}

// Member returns a namespace's member.
func (r *Runtime) Member(ns, id string) (agents.Member, bool) {
	t := r.Program.teams[ns]
	if t == nil {
		return agents.Member{}, false
	}
	m, ok := t.members[id]
	return m, ok
}

// TeamVerify is how a namespace's members get one-time codes, or nil.
func (r *Runtime) TeamVerify(ns string) *agents.Verify {
	if t := r.Program.teams[ns]; t != nil {
		return t.verify
	}
	return nil
}

// Answerers are the members who may answer a run's pending task, and
// open when no queue it reached has roles (anyone the channel trusts).
func (r *Runtime) Answerers(ctx context.Context, runID string) (members []agents.Member, open bool, err error) {
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	t, err := r.PendingTask(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	// Once any queue the task reached has roles, only members holding one
	// of them answer: a task forwarded or escalated from an open queue to
	// the manager's is the manager's, not anyone's.
	roles := map[string]bool{}
	for _, q := range t.reached {
		for _, role := range r.Program.queueRoles[run.Namespace+"/"+q] {
			roles[role] = true
		}
	}
	if len(roles) == 0 {
		return nil, true, nil
	}
	if tm := r.Program.teams[run.Namespace]; tm != nil {
		for _, m := range tm.members {
			for _, role := range m.Roles {
				if roles[role] {
					members = append(members, m)
					break
				}
			}
		}
	}
	return members, false, nil
}

// mayAnswer checks that by may answer the run's pending task.
func (r *Runtime) mayAnswer(ctx context.Context, runID, by string) error {
	members, open, err := r.Answerers(ctx, runID)
	if err != nil || open {
		return err
	}
	for _, m := range members {
		if m.ID == by {
			return nil
		}
	}
	return fmt.Errorf("%w: %q holds none of the roles of the queues this task reached", ErrNotAllowed, by)
}

// Forward hands a run's pending task to another queue: a member of a
// queue the task reached passes it on (the receptionist to the manager).
// The task stays the same (its hash, so links stay valid); the queue's
// members may now answer it, and the channel layer delivers it there.
func (r *Runtime) Forward(ctx context.Context, runID, queue, by string) (Result, error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	t, err := r.PendingTask(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	if _, known := r.Program.queueRoles[run.Namespace+"/"+queue]; !known {
		return Result{}, fmt.Errorf("no HumanQueue %q in namespace %s", queue, run.Namespace)
	}
	if err := r.mayAnswer(ctx, runID, by); err != nil {
		return Result{}, err
	}
	v, err := journal.Encode(messageValue(Message{To: "queue:" + queue, Task: t.Kind, Text: "↪ " + t.Text, Options: t.Options}))
	if err != nil {
		return Result{}, err
	}
	site := fmt.Sprintf("%s/forward/%d", t.Site, len(t.reached))
	if _, err := r.Store.Append(ctx, runID, journal.Record{Site: site, Kind: journal.KindSend, Value: v, At: r.Clock()}); err != nil {
		return Result{}, err
	}
	if by != "" {
		note, _ := journal.Encode(map[string]any{"by": by, "queue": queue})
		if _, err := r.Store.Append(ctx, runID, journal.Record{Site: site + "/by", Kind: journal.KindResult, Value: note, At: r.Clock()}); err != nil {
			return Result{}, err
		}
	}
	outbox, err := r.pendingMessages(ctx, run)
	return Result{RunID: runID, Status: run.Status, Outbox: outbox}, err
}

// reachedQueues are the queues a task at site reached: its own, then
// every escalation and forward, in order.
func reachedQueues(site, first string, recs []journal.Record) []string {
	out := []string{first}
	for _, rec := range recs {
		if rec.Kind != journal.KindSend || !(rec.Site == site+"/sla/escalate" || strings.HasPrefix(rec.Site, site+"/forward/")) {
			continue
		}
		v, err := journal.Decode(rec.Value)
		if err != nil {
			continue
		}
		m, _ := v.(map[string]any)
		if q, ok := strings.CutPrefix(fmt.Sprint(m["to"]), "queue:"); ok {
			out = append(out, q)
		}
	}
	return out
}

// Action is a declared plugin action's spec, by plugin/action.
func (p *Program) Action(ref string) (plugins.ActionSpec, bool) {
	a, ok := p.actions[ref]
	return a, ok
}
