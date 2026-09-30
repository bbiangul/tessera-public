package admission

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/bbiangul/tessera/internal/explain"
)

// Regression tests for the adversarial review of 2026-09-23. Each test names
// the finding it guards.

// sdrPlugin has one action per effect class, for the effect-rule tests.
const sdrPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: crm, version: 1.0.0 }
spec:
  actions:
    get-lead:   { input: { id: string }, effect: read }
    update:     { input: { id: string }, effect: idempotent-write }
    delete:     { input: { id: string }, effect: irreversible }
    send:       { input: { to: string }, effect: external-message }
    delete-v:
      input: { id: string }
      effect: irreversible
      verify:
        mode: probe
        with: get-lead
        args: { id: "${ inputs.id }" }
        happened: "sources.probe.output.deleted"
`

// sdrFunction is valid; tests plant one defect each.
const sdrFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: qualify, namespace: sdr }
spec:
  description: Qualify a lead.
  inputs:
    lead:
      type: crm.Lead
      resolve:
        - id: fetched
          fromAction: crm/get-lead
          with: { id: "${ caller.leadId }" }
      onUnresolved:
        - respond: { template: "Não encontrei seu cadastro." }
  action:
    act: crm/delete-v
    with: { id: "${ inputs.lead.id }" }
    confirm: "Confirma apagar o lead {{ inputs.lead.id }}?"
    idempotencyKey: "inputs.lead.id"
  onError:
    - probe: {}
    - escalate: { queue: sdr, task: input }
`

func codesAt(issues []Issue) string {
	var b strings.Builder
	for _, is := range issues {
		b.WriteString(is.Code + "@" + is.Path + " ")
	}
	return b.String()
}

func TestSDRBaselineIsClean(t *testing.T) {
	if issues := validate(t, map[string]string{"p.yaml": sdrPlugin, "f.yaml": sdrFunction}); len(issues) != 0 {
		t.Fatalf("baseline must be clean: %v", issues)
	}
}

func TestEffectRules(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{"input fallback to an irreversible action (finding: fallback.act unchecked)",
			`        - respond: { template: "Não encontrei seu cadastro." }`,
			`        - fallback: { act: crm/delete, with: { id: x }, idempotencyKey: "'k'" }`,
			"EFFECT_NOT_ALLOWED@spec.inputs.lead.onUnresolved[0].fallback.act"},
		{"fallback action missing parameters",
			`        - respond: { template: "Não encontrei seu cadastro." }`,
			`        - fallback: { act: crm/get-lead }`,
			"MISSING_PARAM@spec.inputs.lead.onUnresolved[0].fallback.with"},
		{"irreversible fallback without idempotency key",
			"    - escalate: { queue: sdr, task: input }",
			"    - fallback: { act: crm/send, with: { to: x } }",
			"IDEMPOTENCY_REQUIRED@spec.onError[1].fallback.idempotencyKey"},
		{"retry after an irreversible fallback without a probe",
			"    - escalate: { queue: sdr, task: input }",
			"    - fallback: { act: crm/send, with: { to: x }, idempotencyKey: \"'k'\" }\n    - retry: { attempts: 3 }",
			"UNSAFE_RETRY@spec.onError[2].retry"},
		{"with on a fallback that calls nothing",
			"    - escalate: { queue: sdr, task: input }",
			"    - fallback: { useNull: true, with: { id: x } }",
			"UNUSED_FIELD@spec.onError[1].fallback.with"},
		{"fallback to another path before probing (M16)",
			"    - probe: {}\n", "    - fallback: { function: other }\n",
			"UNSAFE_FALLBACK@spec.onError[0].fallback"},
		{"external-message counts as irreversible (M15)",
			"    act: crm/delete-v\n    with: { id: \"${ inputs.lead.id }\" }\n    confirm: \"Confirma apagar o lead {{ inputs.lead.id }}?\"\n    idempotencyKey: \"inputs.lead.id\"",
			"    act: crm/send\n    with: { to: \"${ inputs.lead.id }\" }\n    confirm: \"Confirma apagar o lead {{ inputs.lead.id }}?\"",
			"IDEMPOTENCY_REQUIRED@spec.action.idempotencyKey"},
		{"probe on an action that cannot be verified (M17)",
			"    act: crm/delete-v", "    act: crm/delete",
			"PROBE_UNSUPPORTED@spec.onError[0].probe"},
		{"source with arguments but nothing to call (M19)",
			"          fromAction: crm/get-lead\n          with: { id: \"${ caller.leadId }\" }",
			"          value: \"caller.lead\"\n          with: { id: x }",
			"UNUSED_FIELD@spec.inputs.lead.resolve[0].with"},
		{"a function that calls itself",
			"          fromAction: crm/get-lead\n          with: { id: \"${ caller.leadId }\" }",
			"          fromFunction: qualify",
			"RECURSION@spec.inputs.lead.resolve[0].fromFunction"},
		{"unknown input type",
			"      type: crm.Lead", "      type: strng",
			"UNKNOWN_TYPE@spec.inputs.lead.type"},
		{"type from a plugin that is not loaded",
			"      type: crm.Lead", "      type: erp.Lead",
			"UNKNOWN_TYPE@spec.inputs.lead.type"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(sdrFunction, c.old) {
				t.Fatalf("setup: %q not in the fixture", c.old)
			}
			fn := strings.Replace(sdrFunction, c.old, c.new, 1)
			got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": sdrPlugin, "f.yaml": fn})))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %s, got %s", c.want, got)
			}
		})
	}
}

