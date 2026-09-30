package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const selfAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinic, version: 0.1.0 }
spec:
  actions:
    discount: { input: { percent: int }, output: map, effect: irreversible }
    block: { input: { day: string }, output: map, effect: irreversible }
    agenda: { input: {}, output: list, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: sf }
spec:
  description: Recepção da clínica.
  messages: { notUnderstood: "NAO-ENTENDI" }
  humanInTheLoop: { queue: recepcao }
  audiences:
    - name: dentista
      who: "has(caller.roles) && 'dentista' in caller.roles"
      functions: [bloquear, agenda, detalhe]
      autonomy: approve-actions
      freeMode: { instructions: "FREE-DENTISTA", tasks: true }
    - name: outros
      who: "has(caller.other)"
      functions: [desconto]
      freeMode: { instructions: "FREE-OUTROS", self: false }
    - name: paciente
      functions: [desconto, vip]
      freeMode:
        instructions: "FREE-PACIENTE"
        self: true
        functions: [ { function: falar-com-alguem, when: "quer falar com uma pessoa" } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: sf }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
    - { id: dora, name: Dora, roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: sf }
spec: { roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: dono, namespace: sf }
spec: { roles: [dono] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: desconto, namespace: sf }
spec:
  description: A patient asks for a discount.
  selector: { intent: desconto, description: "O paciente pede desconto." }
  inputs:
    percent: { type: int, description: the discount asked, resolve: [ { id: said, extract: {} } ] }
  steps:
    - { id: grande, when: "inputs.percent > 10", call: aprovar }
  action: { act: clinic/discount, with: { percent: "${ inputs.percent }" }, idempotencyKey: "string(inputs.percent)" }
  output: { value: "'ok'" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: sf }
spec:
  description: Internal approval.
  inputs:
    percent: { type: int, description: percent, resolve: [ { id: v, value: "0" } ] }
  steps:
    - id: ok
      escalate: { queue: recepcao, task: approve, message: "ok?", route: [ { when: "inputs.percent > 50", queue: dono } ] }
  output: { value: "'ok'" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: vip, namespace: sf }
spec:
  description: VIP service, for VIPs only.
  selector: { intent: vip, description: "Atendimento VIP.", who: "has(caller.vip) && caller.vip" }
  output: { value: "'vip'" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: falar-com-alguem, namespace: sf }
spec:
  description: Hands the conversation to a person at the reception.
  steps:
    - { id: p, escalate: { queue: recepcao, task: takeover, message: "assuma" } }
  output: { value: "'ok'" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: bloquear, namespace: sf }
spec:
  description: A dentist blocks a day of their schedule.
  selector: { intent: bloquear, description: "Bloquear um dia." }
  inputs:
    day: { type: string, description: the day to block, resolve: [ { id: said, extract: {} } ] }
  action: { act: clinic/block, with: { day: "${ inputs.day }" }, confirm: "Bloqueio {{ inputs.day }}?", idempotencyKey: "inputs.day" }
  output: { value: "'ok'" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: agenda, namespace: sf }
spec:
  description: A dentist reads their schedule.
  selector: { intent: agenda, description: "Ver a agenda." }
  inputs:
    dentist: { type: string, description: the dentist, resolve: [ { id: said, extract: {} }, { id: mine, value: "'D1'" } ] }
  steps:
    - { id: a, act: clinic/agenda, with: {} }
  output: { value: "steps.a.output" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: detalhe, namespace: sf }
spec:
  description: Details of one slot of the schedule just shown.
  selector: { intent: detalhe, description: "Detalhe de um horário.", after: [agenda] }
  output: { value: "'x'" }
