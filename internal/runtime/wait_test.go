package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// A follow-up cadence in the shape of the SDR's (D+1, D+3 at 10h on
// business days): the run sleeps between messages and survives restarts.
const cadence = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cadence, namespace: travel }
spec:
  description: Follows up with a lead.
  steps:
    - id: d0
      respond: { template: "Olá! Posso ajudar?" }
    - id: d1
      wait: { until: "atTime(addBusinessDays(event.time, 1, 'America/Sao_Paulo'), '10:00', 'America/Sao_Paulo')" }
    - id: m1
      respond: { template: "Oi de novo, viu minha mensagem?" }
    - id: d3
      wait: { for: 48h }
    - id: m3
      respond: { template: "Última tentativa: {{ string(steps.d3.output) }}" }
  output: { template: "fim" }
`

func cadenceHarness(t *testing.T, store journal.Store) (*harness, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cadence.yaml"), []byte(cadence), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	if store != nil {
		h.rt.Store = store
	}
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC) // Friday 12h in São Paulo
	h.rt.Clock = func() time.Time { return now }
	return h, &now
}

func sp(t *testing.T, y int, m time.Month, d, hh int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return time.Date(y, m, d, hh, 0, 0, 0, loc)
}

func wakeAll(t *testing.T, h *harness) []Result {
	t.Helper()
	res, err := h.rt.WakeDue(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		h.deliver(r)
	}
	return res
}

func TestWaitSleepsAndWakes(t *testing.T) {
	h, now := cadenceHarness(t, nil)
	ctx := context.Background()
	res := h.startFn("cadence", "")
	expectStatus(t, res, journal.StatusWaitingForTime)
	monday10 := sp(t, 2026, 9, 28, 10)
	if !res.WakeAt.Equal(monday10) {
		t.Fatalf("wakes at %v, want the next business day at 10h (%v)", res.WakeAt, monday10)
	}
	if !reflect.DeepEqual(h.user, []string{"Olá! Posso ajudar?"}) {
		t.Fatalf("a timer is not a message: %q", h.user)
	}
	if _, ok, _ := h.store.ActiveRun(ctx, "c1"); ok {
		t.Error("a sleeping run must not take the user's next message")
	}
	*now = monday10.Add(-time.Minute)
	if got := wakeAll(t, h); len(got) != 0 {
		t.Fatalf("woke early: %+v", got)
	}
	*now = monday10.Add(time.Hour) // a scheduler late by an hour
	if got := wakeAll(t, h); len(got) != 1 || got[0].Status != journal.StatusWaitingForTime {
		t.Fatalf("wake: %+v", got)
	}
	if run, _ := h.store.GetRun(ctx, "run-1"); !run.WakeAt.Equal(monday10.Add(time.Hour + 48*time.Hour)) {
		t.Errorf("for counts from when the step is reached: %v", run.WakeAt)
	}
	*now = now.Add(48 * time.Hour)
	wakeAll(t, h)
	if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded {
		t.Fatalf("status %s", run.Status)
	}
	if len(h.user) != 4 || h.user[1] != "Oi de novo, viu minha mensagem?" {
		t.Errorf("messages %q", h.user)
	}
	if got := wakeAll(t, h); len(got) != 0 {
		t.Errorf("a finished run woke again: %+v", got)
	}
}

// Waking the same run twice (two schedulers, a retry) sends nothing twice.
func TestWakeIsIdempotent(t *testing.T) {
	h, now := cadenceHarness(t, nil)
	h.startFn("cadence", "")
	*now = sp(t, 2026, 9, 28, 11)
	ctx := context.Background()
	for range 3 {
		res, woke, err := h.rt.Wake(ctx, "run-1")
		if err != nil {
			t.Fatal(err)
		}
		if woke {
			h.deliver(res)
		}
	}
	if len(h.user) != 2 {
		t.Errorf("messages %q", h.user)
	}
}

// The process stops while runs sleep; a new one, on the same database,
// wakes them. Also with a store that fails at every write in turn.
func TestWaitSurvivesRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "j.db")
	open := func() *journal.SQLite {
		s, err := journal.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s1 := open()
	h, now := cadenceHarness(t, s1)
	h.startFn("cadence", "")
	_ = s1.Close()

	s2 := open()
	defer func() { _ = s2.Close() }()
	h2, now2 := cadenceHarness(t, s2)
	h2.user = h.user
	*now2 = sp(t, 2026, 9, 28, 10)
	wakeAll(t, h2)
	*now2 = now2.Add(48 * time.Hour)
	wakeAll(t, h2)
	_ = now
	if run, _ := s2.GetRun(context.Background(), "run-1"); run.Status != journal.StatusSucceeded || len(h2.user) != 4 {
		t.Errorf("status %s, messages %q", run.Status, h2.user)
	}
}

func TestWaitCrashAtEveryAppend(t *testing.T) {
	ref, rnow := cadenceHarness(t, nil)
	ref.startFn("cadence", "")
	*rnow = sp(t, 2026, 9, 28, 10)
	wakeAll(t, ref)
	*rnow = rnow.Add(48 * time.Hour)
	wakeAll(t, ref)
	recs, _ := ref.store.Records(context.Background(), "run-1")
	for k := 1; k <= len(recs)+1; k++ {
		h, now := cadenceHarness(t, nil)
		h.store.FailAppendAfter = k
		ctx := context.Background()
		res, err := h.rt.Start(ctx, Start{Namespace: "travel", Function: "cadence", Conversation: "c1"})
		for tries := 0; err != nil; tries++ {
			if !errors.Is(err, journal.ErrInjected) || tries > 3 {
				t.Fatalf("k=%d: %v", k, err)
			}
			res, err = h.rt.Resume(ctx, "run-1")
		}
		h.deliver(res)
		for _, at := range []time.Time{sp(t, 2026, 9, 28, 10), sp(t, 2026, 9, 30, 10)} {
			*now = at
			for tries := 0; ; tries++ {
				got, err := h.rt.WakeDue(ctx, 10)
				for _, r := range got {
					h.deliver(r)
				}
				if err == nil {
					break
				}
				if !errors.Is(err, journal.ErrInjected) || tries > 3 {
					t.Fatalf("k=%d: %v", k, err)
				}
				// A failed execution leaves the run as it was: sleeping and
				// due, or stalled mid-way; Resume finishes the latter.
				if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusWaitingForTime {
					r, err := h.rt.Resume(ctx, "run-1")
					if err == nil {
						h.deliver(r)
					}
				}
			}
		}
		if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded || !reflect.DeepEqual(h.user, ref.user) {
			t.Errorf("k=%d: status %s, messages\n got %q\nwant %q", k, run.Status, h.user, ref.user)
		}
	}
}

// SDR's cadence: a message, then wait 48h for a reply. A reply inside the
// window reaches this run (not the router) and ends the wait; silence
// wakes it with timedOut.
const replyCadence = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: follow-up, namespace: travel }
spec:
  description: Follows up until the lead answers.
  steps:
    - { id: d0, respond: { template: "Olá! Posso ajudar?" } }
    - { id: w, wait: { for: 48h, orReply: true } }
    - id: next
      branch:
        - { when: "steps.w.output.timedOut", respond: { template: "Oi de novo, viu minha mensagem?" } }
        - { else: true, respond: { template: "Obrigado! Você disse: {{ steps.w.output.reply }}" } }
  output: { value: "steps.w.output" }
`

