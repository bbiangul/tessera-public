package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Regression tests for the adversarial review of the runtime (2026-09-23).
// Each test reproduces a finding's scenario.

const reviewPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: p, version: 1.0.0 }
spec:
  actions:
    check-a: { input: { key: string }, effect: read }
    check-b: { input: { key: string }, effect: read }
    book-a:
      input: { key: string }
      effect: irreversible
      verify: { mode: probe, with: check-a, args: { key: "${ inputs.key }" }, happened: "sources.probe.output.done" }
    book-b:
      input: { key: string }
      effect: irreversible
      verify: { mode: probe, with: check-b, args: { key: "${ inputs.key }" }, happened: "sources.probe.output.done" }
    book-n: { input: { key: string }, effect: irreversible, verify: { mode: native } }
    send: { input: { key: string }, effect: irreversible }
    words: { input: {}, output: "list<string>", effect: read }
`

func fn(name, action, onError string) string {
	return `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: ` + name + `, namespace: r }
spec:
  description: review scenario
  action:
    act: ` + action + `
    with: { key: k1 }
    idempotencyKey: "'k1'"
  onError: ` + onError + `
  output: { template: "done" }
`
}

// fakeP records calls; behavior per action is scripted per test.
type fakeP struct {
	mu      sync.Mutex
	calls   []string
	applied map[string]int
	on      map[string]func(c act.Call) (any, error)
}

func (p *fakeP) actor() *act.Registry {
	r := act.NewRegistry()
	for _, a := range []string{"check-a", "check-b", "book-a", "book-b", "book-n", "send", "words"} {
		a := a
		r.Register("p/"+a, func(_ context.Context, c act.Call) (any, error) {
			p.mu.Lock()
			p.calls = append(p.calls, a)
			h := p.on[a]
			p.mu.Unlock()
			if h != nil {
				return h(c)
			}
			return map[string]any{}, nil
		})
	}
	return r
}

func (p *fakeP) apply(what string) {
	p.mu.Lock()
	p.applied[what]++
	p.mu.Unlock()
}

func reviewHarness(t *testing.T, fns ...string) (*harness, *fakeP) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(reviewPlugin), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(strings.Join(fns, "---\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	p := &fakeP{applied: map[string]int{}, on: map[string]func(act.Call) (any, error){}}
	h.rt.Actor = p.actor()
	return h, p
}

func (h *harness) startR(name string) Result {
	h.t.Helper()
	res, err := h.rt.Start(context.Background(), Start{Namespace: "r", Function: name, Conversation: "c1"})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.deliver(res)
}

// Finding 1: after a fallback act failed, later items verified and retried
// the ORIGINAL action, so the booking happened twice.
func TestLaterStrategiesActOnTheFallbackCall(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/book-a", "[ {probe: {}}, {fallback: {act: p/book-b, with: {key: k1}, idempotencyKey: \"'k1'\"}}, {probe: {}}, {retry: {attempts: 1}} ]"))
	p.on["book-a"] = func(act.Call) (any, error) {
		return nil, &act.DefiniteError{Code: "FULL", Message: "no seats"}
	}
	p.on["book-b"] = func(act.Call) (any, error) {
		p.apply("b")
		return nil, &act.AmbiguousError{Message: "timeout"} // applied, answer lost
	}
	p.on["check-b"] = func(act.Call) (any, error) { return map[string]any{"done": p.applied["b"] > 0}, nil }
	res := h.startR("f")
	expectStatus(t, res, journal.StatusSucceeded)
	a := 0
	for _, c := range p.calls {
		if c == "book-a" {
			a++
		}
	}
	if a != 1 || p.applied["b"] != 1 || strings.Contains(strings.Join(p.calls, ","), "check-a") {
		t.Errorf("calls %v: the second probe must check book-b and nothing may run book-a again", p.calls)
	}
}

// Finding 2: any answer to an escalation counted as success.
func TestEscalationAnswerMustSayWhetherItHappened(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/send", "[ {escalate: {queue: team, task: input}} ]"))
	p.on["send"] = func(act.Call) (any, error) { return nil, &act.DefiniteError{Code: "FORBIDDEN", Message: "no"} }
	res := h.startR("f")
	expectStatus(t, res, journal.StatusWaitingForHuman)
	res = h.reply("human", "não consegui falar com a clínica", nil)
	expectStatus(t, res, journal.StatusFailed)
	if len(h.user) > 0 && h.user[len(h.user)-1] == "done" {
		t.Error("a 'no' must never produce the success message")
	}

	h, p = reviewHarness(t, fn("f", "p/send", "[ {escalate: {queue: team, task: input}} ]"))
	p.on["send"] = func(act.Call) (any, error) { return nil, &act.DefiniteError{Code: "FORBIDDEN", Message: "no"} }
	h.startR("f")
	res = h.reply("human", "vou ver isso depois", nil) // unclear: asked again
	expectStatus(t, res, journal.StatusWaitingForHuman)
	res = h.reply("human", "sim, liguei e remarquei", nil)
	expectStatus(t, res, journal.StatusSucceeded)
}

// Finding 3: a finished run's unacked messages were lost.
func TestFinishedRunStillReturnsUnackedMessages(t *testing.T) {
	h, _ := reviewHarness(t, fn("f", "p/send", "[]"))
	res, err := h.rt.Start(context.Background(), Start{Namespace: "r", Function: "f", Conversation: "c1"})
	if err != nil || res.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", res, err)
	}
	// The channel crashed before delivering "done".
	again, err := h.rt.Resume(context.Background(), res.RunID)
	if err != nil || len(again.Outbox) != 1 || again.Outbox[0].Text != "done" {
		t.Errorf("redelivery after the run finished: %+v %v", again.Outbox, err)
	}
}

// Finding 5: an unknown outcome with onError [fail] ended without a person.
func TestUnknownOutcomeAlwaysReachesAPerson(t *testing.T) {
	for _, chain := range []string{"[ {fail: {message: x}} ]", "[ {respond: {template: 'ops'}} ]"} {
		h, p := reviewHarness(t, fn("f", "p/send", chain))
		p.on["send"] = func(act.Call) (any, error) { return nil, &act.AmbiguousError{Message: "timeout after sending"} }
		res := h.startR("f")
		expectStatus(t, res, journal.StatusIndeterminate)
		// A plain-text yes is understood (finding 7).
		res = h.reply("human", "sim, a mensagem chegou", nil)
		expectStatus(t, res, journal.StatusSucceeded)
	}
}

// Finding 6: native verification only ran with an explicit probe item.
func TestNativeVerificationRunsWithoutAProbeItem(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/book-n", "[]"))
	var keys []string
	p.on["book-n"] = func(c act.Call) (any, error) {
		keys = append(keys, c.IdempotencyKey)
		if len(keys) == 1 {
			return nil, &act.AmbiguousError{Message: "timeout"}
		}
		return map[string]any{"ok": true}, nil // the system deduplicates by key
	}
	res := h.startR("f")
	expectStatus(t, res, journal.StatusSucceeded)
	if len(keys) != 2 || keys[0] != keys[1] {
		t.Errorf("native verification is a second call with the same key: %v", keys)
	}
}

// Finding 8: input order changed the path but not the generation.
func TestInputOrderIsPartOfTheGeneration(t *testing.T) {
	a := "apiVersion: agents.tessera.dev/v1alpha1\nkind: Function\nmetadata: { name: g, namespace: r }\nspec:\n  description: d\n  inputs:\n    alpha: { type: string, resolve: [ { id: v, value: \"'a'\" } ] }\n    zeta: { type: string, resolve: [ { id: v, value: \"'z'\" } ] }\n"
	b := "apiVersion: agents.tessera.dev/v1alpha1\nkind: Function\nmetadata: { name: g, namespace: r }\nspec:\n  description: d\n  inputs:\n    zeta: { type: string, resolve: [ { id: v, value: \"'z'\" } ] }\n    alpha: { type: string, resolve: [ { id: v, value: \"'a'\" } ] }\n"
	g := func(src string) string {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(src), 0o600)
		return admit(t, dir).Generation()
	}
	if g(a) == g(b) {
		t.Error("reordering inputs changes the path, so it must change the generation")
	}
}

// Finding 9: a plugin output the codec could not encode wedged the run.
func TestTypedPluginOutputsAreRecorded(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(reviewPlugin), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(`apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: w, namespace: r }
spec:
  description: d
  inputs:
    all: { type: "list<string>", resolve: [ { id: w, fromAction: p/words } ] }
  output: { template: "{{ inputs.all.join(',') }}" }
