package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// wait runs a wait step: a durable timer. The wake time is computed once and
// recorded, so neither a restart nor a clock change moves a timer that is
// running; the run then sleeps (WaitingForTime) until Wake resumes it.
func (e *execution) wait(f *frame, s *step, site string) (any, error) {
	shaped := s.orReply || len(s.events) > 0 // the output says what ended the wait
	if v, ok, err := e.get(site, journal.KindResume); err != nil || ok {
		m, _ := v.(map[string]any)
		if shaped {
			out := map[string]any{"reply": nil, "timedOut": false, "event": nil, "payload": nil}
			switch timer, _ := m["timer"].(bool); {
			case timer:
				out["timedOut"] = true
			case m["event"] != nil:
				out["event"], out["payload"] = m["event"], m["payload"]
			default:
				out["reply"] = m["text"]
			}
			return out, err
		}
		return m["value"], err
	}
	var until time.Time
	if s.until != nil {
		v, err := s.until.Eval(e.ctx, e.activation(f, nil))
		if err != nil {
			return nil, e.evalFailure(f, site+".until", err)
		}
		t, ok := v.(time.Time)
		if !ok {
			return nil, e.fail(f, fmt.Sprintf("%s.until: want a timestamp, got %s", site, describe(v)))
		}
		until = t
	}
	w, err := e.once(site+"/wake", func() (any, error) {
		if s.until != nil {
			return until, nil
		}
		return e.rt.Clock().Add(s.waitFor), nil
	})
	if err != nil {
		return nil, err
	}
	wake, _ := w.(time.Time)
	if !wake.After(e.rt.Clock()) { // already due: no sleep
		if _, err := e.put(site, journal.KindResume, map[string]any{"text": "", "value": wake, "timer": true}); err != nil {
			return nil, err
		}
		if shaped {
			return map[string]any{"reply": nil, "timedOut": true, "event": nil, "payload": nil}, nil
		}
		return wake, nil
	}
	if _, ok, err := e.get(site, journal.KindSleep); err != nil {
		return nil, err
	} else if !ok {
		sleep := map[string]any{"wake": wake}
		if s.includeSideMessages {
			sleep["includeSideMessages"] = true
		}
		if s.relay {
			sleep["relay"] = true
		}
		if _, err := e.put(site, journal.KindSleep, sleep); err != nil {
			return nil, err
		}
	}
	if len(s.events) > 0 {
		// The keys are evaluated once and recorded, so the watch is the
		// same on every execution.
		keys, err := e.once(site+"/keys", func() (any, error) {
			out := make([]any, len(s.events))
			for i, ev := range s.events {
				v, err := ev.key.Eval(e.ctx, e.activation(f, nil))
				if err != nil {
					return nil, err
				}
				if v == nil || fmt.Sprint(v) == "" {
					return nil, fmt.Errorf("the key of event %s is empty", ev.name)
				}
				out[i] = fmt.Sprint(v)
			}
			return out, nil
		})
		if err != nil {
			return nil, e.fail(f, fmt.Sprintf("%s.events: %v", site, err))
		}
		ks, _ := keys.([]any)
		for i, ev := range s.events {
			if err := e.rt.Store.Watch(e.ctx, journal.Watch{RunID: e.run.ID, Namespace: e.run.Namespace, Site: site, Event: ev.name, Key: fmt.Sprint(ks[i])}); err != nil {
				return nil, err
			}
		}
	}
	status := journal.StatusWaitingForTime
	if s.orReply {
		status = journal.StatusWaitingForUser
	}
	return nil, &suspended{status: status, site: site, wake: wake}
}