func replyHarness(t *testing.T) (*harness, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(replyCadence), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	h.rt.Clock = func() time.Time { return now }
	return h, &now
}

func TestWaitOrReply(t *testing.T) {
	ctx := context.Background()

	h, now := replyHarness(t)
	res := h.startFn("follow-up", "")
	expectStatus(t, res, journal.StatusWaitingForUser)
	if run, ok, _ := h.store.ActiveRun(ctx, "c1"); !ok || run.ID != "run-1" {
		t.Fatal("while it waits for a reply the run is the active one: the reply comes here")
	}
	*now = now.Add(10 * time.Hour)
	res = h.reply("user", "tenho interesse, sim", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[len(h.user)-1] != "Obrigado! Você disse: tenho interesse, sim" {
		t.Errorf("messages %q", h.user)
	}
	*now = now.Add(48 * time.Hour)
	if got := wakeAll(t, h); len(got) != 0 {
		t.Errorf("an answered wait must not wake: %+v", got)
	}

	h, now = replyHarness(t)
	h.startFn("follow-up", "")
	*now = now.Add(47 * time.Hour)
	if got := wakeAll(t, h); len(got) != 0 {
		t.Fatal("woke early")
	}
	*now = now.Add(2 * time.Hour)
	if got := wakeAll(t, h); len(got) != 1 || got[0].Status != journal.StatusSucceeded {
		t.Fatalf("silence: %+v", got)
	}
	if h.user[len(h.user)-1] != "Oi de novo, viu minha mensagem?" {
		t.Errorf("messages %q", h.user)
	}
	// A reply after the wait ended is not an answer to it.
	if _, err := h.rt.Deliver(ctx, "run-1", Reply{From: "user", Text: "oi"}); err == nil {
		t.Error("a finished run must refuse a late reply (the router takes it)")
	}
}

func TestWaitOrReplyCrashAtEveryAppend(t *testing.T) {
	ref, rnow := replyHarness(t)
	ref.startFn("follow-up", "")
	*rnow = rnow.Add(time.Hour)
	ref.reply("user", "quero saber mais", nil)
	recs, _ := ref.store.Records(context.Background(), "run-1")
	for k := 1; k <= len(recs)+1; k++ {
		h, now := replyHarness(t)
		h.store.FailAppendAfter = k
		ctx := context.Background()
		retry := func(res Result, err error, again func() (Result, error)) {
			for tries := 0; err != nil; tries++ {
				if !errors.Is(err, journal.ErrInjected) && !errors.Is(err, journal.ErrDuplicate) || tries > 3 {
					t.Fatalf("k=%d: %v", k, err)
				}
				if res, err = again(); errors.Is(err, journal.ErrDuplicate) {
					res, err = h.rt.Resume(ctx, "run-1")
				}
			}
			h.deliver(res)
		}
		res, err := h.rt.Start(ctx, Start{Namespace: "travel", Function: "follow-up", Conversation: "c1"})
		retry(res, err, func() (Result, error) { return h.rt.Resume(ctx, "run-1") })
		*now = now.Add(time.Hour)
		rep := Reply{From: "user", Text: "quero saber mais"}
		res, err = h.rt.Deliver(ctx, "run-1", rep)
		retry(res, err, func() (Result, error) { return h.rt.Deliver(ctx, "run-1", rep) })
		if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded || !reflect.DeepEqual(h.user, ref.user) {
			t.Errorf("k=%d: %s, messages\n got %q\nwant %q", k, run.Status, h.user, ref.user)
		}
	}
}

// A run that cannot wake (broken, or of other definitions) does not keep
// the others asleep: runs of other generations are not even picked, and a
// failing one is reported while the rest wake.
func TestWakeDueSurvivesBadRuns(t *testing.T) {
	h, now := cadenceHarness(t, nil)
	ctx := context.Background()
	h.startFn("cadence", "")
	early := sp(t, 2026, 9, 26, 10)
	for _, r := range []journal.Run{
		{ID: "old", Namespace: "travel", Function: "cadence", Generation: "g-old", Conversation: "c-old", Status: journal.StatusResolving},
		{ID: "broken", Namespace: "travel", Function: "gone", Generation: h.rt.Program.Generation(), Conversation: "c-broken", Status: journal.StatusResolving},
	} {
		if err := h.store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := h.store.Sleep(ctx, r.ID, journal.StatusWaitingForTime, "d1", early); err != nil {
			t.Fatal(err)
		}
	}
	if due, _ := h.store.DueRuns(ctx, early, h.rt.Program.Generation(), 10); len(due) != 1 || due[0].ID != "broken" {
		t.Fatalf("other generations are not picked: %+v", due)
	}
	*now = sp(t, 2026, 9, 28, 11)
	res, err := h.rt.WakeDue(ctx, 100)
	if err == nil || len(res) != 1 || res[0].RunID != "run-1" {
		t.Fatalf("the good run wakes and the broken one is reported: %+v %v", res, err)
	}
}

// An event ends a wait: the same outcome whatever write of the delivery
// fails, once the delivery is tried again (the sender retries).
const eventCadence = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: meeting, namespace: travel }
spec:
  description: Waits for the CRM to report the meeting.
  steps:
    - { id: d0, respond: { template: "Olá!" } }
    - id: w
      wait: { for: 48h, events: [ { name: booked, key: "'lead-1'" } ] }
    - id: next
      branch:
        - { when: "steps.w.output.event == 'booked'", respond: { template: "Até {{ steps.w.output.payload.day }}!" } }
        - { else: true, respond: { template: "Oi de novo" } }
  output: { value: "true" }