func TestListTypesAndPluginTypesAreAccepted(t *testing.T) {
	fn := strings.Replace(sdrFunction, "      type: crm.Lead", "      type: list<crm.Lead>", 1)
	if issues := errorsOnly(validate(t, map[string]string{"p.yaml": sdrPlugin, "f.yaml": fn})); len(issues) != 0 {
		t.Errorf("list<crm.Lead> must be accepted: %v", issues)
	}
}

func TestFromFunctionArguments(t *testing.T) {
	other := strings.NewReplacer("name: qualify", "name: other").Replace(sdrFunction)
	fn := strings.Replace(sdrFunction,
		"          fromAction: crm/get-lead\n          with: { id: \"${ caller.leadId }\" }",
		"          fromFunction: other\n          with: { nope: 1 }", 1)
	got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": sdrPlugin, "f.yaml": fn, "o.yaml": other})))
	if !strings.Contains(got, "UNKNOWN_PARAM@spec.inputs.lead.resolve[0].with.nope") { // M18
		t.Errorf("unknown argument to another function: got %s", got)
	}
}

func TestPluginRules(t *testing.T) {
	cases := []struct{ name, old, new, want string }{
		{"verify.with must be read (M20)", "with: get-lead\n", "with: update\n", "EFFECT_NOT_ALLOWED@spec.actions.delete-v.verify.with"},
		{"probe needs happened", "        happened: \"sources.probe.output.deleted\"\n", "", "MISSING_FIELD@spec.actions.delete-v.verify.happened"},
		{"verify output must compile", "output.deleted\"", "output.name\" \n        output: \"1 +\"", "INVALID_EXPR@spec.actions.delete-v.verify.output"},
		{"probe args must match the probe's parameters", "args: { id:", "args: { lead:", "UNKNOWN_PARAM@spec.actions.delete-v.verify.args.lead"},
		{"probe args cover every parameter", "args: { id: \"${ inputs.id }\" }", "args: {}", "MISSING_PARAM@spec.actions.delete-v.verify.args"},
		{"verify reads only sources.probe", "sources.probe.output.deleted", "sources.other.deleted", "UNRESOLVED_REF@spec.actions.delete-v.verify.happened"},
		{"verify args read the action's parameters", "${ inputs.id }", "${ inputs.lead }", "UNRESOLVED_REF@spec.actions.delete-v.verify.args"},
		{"authorize must compile (M22)", "effect: read }", "effect: read, authorize: \"inputs.id +\" }", "INVALID_EXPR@spec.actions.get-lead.authorize"},
		{"authorize reads only the action's parameters", "effect: read }", "effect: read, authorize: \"inputs.phone == caller.phone\" }", "UNRESOLVED_REF@spec.actions.get-lead.authorize"},
		{"authorize cannot read steps", "effect: read }", "effect: read, authorize: \"steps.x.ok\" }", "SCOPE_NOT_ALLOWED@spec.actions.get-lead.authorize"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := strings.Replace(sdrPlugin, c.old, c.new, 1)
			got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": p})))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %s, got %s", c.want, got)
			}
		})
	}
	dup := validate(t, map[string]string{"a.yaml": sdrPlugin, "b.yaml": sdrPlugin}) // M21
	if !strings.Contains(codesAt(dup), "DUPLICATE@") {
		t.Errorf("a plugin declared twice must be reported: %s", codesAt(dup))
	}
}

