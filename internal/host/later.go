package host

import (
	"context"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// "Mando os dados amanhã" (agency-004: g03, g06). The message was taken as
// the pending question's answer, the process found nothing in it and asked
// again, turn after turn, and what the message also asked went
// unanswered. A person says "sem problema" and waits. This is its own
// question, apart from the measured interruption classifier.

const defaultLater = "No problem. When you have it, just send it here and I'll pick up where we left off."

// laterMessage is messages.later, or the English default.
func (h *Host) laterMessage() string {
	if h.opts.Later != "" {
		return h.opts.Later
	}
	return defaultLater
}

// later asks Decide whether text puts off what the pending question asks
// for. Unsure or failed: no, and the message goes on as before.
func (h *Host) later(ctx context.Context, pending, text string) bool {
	state := map[string]any{"agentSaid": h.Runtime.MessageState(pending)["message"], "message": h.Runtime.MessageState(text)["message"]}
	resp, err := h.opts.Decider.Decide(ctx, decide.Request{State: state, Questions: map[string]decide.Question{"later": {
		Kind:         decide.KindNoul,
		Instructions: "The agent asked AGENTSAID and is waiting for the reply. Does MESSAGE say the user cannot give it now and will give it later, or is not ready to decide yet (does not have the data at hand, will send it tomorrow, needs to ask someone, wants to think), instead of giving it? A message that gives what was asked, changes the request, says no, or gives up is not that.",
	}}})
	if err != nil || resp == nil {
		return false
	}
	return resp.Answers["later"].Noul >= 0.7
}

// holdPending answers what text asks, says the process waits for the rest,
// and leaves the run waiting: the message is not its answer, and the
// pending question is not asked again.
func (h *Host) holdPending(ctx context.Context, active journal.Run, conversation string, caller map[string]any, text, pending string) (Reply, error) {
	var out []runtime.Message
	if strings.Contains(text, "?") {
		facts, err := h.Runtime.RunReadFacts(ctx, active.ID, h.opts.Namespace, conversation, caller, h.freeFor(caller).runReads)
		if err != nil {
			return Reply{}, err
		}
		if answer := h.sideAnswer(ctx, conversation, caller, text, pending, facts); answer != "" {
			out = append(out, runtime.Message{To: "user", Text: answer})
		}
	}
	if err := h.Runtime.NoteAside(ctx, active.ID, active.Pending); err != nil {
		return Reply{}, err
	}
	out = append(out, runtime.Message{To: "user", Text: h.laterMessage()})
	return Reply{Result: runtime.Result{RunID: active.ID, Status: active.Status, WaitingFor: "user", Outbox: out}}, nil
}

// sideAnswer is the answer to what a message asks while a process waits.
// In agent mode it also gets the web search (agency-007), as a turn's
// free answer does.
func (h *Host) sideAnswer(ctx context.Context, conversation string, caller map[string]any, text, pending string, runFacts map[string]any) string {
	o := freeOpts{pending: pending, runFacts: runFacts}
	if fm := h.freeFor(caller); fm != nil && fm.agentSteps > 0 {
		facts := h.freeFacts(ctx, caller)
		if results, _ := h.webSearch(ctx, caller, text, h.recentLines(ctx, conversation, 8), facts); len(results) > 0 {
			facts["web"] = results
		}
		o.facts = facts
	}
	return h.freeAnswer(ctx, conversation, caller, text, o)
}