`

func TestEventCrashAtEveryAppend(t *testing.T) {
	setup := func() (*harness, *journal.Memory) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(eventCadence), 0o600); err != nil {
			t.Fatal(err)
		}
		h := newHarness(t, dir)
		now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
		h.rt.Clock = func() time.Time { return now }
		return h, h.store
	}
	ctx := context.Background()
	for k := 1; k <= 4; k++ {
		h, store := setup()
		h.startFn("meeting", "")
		store.FailAppendAfter = k
		for tries := 0; ; tries++ {
			got, err := h.rt.DeliverEvent(ctx, "travel", "booked", "lead-1", map[string]any{"day": "terça"})
			for _, r := range got {
				h.deliver(r)
			}
			if err == nil {
				break
			}
			if !errors.Is(err, journal.ErrInjected) || tries > 3 {
				t.Fatalf("k=%d: %v", k, err)
			}
		}
		if run, _ := store.GetRun(ctx, "run-1"); run.Status != journal.StatusSucceeded || !reflect.DeepEqual(h.user, []string{"Olá!", "Até terça!"}) {
			t.Errorf("k=%d: %s %q", k, run.Status, h.user)
		}
		if got, _ := h.rt.DeliverEvent(ctx, "travel", "booked", "lead-1", map[string]any{"day": "quarta"}); len(got) != 0 {
			t.Errorf("k=%d: a second event resumed the run again", k)
		}
	}
}

// An event ends only the wait that listens for it: after that wait timed
// out, the run's next wait (which does not listen) is not ended by it.
const twoWaits = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: two, namespace: travel }
spec:
  description: A wait for an event, then a plain wait.
  steps:
    - { id: w1, wait: { for: 1h, events: [ { name: booked, key: "'lead-1'" } ] } }
    - { id: w2, wait: { for: 48h } }
    - { id: fim, respond: { template: "fim" } }
  output: { value: "true" }
`

func TestEventOnlyEndsItsWait(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(twoWaits), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	h.rt.Clock = func() time.Time { return now }
	ctx := context.Background()
	h.startFn("two", "")
	now = now.Add(2 * time.Hour)
	wakeAll(t, h) // w1 times out; the run now sleeps at w2
	got, err := h.rt.DeliverEvent(ctx, "travel", "booked", "lead-1", map[string]any{})
	if err != nil || len(got) != 0 {
		t.Fatalf("a wait that does not listen was ended: %+v %v", got, err)
	}
	if run, _ := h.store.GetRun(ctx, "run-1"); run.Status != journal.StatusWaitingForTime || run.Pending != "steps.w2" {
		t.Fatalf("%s at %s", run.Status, run.Pending)
	}
}