`

type selfGen struct {
	mu           sync.Mutex
	self         []any // FACTS.self per free answer, nil when absent
	instructions []string
}

func (*selfGen) Name() string { return "self" }
func (g *selfGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st, _ := req.State.(map[string]any)
	facts, _ := st["facts"].(map[string]any)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.self = append(g.self, facts["self"])
	g.instructions = append(g.instructions, req.Instructions)
	return &generate.Response{Text: "Faço isto e aquilo."}, nil
}

func loadSelf(t *testing.T) (*Host, *selfGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(selfAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &selfGen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "sf", Store: journal.NewMemory(), Decider: audienceDecider{&audienceWorld{}}, Generator: g})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, g
}

func selfJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// self-001: FACTS.self is what this caller's audience is offered, read
// from the definitions: what each Function asks for, what a person
// approves (in a called Function too, conditionally, routed), what the
// user confirms, follow-ups, and the free mode's functions. Another
// audience's Functions never appear.
func TestSelfFacts(t *testing.T) {
	h, _ := loadSelf(t)
	ctx := context.Background()
	facts := func(caller map[string]any) map[string]any {
		c, _ := h.withAudience(ctx, caller)
		return h.selfFacts(ctx, c)
	}
	byWhat := func(f map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, it := range f["does"].([]any) {
			m := it.(map[string]any)
			out[m["what"].(string)] = m
		}
		return out
	}

	p := facts(map[string]any{"channel": "whatsapp"})
	if p["agent"] != "Recepção da clínica." || p["autonomy"] != nil {
		t.Errorf("patient: %s", selfJSON(t, p))
	}
	does := byWhat(p)
	if len(does) != 2 {
		t.Fatalf("patient does %s", selfJSON(t, p["does"]))
	}
	d := does["A patient asks for a discount."]
	if d == nil || d["personApproves"] != "sometimes" || selfJSON(t, d["approvedBy"]) != `["recepcao","dono"]` ||
		selfJSON(t, d["asksFor"]) != `["the discount asked"]` || d["youConfirm"] != nil {
		t.Errorf("discount: %s", selfJSON(t, d))
	}
	if f := does["Hands the conversation to a person at the reception."]; f == nil || f["personHelps"] != true || f["personApproves"] != nil {
		t.Errorf("free-mode function: %s", selfJSON(t, f))
	}
	all := selfJSON(t, p)
	for _, other := range []string{"block", "schedule", "VIP", "Internal approval"} {
		if strings.Contains(all, other) {
			t.Errorf("patient's self names %q: %s", other, all)
		}
	}
	// selector.who holds: the VIP Function appears.
	if _, ok := byWhat(facts(map[string]any{"vip": true}))["VIP service, for VIPs only."]; !ok {
		t.Errorf("a VIP is not offered the VIP Function")
	}

	dn := facts(map[string]any{"roles": []any{"dentista"}})
	// freeMode.tasks without a member (the API): no tasks item.
	if len(dn["does"].([]any)) != 3 {
		t.Fatalf("dentist without a member: %s", selfJSON(t, dn["does"]))
	}
	if m := facts(map[string]any{"member": "rita", "roles": []any{"dentista"}}); len(m["does"].([]any)) != 4 || !strings.Contains(selfJSON(t, m), "open tasks") {
		t.Errorf("a member's self lacks their tasks: %s", selfJSON(t, m))
	}
	if dn["autonomy"] == nil {
		t.Errorf("dentist's autonomy missing: %s", selfJSON(t, dn))
	}
	does = byWhat(dn)
	if len(does) != 3 {
		t.Fatalf("dentist does %s", selfJSON(t, dn["does"]))
	}
	// approve-actions: an irreversible effect is always approved by
	// humanInTheLoop; a read is not.
	b := does["A dentist blocks a day of their schedule."]
	if b == nil || b["personApproves"] != "always" || selfJSON(t, b["approvedBy"]) != `["recepcao"]` || b["youConfirm"] != true || selfJSON(t, b["asksFor"]) != `["the day to block"]` {
		t.Errorf("block: %s", selfJSON(t, b))
	}
	if a := does["A dentist reads their schedule."]; a == nil || a["personApproves"] != nil || a["asksFor"] != nil {
		t.Errorf("agenda: %s", selfJSON(t, a))
	}
	if f := does["Details of one slot of the schedule just shown."]; f == nil || selfJSON(t, f["onlyAfter"]) != `["A dentist reads their schedule."]` {
		t.Errorf("follow-up: %s", selfJSON(t, f))
	}
	if strings.Contains(selfJSON(t, dn), "discount") {
		t.Errorf("the dentist's self names the patients' Function: %s", selfJSON(t, dn))
	}
}

// self-001, #60: FACTS.self and its rules reach the free mode's answer
// unless freeMode.self is false (the dentist's free mode leaves it unset:
// on by default).
func TestSelfOnlyWhenSet(t *testing.T) {
	h, g := loadSelf(t)
	ctx := context.Background()
	if _, err := h.Handle(ctx, "p", map[string]any{"channel": "whatsapp"}, "o que você faz?"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, "o", map[string]any{"other": true}, "o que você faz?"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, "d", map[string]any{"roles": []any{"dentista"}}, "o que você faz?"); err != nil {
		t.Fatal(err)
	}
	if len(g.self) != 3 || g.self[2] == nil {
		t.Fatalf("free answers: %d; the default (unset) must give self", len(g.self))
	}
	if len(g.self) != 3 {
		t.Fatalf("free answers: %d", len(g.self))
	}
	if g.self[0] == nil || !strings.Contains(g.instructions[0], "FACTS.self") {
		t.Errorf("self set: facts %v", g.self[0])
	}
	if g.self[1] != nil || strings.Contains(g.instructions[1], "FACTS.self") {
		t.Errorf("self not set, yet given: %v", g.self[1])
	}
}
