package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// milestoneTrigger starts a Function for each milestone of a name.
type milestoneTrigger struct {
	name, function, subject, milestone string
}

func milestoneTriggers(docs []admission.Doc, namespace string) []milestoneTrigger {
	var out []milestoneTrigger
	for _, d := range docs {
		t := d.Trigger
		if t == nil || t.Metadata.Namespace != namespace || t.Spec.Milestone == nil {
			continue
		}
		out = append(out, milestoneTrigger{name: t.Metadata.Name, function: t.Spec.Function, subject: t.Spec.Milestone.Subject, milestone: t.Spec.Milestone.Name})
	}
	return out
}

func (h *Host) cursorName(t milestoneTrigger) string {
	return "milestone-trigger/" + h.opts.Namespace + "/" + t.name
}

// startCursors places each new milestone trigger after the milestones
// already recorded: a trigger starts for what happens after it exists,
// never for the history.
func (h *Host) startCursors(ctx context.Context) error {
	for _, t := range h.milestoneTriggers {
		if _, ok, err := h.Runtime.Store.Cursor(ctx, h.cursorName(t)); err != nil || ok {
			if err != nil {
				return err
			}
			continue
		}
		last, err := h.Runtime.Store.LastMilestone(ctx, h.opts.Namespace)
		if err != nil {
			return err
		}
		if err := h.Runtime.Store.SetCursor(ctx, h.cursorName(t), last); err != nil {
			return err
		}
	}
	return nil
}

// milestoneBatch is how many milestones a trigger handles per tick.
const milestoneBatch = 500

// checkMilestones starts the Functions of milestone triggers for the
// milestones recorded since each trigger's cursor. The run's ID is the
// trigger and the milestone's record, so a second scheduler, or a crash
// before the cursor moved, starts nothing twice.
func (h *Host) checkMilestones(ctx context.Context) ([]runtime.Result, error) {
	var out []runtime.Result
	for _, t := range h.milestoneTriggers {
		name := h.cursorName(t)
		cur, _, err := h.Runtime.Store.Cursor(ctx, name)
		if err != nil {
			return out, err
		}
		ms, err := h.Runtime.Store.MilestonesAfter(ctx, h.opts.Namespace, cur, milestoneBatch)
		if err != nil {
			return out, err
		}
		for _, m := range ms {
			if m.Subject == t.subject && m.Name == t.milestone {
				res, err := h.startForMilestone(ctx, t, m)
				if err != nil {
					_ = h.Runtime.Store.SetCursor(ctx, name, cur) // up to the last one handled
					return out, fmt.Errorf("trigger %s: %w", t.name, err)
				}
				if res != nil {
					out = append(out, *res)
				}
			}
			cur = m.Seq
		}
		if len(ms) > 0 {
			if err := h.Runtime.Store.SetCursor(ctx, name, cur); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func (h *Host) startForMilestone(ctx context.Context, t milestoneTrigger, m journal.Milestone) (*runtime.Result, error) {
	id := fmt.Sprintf("%s/%s#%s/%s", h.opts.Namespace, t.name, m.RunID, m.Site)
	conversation := "milestone:" + id
	if run, err := h.Runtime.Store.GetRun(ctx, m.RunID); err == nil {
		conversation = run.Conversation // where the subject is being talked to
	}
	attrs := map[string]any{}
	for k, v := range m.Attributes {
		attrs[k] = v
	}
	res, err := h.Runtime.Start(ctx, runtime.Start{
		Namespace: h.opts.Namespace, Function: t.function, Conversation: conversation,
		Caller: map[string]any{"trigger": t.name}, Trigger: t.name, RunID: id,
		Payload: map[string]any{"subject": m.Subject, "id": m.ID, "name": m.Name, "at": m.At, "attributes": attrs},
	})
	if errors.Is(err, journal.ErrExists) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}
