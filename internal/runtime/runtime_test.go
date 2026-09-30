package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// The reschedule example of RFC-0001, end to end: two asks, a confirmation
// and an irreversible action.
func TestRescheduleHappyPath(t *testing.T) {
	h := newHarness(t)
	res := h.start("quero remarcar minha consulta")
	expectStatus(t, res, journal.StatusWaitingForUser)
	if res.WaitingFor != "user" {
		t.Errorf("waiting for %q", res.WaitingFor)
	}
	res = h.reply("user", "a de terça", nil)
	expectStatus(t, res, journal.StatusWaitingForUser)
	res = h.reply("user", "às 16h", nil)
	expectStatus(t, res, journal.StatusConfirming)
	res = h.reply("user", "sim, pode", nil)
	expectStatus(t, res, journal.StatusSucceeded)

	want := []string{
		"Qual consulta você quer remarcar?",
		"Tenho estes horários: Qui 24/09 10:00, Qui 24/09 16:00. Qual prefere?",
		"Posso remarcar sua consulta de Ter 22/09 14:00 para Qui 24/09 16:00? Responda sim para confirmar.",
		"Pronto! Sua consulta foi remarcada para Qui 24/09 16:00.",
	}
	if !reflect.DeepEqual(h.user, want) {
		t.Errorf("messages:\n got %q\nwant %q", h.user, want)
	}
	if h.clin.reschedules != 1 || h.clin.appointments[1]["slotId"] != "s2" {
		t.Errorf("reschedules %d, appointment %v", h.clin.reschedules, h.clin.appointments[1])
	}
	if _, err := h.rt.Deliver(context.Background(), "run-1", Reply{From: "user", Text: "oi"}); !errors.Is(err, ErrFinished) {
		t.Errorf("a finished run takes no replies: %v", err)
	}
}

// The first message can already answer the ask: no question is sent.
func TestAskAnsweredByTheFirstMessage(t *testing.T) {
	h := newHarness(t)
	res := h.start("quero remarcar a de terça")
	expectStatus(t, res, journal.StatusWaitingForUser)
	if len(h.user) != 1 || !strings.HasPrefix(h.user[0], "Tenho estes horários") {
		t.Errorf("the appointment question must be skipped: %q", h.user)
	}
}

func TestReplayDoesNotRepeatWork(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar")
	h.reply("user", "a de terça", nil)
	before, _ := h.store.Records(context.Background(), "run-1")
	calls, decides := h.clin.calls["list"]+h.clin.calls["slots"], h.dec.calls
	for range 3 {
		res, err := h.rt.Resume(context.Background(), "run-1")
		if err != nil {
			t.Fatal(err)
		}
		expectStatus(t, res, journal.StatusWaitingForUser)
		if len(res.Outbox) != 0 {
			t.Errorf("acked messages must not come back: %v", res.Outbox)
		}
	}
	after, _ := h.store.Records(context.Background(), "run-1")
	if len(after) != len(before) {
		t.Errorf("replay appended %d records", len(after)-len(before))
	}
	if h.clin.calls["list"]+h.clin.calls["slots"] != calls || h.dec.calls != decides {
		t.Error("replay called a plugin or Decide again")
	}
}

func TestUnackedMessagesAreDeliveredAgain(t *testing.T) {
	h := newHarness(t)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "agenda-exemplo", Function: "reschedule-appointment", Conversation: "c1", Caller: patient})
	if err != nil || len(res.Outbox) != 1 {
		t.Fatalf("%v %v", res, err)
	}
	// The channel crashed before acking: the question comes back, same ID.
	again, err := h.rt.Resume(context.Background(), "run-1")
	if err != nil || len(again.Outbox) != 1 || again.Outbox[0].ID != res.Outbox[0].ID {
		t.Fatalf("redelivery: %+v %v", again.Outbox, err)
	}
}

