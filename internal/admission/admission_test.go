package admission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	exampleFunction = "../../testdata/agenda/reschedule-appointment.yaml"
	examplePlugin   = "../../testdata/agenda/plugin-agenda.yaml"
	exampleSubject  = "../../testdata/agenda/subject.yaml"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// validate runs both layers on in-memory documents.
func validate(t *testing.T, files map[string]string) []Issue {
	t.Helper()
	l, err := NewLoader()
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewChecker()
	if err != nil {
		t.Fatal(err)
	}
	var docs []Doc
	var issues []Issue
	for name, content := range files {
		d, is := l.Load(name, []byte(content))
		docs = append(docs, d...)
		issues = append(issues, is...)
	}
	return append(issues, c.Check(docs)...)
}

func errorsOnly(is []Issue) []Issue {
	var out []Issue
	for _, i := range is {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

// TestEveryExampleValidates enforces the rule that every example in the
// repository passes validation (RFC-0001, principle 5).
func TestEveryExampleValidates(t *testing.T) {
	l, _ := NewLoader()
	c, _ := NewChecker()
	docs, issues := l.LoadPaths([]string{"../../examples"})
	issues = append(issues, c.Check(docs)...)
	for _, is := range issues {
		t.Errorf("%s", is)
	}
	if len(docs) < 2 {
		t.Errorf("found %d example resources, want at least 2", len(docs))
	}
}

func TestDefects(t *testing.T) {
	fn, plugin := read(t, exampleFunction), read(t, examplePlugin)
	cases := []struct {
		name     string
		old, new string // replacement applied to the example function
		code     string
		path     string // prefix of the issue path
	}{
		{"typo in a field name", "  description: Muda", "  descripton: Muda", "UNKNOWN_FIELD", "spec"},
		{"missing required field", "  description: Muda a data de uma consulta já existente.\n", "", "MISSING_FIELD", "spec"},
		{"value outside the enum", "onLowConfidence: ask-user\n      onUnresolved", "onLowConfidence: maybe\n      onUnresolved", "INVALID_FIELD", "spec.inputs.appointment.resolve[2].ask.onLowConfidence"},
		{"reference to an undeclared input", `idempotencyKey: "inputs.appointment.id`, `idempotencyKey: "inputs.apointment.id`, "UNRESOLVED_REF", "spec.action.idempotencyKey"},
		{"source reads a later source", `when: "size(sources.upcoming.output) == 1"`, `when: "size(sources.pick.output) == 1"`, "UNRESOLVED_REF", "spec.inputs.appointment.resolve[1].when"},
		{"steps scope in a Function", `cel: "inputs.newSlot.start > event.time + duration('2h')"`, `cel: "steps.x.status == 'ok'"`, "SCOPE_NOT_ALLOWED", "spec.preconditions[0].cel"},
		{"sources scope outside inputs", `confirm: "Posso remarcar sua consulta de {{ inputs.appointment.label }} para {{ inputs.newSlot.label }}? Responda sim para confirmar."`, `confirm: "Confirmo {{ sources.slots.output }}?"`, "SCOPE_NOT_ALLOWED", "spec.action.confirm"},
		{"text mixed with ${ }", `patientId: "${ caller.patientId }"`, `patientId: "id-${ caller.patientId }"`, "INVALID_EXPR", "spec.inputs.appointment.resolve[0].with"},
		{"now() does not exist", `cel: "inputs.newSlot.start > event.time + duration('2h')"`, `cel: "inputs.newSlot.start > now()"`, "INVALID_EXPR", "spec.preconditions[0].cel"},
		{"condition that is not boolean", `authorize: "caller.role == 'patient'"`, `authorize: "caller.role + 'x'"`, "INVALID_EXPR", "spec.authorize"},
		{"unknown action", "act: agenda/reschedule", "act: agenda/reschedul", "UNKNOWN_ACTION", "spec.action.act"},
		{"unknown parameter", `slotId: "${ inputs.newSlot.id }"`, `slot: "${ inputs.newSlot.id }"`, "UNKNOWN_PARAM", "spec.action.with.slot"},
		{"missing parameter", `slotId: "${ inputs.newSlot.id }"`, `slot: "${ inputs.newSlot.id }"`, "MISSING_PARAM", "spec.action.with"},
		{"irreversible without idempotency key", "    idempotencyKey: \"inputs.appointment.id + ':' + inputs.newSlot.id\"\n", "", "IDEMPOTENCY_REQUIRED", "spec.action.idempotencyKey"},
		{"retry after an irreversible action without a probe", "    - probe: {}\n", "", "UNSAFE_RETRY", "spec.onError[0].retry"},
		{"two strategies in one item", "    - probe: {}\n", "    - probe: {}\n      fail: {}\n", "ONE_STRATEGY", "spec.onError[0]"},
		{"source with two kinds", "        - id: only\n          when:", "        - id: only\n          fromFunction: x\n          when:", "SOURCE_KIND", "spec.inputs.appointment.resolve[1]"},
		{"ask without a threshold", "            minConfidence: 0.7\n            onLowConfidence: ask-user\n      onUnresolved", "            onLowConfidence: ask-user\n      onUnresolved", "MISSING_FIELD", "spec.inputs.appointment.resolve[2].ask"},
		{"source calls an irreversible action", "fromAction: agenda/find-slots", "fromAction: agenda/reschedule", "EFFECT_NOT_ALLOWED", "spec.inputs.newSlot.resolve[0].fromAction"},
		{"inputs that need each other", `patientId: "${ caller.patientId }"`, `patientId: "${ inputs.newSlot.id }"`, "INPUT_CYCLE", "spec.inputs."},
		{"duplicate source id", "        - id: only\n", "        - id: upcoming\n", "DUPLICATE_ID", "spec.inputs.appointment.resolve[1].id"},
		{"unterminated template", "Qual consulta você quer remarcar?", "Qual {{ consulta", "INVALID_TEMPLATE", "spec.inputs.appointment.resolve[2].ask.text"},
		{"format on a non-string input", "      type: agenda.Appointment\n", "      type: agenda.Appointment\n      format: email\n", "INVALID_FIELD", "spec.inputs.appointment.format"},
		{"format with choices", "      type: agenda.Slot\n", "      type: string\n      format: phone\n", "INVALID_FIELD", "spec.inputs.newSlot.resolve[1].ask.choices"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(fn, c.old) {
				t.Fatalf("test setup: %q not found in the example", c.old)
			}
			mutated := strings.Replace(fn, c.old, c.new, 1)
			issues := errorsOnly(validate(t, map[string]string{"fn.yaml": mutated, "plugin.yaml": plugin, "subject.yaml": read(t, exampleSubject)}))
			for _, is := range issues {
				if is.Code == c.code && strings.HasPrefix(is.Path, c.path) {
					if is.Message == "" {
						t.Error("issue without a message")
					}
					return
				}
			}
			t.Errorf("want %s at %s, got:", c.code, c.path)
			for _, is := range issues {
				t.Logf("  %s", is)
			}
		})
	}
}

func TestNoCascadeAfterUnknownAction(t *testing.T) {
	fn := strings.Replace(read(t, exampleFunction), "act: agenda/reschedule", "act: agenda/reschedul", 1)
	issues := errorsOnly(validate(t, map[string]string{"fn.yaml": fn, "plugin.yaml": read(t, examplePlugin), "subject.yaml": read(t, exampleSubject)}))
	if len(issues) != 1 || issues[0].Code != "UNKNOWN_ACTION" {
		t.Errorf("want only UNKNOWN_ACTION, got %v", issues)
	}
}

func TestExampleIsClean(t *testing.T) {
	issues := validate(t, map[string]string{"fn.yaml": read(t, exampleFunction), "plugin.yaml": read(t, examplePlugin), "subject.yaml": read(t, exampleSubject)})
	if len(issues) != 0 {
		for _, is := range issues {
			t.Errorf("%s", is)
		}
	}
}

func TestMissingPluginManifest(t *testing.T) {
	issues := errorsOnly(validate(t, map[string]string{"fn.yaml": read(t, exampleFunction)}))
	found := false
	for _, is := range issues {
		if is.Code == "UNKNOWN_PLUGIN" && strings.Contains(is.Fix, "manifest") {
			found = true
		}
	}
	if !found {
		t.Errorf("want UNKNOWN_PLUGIN with a fix, got %v", issues)
	}
}

func TestIrreversibleWithoutConfirmIsAWarning(t *testing.T) {
	fn := strings.Replace(read(t, exampleFunction), "    confirm: \"Posso remarcar sua consulta de {{ inputs.appointment.label }} para {{ inputs.newSlot.label }}? Responda sim para confirmar.\"\n", "", 1)
	issues := validate(t, map[string]string{"fn.yaml": fn, "plugin.yaml": read(t, examplePlugin), "subject.yaml": read(t, exampleSubject)})
	if len(issues) != 1 || issues[0].Code != "NO_CONFIRMATION" || issues[0].Severity != SeverityWarning {
		t.Errorf("want one NO_CONFIRMATION warning, got %v", issues)
	}
}

func TestMultiDocumentFileAndUnknownKind(t *testing.T) {
	both := read(t, examplePlugin) + "\n---\n# comment only\n---\n" + read(t, exampleFunction) + "\n---\napiVersion: x/v1\nkind: Gadget\n---\n" + read(t, exampleSubject)
	issues := validate(t, map[string]string{"all.yaml": both})
	if len(issues) != 1 || issues[0].Code != "UNKNOWN_KIND" || issues[0].File != "all.yaml#3" {
		t.Errorf("want only UNKNOWN_KIND in the 4th document, got %v", issues)
	}
}

func TestPluginManifestChecks(t *testing.T) {
	bad := strings.Replace(read(t, examplePlugin), "with: list-upcoming-appointments", "with: find-slotz", 1)
	bad = strings.Replace(bad, "compensate: reschedule", "compensate: undo", 1)
	issues := errorsOnly(validate(t, map[string]string{"plugin.yaml": bad}))
	var codes []string
	for _, is := range issues {
		codes = append(codes, is.Code+"@"+is.Path)
	}
	got := strings.Join(codes, " ")
	for _, want := range []string{"UNKNOWN_ACTION@spec.actions.reschedule.verify.with", "UNKNOWN_ACTION@spec.actions.reschedule.compensate"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s, got %s", want, got)
		}
	}
}

func TestPointerToPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                               "",
		"/spec/inputs/newSlot/resolve/1": "spec.inputs.newSlot.resolve[1]",
		"/metadata/labels/a~1b":          "metadata.labels.a/b",
	} {
		if got := pointerToPath(in); got != want {
			t.Errorf("pointerToPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadPathsWalksDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(read(t, examplePlugin)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, _ := NewLoader()
	docs, issues := l.LoadPaths([]string{dir})
	if len(docs) != 1+len(BuiltinDocs()) || len(issues) != 0 {
		t.Errorf("docs=%d (with the built-in plugins) issues=%v", len(docs), issues)
	}
	if _, issues := l.LoadPaths([]string{filepath.Join(dir, "missing")}); len(issues) != 1 || issues[0].Code != "READ_ERROR" {
		t.Errorf("want READ_ERROR, got %v", issues)
	}
}

func TestTriggerChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: report, namespace: ns }
spec:
  description: A report.
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: t, namespace: ns }
spec:
  function: report
  schedule: { cron: "0 9 * * 1-5", timezone: America/Sao_Paulo }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"t.yaml": base}))); got != "" {
		t.Fatalf("a valid trigger was refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown function", "function: report", "function: nope", "UNKNOWN_FUNCTION@spec.function"},
		{"bad cron", `cron: "0 9 * * 1-5"`, `cron: "0 25 * * *"`, "INVALID_FIELD@spec.schedule.cron"},
		{"no time zone", ", timezone: America/Sao_Paulo", "", "MISSING_FIELD@spec.schedule.timezone"},
		{"server zone", "timezone: America/Sao_Paulo", "timezone: Local", "INVALID_FIELD@spec.schedule.timezone"},
		{"bad maxDelay", "timezone: America/Sao_Paulo }", "timezone: America/Sao_Paulo, maxDelay: 30d }", "INVALID_FIELD@spec.schedule.maxDelay"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"t.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
	agent := strings.Replace(base, ", timezone: America/Sao_Paulo", "", 1) + `---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec: { timezone: America/Sao_Paulo }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"t.yaml": agent}))); got != "" {
		t.Errorf("the Agent's time zone must do: %s", got)
	}
}

func TestMilestoneChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: lead, namespace: ns }
spec:
  id: "caller.leadId"
  attributes:
    canal: { type: string, enum: [whatsapp, site] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: qualify, namespace: ns }
spec:
  description: Qualifies a lead.
  steps:
    - id: ok
      compute: { cel: "true" }
      milestone: { name: qualificou, subject: lead, attributes: { canal: "'whatsapp'" } }
  output: { template: "ok" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"m.yaml": base}))); got != "" {
		t.Fatalf("valid milestone refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown subject", "subject: lead, attributes", "subject: cliente, attributes", "UNKNOWN_SUBJECT@spec.steps[0].milestone.subject"},
		{"undeclared attribute", "attributes: { canal:", "attributes: { email:", "UNKNOWN_PARAM@spec.steps[0].milestone.attributes.email"},
		{"free-text attribute", "canal: { type: string, enum: [whatsapp, site] }", "canal: { type: string }", "MISSING_FIELD@spec.attributes.canal.enum"},
		{"bad id expression", `id: "caller.leadId"`, `id: "caller.leadId +"`, "INVALID_EXPR@spec.id"},
		{"bad name", "name: qualificou", "name: Qualificou", "INVALID_FIELD@spec.steps[0].milestone.name"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"m.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func TestExperimentChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: lead, namespace: ns }
spec: { id: "caller.leadId" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Funnel
metadata: { name: prosp, namespace: ns }
spec:
  subject: lead
  stages: [ { name: contato, milestone: contatado }, { name: reuniao, milestone: reuniao-marcada, within: 14d } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Experiment
metadata: { name: msg, namespace: ns }
spec:
  subject: lead
  variants: [ { name: a, weight: 1 }, { name: b, weight: 1 } ]
  primary: { funnel: prosp, stage: reuniao }
  guardrails: [ { name: opt-out, milestone: opt-out, within: 14d, maxIncrease: 0.02 } ]
  onBreach: { function: greet }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: greet, namespace: ns }
spec:
  description: Greets.
  steps: [ { id: v, experiment: msg } ]
  output: { value: "steps.v.output" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"x.yaml": base}))); got != "" {
		t.Fatalf("valid experiment refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown funnel", "funnel: prosp, stage", "funnel: nope, stage", "UNKNOWN_FUNNEL@spec.primary.funnel"},
		{"guardrails without onBreach", "  onBreach: { function: greet }\n", "", "MISSING_FIELD@spec.onBreach"},
		{"onBreach without guardrails", "  guardrails: [ { name: opt-out, milestone: opt-out, within: 14d, maxIncrease: 0.02 } ]\n", "", "UNUSED_FIELD@spec.onBreach"},
		{"unknown onBreach function", "onBreach: { function: greet }", "onBreach: { function: nope }", "UNKNOWN_FUNCTION@spec.onBreach.function"},
		{"bad guardrail window", "within: 14d, maxIncrease", "within: 2w, maxIncrease", "INVALID_FIELD@spec.guardrails[0].within"},
		{"no increase", "maxIncrease: 0.02", "maxIncrease: 0", "@spec.guardrails[0].maxIncrease"},
		{"entry stage as outcome", "stage: reuniao }", "stage: contato }", "INVALID_FIELD@spec.primary.stage"},
		{"unknown stage", "stage: reuniao }", "stage: venda }", "UNKNOWN_PARAM@spec.primary.stage"},
		{"duplicate variant", "{ name: b, weight: 1 }", "{ name: a, weight: 1 }", "DUPLICATE@spec.variants[1].name"},
		{"unknown experiment in a step", "experiment: msg }", "experiment: other }", "UNKNOWN_EXPERIMENT@spec.steps[0].experiment"},
		{"outcome without a window", ", within: 14d }", " }", "MISSING_FIELD@spec.primary.stage"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"x.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func TestChannelChecks(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: whatsapp, version: 0.1.0 }
spec:
  channel: { window: 24h, consent: "has(caller.optIn) && caller.optIn" }
  actions:
    send-template: { input: { template: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec: { channel: whatsapp }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: nudge, namespace: ns }
spec:
  description: Nudges.
  steps:
    - { id: w, wait: { for: 48h } }
    - id: text
      respond: { template: "Oi de novo" }
      onError:
        - fallback: { act: whatsapp/send-template, with: { template: followup }, idempotencyKey: "conversation.id" }
  output: { value: "true" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"c.yaml": base}))); got != "" {
		t.Fatalf("valid channel use refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"free text after a long wait, no fallback", "      onError:\n        - fallback: { act: whatsapp/send-template, with: { template: followup }, idempotencyKey: \"conversation.id\" }\n", "", "WINDOW_RISK@spec.steps[1].respond"},
		{"output text after a long wait", `output: { value: "true" }`, `output: { template: "tchau" }`, "WINDOW_RISK@spec.output.template"},
		{"a short wait is fine", "for: 48h", "for: 2h", ""},
		{"unknown channel", "spec: { channel: whatsapp }", "spec: { channel: telegram }", "UNKNOWN_PLUGIN@spec.channel"},
		{"bad window", "window: 24h", "window: 1d", "INVALID_FIELD@spec.channel.window"},
		{"bad consent", "consent: \"has(caller.optIn) && caller.optIn\"", "consent: \"caller.optIn &&\"", "INVALID_EXPR@spec.channel.consent"},
		{"freeText on a read", "effect: external-message }", "effect: read, freeText: true }", "INVALID_FIELD@spec.actions.send-template.freeText"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"c.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if c.want == "" && strings.Contains(got, "WINDOW_RISK") || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q, got %s", c.name, c.want, got)
		}
	}
}

func TestQueueChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: salao, namespace: ns }
spec:
  sla: { remind: 1h, escalate: 2h, escalateTo: gerencia, timeout: 4h, onTimeout: { answer: false } }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"q.yaml": base}))); got != "" {
		t.Fatalf("valid queue refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"bad duration", "remind: 1h", "remind: 1d", "INVALID_FIELD@spec.sla.remind"},
		{"escalate alone", ", escalateTo: gerencia", "", "MISSING_FIELD@spec.sla"},
		{"timeout without onTimeout", ", onTimeout: { answer: false }", "", "MISSING_FIELD@spec.sla.onTimeout"},
		{"both answer and fail", "{ answer: false }", "{ answer: false, fail: true }", "STEP_KIND@spec.sla.onTimeout"},
		{"business hours without a zone", "sla: {", "sla: { businessHours: { from: \"09:00\", to: \"18:00\" },", "MISSING_FIELD@spec.sla.businessHours"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"q.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func TestNotifyChecks(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  egress: { hosts: [slack.com] }
  connection: { baseUrl: "https://slack.com/api" }
  actions:
    post:
      input: { channel: string, text: string }
      output: map
      effect: external-message
      http: { method: POST, path: /chat.postMessage, body: { channel: "${ inputs.channel }", text: "${ inputs.text }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: salao, namespace: ns }
spec:
  notify:
    - act: slack/post
      with: { channel: "#salao", text: "${ inputs.text + ' ' + inputs.link }" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"q.yaml": base}))); got != "" {
		t.Fatalf("valid notify refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no such plugin", "act: slack/post", "act: teams/post", "UNKNOWN_PLUGIN@spec.notify[0].act"},
		{"no such action", "act: slack/post", "act: slack/send", "UNKNOWN_ACTION@spec.notify[0].act"},
		{"not plugin/action", "act: slack/post", "act: slack", "INVALID_FIELD@spec.notify[0].act"},
		{"missing parameter", `channel: "#salao", `, "", "MISSING_PARAM@spec.notify[0].with"},
		{"unknown parameter", `channel: "#salao"`, `channel: "#salao", emoji: ":bell:"`, "UNKNOWN_PARAM@spec.notify[0].with.emoji"},
		{"reads beyond the task", "inputs.link", "steps.x.output", "UNKNOWN_REF@spec.notify[0].with"},
		{"unknown task field", "inputs.link", "inputs.phone", "UNKNOWN_REF@spec.notify[0].with"},
		{"bad expression", "inputs.link }", "inputs.link + }", "INVALID_EXPR@spec.notify[0].with"},
		{"not a message", "effect: external-message", "effect: irreversible", "EFFECT_NOT_ALLOWED@spec.notify[0].act"},
		{"the users' channel", "egress: { hosts: [slack.com] }", "channel: { window: 24h }\n  egress: { hosts: [slack.com] }", "NOTIFY_USER_CHANNEL@spec.notify[0].act"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"q.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// Every way the review found to reach a user with free text outside the
// window, and the paths that must stay allowed.
func TestWindowPaths(t *testing.T) {
	head := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: whatsapp, version: 0.1.0 }
spec:
  channel: { window: 24h }
  actions:
    send-template: { input: { template: string }, effect: external-message }
    send-text: { input: { text: string }, effect: external-message, freeText: true }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec: { channel: whatsapp }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: texto, namespace: ns }
spec:
  description: Free text.
  steps:
    - { id: t, respond: { template: "Oi" } }
  output: { value: "true" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: modelo, namespace: ns }
spec:
  description: A template.
  steps:
    - { id: t, act: whatsapp/send-template, with: { template: t }, idempotencyKey: "conversation.id" }
  output: { value: "true" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: The Function under test.
  steps:
`
	const template = `      onError:
        - fallback: { act: whatsapp/send-template, with: { template: t }, idempotencyKey: "conversation.id" }
`
	trigger := `---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: hook, namespace: ns }
spec: { function: f, webhook: { secret: HOOK } }
`
	for _, c := range []struct {
		name, steps, extra, want string
	}{
		{"call after a long wait", "    - { id: w, wait: { for: 48h } }\n    - { id: c, call: texto }\n", "", "WINDOW_RISK@spec.steps[1].call"},
		{"waits that add up", "    - { id: w1, wait: { for: 23h } }\n    - { id: w2, wait: { for: 2h } }\n    - { id: r, respond: { template: \"Oi\" } }\n", "", "WINDOW_RISK@spec.steps[2].respond"},
		{"started by a webhook", "    - { id: r, respond: { template: \"Oi\" } }\n", trigger, "WINDOW_RISK@spec.steps[0].respond"},
		{"respond after an escalate", "    - { id: e, escalate: { queue: q, task: answer, message: \"?\" } }\n    - { id: r, respond: { template: \"Oi\" } }\n", "", "WINDOW_RISK@spec.steps[1].respond"},
		{"orReply timeout branch", "    - { id: w, wait: { for: 48h, orReply: true } }\n    - id: b\n      branch:\n        - { when: \"!steps.w.output.timedOut\", respond: { template: \"Que bom!\" } }\n        - { else: true, respond: { template: \"Sumiu?\" } }\n", "", "WINDOW_RISK@spec.steps[1].branch[1].respond"},
		{"a freeText action", "    - { id: w, wait: { for: 48h } }\n    - { id: s, act: whatsapp/send-text, with: { text: oi } }\n", "", "WINDOW_RISK@spec.steps[1].act"},
		{"an ask", "    - { id: w, wait: { for: 48h } }\n    - { id: q, ask: { text: \"Pode falar?\", choices: \"['sim', 'nao']\", minConfidence: 0.7, onLowConfidence: ask-user } }\n", "", "WINDOW_RISK@spec.steps[1].ask"},
		{"a retry is not a template", "    - { id: w, wait: { for: 48h } }\n    - id: r\n      respond: { template: \"Oi\" }\n      onError: [{ retry: { attempts: 3 } }]\n", "", "WINDOW_RISK@spec.steps[1].respond"},
		{"not timed out may be an event", "    - { id: w, wait: { for: 48h, orReply: true, events: [ { name: booked, key: \"'x'\" } ] } }\n    - id: b\n      branch:\n        - { when: \"!steps.w.output.timedOut\", call: texto }\n        - { else: true, call: modelo }\n", "", "WINDOW_RISK@spec.steps[1].branch[0].call"},
		// Allowed:
		{"the reply path with events", "    - { id: w, wait: { for: 48h, orReply: true, events: [ { name: booked, key: \"'x'\" } ] } }\n    - id: b\n      branch:\n        - { when: \"steps.w.output.reply != null\", call: texto }\n        - { else: true, call: modelo }\n", "", ""},
		{"the reply path", "    - { id: w, wait: { for: 48h, orReply: true } }\n    - id: b\n      branch:\n        - { when: \"!steps.w.output.timedOut\", call: texto }\n        - { else: true, call: modelo }\n", "", ""},
		{"a template fallback", "    - { id: w, wait: { for: 48h } }\n    - id: r\n      respond: { template: \"Oi\" }\n" + template, "", ""},
		{"a note to a queue", "    - { id: w, wait: { for: 48h, orReply: true } }\n    - id: b\n      branch:\n        - { else: true, respond: { template: \"sem resposta\", to: \"queue:vendas\" } }\n", "", ""},
		{"after the user answered", "    - { id: w, wait: { for: 48h } }\n    - { id: q, ask: { text: \"Pode falar?\", choices: \"['sim', 'nao']\", minConfidence: 0.7, onLowConfidence: ask-user }, onError: [{ fallback: { act: whatsapp/send-template, with: { template: t }, idempotencyKey: \"conversation.id\" } }] }\n    - { id: r, respond: { template: \"Oi\" } }\n", "", ""},
		{"short waits", "    - { id: w1, wait: { for: 2h } }\n    - { id: r, respond: { template: \"Oi\" } }\n", "", ""},
	} {
		doc := head + c.steps + "  output: { value: \"true\" }\n" + c.extra
		got := codesAt(errorsOnly(validate(t, map[string]string{"w.yaml": doc})))
		switch {
		case c.want == "" && got != "":
			t.Errorf("%s: allowed, got %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// On a channel with a consent rule, the recipient's consent authorizes an
// external message the way a confirmation authorizes other irreversible
// effects: no NO_CONFIRMATION warning. Without the rule, the warning.
func TestConsentStandsForConfirmation(t *testing.T) {
	doc := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: whatsapp, version: 0.1.0 }
spec:
  channel: { window: 24h, consent: "has(caller.optIn) && caller.optIn" }
  actions:
    send-template: { input: { template: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: Sends a template.
  steps:
    - { id: t, act: whatsapp/send-template, with: { template: t }, idempotencyKey: "conversation.id" }
  output: { value: "true" }
`
	codes := func(doc string) string { return codesAt(validate(t, map[string]string{"c.yaml": doc})) }
	if got := codes(doc); strings.Contains(got, "NO_CONFIRMATION") {
		t.Errorf("consent authorizes the message: %s", got)
	}
	if got := codes(strings.Replace(doc, `, consent: "has(caller.optIn) && caller.optIn"`, "", 1)); !strings.Contains(got, "NO_CONFIRMATION@spec.steps[0].confirm") {
		t.Errorf("without a consent rule the send needs a confirm: %s", got)
	}
}

func TestEventChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: reuniao, namespace: ns }
spec:
  event: { name: booked, key: "event.payload.leadId" }
  webhook: { secret: CRM }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: novo, namespace: ns }
spec:
  function: f
  webhook: { secret: CRM, key: "event.payload.leadId" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: Waits for the meeting.
  authorize: "has(caller.trigger)"
  steps:
    - { id: w, wait: { for: 48h, events: [ { name: booked, key: "event.payload.leadId" } ] } }
  output: { value: "steps.w.output" }
`
	if got := codesAt(validate(t, map[string]string{"e.yaml": base})); got != "" {
		t.Fatalf("valid events refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"neither function nor event", "  event: { name: booked, key: \"event.payload.leadId\" }\n", "", "MISSING_FIELD@spec.function"},
		{"event on a schedule", "  webhook: { secret: CRM }\n---", "  schedule: { cron: \"0 9 * * *\", timezone: UTC }\n---", "INVALID_FIELD@spec.event"},
		{"event key reads beyond the payload", "event: { name: booked, key: \"event.payload.leadId\" }", "event: { name: booked, key: \"caller.id\" }", "UNKNOWN_REF@spec.event.key"},
		{"nobody listens", "event: { name: booked,", "event: { name: cancelled,", "UNUSED_FIELD@spec.event.name"},
		{"bad webhook key", "key: \"event.payload.leadId\" }\n---\napiVersion: agents.tessera.dev/v1alpha1\nkind: Function", "key: \"event.payload.leadId +\" }\n---\napiVersion: agents.tessera.dev/v1alpha1\nkind: Function", "INVALID_EXPR@spec.webhook.key"},
		{"duplicate event", "events: [ { name: booked, key: \"event.payload.leadId\" } ]", "events: [ { name: booked, key: \"'a'\" }, { name: booked, key: \"'b'\" } ]", "DUPLICATE@spec.steps[0].wait.events[1].name"},
		{"bad wait key", "events: [ { name: booked, key: \"event.payload.leadId\" } ]", "events: [ { name: booked, key: \"steps.nope.output\" } ]", "@spec.steps[0].wait.events[0].key"},
	} {
		got := codesAt(validate(t, map[string]string{"e.yaml": strings.Replace(base, c.old, c.new, 1)}))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func TestMilestoneTriggerChecks(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: loja, namespace: ns }
spec: { id: "event.payload.unidade" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: inaugurou, namespace: ns }
spec:
  function: f
  milestone: { subject: loja, name: loja-aberta }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: Celebrates.
  authorize: "has(caller.trigger)"
  steps: [ { id: n, respond: { template: "ok", to: "queue:diretoria" } } ]
  output: { value: "true" }
`
	if got := codesAt(validate(t, map[string]string{"m.yaml": base})); got != "" {
		t.Fatalf("valid milestone trigger refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"unknown subject", "subject: loja, name", "subject: filial, name", "UNKNOWN_SUBJECT@spec.milestone.subject"},
		{"no function", "  function: f\n  milestone", "  milestone", "MISSING_FIELD@spec.function"},
		{"two kinds", "  milestone: { subject: loja, name: loja-aberta }\n", "  milestone: { subject: loja, name: loja-aberta }\n  webhook: { secret: X }\n", "STEP_KIND@spec"},
	} {
		got := codesAt(validate(t, map[string]string{"m.yaml": strings.Replace(base, c.old, c.new, 1)}))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// The free mode only reads: an action with any other effect is refused.
func TestFreeModeOnlyReads(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinica, version: 0.1.0 }
spec:
  actions:
    info: { input: { unidade: string }, output: map, effect: read }
    cancelar: { input: { id: string }, output: map, effect: irreversible }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: a, namespace: ns }
spec:
  freeMode:
    instructions: "Tire dúvidas."
    reads: [ { act: clinica/info, with: { unidade: "${ caller.unidade }" } } ]
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid free mode refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"an effect", "act: clinica/info, with: { unidade: \"${ caller.unidade }\" }", "act: clinica/cancelar, with: { id: \"x\" }", "EFFECT_NOT_ALLOWED@spec.freeMode.reads[0].act"},
		{"reads beyond the caller", "${ caller.unidade }", "${ event.payload.x }", "UNKNOWN_REF@spec.freeMode.reads[0].with"},
		{"missing parameter", "act: clinica/info, with: { unidade: \"${ caller.unidade }\" }", "act: clinica/info", "MISSING_PARAM@spec.freeMode.reads[0].with"},
		{"an unknown function", "    reads:", "    functions: [ { function: nao-existe, when: \"precisa da equipe\" } ]\n    reads:", "UNKNOWN_FUNCTION@spec.freeMode.functions[0].function"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// #30: onError's when reads sources.error; a conditional probe does not
// make a later retry safe.
func TestOnErrorWhen(t *testing.T) {
	base := `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: p, version: 1.0.0 }
spec:
  actions:
    check: { input: { key: string }, effect: read }
    book:
      input: { key: string }
      effect: irreversible
      verify: { mode: probe, with: check, args: { key: "${ inputs.key }" }, happened: "sources.probe.output.done" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: f
  action: { act: p/book, with: { key: k }, idempotencyKey: "'k'", confirm: "Reservo k?" }
  onError:
    - { when: "sources.error.code == 'CONFLICT'", respond: { template: "taken ({{ sources.error.code }})" } }
    - { probe: {} }
    - { retry: { attempts: 1 } }
  output: { template: "ok" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid chain refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"an unknown source", "sources.error.code == 'CONFLICT'", "sources.other == 'x'", "SCOPE_NOT_ALLOWED@spec.onError[0].when"},
		{"not an expression", "sources.error.code == 'CONFLICT'", "sources.error.code ==", "INVALID_EXPR@spec.onError[0].when"},
		{"a conditional probe before a retry", "- { probe: {} }", "- { when: \"sources.error.definite\", probe: {} }", "UNSAFE_RETRY@spec.onError[2].retry"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}
