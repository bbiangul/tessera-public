package admission

import (
	"strings"
	"testing"
)

// #49: the free mode's search is a read taking exactly {query}.
func TestFreeModeSearch(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: web, version: 0.1.0 }
spec:
  actions:
    search: { input: { query: string }, output: list, effect: read }
    post: { input: { query: string }, output: map, effect: external-message }
    lookup: { input: { query: string, lang: string }, output: list, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  freeMode:
    instructions: "Tire dúvidas."
    search: { act: web/search }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid search refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"an unknown plugin", "act: web/search", "act: busca/search", "UNKNOWN_PLUGIN@spec.freeMode.search.act"},
		{"an unknown action", "act: web/search", "act: web/find", "UNKNOWN_ACTION@spec.freeMode.search.act"},
		{"an effect", "act: web/search", "act: web/post", "EFFECT_NOT_ALLOWED@spec.freeMode.search.act"},
		{"other inputs", "act: web/search", "act: web/lookup", "INVALID_FIELD@spec.freeMode.search.act"},
		{"too many results", "act: web/search }", "act: web/search, maxResults: 50 }", "spec.freeMode.search.maxResults"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// drift-007: improvise.repair lists http actions that send a body, and a
// person approves repairs.
func TestImproviseRepair(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: c, version: 0.1.0 }
spec:
  egress: { hosts: [x.test] }
  connection: { baseUrl: "https://x.test" }
  actions:
    move: { input: { id: string }, effect: irreversible, http: { method: POST, path: /move, body: { id: "${ inputs.id }" } } }
    look: { input: { id: string }, effect: read, http: { method: GET, path: /look } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: equipe, namespace: ns }
spec: { description: a equipe }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  humanInTheLoop: { queue: equipe }
  improvise: { repair: [c/move] }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid improvise refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no body", "repair: [c/move]", "repair: [c/look]", "INVALID_FIELD@spec.improvise.repair[0]"},
		{"unknown", "repair: [c/move]", "repair: [c/nope]", "UNKNOWN_ACTION@spec.improvise.repair[0]"},
		{"nobody approves", "  humanInTheLoop: { queue: equipe }\n", "", "MISSING_FIELD@spec.humanInTheLoop"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// drift-008: declared types have valid names and field types, and an
// output naming the plugin's own type names a declared one.
func TestPluginTypes(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: c, version: 0.1.0 }
spec:
  types:
    Appt: { fields: { id: string, start: timestamp, slots: "list<c.Slot>", note: "string?" } }
    Slot: { fields: { id: string } }
  actions:
    list: { input: {}, output: list<c.Appt>, effect: read }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid types refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"bad name", "    Slot:", "    slot:", "INVALID_FIELD@spec.types.slot"},
		{"unknown field type", "note: \"string?\"", "note: \"text?\"", "UNKNOWN_TYPE@spec.types.Appt.fields.note"},
		{"undeclared output", "output: list<c.Appt>", "output: list<c.Visit>", "UNKNOWN_TYPE@spec.actions.list.output"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}