// DeliverEvent ends the waits listening for an event with this key: each
// run still waiting at the watched site gets the event as its wait's
// outcome and continues. A watch whose wait already ended (a timer, a
// reply, another event) is forgotten. Several deliveries of the same event
// resume a run once.
func (r *Runtime) DeliverEvent(ctx context.Context, namespace, event, key string, payload map[string]any) ([]Result, error) {
	r.defaults()
	watches, err := r.Store.Watchers(ctx, namespace, event, key)
	if err != nil {
		return nil, err
	}
	var out []Result
	var errs []error
	for _, w := range watches {
		res, ok, err := r.deliverEvent(ctx, w, payload)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("run %s: %w", w.RunID, err))
		case ok:
			out = append(out, res)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) deliverEvent(ctx context.Context, w journal.Watch, payload map[string]any) (Result, bool, error) {
	run, err := r.Store.GetRun(ctx, w.RunID)
	if err != nil {
		return Result{}, false, err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	if run, err = r.Store.GetRun(ctx, w.RunID); err != nil {
		return Result{}, false, err
	}
	waiting := run.Status == journal.StatusWaitingForTime || run.Status == journal.StatusWaitingForUser
	if !waiting || run.Pending != w.Site {
		return Result{}, false, r.Store.Unwatch(ctx, w.RunID) // its wait ended some other way
	}
	v, err := journal.Encode(map[string]any{"text": "", "event": w.Event, "payload": payload, "from": "event"})
	if err != nil {
		return Result{}, false, err
	}
	// A duplicate: the wait's outcome was recorded (this event, a timer,
	// a reply) and the execution after it stopped; finishing it is right.
	if _, err := r.Store.Append(ctx, run.ID, journal.Record{Site: run.Pending, Kind: journal.KindResume, Value: v, At: r.Clock()}); err != nil && !errors.Is(err, journal.ErrDuplicate) {
		return Result{}, false, err
	}
	res, err := r.execute(ctx, run)
	return res, err == nil, err
}

// Wake resumes a sleeping run whose time has come. A run that is not
// sleeping, or not due yet, is left alone (woke is false), so schedulers may
// call it more than once and from more than one process.
func (r *Runtime) Wake(ctx context.Context, runID string) (res Result, woke bool, err error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return Result{}, false, err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	if run, err = r.Store.GetRun(ctx, runID); err != nil {
		return Result{}, false, err
	}
	waiting := run.Status == journal.StatusWaitingForTime || run.Status == journal.StatusWaitingForUser ||
		run.Status == journal.StatusWaitingForHuman || run.Status == journal.StatusIndeterminate
	if !waiting || run.WakeAt.IsZero() || run.WakeAt.After(r.Clock()) {
		return Result{}, false, nil
	}
	if waitingFor(run.Status) == "human" {
		// A person's task whose SLA came due: executing applies the stage
		// (a reminder, an escalation, the timeout); nothing answers for
		// the person here.
		res, err = r.execute(ctx, run)
		return res, err == nil, err
	}
	v, err := journal.Encode(map[string]any{"text": "", "value": run.WakeAt, "timer": true})
	if err != nil {
		return Result{}, false, err
	}
	_, err = r.Store.Append(ctx, runID, journal.Record{Site: run.Pending, Kind: journal.KindResume, Value: v, At: r.Clock()})
	if err != nil && !errors.Is(err, journal.ErrDuplicate) { // a duplicate: woken before, the execution after it failed
		return Result{}, false, err
	}
	res, err = r.execute(ctx, run)
	return res, err == nil, err
}

// WakeDue wakes up to limit runs of this runtime's generation that are
// due, earliest first, and returns their results (their outboxes are for
// the channel layer to deliver). A run that fails to wake does not stop
// the others; the errors are returned together. Runs of other generations
// (started with earlier definitions) are left to a process running those.
func (r *Runtime) WakeDue(ctx context.Context, limit int) ([]Result, error) {
	r.defaults()
	due, err := r.Store.DueRuns(ctx, r.Clock(), r.Program.Generation(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(due))
	var errs []error
	for _, run := range due {
		res, woke, err := r.Wake(ctx, run.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("waking %s: %w", run.ID, err))
			continue
		}
		if woke {
			out = append(out, res)
		}
	}
	return out, errors.Join(errs...)
}

// Outbox returns the messages of a run not acknowledged yet, without
// executing it: what a channel layer polls, for runs that produced
// messages in the background (a timer, a schedule).
func (r *Runtime) Outbox(ctx context.Context, runID string) ([]Message, error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return r.pendingMessages(ctx, run)
}

// Stop ends a run that waits for the user because the user left the
// process (they cancelled it, or said goodbye). Nothing is done on their
// behalf; the reason is recorded and the run finishes.
func (r *Runtime) Stop(ctx context.Context, runID, reason string) error {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	if run, err = r.Store.GetRun(ctx, runID); err != nil {
		return err
	}
	if run.Status != journal.StatusWaitingForUser && run.Status != journal.StatusConfirming {
		return fmt.Errorf("%w: %s is %s", ErrNotWaiting, runID, run.Status)
	}
	v, err := journal.Encode(map[string]any{"reason": reason, "at": run.Pending})
	if err != nil {
		return err
	}
	if _, err := r.Store.Append(ctx, runID, journal.Record{Site: "stopped", Kind: journal.KindResult, Value: v, At: r.Clock()}); err != nil && !errors.Is(err, journal.ErrDuplicate) {
		return err
	}
	return r.finish(ctx, runID, journal.StatusSucceeded)
}