func TestConfirmationNoCancels(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	res := h.reply("user", "não, deixa como está", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if h.clin.calls["reschedule"] != 0 {
		t.Error("a refused confirmation must not act")
	}
	if last := h.user[len(h.user)-1]; last != DefaultMessages.Cancelled {
		t.Errorf("last message %q", last)
	}
}

func TestUnclearConfirmationGoesToAHuman(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	h.reply("user", "hmm", nil)
	res := h.reply("user", "talvez", nil)
	expectStatus(t, res, journal.StatusWaitingForHuman)
	if len(h.queues) != 1 || h.queues[0].Task != "approve" {
		t.Fatalf("queue messages %+v", h.queues)
	}
	if _, err := h.rt.Deliver(context.Background(), "run-1", Reply{From: "user", Text: "sim"}); !errors.Is(err, ErrNotWaiting) {
		t.Errorf("the user cannot answer for the human: %v", err)
	}
	res = h.reply("human", "", true)
	expectStatus(t, res, journal.StatusSucceeded)
	if h.clin.reschedules != 1 {
		t.Errorf("reschedules %d", h.clin.reschedules)
	}
}

func TestAuthorizeAndPreconditions(t *testing.T) {
	h := newHarness(t)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "agenda-exemplo", Function: "reschedule-appointment",
		Conversation: "c1", Caller: map[string]any{"role": "visitor"}})
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, h.deliver(res), journal.StatusFailed)
	if h.user[0] != DefaultMessages.NotAllowed || h.clin.calls["list"] != 0 {
		t.Errorf("unauthorized caller: %q, %d calls", h.user, h.clin.calls["list"])
	}

	// A slot less than 2 hours away fails the precondition with its message.
	h = newHarness(t)
	h.rt.Clock = func() time.Time { return testNow.Add(30 * time.Minute) } // 09:30
	h.clin.slotDays = 0
	h.start("quero remarcar a de terça")
	res = h.reply("user", "às 10h", nil) // today 10:00: only 30 minutes away
	expectStatus(t, res, journal.StatusFailed)
	if last := h.user[len(h.user)-1]; last != "Remarcações precisam de 2 horas de antecedência." {
		t.Errorf("last message %q", last)
	}
}

func TestNoAppointmentsRespondsAndFails(t *testing.T) {
	h := newHarness(t)
	h.clin.appointments = nil
	res := h.start("quero remarcar")
	expectStatus(t, res, journal.StatusFailed)
	if len(h.user) != 1 || h.user[0] != "Não encontrei consultas futuras no seu cadastro." {
		t.Errorf("messages %q", h.user)
	}
}

// The manifest's authorize rule is checked by the platform before the call:
// a caller who does not own the appointment never reaches the clinic, even
// when the Function itself would let the call through.
func TestPluginAuthorizeIsEnforced(t *testing.T) {
	h := newHarness(t)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "agenda-exemplo", Function: "reschedule-appointment",
		Conversation: "c1", Message: "quero remarcar a de terça",
		Caller: map[string]any{"role": "patient", "patientId": "p1", "appointmentIds": []any{"a1"}}})
	if err != nil {
		t.Fatal(err)
	}
	h.deliver(res)
	h.reply("user", "às 16h", nil)
	res = h.reply("user", "sim", nil)
	// FORBIDDEN is definite: probe is skipped, the retry is refused again,
	// and the escalation takes over.
	expectStatus(t, res, journal.StatusWaitingForHuman)
	if h.clin.calls["reschedule"] != 0 {
		t.Errorf("the clinic was called %d times", h.clin.calls["reschedule"])
	}
	if len(h.queues) != 1 || h.queues[0].To != "queue:recepcao" || h.queues[0].Task != "input" {
		t.Errorf("escalation %+v", h.queues)
	}
	if last := h.user[len(h.user)-1]; last != "Vou pedir para a equipe confirmar e te retorno." {
		t.Errorf("whileWaiting not sent: %q", h.user)
	}
}

