package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// queueSLA is a compiled HumanQueue's service level.
type queueSLA struct {
	remind, escalate, timeout time.Duration
	escalateTo                string
	answer                    any
	fail                      bool
	hours                     *expr.WorkHours
}

func (p *Program) compileQueues(docs []*agents.HumanQueue, zones map[string]string) error {
	p.queues = map[string]*queueSLA{}
	for _, q := range docs {
		s := q.Spec.SLA
		if s == nil {
			continue
		}
		out := &queueSLA{escalateTo: s.EscalateTo}
		for _, d := range []struct {
			src string
			dst *time.Duration
		}{{s.Remind, &out.remind}, {s.Escalate, &out.escalate}, {s.Timeout, &out.timeout}} {
			if d.src == "" {
				continue
			}
			v, err := time.ParseDuration(d.src)
			if err != nil || v <= 0 {
				return fmt.Errorf("queue %s: %q is not a positive duration", q.Metadata.Name, d.src)
			}
			*d.dst = v
		}
		if o := s.OnTimeout; o != nil {
			out.fail, out.answer = o.Fail, o.Answer
		}
		if bh := s.BusinessHours; bh != nil {
			loc, err := time.LoadLocation(zones[q.Metadata.Namespace])
			if err != nil || zones[q.Metadata.Namespace] == "" {
				return fmt.Errorf("queue %s: business hours need the Agent's timezone", q.Metadata.Name)
			}
			w, err := expr.ParseWorkHours(bh.Days, bh.From, bh.To, loc)
			if err != nil {
				return fmt.Errorf("queue %s: %w", q.Metadata.Name, err)
			}
			out.hours = &w
		}
		p.queues[q.Metadata.Namespace+"/"+q.Metadata.Name] = out
	}
	return nil
}

// humanWait applies the queue's SLA to a task waiting at site: each stage
// that came due is recorded once (a reminder, an escalation, the timeout
// answer); the run then sleeps until the next one.
func (e *execution) humanWait(site string, status journal.Status, m Message) (string, any, error) {
	q := e.rt.Program.queues[e.run.Namespace+"/"+strings.TrimPrefix(m.To, "queue:")]
	if q == nil {
		return "", nil, &suspended{status: status, site: site}
	}
	created := e.recs[site+"|"+string(journal.KindSuspend)].At
	now := e.rt.Clock()
	due := func(d time.Duration) time.Time {
		if q.hours != nil {
			return q.hours.Add(created, d)
		}
		return created.Add(d)
	}
	var next time.Time
	reached := func(d time.Duration) bool {
		if d <= 0 {
			return false
		}
		at := due(d)
		if !now.Before(at) {
			return true
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
		return false
	}
	if reached(q.remind) {
		if err := e.send(site+"/sla/remind", Message{To: m.To, Task: m.Task, Text: "⏰ " + m.Text}); err != nil {
			return "", nil, err
		}
	}
	if q.escalateTo != "" && reached(q.escalate) {
		if err := e.send(site+"/sla/escalate", Message{To: "queue:" + q.escalateTo, Task: m.Task, Text: "⤴ " + m.Text}); err != nil {
			return "", nil, err
		}
	}
	if reached(q.timeout) {
		if q.fail {
			return "", nil, &finished{status: journal.StatusFailed, reason: "nobody answered the task in time (" + m.To + ")"}
		}
		if _, err := e.put(site, journal.KindResume, map[string]any{"text": "", "value": q.answer, "from": "timeout"}); err != nil {
			return "", nil, err
		}
		return "", q.answer, nil
	}
	return "", nil, &suspended{status: status, site: site, wake: next}
}

// Task is a person's pending work: what a run waits for.
type Task struct {
	RunID    string            `json:"runId"`
	Function string            `json:"function"`
	Site     string            `json:"site"`
	Queue    string            `json:"queue"`
	Kind     string            `json:"kind"` // approve, answer, input, takeover, indeterminate
	Text     string            `json:"text"`
	Options  map[string]string `json:"options,omitempty"`
	Since    time.Time         `json:"since"`
	// Next is when the SLA acts next (a reminder, an escalation, the
	// timeout), if the queue has one.
	Next time.Time `json:"next,omitzero"`
	// Hash binds an answer to this exact task: an approval must send it
	// back, and is refused if the task changed.
	Hash string `json:"hash"`
	// Reached are the queues the task reached: its own first, then each
	// escalation and forward. Queue is the last.
	Reached []string `json:"reached,omitempty"`

	firstQueue string
	reached    []string
}

// ErrTaskChanged means an answer was bound to a task that is no longer the
// one the run waits for.
var ErrTaskChanged = errors.New("the task changed since it was read: read it again")

// PendingTask is the task a run waiting for a person waits on.
func (r *Runtime) PendingTask(ctx context.Context, runID string) (Task, error) {
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return Task{}, err
	}
	if waitingFor(run.Status) != "human" || run.Pending == "" {
		return Task{}, fmt.Errorf("%w: %s is %s", ErrNotWaiting, runID, run.Status)
	}
	recs, err := r.Store.Records(ctx, runID)
	if err != nil {
		return Task{}, err
	}
	for _, rec := range recs {
		if rec.Site != run.Pending || rec.Kind != journal.KindSuspend {
			continue
		}
		v, err := journal.Decode(rec.Value)
		if err != nil {
			return Task{}, err
		}
		m, _ := v.(map[string]any)
		t := Task{RunID: runID, Function: run.Function, Site: rec.Site, Since: rec.At, Next: run.WakeAt}
		t.Queue = strings.TrimPrefix(fmt.Sprint(m["to"]), "queue:")
		t.firstQueue = t.Queue
		t.reached = reachedQueues(rec.Site, t.Queue, recs)
		t.Reached, t.Queue = t.reached, t.reached[len(t.reached)-1]
		t.Kind, _ = m["task"].(string)
		t.Text, _ = m["text"].(string)
		if opts, ok := m["options"].(map[string]any); ok {
			t.Options = map[string]string{}
			for k, x := range opts {
				t.Options[k] = fmt.Sprint(x)
			}
		}
		sum := sha256.Sum256(append([]byte(runID+"\x00"+rec.Site+"\x00"), rec.Value...))
		t.Hash = hex.EncodeToString(sum[:16])
		return t, nil
	}
	return Task{}, fmt.Errorf("%w: %s has no pending task", ErrNotWaiting, runID)
}
