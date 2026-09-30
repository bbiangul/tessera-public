package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const contractDocs = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: q, version: 1.0.0 }
spec:
  types:
    Appt: { fields: { id: string, start: string, label: string, note: "string?" } }
  actions:
    list: { input: {}, output: list<q.Appt>, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: r }
spec:
  description: contract scenario
  inputs:
    appts:
      type: list<q.Appt>
      resolve: [ { id: listed, fromAction: q/list } ]
  output: { template: "{{ inputs.appts[0].label }} {{ inputs.appts[0].start }}" }
`

type renameGen struct{ rename map[string]any }

func (renameGen) Name() string { return "rename" }
func (g renameGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	if req.Instructions == renameRules {
		return &generate.Response{Value: map[string]any{"rename": g.rename}}, nil
	}
	return &generate.Response{Text: "NONE"}, nil
}

func contractHarness(t *testing.T, answer []any, rename map[string]any, repair bool) *harness {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(contractDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	r := act.NewRegistry()
	r.Register("q/list", func(context.Context, act.Call) (any, error) { return answer, nil })
	h.rt.Actor, h.rt.Generator = r, renameGen{rename: rename}
	h.rt.Limits.RepairReads = repair
	return h
}

var renamed = []any{map[string]any{"id": "a1", "startsAt": "2026-10-02T09:00", "description": "sexta 02/10 às 9h", "createdAt": "2026-09-20T10:00", "extra": 1}}

// An answer that meets its declared type passes untouched, extra fields
// and all; one that does not is a CONTRACT failure naming the fields.
func TestReadsAreCheckedAgainstTheirType(t *testing.T) {
	h := contractHarness(t, []any{map[string]any{"id": "a1", "start": "s", "label": "l", "extra": true}}, nil, false)
	expectStatus(t, h.startF(), journal.StatusSucceeded)
	if h.user[len(h.user)-1] != "l s" {
		t.Errorf("said %v", h.user)
	}
	h = contractHarness(t, renamed, nil, false)
	res := h.startF()
	expectStatus(t, res, journal.StatusFailed)
	recs, _ := h.store.Records(context.Background(), res.RunID)
	var contract bool
	for _, r := range recs {
		contract = contract || (strings.Contains(string(r.Value), "CONTRACT") && strings.Contains(string(r.Value), "missing label, start"))
	}
	if !contract {
		t.Error("the failure must say which fields are missing")
	}
}

// With repairReads, renamed fields are mapped back, checked, and the
// rename is journaled.
func TestRenamedFieldsAreRepaired(t *testing.T) {
	h := contractHarness(t, renamed, map[string]any{"start": "startsAt", "label": "description"}, true)
	res := h.startF()
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[len(h.user)-1] != "sexta 02/10 às 9h 2026-10-02T09:00" {
		t.Errorf("said %v", h.user)
	}
	recs, _ := h.store.Records(context.Background(), res.RunID)
	var journaled bool
	for _, r := range recs {
		journaled = journaled || (strings.HasSuffix(r.Site, "/repaired") && strings.Contains(string(r.Value), `"start":"startsAt"`))
	}
	if !journaled {
		t.Error("the rename must be journaled")
	}
}

// A rename that misses a field, moves a declared field, uses a source
// twice or maps a value of the wrong type is refused: the read fails.
func TestBadRenamesAreRefused(t *testing.T) {
	for name, rename := range map[string]map[string]any{
		"misses one":       {"start": "startsAt"},
		"moves a declared": {"start": "startsAt", "label": "id"},
		"source twice":     {"start": "startsAt", "label": "startsAt"},
		"wrong type":       {"start": "startsAt", "label": "extra"},
	} {
		h := contractHarness(t, renamed, rename, true)
		if res := h.startF(); res.Status != journal.StatusFailed {
			t.Errorf("%s: status %s, said %v", name, res.Status, h.user)
		}
	}
}
