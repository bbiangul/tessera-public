package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func TestRunReadFactsIsolation(t *testing.T) {
	h := newHarness(t)
	h.start("quero remarcar a de terça")
	ctx := context.Background()
	for _, tc := range []struct {
		name, namespace, conversation string
		caller                        map[string]any
		allowed                       []string
		wantError                     bool
	}{
		{"declared", "agenda-exemplo", "c1", patient, []string{"agenda/find-slots"}, false},
		{"default-private", "agenda-exemplo", "c1", patient, nil, false},
		{"other-namespace", "other", "c1", patient, []string{"agenda/find-slots"}, true},
		{"other-conversation", "agenda-exemplo", "other", patient, []string{"agenda/find-slots"}, true},
		{"other-patient", "agenda-exemplo", "c1", map[string]any{"patientId": "p2"}, []string{"agenda/find-slots"}, true},
		{"no-caller", "agenda-exemplo", "c1", nil, []string{"agenda/find-slots"}, true},
		{"effect", "agenda-exemplo", "c1", patient, []string{"agenda/reschedule"}, true},
		{"unknown-action", "agenda-exemplo", "c1", patient, []string{"agenda/missing"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := h.rt.RunReadFacts(ctx, "run-1", tc.namespace, tc.conversation, tc.caller, tc.allowed)
			if (err != nil) != tc.wantError {
				t.Fatalf("facts=%v err=%v", facts, err)
			}
			if tc.name == "declared" {
				b, err := json.Marshal(facts)
				if err != nil {
					t.Fatal(err)
				}
				if len(facts) != 1 || !strings.Contains(string(b), "16:00") || strings.Contains(string(b), "Seg 21/09") {
					t.Fatalf("only the slots, not the other reads: %s", b)
				}
			} else if len(facts) != 0 {
				t.Fatalf("unexpected disclosure: %v", facts)
			}
		})
	}
	if h.clin.calls["slots"] != 1 || h.clin.reschedules != 0 {
		t.Fatalf("context executed a plugin: %v", h.clin.calls)
	}
	// A completed run or changed generation cannot supply context.
	run, err := h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	gen := h.rt.Program.gen
	h.rt.Program.gen = "another-generation"
	if _, err := h.rt.RunReadFacts(ctx, run.ID, run.Namespace, run.Conversation, patient, []string{"agenda/find-slots"}); err == nil {
		t.Fatal("another generation was read")
	}
	h.rt.Program.gen = gen
	if err := h.store.UpdateRun(ctx, run.ID, journal.StatusSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.RunReadFacts(ctx, run.ID, run.Namespace, run.Conversation, patient, []string{"agenda/find-slots"}); err == nil {
		t.Fatal("finished run was read")
	}
}

type readFactsActor struct {
	act.Actor
	huge bool
}

func (a readFactsActor) Call(ctx context.Context, c act.Call) (any, error) {
	out, err := a.Actor.Call(ctx, c)
	if err == nil && c.Action == "agenda/find-slots" {
		first := out.([]any)[0].(map[string]any)
		first["contact"] = "ana@example.com"
		if a.huge {
			first["note"] = strings.Repeat("x", 33*1024)
		}
	}
	return out, err
}

func TestRunReadFactsOnlySuccessfulTopLevelSources(t *testing.T) {
	for _, tc := range []struct {
		name, site string
		kind       journal.Kind
		value      []byte
		wantError  bool
	}{
		{"failed-read", "inputs.newSlot/slots", journal.KindError, []byte(`{"code":"FORBIDDEN"}`), false},
		{"nested-read", "call/helper/inputs.newSlot/slots", journal.KindResult, []byte(`"private nested output"`), false},
		{"arbitrary-result", "inputs.other/value", journal.KindResult, []byte(`"private input"`), false},
		{"corrupt-selected-read", "inputs.newSlot/slots", journal.KindResult, []byte(`{`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			scopes, err := journal.Encode(map[string]any{"caller": patient})
			if err != nil {
				t.Fatal(err)
			}
			run := journal.Run{ID: "pending", Namespace: "agenda-exemplo", Conversation: "c1", Function: "reschedule-appointment", Generation: h.rt.Program.Generation(), Scopes: scopes, Status: journal.StatusWaitingForUser}
			if err := h.store.CreateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			if _, err := h.store.Append(ctx, run.ID, journal.Record{Site: tc.site, Kind: tc.kind, Value: tc.value}); err != nil {
				t.Fatal(err)
			}
			facts, err := h.rt.RunReadFacts(ctx, run.ID, run.Namespace, run.Conversation, patient, []string{"agenda/find-slots"})
			if (err != nil) != tc.wantError || len(facts) != 0 {
				t.Fatalf("facts=%v error=%v", facts, err)
			}
		})
	}
}

func TestRunReadFactsMaskingAndBounds(t *testing.T) {
	for _, huge := range []bool{false, true} {
		h := newHarness(t)
		// Decorate the read response before it is recorded, not the context.
		actor := h.rt.Actor
		h.rt.Actor = readFactsActor{Actor: actor, huge: huge}
		h.start("quero remarcar a de terça")
		facts, err := h.rt.RunReadFacts(context.Background(), "run-1", "agenda-exemplo", "c1", patient, []string{"agenda/find-slots"})
		if huge {
			if err == nil || !strings.Contains(err.Error(), "32 KiB") || facts != nil {
				t.Fatalf("oversized context was not refused: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(facts)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "ana@example.com") || !strings.Contains(string(b), "[EMAIL]") {
			t.Fatalf("read output was not masked: %s", b)
		}
	}
}