`), 0o600)
	h := newHarness(t, dir)
	r := act.NewRegistry()
	r.Register("p/words", func(context.Context, act.Call) (any, error) { return []string{"a", "b"}, nil })
	h.rt.Actor = r
	res := h.startR("w")
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[0] != "a,b" {
		t.Errorf("messages %q", h.user)
	}
}

// Finding 15: the intent did not record what was attempted.
func TestIntentRecordsTheArguments(t *testing.T) {
	h, _ := reviewHarness(t, fn("f", "p/send", "[]"))
	res := h.startR("f")
	recs, _ := h.store.Records(context.Background(), res.RunID)
	for _, r := range recs {
		if r.Kind == journal.KindIntent && !strings.Contains(string(r.Value), `"args":{"key":"k1"}`) {
			t.Errorf("intent %s", r.Value)
		}
	}
}

// Finding 4: a reply recorded before a failed execution blocked the
// conversation; the same reply delivered again now finishes it.
func TestRetriedReplyFinishesTheRun(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar")
	h.store.FailAppendAfter = 2 // the reply is stored, the next write fails
	if _, err := h.rt.Deliver(context.Background(), "run-1", Reply{From: "user", Text: "a de terça"}); !errors.Is(err, journal.ErrInjected) {
		t.Fatalf("want the injected failure, got %v", err)
	}
	res := h.reply("user", "a de terça", nil)
	expectStatus(t, res, journal.StatusWaitingForUser)
	if last := h.user[len(h.user)-1]; !strings.Contains(last, "Tenho estes horários") {
		t.Errorf("the run must have moved on: %q", h.user)
	}
}

// #30: a strategy's when reads the failure (sources.error), so a refused
// move (409) and a vanished appointment (404) get different answers.
func TestOnErrorWhenReadsTheFailure(t *testing.T) {
	chain := `[ {when: "sources.error.code == 'CONFLICT'", respond: {template: "taken"}}, {respond: {template: "gone: {{ sources.error.code }}"}} ]`
	for code, want := range map[string]string{"CONFLICT": "taken", "NOT_FOUND": "gone: NOT_FOUND"} {
		h, p := reviewHarness(t, fn("f", "p/send", chain))
		p.on["send"] = func(act.Call) (any, error) { return nil, &act.DefiniteError{Code: code, Message: "no"} }
		res := h.startR("f")
		expectStatus(t, res, journal.StatusFailed)
		if len(h.user) == 0 || h.user[len(h.user)-1] != want {
			t.Errorf("%s: said %v, want %q", code, h.user, want)
		}
	}
}

// when only narrows: an unknown outcome still reaches a person, even when
// every strategy's condition says respond.
func TestOnErrorWhenCannotSkipTheSafetyRules(t *testing.T) {
	h, p := reviewHarness(t, fn("f", "p/send", `[ {when: "true", respond: {template: "sorry"}} ]`))
	p.on["send"] = func(act.Call) (any, error) {
		p.apply("send")
		return nil, &act.AmbiguousError{Message: "timeout"}
	}
	res := h.startR("f")
	expectStatus(t, res, journal.StatusIndeterminate)
	for _, m := range h.user {
		if m == "sorry" {
			t.Error("an unknown outcome was answered as a plain failure")
		}
	}
}

// onUnresolved honours when too: the input's sources and the last failure
// are readable, and a false condition skips the strategy.
func TestOnUnresolvedWhen(t *testing.T) {
	doc := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: u, namespace: r }
spec:
  description: unresolved scenario
  inputs:
    w:
      type: string
      resolve:
        - { id: got, fromAction: p/words }
        - { id: first, when: "size(sources.got.output) > 0", value: "sources.got.output[0]" }
      onUnresolved:
        - { when: "sources.error.code == 'UNAVAILABLE'", respond: { template: "offline" } }
        - { respond: { template: "none: {{ size(sources.got.output) }}" } }
  output: { template: "{{ inputs.w }}" }
`
	h, p := reviewHarness(t, doc)
	p.on["words"] = func(act.Call) (any, error) { return []any{}, nil }
	res := h.startR("u")
	expectStatus(t, res, journal.StatusFailed)
	if len(h.user) == 0 || h.user[len(h.user)-1] != "none: 0" {
		t.Errorf("empty list: said %v", h.user)
	}
}