// Finding: documents after "--- # comment" or CRLF separators were dropped
// silently. The function below hides an unknown action; it must surface.
func TestSeparatorsNeverDropDocuments(t *testing.T) {
	bad := strings.Replace(sdrFunction, "act: crm/delete-v", "act: crm/nope", 1)
	cases := map[string]string{
		"comment after ---": sdrPlugin + "--- # the function\n" + bad,
		"CRLF":              strings.ReplaceAll(sdrPlugin+"---\n"+bad, "\n", "\r\n"),
		"byte-order mark":   "\xef\xbb\xbf" + sdrPlugin + "---\n" + bad,
		"leading ---":       "---\n" + sdrPlugin + "---\n" + bad,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			l, _ := NewLoader()
			docs, issues := l.Load("x.yaml", []byte(content))
			c, _ := NewChecker()
			issues = append(issues, c.Check(docs)...)
			if len(docs) != 2 || !strings.Contains(codesAt(issues), "UNKNOWN_ACTION@spec.action.act") {
				t.Errorf("docs=%d issues=%s", len(docs), codesAt(issues))
			}
		})
	}
}

func TestDuplicateKeysAreErrors(t *testing.T) {
	fn := strings.Replace(sdrFunction, "  description: Qualify a lead.\n", "  description: Qualify a lead.\n  description: Something else.\n", 1)
	got := codesAt(validate(t, map[string]string{"p.yaml": sdrPlugin, "f.yaml": fn}))
	if !strings.Contains(got, "PARSE_ERROR") {
		t.Errorf("a duplicate key must fail, got %s", got)
	}
}

func TestSizeLimits(t *testing.T) {
	l, _ := NewLoader()
	if _, is := l.Load("big.yaml", []byte(strings.Repeat("#", MaxFileBytes+1))); len(is) != 1 || is[0].Code != "TOO_LARGE" {
		t.Errorf("oversized file: %v", is)
	}
	long := strings.Replace(sdrFunction, "Qualify a lead.", strings.Repeat("x", MaxStringLength+1), 1)
	if _, is := l.Load("long.yaml", []byte(long)); len(is) != 1 || is[0].Code != "TOO_LARGE" {
		t.Errorf("oversized value: %v", is)
	}
	many := strings.Repeat("---\na: 1\n", MaxDocsPerFile+1)
	if _, is := l.Load("many.yaml", []byte(many)); !strings.Contains(codesAt(is), "TOO_LARGE") {
		t.Errorf("too many documents: %v", codesAt(is))
	}
}

func TestNonResourcesAreWarnings(t *testing.T) {
	l, _ := NewLoader()
	_, is := l.Load("summary.json", []byte(`{"accuracy": 0.97}`))
	if len(is) != 1 || is[0].Code != "NOT_A_RESOURCE" || is[0].Severity != SeverityWarning {
		t.Errorf("want one NOT_A_RESOURCE warning, got %v", is)
	}
}

// Finding: fix hints pointed to explain paths that failed. Every hint that
// names an explain path must work.
var explainHint = regexp.MustCompile(`tessera explain (\S+)`)

func TestEveryExplainHintWorks(t *testing.T) {
	fn := read(t, exampleFunction)
	broken := []string{
		strings.Replace(fn, "  description: Muda", "  descripton: Muda", 1),
		strings.Replace(fn, "onLowConfidence: ask-user\n      onUnresolved", "onLowConfidence: maybe\n      onUnresolved", 1),
		strings.Replace(fn, "    - escalate:\n        queue: recepcao", "    - escalate:\n        queu: recepcao", 1),
	}
	hints := 0
	for _, b := range broken {
		for _, is := range validate(t, map[string]string{"fn.yaml": b, "plugin.yaml": read(t, examplePlugin), "subject.yaml": read(t, exampleSubject)}) {
			for _, m := range explainHint.FindAllStringSubmatch(is.Fix, -1) {
				hints++
				if _, err := explain.Explain(m[1]); err != nil {
					t.Errorf("hint %q fails: %v", m[1], err)
				}
			}
		}
	}
	if hints < 3 {
		t.Errorf("expected hints to check, found %d", hints)
	}
}

