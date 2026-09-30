package drift

import (
	"context"
	"strings"
	"testing"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

var now = time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)

func run(t *testing.T, st *journal.Memory, id string, recs ...journal.Record) {
	t.Helper()
	if err := st.CreateRun(context.Background(), journal.Run{ID: id, Namespace: "c", Function: "f", Status: journal.StatusSucceeded, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		r.At = now
		if _, err := st.Append(context.Background(), id, r); err != nil {
			t.Fatal(err)
		}
	}
}

func rec(site string, kind journal.Kind, v any) journal.Record {
	b, err := journal.Encode(v)
	if err != nil {
		panic(err)
	}
	return journal.Record{Site: site, Kind: kind, Value: b}
}

var manifest = &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "clinic"}, Spec: plugins.PluginSpec{
	Types: map[string]plugins.TypeSpec{"Appt": {Fields: map[string]string{"id": "string", "start": "string", "label": "string", "note": "string?"}}},
	Actions: map[string]plugins.ActionSpec{
		"list": {Effect: plugins.EffectRead, Output: "list<clinic.Appt>", HTTP: &plugins.HTTPCall{Method: "GET", Path: "/a"}},
	},
}}

// Every kind of signal lands on its action, and proposals follow.
func TestTheReportFindsEachSignal(t *testing.T) {
	st := journal.NewMemory()
	contract := rec("inputs.a/upcoming", journal.KindError, map[string]any{"definite": true, "code": "CONTRACT", "action": "clinic/list",
		"message": "clinic/list: the answer does not match list<clinic.Appt>: item 0: missing label, start (has id, startsAt)"})
	renamed := rec("inputs.a/upcoming/repaired", journal.KindResult, map[string]any{"action": "clinic/list", "rename": map[string]any{"start": "startsAt", "label": "description"}})
	for _, id := range []string{"r1", "r2"} {
		run(t, st, id, contract, renamed)
	}
	run(t, st, "r3",
		rec("action", journal.KindError, map[string]any{"definite": true, "code": "INVALID", "action": "clinic/move", "message": "move: server answered 402", "detail": "a fee of R$ 50 applies"}),
		rec("action/repair/proposal", journal.KindResult, map[string]any{"action": "clinic/move", "detail": "a fee of R$ 50 applies",
			"proposal": map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}, "userMustAgree": "Taxa de R$ 50. Concorda?"}}),
		rec("action/repair/approve", journal.KindResume, map[string]any{"text": "yes"}),
		rec("action/repair/call", journal.KindResult, map[string]any{"ok": true}),
	)
	r, err := Analyze(context.Background(), []Store{st}, "c", now.Add(-time.Hour), map[string]*plugins.PluginManifest{"clinic": manifest})
	if err != nil {
		t.Fatal(err)
	}
	if r.Runs != 3 || len(r.Actions) != 2 {
		t.Fatalf("runs %d, actions %+v", r.Runs, r.Actions)
	}
	list, move := r.Actions[0], r.Actions[1]
	if len(list.Contracts) != 2 || list.Contracts[0].Count != 2 {
		t.Errorf("contracts %+v", list.Contracts)
	}
	if list.Renames[0].Key != "label ← description, start ← startsAt" {
		t.Errorf("renames %+v", list.Renames)
	}
	if len(list.Proposals) != 1 || list.Proposals[0].Kind != "binding-output" {
		t.Fatalf("proposals %+v", list.Proposals)
	}
	if move.Refusals[0].Key != "INVALID: a fee of R$ 50 applies" || move.Repairs[0].Succeeded != 1 || move.Repairs[0].Approved != 1 {
		t.Errorf("move %+v", move)
	}
	if len(move.Proposals) != 1 || !strings.Contains(move.Proposals[0].Text, "name it in the action's confirm") {
		t.Errorf("an author note that keeps the fee asked: %+v", move.Proposals)
	}
	md := r.Markdown()
	for _, want := range []string{"## clinic/list", "## clinic/move", "```yaml"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
}

// The proposed output compiles and maps the new answer back.
func TestTheProposedOutputMapsBack(t *testing.T) {
	a := &Action{Action: "clinic/list", Renames: []Signal{{Key: "label ← description, start ← startsAt", Count: 3}}}
	p, ok := bindingOutput(a, map[string]*plugins.PluginManifest{"clinic": manifest})
	if !ok {
		t.Fatal("no proposal")
	}
	_, quoted, _ := strings.Cut(p.YAML, "output: ")
	src := strings.Trim(strings.TrimSpace(quoted), `"`)
	src = strings.ReplaceAll(src, `\"`, `"`)
	e, err := expr.New()
	if err != nil {
		t.Fatal(err)
	}
	x, err := e.Compile(src, expr.Any)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, err := x.Eval(context.Background(), expr.Activation{"sources": map[string]any{"response": map[string]any{"body": []any{
		map[string]any{"id": "a1", "startsAt": "s", "description": "l", "createdAt": "decoy"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	got := v.([]any)[0].(map[string]any)
	if got["start"] != "s" || got["label"] != "l" || got["id"] != "a1" || got["note"] != nil {
		t.Errorf("mapped %v", got)
	}
}

// Runs with no signal give an empty report: no false alarm.
func TestNoSignalsNoFindings(t *testing.T) {
	st := journal.NewMemory()
	run(t, st, "r1", rec("action", journal.KindResult, map[string]any{"ok": true}),
		rec("inputs.x/y", journal.KindError, map[string]any{"definite": true, "code": "NOT_FOUND", "action": "clinic/list", "message": "no"}))
	r, err := Analyze(context.Background(), []Store{st}, "c", now.Add(-time.Hour), nil)
	if err != nil || len(r.Actions) != 0 || !strings.Contains(r.Markdown(), "No drift signals") {
		t.Errorf("%v %+v", err, r.Actions)
	}
}
