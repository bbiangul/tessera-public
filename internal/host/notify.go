package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// notifier is a queue's notification actions.
type notifier struct {
	act  string
	with *expr.Value
}

func notifiers(docs []admission.Doc, namespace string) (map[string][]notifier, error) {
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	out := map[string][]notifier{}
	for _, d := range docs {
		q := d.Queue
		if q == nil || q.Metadata.Namespace != namespace {
			continue
		}
		for i, n := range q.Spec.Notify {
			with := n.With
			if with == nil {
				with = map[string]any{}
			}
			v, err := e.CompileValue(with)
			if err != nil {
				return nil, fmt.Errorf("queue %s notify[%d]: %w", q.Metadata.Name, i, err)
			}
			out[q.Metadata.Name] = append(out[q.Metadata.Name], notifier{act: n.Act, with: v})
		}
	}
	return out, nil
}

// notifyState remembers, in memory, which runs are fully notified (so
// the next tick does not read their journal again) and when a failing
// notification may be tried again.
type notifyState struct {
	mu    sync.Mutex
	done  map[string]string // run ID -> the run's state when all its notifications were sent
	retry map[string]retry  // mark site -> backoff
}

type retry struct {
	at    time.Time
	delay time.Duration
}

const (
	notifyFirstRetry = time.Minute
	notifyMaxRetry   = time.Hour
)

// Notify tells people about tasks through their queues' notify actions:
// each message of the task a run waits on (created, reminded, escalated),
// once per message and action. A notification is recorded in the run's
// journal after the call succeeded: a failed call is tried again with a
// growing delay (a minute, doubling up to an hour), and only a crash
// between the call and the record (or two schedulers at once) can notify
// twice; for a note to the team a duplicate is better than silence. The
// action gets an idempotency key, for the plugins that can use one. One
// failing run or queue does not hold the others back: every run is tried,
// and the errors are returned together.
// link makes the signed page of a task, or "" without a public URL.
// Notifications go to the agent's own team; admission refuses actions of a
// users' channel plugin (consent, window) and irreversible ones.
func (h *Host) Notify(ctx context.Context, link func(runtime.Task) string) (int, error) {
	if len(h.notify) == 0 {
		return 0, nil
	}
	sent := 0
	var errs []error
	for _, st := range []journal.Status{journal.StatusWaitingForHuman, journal.StatusIndeterminate} {
		f := journal.RunFilter{Namespace: h.opts.Namespace, Status: st, Limit: 500}
		for {
			runs, err := h.Runtime.Store.Runs(ctx, f)
			if err != nil {
				return sent, errors.Join(append(errs, err)...)
			}
			for _, run := range runs {
				n, err := h.notifyRun(ctx, run, link)
				sent += n
				if err != nil {
					errs = append(errs, fmt.Errorf("run %s: %w", run.ID, err))
				}
			}
			if len(runs) < 500 || ctx.Err() != nil {
				break
			}
			f.Before = runs[len(runs)-1].CreatedAt
		}
	}
	return sent, errors.Join(errs...)
}

func runState(r journal.Run) string {
	return fmt.Sprintf("%s|%s|%d", r.Status, r.Pending, r.WakeAt.UnixNano())
}

func (h *Host) notifyRun(ctx context.Context, run journal.Run, link func(runtime.Task) string) (int, error) {
	if run.Generation != h.Runtime.Program.Generation() {
		return 0, nil // another version's run: answered through a process running it
	}
	state := runState(run)
	h.notified.mu.Lock()
	skip := h.notified.done[run.ID] == state
	h.notified.mu.Unlock()
	if skip {
		return 0, nil
	}
	task, err := h.Runtime.PendingTask(ctx, run.ID)
	if err != nil {
		return 0, nil // it moved on meanwhile
	}
	recs, err := h.Runtime.Store.Records(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	done := map[string]bool{}
	for _, r := range recs {
		if strings.HasSuffix(r.Site, "/notified") {
			done[r.Site] = true
		}
	}
	// Only the messages of the task the run waits on now: an earlier
	// task's message would go out with this task's link.
	stages := map[string]string{task.Site: "created", task.Site + "/sla/remind": "remind", task.Site + "/sla/escalate": "escalate"}
	sent, pending := 0, false
	now := h.Runtime.Clock()
	for _, r := range recs {
		stage, ok := stages[r.Site]
		if !ok || (r.Kind != journal.KindSuspend && r.Kind != journal.KindSend) {
			continue
		}
		v, err := journal.Decode(r.Value)
		if err != nil {
			return sent, err
		}
		m, _ := v.(map[string]any)
		to, _ := m["to"].(string)
		queue, ok := strings.CutPrefix(to, "queue:")
		if !ok || len(h.notify[queue]) == 0 {
			continue
		}
		text, _ := m["text"].(string)
		inputs := map[string]any{"runId": run.ID, "queue": queue, "kind": task.Kind, "text": text, "stage": stage,
			"since": task.Since, "link": ""}
		if link != nil {
			inputs["link"] = link(task)
		}
		for i, n := range h.notify[queue] {
			site := fmt.Sprintf("%s/notify[%s:%d]/notified", r.Site, queue, i)
			if done[site] {
				continue
			}
			pending = true
			h.notified.mu.Lock()
			wait := h.notified.retry[site]
			h.notified.mu.Unlock()
			if now.Before(wait.at) {
				continue
			}
			if err := h.notifyOne(ctx, run, n, site, stage, inputs); err != nil {
				wait.delay = min(max(2*wait.delay, notifyFirstRetry), notifyMaxRetry)
				wait.at = now.Add(wait.delay)
				h.notified.mu.Lock()
				h.notified.retry[site] = wait
				h.notified.mu.Unlock()
				return sent, fmt.Errorf("queue %s: %s: %w (next try in %s)", queue, n.act, err, wait.delay)
			}
			sent++
			h.notified.mu.Lock()
			delete(h.notified.retry, site)
			h.notified.mu.Unlock()
		}
	}
	h.notified.mu.Lock()
	if sent == 0 && !pending {
		h.notified.done[run.ID] = state
	}
	h.notified.mu.Unlock()
	return sent, nil
}

func (h *Host) notifyOne(ctx context.Context, run journal.Run, n notifier, site, stage string, inputs map[string]any) error {
	args, err := n.with.Resolve(ctx, expr.Activation{"inputs": inputs})
	if err != nil {
		return err
	}
	a, _ := args.(map[string]any)
	if _, err := h.Runtime.Actor.Call(ctx, act.Call{Action: n.act, Args: a, IdempotencyKey: run.ID + "/" + site}); err != nil {
		return err // not marked: tried again
	}
	mark, err := journal.Encode(map[string]any{"act": n.act, "stage": stage})
	if err != nil {
		return err
	}
	if _, err := h.Runtime.Store.Append(ctx, run.ID, journal.Record{Site: site, Kind: journal.KindResult, Value: mark, At: h.Runtime.Clock()}); err != nil && !errors.Is(err, journal.ErrDuplicate) {
		return err
	}
	return nil
}
