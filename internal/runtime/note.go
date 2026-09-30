package runtime

import (
	"context"
	"strconv"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// FreeRun is the Function name of a run that records an exchange outside
// any Function (the free mode). No Function can have it: names are
// lowercase letters, digits and hyphens.
const FreeRun = "(free)"

// Note is an exchange that happened outside any Function: the user's
// message and what the agent answered.
type Note struct {
	Namespace    string
	Conversation string
	Caller       map[string]any
	Message      string
	Replies      []string
	// Decision is what chose to answer rather than start a Function (the
	// router's and the free mode's probabilities), recorded as the
	// run's router result.
	Decision map[string]any
	// After is the Function of the conversation's last Function run, kept
	// so follow-ups (selector.after) still route after a free exchange.
	After string
	// Continues: the message was already journaled by a run of the same
	// turn (agent mode, RFC-0010), so this note does not repeat it in the
	// conversation's history.
	Continues bool
}

// Record journals a free-mode exchange as a finished run, so the
// conversation's history, the voice, the inspector and redelivery see it
// (issue #27). Its messages are delivered and acknowledged like any
// run's: the result carries their seqs.
func (r *Runtime) Record(ctx context.Context, n Note) (Result, error) {
	r.defaults()
	now := r.Clock()
	conv := map[string]any{"id": n.Conversation, "after": n.After}
	unlock := r.locks.lock(n.Conversation)
	defer unlock()
	if last, ok, err := r.Store.LastRun(ctx, n.Conversation); err == nil && ok {
		if lines, err := r.Conversation(ctx, last.ID, 6); err == nil && len(lines) > 0 {
			conv["earlier"] = anyList(lines)
		}
	}
	event := map[string]any{"time": now, "message": n.Message}
	if n.Continues {
		event = map[string]any{"time": now, "continues": n.Message}
	}
	scopes, err := journal.Encode(map[string]any{
		"caller":       normalizeMap(n.Caller),
		"conversation": conv,
		"event":        event,
	})
	if err != nil {
		return Result{}, err
	}
	run := journal.Run{ID: r.NewID(), Namespace: n.Namespace, Function: FreeRun, Generation: r.Program.Generation(),
		Conversation: n.Conversation, Scopes: scopes, Status: journal.StatusSucceeded, CreatedAt: now, UpdatedAt: now}
	if err := r.Store.CreateRun(ctx, run); err != nil {
		return Result{}, err
	}
	if n.Decision != nil {
		v, err := journal.Encode(n.Decision)
		if err != nil {
			return Result{}, err
		}
		if _, err := r.Store.Append(ctx, run.ID, journal.Record{Site: "router", Kind: journal.KindResult, Value: v, At: now}); err != nil {
			return Result{}, err
		}
	}
	res := Result{RunID: run.ID, Status: journal.StatusSucceeded}
	for i, text := range n.Replies {
		m := Message{To: "user", Text: text}
		v, err := journal.Encode(messageValue(m))
		if err != nil {
			return Result{}, err
		}
		seq, err := r.Store.Append(ctx, run.ID, journal.Record{Site: "free/" + strconv.Itoa(i), Kind: journal.KindSend, Value: v, At: now})
		if err != nil {
			return Result{}, err
		}
		m.Seq = seq
		res.Outbox = append(res.Outbox, m)
	}
	return res, nil
}