func TestDirectoryWalkSkipsTestdataAndHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"testdata", ".cache", "real"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "x.json"), []byte(`{"not": "a resource"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := NewLoader()
	_, issues := l.LoadPaths([]string{dir})
	if len(issues) != 1 || !strings.Contains(issues[0].File, "real") {
		t.Errorf("only real/x.json must be read: %v", issues)
	}
	_, issues = l.LoadPaths([]string{filepath.Join(dir, "testdata", "x.json")})
	if len(issues) != 1 {
		t.Errorf("an explicit path is always read: %v", issues)
	}
}

// Review 2026-09-23: a YAML 1.1 converter turned the key n into false.
func TestYAML12KeepsShortWordsAsStrings(t *testing.T) {
	fn := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: yn, namespace: t }
spec:
  description: d
  inputs:
    n: { type: string, resolve: [ { id: v, value: "'x'" } ] }
    y: { type: string, resolve: [ { id: v, value: "'x'" } ] }
    on: { type: string, resolve: [ { id: v, value: "'NO'" } ] }
  output: { template: "{{ inputs.n }}{{ inputs.y }}{{ inputs.on }}" }
`
	l, _ := NewLoader()
	docs, issues := l.Load("f.yaml", []byte(fn))
	if len(issues) > 0 || len(docs) != 1 {
		t.Fatalf("%v", issues)
	}
	in := docs[0].Function.Spec.Inputs
	for _, k := range []string{"n", "y", "on"} {
		if _, ok := in[k]; !ok {
			t.Errorf("input %q was renamed: %v", k, docs[0].Function.InputOrder)
		}
	}
	for src, want := range map[string]string{
		"a: 7.0":           `{"a":7.0}`,
		"a: 0x1F":          `{"a":31}`,
		"a: NO":            `{"a":"NO"}`,
		"a: 2026-09-23":    `{"a":"2026-09-23"}`,
		"a: &x [1]\nb: *x": `{"a":[1],"b":[1]}`,
	} {
		var n yamlv3.Node
		if err := yamlv3.Unmarshal([]byte(src), &n); err != nil {
			t.Fatal(err)
		}
		got, err := toJSON(&n)
		if err != nil || string(got) != want {
			t.Errorf("%q -> %s %v, want %s", src, got, err, want)
		}
	}
	var n yamlv3.Node
	_ = yamlv3.Unmarshal([]byte("a: .inf"), &n)
	if _, err := toJSON(&n); err == nil {
		t.Error("infinity is not JSON")
	}
}

// Review 2026-09-23: a fromFunction source could run a Function with
// effects while resolving an input.
func TestInputsCannotCallFunctionsWithEffects(t *testing.T) {
	plugin := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: pay, version: 1.0.0 }
spec:
  actions:
    charge: { input: { amount: int }, effect: irreversible }
    quote: { input: { amount: int }, effect: read }
`
	fns := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: charger, namespace: t }
spec:
  description: charges
  action: { act: pay/charge, with: { amount: 10 }, idempotencyKey: "'k'", confirm: "ok?" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: indirect, namespace: t }
spec:
  description: calls the charger
  steps: [ { id: c, call: charger } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: reader, namespace: t }
spec:
  description: only reads
  inputs:
    q: { type: any, resolve: [ { id: q, fromAction: pay/quote, with: { amount: 1 } } ] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: uses, namespace: t }
spec:
  description: resolves inputs
  inputs:
    a: { type: any, resolve: [ { id: s, fromFunction: indirect } ] }
    b: { type: any, resolve: [ { id: s, fromFunction: reader } ] }
    c:
      type: any
      resolve: [ { id: s, value: "null" } ]
      onUnresolved: [ { fallback: { function: charger } } ]
`
	got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": plugin, "f.yaml": fns})))
	for _, want := range []string{"EFFECT_NOT_ALLOWED@spec.inputs.a.resolve[0].fromFunction", "EFFECT_NOT_ALLOWED@spec.inputs.c.onUnresolved[0].fallback.function"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s, got %s", want, got)
		}
	}
	if strings.Contains(got, "spec.inputs.b") {
		t.Errorf("a Function that only reads is a fine source: %s", got)
	}
}
