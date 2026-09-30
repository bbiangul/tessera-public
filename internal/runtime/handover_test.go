package runtime

import (
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// #52: messages.failed tells the user a person will follow up, so with a
// failure queue the run hands the case over as a takeover task, and ends
// Failed once someone takes it.
func TestAFailedRunIsHandedToAPerson(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/send", `[ {fail: {message: "send refused"}} ]`))
	p.on["send"] = func(act.Call) (any, error) { return nil, &act.DefiniteError{Code: "INVALID", Message: "no"} }
	h.rt.Limits.FailureQueue = "recepcao"
	res := h.startR("f")
	expectStatus(t, res, journal.StatusWaitingForHuman)
	if len(h.user) == 0 || h.user[len(h.user)-1] != h.rt.Messages.Failed {
		t.Errorf("the user is told first: %v", h.user)
	}
	last := h.queues[len(h.queues)-1]
	if last.To != "queue:recepcao" || last.Task != "takeover" || !strings.Contains(last.Text, "send refused") {
		t.Errorf("want a takeover task with the reason: %+v", last)
	}
	expectStatus(t, h.reply("human", "assumi a conversa", nil), journal.StatusFailed)
}

// Without a failure queue (no humanInTheLoop), and for runs recorded
// before the setting existed, a failure just fails.
func TestFailuresWithoutAQueueJustFail(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/send", `[ {fail: {message: "send refused"}} ]`))
	p.on["send"] = func(act.Call) (any, error) { return nil, &act.DefiniteError{Code: "INVALID", Message: "no"} }
	expectStatus(t, h.startR("f"), journal.StatusFailed)
	if l := limitsFrom(map[string]any{"defaultQueue": "recepcao"}, Limits{FailureQueue: "recepcao"}); l.FailureQueue != "" {
		t.Errorf("an old run's limits must not gain a failure queue: %+v", l)
	}
}