func TestDecideFailureDegradesToAsking(t *testing.T) {
	h := newHarness(t)
	h.dec.fail = true
	res := h.start("quero remarcar a de terça")
	expectStatus(t, res, journal.StatusWaitingForUser)
	if h.user[0] != "Qual consulta você quer remarcar?" {
		t.Errorf("a failed Decide must fall back to asking: %q", h.user)
	}
}

// A timeout after the clinic acted: the probe sees the change and the run
// succeeds without calling again.
func TestAmbiguousFailureIsVerifiedByProbe(t *testing.T) {
	for _, performed := range []bool{true, false} {
		h := newHarness(t)
		h.clin.failReschedule = &act.AmbiguousError{Message: "timeout"}
		h.clin.performFirst = performed
		h.start("quero remarcar a de terça")
		h.reply("user", "às 16h", nil)
		res := h.reply("user", "sim", nil)
		expectStatus(t, res, journal.StatusSucceeded)
		if h.clin.reschedules != 1 {
			t.Errorf("performed=%v: %d reschedules", performed, h.clin.reschedules)
		}
		wantCalls, wantSleeps := 1, 0
		if !performed {
			wantCalls, wantSleeps = 2, 1 // probe says no, the retry runs after its backoff
		}
		if h.clin.calls["reschedule"] != wantCalls || len(h.slept) != wantSleeps {
			t.Errorf("performed=%v: %d calls, slept %v", performed, h.clin.calls["reschedule"], h.slept)
		}
	}
}

// The process dies during the irreversible call: the intent is recorded, the
// outcome is not. Resume must verify, never call blindly.
func TestCrashDuringTheEffect(t *testing.T) {
	for _, performed := range []bool{true, false} {
		h := newHarness(t)
		h.start("quero remarcar a de terça")
		h.reply("user", "às 16h", nil)
		h.clin.failReschedule = context.Canceled // our own cancellation: nothing recorded
		h.clin.performFirst = performed
		if _, err := h.rt.Deliver(context.Background(), "run-1", Reply{From: "user", Text: "sim"}); !errors.Is(err, context.Canceled) {
			t.Fatalf("want the crash, got %v", err)
		}
		res, err := h.rt.Resume(context.Background(), "run-1")
		if err != nil {
			t.Fatal(err)
		}
		expectStatus(t, h.deliver(res), journal.StatusSucceeded)
		if h.clin.reschedules != 1 {
			t.Errorf("performed=%v: %d reschedules", performed, h.clin.reschedules)
		}
	}
}

// Crash injection (RFC-0003, "Measurements"): fail the store at every
// append of the whole conversation; after recovering, the run must end as
// an uninterrupted run does, with no effect or message duplicated.
func TestCrashAtEveryAppend(t *testing.T) {
	steps := []Reply{{From: "user", Text: "a de terça"}, {From: "user", Text: "às 16h"}, {From: "user", Text: "sim"}}
	ref := newHarness(t)
	ref.start("quero remarcar")
	for _, s := range steps {
		ref.reply(s.From, s.Text, nil)
	}
	recs, _ := ref.store.Records(context.Background(), "run-1")

	for k := 1; k <= len(recs)+1; k++ {
		h := newHarness(t)
		h.store.FailAppendAfter = k
		ctx := context.Background()
		recover := func(res Result, err error, redo func() (Result, error)) {
			for tries := 0; err != nil; tries++ {
				if !errors.Is(err, journal.ErrInjected) && !errors.Is(err, journal.ErrDuplicate) || tries > 3 {
					t.Fatalf("k=%d: %v", k, err)
				}
				if res, err = redo(); errors.Is(err, journal.ErrDuplicate) {
					res, err = h.rt.Resume(ctx, "run-1")
				}
			}
			h.deliver(res)
		}
		res, err := h.rt.Start(ctx, Start{Namespace: "agenda-exemplo", Function: "reschedule-appointment", Conversation: "c1", Caller: patient, Message: "quero remarcar"})
		recover(res, err, func() (Result, error) { return h.rt.Resume(ctx, "run-1") })
		for _, s := range steps {
			res, err := h.rt.Deliver(ctx, "run-1", s)
			recover(res, err, func() (Result, error) { return h.rt.Deliver(ctx, "run-1", s) })
		}
		run, _ := h.store.GetRun(ctx, "run-1")
		if run.Status != journal.StatusSucceeded || h.clin.reschedules != 1 || h.clin.calls["reschedule"] != 1 {
			t.Errorf("k=%d: status %s, %d reschedules in %d calls", k, run.Status, h.clin.reschedules, h.clin.calls["reschedule"])
		}
		// The milestone follows the effect: exactly once, whatever failed.
		if ms, _ := h.store.Milestones(ctx, "agenda-exemplo", "paciente"); len(ms) != 1 || ms[0].Name != "remarcou" || ms[0].ID != "p1" {
			t.Errorf("k=%d: milestones %+v", k, ms)
		}
		if !reflect.DeepEqual(h.user, ref.user) {
			t.Errorf("k=%d: messages\n got %q\nwant %q", k, h.user, ref.user)
		}
	}
}

