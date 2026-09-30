package admission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// travelPlugin and travelFunction are a valid baseline with every step
// kind; each case below plants one defect.
const travelPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: booking, version: 1.0.0 }
spec:
  actions:
    search-flights: { input: { from: string, to: string }, output: "list<booking.Offer>", effect: read }
    book: { input: { token: string }, output: booking.Booking, effect: irreversible }
`

const travelScript = `
def score(offers, weights = None):
    return sorted(offers, key = lambda o: o["price"])

def top(ranked, n):
    return ranked[:n]
`

const travelFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: search, namespace: travel }
spec:
  description: Searches flights and books the best one.
  inputs:
    to: { type: string, resolve: [ { id: v, value: "event.message" } ] }
  steps:
    - id: offers
      act: booking/search-flights
      with: { from: "GRU", to: "${ inputs.to }" }
    - id: ranked
      compute:
        starlark: { script: score.star, function: score, with: { offers: "${ steps.offers.output }" } }
    - id: best
      compute: { cel: "steps.ranked.output[0]" }
    - id: urgent
      decide:
        state: { message: "${ event.message }" }
        instructions: Is the trip urgent?
        yesNo: { ifTrue: travels within 48 hours }
        minConfidence: 0.8
        onLowConfidence: continue
    - id: tell
      when: "steps.urgent.output.value == true"
      respond: { template: "Vou priorizar voos para hoje." }
    - id: route
      branch:
        - when: "size(steps.offers.output) == 0"
          respond: { template: "Não achei voos." }
        - else: true
          respond: { template: "Achei {{ size(steps.offers.output) }} voos." }
  action:
    act: booking/book
    with: { token: "${ steps.best.output.token }" }
    confirm: "Reservo o voo de {{ steps.best.output.price }}?"
    idempotencyKey: "steps.best.output.token"
  output:
    template: "Reservado: {{ steps.best.output.token }}"
`

func validateDir(t *testing.T, files map[string]string) []Issue {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l, err := NewLoader()
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewChecker()
	if err != nil {
		t.Fatal(err)
	}
	docs, issues := l.LoadPaths([]string{dir})
	return append(issues, c.Check(docs)...)
}

func travelFiles(fn string) map[string]string {
	return map[string]string{"plugin.yaml": travelPlugin, "fn.yaml": fn, "score.star": travelScript}
}

func TestStepsBaselineIsClean(t *testing.T) {
	if is := errorsOnly(validateDir(t, travelFiles(travelFunction))); len(is) > 0 {
		t.Fatalf("baseline must be clean: %v", is)
	}
}

func TestStepRules(t *testing.T) {
	cases := []struct{ name, old, new, want string }{
		{"two kinds in one step", "      compute: { cel: \"steps.ranked.output[0]\" }", "      compute: { cel: \"steps.ranked.output[0]\" }\n      respond: { template: x }", "STEP_KIND@spec.steps[2]"},
		{"a step reads a later step", "steps.ranked.output[0]", "steps.route.output", "UNRESOLVED_REF@spec.steps[2].compute.cel"},
		{"a step reads itself", "steps.ranked.output[0]", "steps.best.output", "UNRESOLVED_REF@spec.steps[2].compute.cel"},
		{"inputs cannot read steps", "value: \"event.message\"", "value: \"steps.best.output\"", "SCOPE_NOT_ALLOWED@spec.inputs.to.resolve[0].value"},
		{"duplicate step ids", "- id: best", "- id: offers", "DUPLICATE_ID@spec.steps[2].id"},
		{"missing script", "script: score.star", "script: nope.star", "INVALID_SCRIPT@spec.steps[1].compute.starlark.script"},
		{"unknown script function", "function: score,", "function: rank,", "UNKNOWN_FUNCTION@spec.steps[1].compute.starlark.function"},
		{"unknown script parameter", "with: { offers:", "with: { flights:", "UNKNOWN_PARAM@spec.steps[1].compute.starlark.with.flights"},
		{"missing required parameter", "with: { offers: \"${ steps.offers.output }\" }", "with: {}", "MISSING_PARAM@spec.steps[1].compute.starlark.with"},
		{"cel and starlark together", "compute: { cel: \"steps.ranked.output[0]\" }", "compute: { cel: \"1\", starlark: { script: score.star, function: top, with: { ranked: [], n: 1 } } }", "STEP_KIND@spec.steps[2].compute"},
		{"irreversible act step without a key", "      act: booking/search-flights\n      with: { from: \"GRU\", to: \"${ inputs.to }\" }", "      act: booking/book\n      with: { token: x }", "IDEMPOTENCY_REQUIRED@spec.steps[0].idempotencyKey"},
		{"act step arguments", "with: { from: \"GRU\",", "with: { origin: \"GRU\",", "UNKNOWN_PARAM@spec.steps[0].with.origin"},
		{"with outside act and call", "      compute: { cel: \"steps.ranked.output[0]\" }", "      compute: { cel: \"steps.ranked.output[0]\" }\n      with: { a: 1 }", "UNUSED_FIELD@spec.steps[2].with"},
		{"decide with two kinds", "yesNo: { ifTrue: travels within 48 hours }", "yesNo: {}\n        score: [low, high]", "STEP_KIND@spec.steps[3].decide"},
		{"branch without else", "        - else: true\n", "        - when: \"true\"\n", "MISSING_FIELD@spec.steps[5].branch[1]"},
		{"else not last", "        - when: \"size(steps.offers.output) == 0\"\n          respond: { template: \"Não achei voos.\" }\n        - else: true\n          respond: { template: \"Achei {{ size(steps.offers.output) }} voos.\" }",
			"        - else: true\n          respond: { template: a }\n        - when: \"true\"\n          respond: { template: b }", "STEP_KIND@spec.steps[5].branch[0]"},
		{"extract needs a schema", "      compute: { cel: \"steps.ranked.output[0]\" }", "      extract: { state: { m: \"${ event.message }\" }, instructions: pick }", "MISSING_FIELD@spec.steps[2].extract.schema"},
		{"schemas must be valid", "      compute: { cel: \"steps.ranked.output[0]\" }", "      generate: { state: { m: x }, instructions: write, schema: { type: nonsense } }", "INVALID_SCHEMA@spec.steps[2].generate.schema"},
		{"probe on a step that is not an act", "      compute: { cel: \"steps.ranked.output[0]\" }", "      compute: { cel: \"steps.ranked.output[0]\" }\n      onError: [ { probe: {} } ]", "PROBE_UNSUPPORTED@spec.steps[2].onError[0].probe"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(travelFunction, c.old) {
				t.Fatalf("fixture does not contain %q", c.old)
			}
			fn := strings.Replace(travelFunction, c.old, c.new, 1)
			got := codesAt(errorsOnly(validateDir(t, travelFiles(fn))))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %s, got %s", c.want, got)
			}
		})
	}
}
