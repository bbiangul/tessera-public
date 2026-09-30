package runtime

import (
	"context"
	"errors"
	"slices"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// The task surface channels share (RFC-0006 section 4). A channel
// presents a task and attests who answered it; the platform, here, says
// which tasks go to whom and decides whether an answer counts (the
// member's role, the task still open with the same hash).

// ChannelTask is an approval task to present to one member.
type ChannelTask struct {
	Task   Task
	Member agents.Member
}

// ChannelTasks are the open approval tasks of the namespace's queues with
// this channel, one per member who may answer them. A task that reached a
// new queue (an escalation, a forward) lists that queue's members too.
// Answers in words go through the link: only approvals are listed.
func (r *Runtime) ChannelTasks(ctx context.Context, ns, channel string) ([]ChannelTask, error) {
	runs, err := r.waitingRuns(ctx, ns)
	if err != nil {
		return nil, err
	}
	var out []ChannelTask
	for _, run := range runs {
		t, err := r.PendingTask(ctx, run.ID)
		if err != nil || t.Kind != "approve" || !slices.Contains(r.QueueChannels(ns, t.Queue), channel) {
			continue
		}
		members, open, err := r.Answerers(ctx, run.ID)
		if err != nil || open {
			continue
		}
		for _, m := range members {
			out = append(out, ChannelTask{Task: t, Member: m})
		}
	}
	return out, nil
}

// MemberTasks are the open tasks the member may answer (any kind), newest
// first: what the member asks about in the conversation (backoffice-006).
func (r *Runtime) MemberTasks(ctx context.Context, ns, member string) ([]Task, error) {
	runs, err := r.waitingRuns(ctx, ns)
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, run := range runs {
		t, err := r.PendingTask(ctx, run.ID)
		if err != nil {
			continue
		}
		members, open, err := r.Answerers(ctx, run.ID)
		if err != nil || open {
			continue // a queue without roles is nobody's in particular
		}
		for _, m := range members {
			if m.ID == member {
				out = append(out, t)
				break
			}
		}
	}
	return out, nil
}

// Outcome is what became of an answer from a channel, for the channel to
// tell the member.
type Outcome int

const (
	// Recorded: the answer counted and the run went on.
	Recorded Outcome = iota
	// NotAllowed: the member holds none of the roles the task reached.
	NotAllowed
	// Stale: the task was answered already, or changed.
	Stale
	// Failed: not recorded now; the member may try again.
	Failed
)

// AnswerTask records a member's approval or denial of a run's task, as a
// channel attested it. hash is the task's, as the channel presented it.
func (r *Runtime) AnswerTask(ctx context.Context, runID, hash string, m agents.Member, approve bool) (Outcome, error) {
	text := "denied"
	if approve {
		text = "approved"
	}
	_, err := r.Deliver(ctx, runID, Reply{From: "human", TaskHash: hash, By: m.ID, Value: approve, Text: text})
	switch {
	case err == nil:
		return Recorded, nil
	case errors.Is(err, ErrNotAllowed):
		return NotAllowed, nil
	case errors.Is(err, ErrTaskChanged), errors.Is(err, ErrNotWaiting), errors.Is(err, ErrFinished):
		return Stale, nil
	default:
		return Failed, err
	}
}

// MemberByTeams is the member with this Microsoft Entra object ID, if
// any (compared ignoring case: GUIDs are written both ways).
func (r *Runtime) MemberByTeams(ns, id string) (agents.Member, bool) {
	if t := r.Program.teams[ns]; t != nil && id != "" {
		for _, m := range t.members {
			if m.Teams != "" && strings.EqualFold(m.Teams, id) {
				return m, true
			}
		}
	}
	return agents.Member{}, false
}

// MemberBySlack is the member with this Slack user ID, if any.
func (r *Runtime) MemberBySlack(ns, id string) (agents.Member, bool) {
	if t := r.Program.teams[ns]; t != nil && id != "" {
		for _, m := range t.members {
			if m.Slack == id {
				return m, true
			}
		}
	}
	return agents.Member{}, false
}

// AsMember is caller as the Team member m, whose identity a channel
// attested (their WhatsApp number): caller.member, caller.roles,
// caller.name and caller.attributes. Attributes stay under attributes,
// never at the caller's top level, so no attribute poses as
// caller.trigger or caller.role. It replaces whatever the caller said of
// itself under those names.
func AsMember(caller map[string]any, m agents.Member) map[string]any {
	out := make(map[string]any, len(caller)+4)
	for k, v := range caller {
		out[k] = v
	}
	roles := make([]any, len(m.Roles))
	for i, r := range m.Roles {
		roles[i] = r
	}
	attrs := make(map[string]any, len(m.Attributes))
	for k, v := range m.Attributes {
		attrs[k] = v
	}
	out["member"], out["roles"], out["name"], out["attributes"] = m.ID, roles, m.Name, attrs
	return out
}

// waitingRuns are every run of ns waiting for a person, page by page: an
// old task forwarded to a new queue must be found however many newer runs
// wait.
func (r *Runtime) waitingRuns(ctx context.Context, ns string) ([]journal.Run, error) {
	var runs []journal.Run
	f := journal.RunFilter{Namespace: ns, Status: journal.StatusWaitingForHuman, Limit: 500}
	for {
		page, err := r.Store.Runs(ctx, f)
		if err != nil {
			return nil, err
		}
		runs = append(runs, page...)
		if len(page) < f.Limit {
			return runs, nil
		}
		f.Before = page[len(page)-1].CreatedAt
	}
}
