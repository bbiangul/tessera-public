package admission

import (
	"os"
	"strings"
	"testing"
)

// knowledge-001: KnowledgeBase rules.
func TestKnowledgeRules(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  audiences:
    - { name: equipe, who: "size(caller.roles) > 0", functions: [], freeMode: { instructions: "x", knowledge: { bases: [docs] } } }
    - { name: paciente, functions: [horarios], freeMode: { instructions: "x", knowledge: { bases: [docs] } } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: docs, namespace: ns }
spec: { description: d, sources: [ { files: ["*.md"] } ], audiences: [equipe] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: ns }
spec:
  description: d
  selector: { intent: horarios }
  output: { template: "ok" }
`
	notext, err := os.ReadFile("../knowledge/testdata/notext.pdf")
	if err != nil {
		t.Fatal(err)
	}
	files := func(agent string) map[string]string {
		return map[string]string{"a.yaml": agent, "faq.md": "# FAQ\n\nok\n", "corrupt.pdf": "not a pdf", "notext.pdf": string(notext)}
	}
	if got := codesAt(validateDir(t, files(base))); got != "" {
		t.Fatalf("valid knowledge refused or warned: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no files", `files: ["*.md"]`, `files: ["nada/*.md"]`, "NO_FILES@spec.sources[0].files"},
		{"unsupported", `files: ["*.md"]`, `files: ["*.md", "a.yaml"]`, "UNSUPPORTED_FILE@spec.sources[0].files"},
		{"corrupt pdf", `files: ["*.md"]`, `files: ["*.md", "corrupt.pdf"]`, "UNREADABLE_FILE@spec.sources[0].files"},
		{"no text", `files: ["*.md"]`, `files: ["*.md", "notext.pdf"]`, "NO_TEXT@spec.sources[0].files"},
		{"unknown audience", "audiences: [equipe]", "audiences: [equipes]", "UNKNOWN_AUDIENCE@spec.audiences[0]"},
		{"unknown base", "knowledge: { bases: [docs] } } }\n    - { name: paciente", "knowledge: { bases: [doc] } } }\n    - { name: paciente", "UNKNOWN_KNOWLEDGE@spec.audiences[0].freeMode.knowledge.bases[0]"},
	} {
		if !strings.Contains(base, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := codesAt(validateDir(t, files(strings.Replace(base, c.old, c.new, 1)))); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// plugin-001: a plugin source's rules.
func TestKnowledgePluginSourceRules(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: docs, version: 0.1.0 }
spec:
  actions:
    list: { input: {}, output: list, effect: read }
    get: { input: { id: string }, output: map, effect: read }
    send: { input: { id: string }, output: map, effect: idempotent-write }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: docs, namespace: ns }
spec:
  description: d
  sources:
    - list: { act: docs/list }
      fetch: { act: docs/get, with: { id: "${ inputs.id }" } }
      refresh: 1h
`
	if got := codesAt(validate(t, map[string]string{"a.yaml": base})); got != "" {
		t.Fatalf("valid source refused or warned: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"not a read", "fetch: { act: docs/get", "fetch: { act: docs/send", "EFFECT_NOT_ALLOWED@spec.sources[0].fetch.act"},
		{"unknown action", "list: { act: docs/list }", "list: { act: docs/lista }", "UNKNOWN_ACTION@spec.sources[0].list.act"},
		{"refresh too short", "refresh: 1h", "refresh: 30s", "INVALID_FIELD@spec.sources[0].refresh"},
		{"files and list", "    - list: { act: docs/list }", "    - files: [\"*.md\"]\n      list: { act: docs/list }", "INVALID_FIELD@spec.sources[0]"},
		{"bad expression", `"${ inputs.id }"`, `"${ inputs.id + }"`, "INVALID_EXPR@spec.sources[0].fetch.with"},
	} {
		if !strings.Contains(base, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := codesAt(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// retrieve-001: a retrieve step's rules.
func TestRetrieveStepRules(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: docs, namespace: ns }
spec: { description: d, sources: [ { files: ["*.md"] } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: d
  selector: { intent: f }
  steps:
    - { id: r, retrieve: { bases: [docs], query: "event.message", k: 3 } }
  output: { template: "{{ steps.r.output.size() }}" }
`
	files := func(s string) map[string]string { return map[string]string{"a.yaml": s, "faq.md": "# FAQ\n\nok\n"} }
	if got := codesAt(validateDir(t, files(base))); got != "" {
		t.Fatalf("valid retrieve refused or warned: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown base", "bases: [docs]", "bases: [doc]", "UNKNOWN_KNOWLEDGE@spec.steps[0].retrieve.bases[0]"},
		{"not a string", `query: "event.message"`, `query: "1 + 1"`, "spec.steps[0].retrieve.query"},
		{"two kinds", "retrieve: { bases: [docs]", "respond: { template: x }, retrieve: { bases: [docs]", "STEP_KIND@spec.steps[0]"},
	} {
		if !strings.Contains(base, c.old) {
			t.Fatalf("%s: fixture lacks %q", c.name, c.old)
		}
		if got := codesAt(validateDir(t, files(strings.Replace(base, c.old, c.new, 1)))); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// learn-003: a learning base's rules.
func TestLessonsRules(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec: { humanInTheLoop: { queue: recepcao } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: licoes, namespace: ns }
spec: { description: d, lessons: { every: 24h } }
`
	if got := codesAt(validate(t, map[string]string{"a.yaml": base})); got != "" {
		t.Fatalf("valid lessons refused or warned: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown queue", "lessons: { every: 24h }", "lessons: { every: 24h, queue: gerencia }", "UNKNOWN_QUEUE@spec.lessons.queue"},
		{"no queue", "spec: { humanInTheLoop: { queue: recepcao } }", "spec: {}", "MISSING_FIELD@spec.lessons.queue"},
		{"too often", "every: 24h", "every: 30m", "INVALID_FIELD@spec.lessons.every"},
		{"nothing", "lessons: { every: 24h }", "audiences: []", "MISSING_FIELD@spec.sources"},
	} {
		if got := codesAt(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}
