package admission

import (
	"strings"
	"testing"
)

// RFC-0006: Team members are unique, verify sends a message, queue roles
// are held by someone, routes and who compile.
func TestTeamsRolesRoutesWho(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ns }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
  verify: { act: msg/code, with: { to: "${ inputs.contact }" } }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: msg, version: 0.1.0 }
spec:
  actions:
    code: { input: { to: string }, effect: external-message }
    look: { input: { to: string }, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: { roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: ns }
spec:
  description: d
  selector: { intent: f, who: "caller.role == 'patient'" }
  steps:
    - { id: s, escalate: { queue: recepcao, task: approve, route: [ { when: "caller.vip == true", queue: recepcao } ] } }
  output: { template: "ok" }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid team refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"duplicate member", "    - { id: rita, name: Rita, roles: [recepcao] }", "    - { id: rita, name: Rita, roles: [recepcao] }\n    - { id: rita, name: R2, roles: [x] }", "DUPLICATE@spec.members[1].id"},
		{"verify must send", "act: msg/code", "act: msg/look", "INVALID_FIELD@spec.verify.act"},
		{"role nobody holds", "spec: { roles: [recepcao] }", "spec: { roles: [dono] }", "UNKNOWN_ROLE@spec.roles[0]"},
		{"bad who", `who: "caller.role == 'patient'"`, `who: "caller.role =="`, "INVALID_EXPR@spec.selector.who"},
		{"bad route", `when: "caller.vip == true"`, `when: "caller.vip =="`, "INVALID_EXPR@spec.steps[0].escalate.route[0].when"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// backoffice-003: a Slack queue needs the slack plugin's post-task and
// update-task; a member's Slack ID looks like one.
func TestSlackQueues(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ns }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao], slack: U0RITA }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: { roles: [recepcao], channels: [slack] }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid Slack queue refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no update-task", "    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }\n", "", "MISSING_FIELD@spec.channels[0]"},
		{"no slack plugin", "name: slack, version", "name: other, version", "MISSING_FIELD@spec.channels[0]"},
		{"a name, not an ID", "slack: U0RITA", "slack: rita", "INVALID_FIELD@spec.members[0].slack"},
		{"unknown channel", "channels: [slack]", "channels: [telegram]", "INVALID_FIELD@spec.channels[0]"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// backoffice-004: a Teams queue needs the teams plugin's three actions; a
// member's Teams ID is an Entra object ID.
func TestTeamsQueues(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ns }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao], teams: 0a0a0a0a-0000-0000-0000-000000000001 }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: teams, version: 0.1.0 }
spec:
  actions:
    open-dm: { input: { user: string, tenant: string, bot: string }, output: map, effect: idempotent-write }
    post-task: { input: { conversation: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { conversation: string, activity: string, text: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: { roles: [recepcao], channels: [teams, slack] }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid Teams queue refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no open-dm", "    open-dm: { input: { user: string, tenant: string, bot: string }, output: map, effect: idempotent-write }\n", "", "MISSING_FIELD@spec.channels[0]"},
		{"a UPN, not an object ID", "teams: 0a0a0a0a-0000-0000-0000-000000000001", "teams: rita@clinica.com", "INVALID_FIELD@spec.members[0].teams"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// backoffice-005: an e-mail queue needs the email plugin's send-task and
// the Team's verify (the link answers only with a code).
func TestEmailQueues(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ns }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao], email: rita@clinica.example, contact: "+5511900000001" }
  verify: { act: email/send-code, with: { to: "${ inputs.contact }" } }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: email, version: 0.1.0 }
spec:
  actions:
    send-task: { input: { to: string, subject: string, text: string, link: string, raw: string }, effect: external-message }
    send-code: { input: { to: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: { roles: [recepcao], channels: [email] }
`
	if got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": base}))); got != "" {
		t.Fatalf("valid e-mail queue refused: %s", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"no verify", "  verify: { act: email/send-code, with: { to: \"${ inputs.contact }\" } }\n", "", "MISSING_FIELD@spec.channels[0]"},
		{"no send-task", "    send-task: { input: { to: string, subject: string, text: string, link: string, raw: string }, effect: external-message }\n", "", "MISSING_FIELD@spec.channels[0]"},
		{"not an address", "email: rita@clinica.example", "email: rita", "INVALID_FIELD@spec.members[0].email"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

// #54: a timeout's answer on a queue that gets approvals: true approves
// unseen, and anything but a boolean is no yes.
func TestTimeoutAnswerWarnings(t *testing.T) {
	base := `apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: q, namespace: ns }
spec: { sla: { timeout: 4h, onTimeout: { answer: false } } }
`
	if got := codesAt(validate(t, map[string]string{"a.yaml": base})); got != "" {
		t.Fatalf("answer: false is the safe default: %s", got)
	}
	for _, c := range []struct{ answer, want string }{
		{"true", "APPROVES_ON_TIMEOUT@spec.sla.onTimeout.answer"},
		{`"sim"`, "TIMEOUT_ANSWER_NOT_BOOL@spec.sla.onTimeout.answer"},
	} {
		got := codesAt(validate(t, map[string]string{"a.yaml": strings.Replace(base, "answer: false", "answer: "+c.answer, 1)}))
		if !strings.Contains(got, c.want) {
			t.Errorf("answer %s: want %s, got %s", c.answer, c.want, got)
		}
	}
}