// The same conversation on the SQLite store, closed and reopened between
// messages, as a restart would.
func TestRescheduleOnSQLiteAcrossRestarts(t *testing.T) {
	path := t.TempDir() + "/runs.db"
	h := newHarness(t)
	open := func() *journal.SQLite {
		s, err := journal.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		h.rt.Store = s
		return s
	}
	s := open()
	h.start("quero remarcar")
	for _, text := range []string{"a de terça", "às 16h", "sim"} {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s = open()
		h.reply("user", text, nil)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	run, err := s.GetRun(context.Background(), "run-1")
	if err != nil || run.Status != journal.StatusSucceeded || h.clin.reschedules != 1 || len(h.user) != 4 {
		t.Errorf("status %s, %d reschedules, messages %q, %v", run.Status, h.clin.reschedules, h.user, err)
	}
}

// A failed read leaves later sources that read it with nothing to work
// with: the input is unresolved (onUnresolved applies), never an
// expression error on a missing key.
func TestFailedSourceSkipsTheSourcesThatReadIt(t *testing.T) {
	h := newHarness(t)
	reg := h.clin.actor()
	h.rt.Actor = act.NewRegistry()
	for _, a := range reg.Actions() {
		a := a
		if a == "agenda/find-slots" {
			h.rt.Actor.(*act.Registry).Register(a, func(context.Context, act.Call) (any, error) {
				return nil, &act.DefiniteError{Code: "UNAVAILABLE", Message: "agenda offline"}
			})
			continue
		}
		h.rt.Actor.(*act.Registry).Register(a, func(ctx context.Context, c act.Call) (any, error) { return reg.Call(ctx, c) })
	}
	res := h.start("quero remarcar a de terça")
	expectStatus(t, res, journal.StatusFailed)
	if !strings.Contains(res.Error, "input newSlot could not be resolved") || !strings.Contains(res.Error, "UNAVAILABLE") {
		t.Errorf("error %q: want the unresolved input and the plugin's reason", res.Error)
	}
}

// "No, I prefer another one" is a correction, not a refusal: the run ends
// without acting and asks the host to start again from the reply.
func TestConfirmationCorrection(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	res := h.reply("user", "não, mudei de ideia, prefiro às 10h", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if res.Restart != "não, mudei de ideia, prefiro às 10h" {
		t.Fatalf("restart %q", res.Restart)
	}
	if h.clin.calls["reschedule"] != 0 {
		t.Error("a correction must not act")
	}
	for _, m := range h.user {
		if m == DefaultMessages.Cancelled {
			t.Error("a correction is not answered with 'nothing changed'")
		}
	}
}
