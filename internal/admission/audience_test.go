package admission

import (
	"strings"
	"testing"
)

// audiences-001: audience rules.
func TestAudienceRules(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  audiences:
    - { name: dentista, who: "'dentista' in caller.roles", functions: [agenda] }
    - { name: paciente, functions: [horarios] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: agenda, namespace: ns }
spec:
  description: d
  selector: { intent: agenda }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: ns }
spec:
  description: d
  selector: { intent: horarios }
  output: { template: "ok" }
`
	if got := codesAt(validate(t, map[string]string{"a.yaml": base})); got != "" {
		t.Fatalf("valid audiences refused or warned: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown function", "functions: [horarios]", "functions: [horario]", "UNKNOWN_FUNCTION@spec.audiences[1].functions[0]"},
		{"duplicate name", "name: paciente", "name: dentista", "DUPLICATE@spec.audiences[1].name"},
		{"catch-all not last", "    - { name: dentista, who: \"'dentista' in caller.roles\", functions: [agenda] }\n    - { name: paciente, functions: [horarios] }",
			"    - { name: paciente, functions: [horarios] }\n    - { name: dentista, who: \"'dentista' in caller.roles\", functions: [agenda] }", "INVALID_FIELD@spec.audiences[0].who"},
		{"no catch-all", "{ name: paciente, functions", "{ name: paciente, who: \"true\", functions", "NO_CATCH_ALL@spec.audiences"},
		{"who reads beyond the caller", "'dentista' in caller.roles", "'dentista' in inputs.roles", "UNKNOWN_REF@spec.audiences[0].who"},
		{"who does not compile", "'dentista' in caller.roles", "caller.roles ==", "INVALID_EXPR@spec.audiences[0].who"},
		{"a function on no menu", "functions: [horarios]", "functions: []", "NO_AUDIENCE@spec.audiences"},
		{"not routable", "  selector: { intent: horarios }\n", "", "NOT_ROUTABLE@spec.audiences[1].functions[0]"},
		{"audience free mode read unknown", "{ name: paciente, functions: [horarios] }", "{ name: paciente, functions: [horarios], freeMode: { instructions: x, reads: [ { act: nope/x } ] } }", "UNKNOWN_PLUGIN@spec.audiences[1].freeMode.reads[0].act"},
	} {
		got := codesAt(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)}))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// audiences-002: autonomy levels and context visibility.
func TestAutonomyAndSees(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  humanInTheLoop: { queue: q }
  audiences:
    - { name: equipe, who: "has(caller.roles)", functions: [agenda], autonomy: approve-actions, sees: [ { audience: paciente, conversation: masked } ] }
    - { name: paciente, functions: [horarios], autonomy: draft }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: q, namespace: ns }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: agenda, namespace: ns }
spec:
  description: d
  selector: { intent: agenda }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: ns }
spec:
  description: d
  selector: { intent: horarios }
  output: { template: "ok" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid levels refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no one to approve", "  humanInTheLoop: { queue: q }\n", "", "MISSING_FIELD@spec.audiences[0].autonomy"},
		{"draft with a free mode", "autonomy: draft }", "autonomy: draft, freeMode: { instructions: x } }", "INVALID_FIELD@spec.audiences[1].autonomy"},
		{"draft inheriting the Agent's free mode", "  humanInTheLoop: { queue: q }\n", "  humanInTheLoop: { queue: q }\n  freeMode: { instructions: x }\n", "INVALID_FIELD@spec.audiences[1].autonomy"},
		{"an unknown level", "autonomy: draft }", "autonomy: yolo }", "INVALID_FIELD@"},
		{"sees an unknown audience", "audience: paciente,", "audience: pacientes,", "UNKNOWN_AUDIENCE@spec.audiences[0].sees[0].audience"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}
