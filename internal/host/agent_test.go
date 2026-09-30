package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Agent mode (RFC-0010): the planner's steps, what it may give a
// Function, and the checks on its reply.

const agentDocs = agendaDocs + `---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinica, version: 0.1.0 }
spec:
  actions:
    info: { input: {}, output: map, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: clinica }
spec:
  description: Says which slots are free on a day.
  selector: { intent: horarios }
  inputs:
    dia:
      type: string
      description: the day asked about
      resolve:
        - { id: said, extract: {} }
        - id: asked
          ask: { text: "Para qual dia?", minConfidence: 0.7, onLowConfidence: ask-user }
  action: { act: clinica/info }
  output: { template: "Livre {{ inputs.dia }} às 16h." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: recepcao, namespace: clinica }
spec:
  freeMode:
    instructions: "Responda dúvidas da clínica com os fatos."
    reads: [ { act: clinica/info } ]
    agent: { maxSteps: 3 }
`

// planGen plays the planner from a script, one plan per step; any other
// request (an extraction) gets nulls.
type planGen struct {
	mu     sync.Mutex
	plans  []map[string]any
	states []map[string]any
	fail   bool
	free   string // the free answer's text; default about parking
	frees  []map[string]any
}

func (g *planGen) Name() string { return "plans" }
func (g *planGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, _ := req.State.(map[string]any)
	if _, ok := st["tools"]; !ok {
		out := map[string]any{}
		if props, ok := req.Schema["properties"].(map[string]any); ok {
			for k := range props {
				out[k] = nil
			}
		}
		free := g.free
		if free == "" {
			free = "Temos estacionamento conveniado."
		}
		if req.Schema == nil {
			g.frees = append(g.frees, st)
		}
		return &generate.Response{Value: out, Text: free}, nil
	}
	g.states = append(g.states, st)
	if g.fail {
		return nil, errors.New("provider down")
	}
	if len(g.plans) == 0 {
		return &generate.Response{Value: map[string]any{"action": "reply", "text": ""}}, nil
	}
	p := g.plans[0]
	g.plans = g.plans[1:]
	return &generate.Response{Value: p}, nil
}

func call(tool string, args ...string) map[string]any {
	var list []any
	for i := 0; i+1 < len(args); i += 2 {
		list = append(list, map[string]any{"name": args[i], "value": args[i+1]})
	}
	return map[string]any{"thought": "", "action": "call", "tool": tool, "args": list, "text": ""}
}

func reply(text string) map[string]any {
	return map[string]any{"thought": "", "action": "reply", "tool": "", "args": []any{}, "text": text}
}

func agentHost(t *testing.T, plans ...map[string]any) (*Host, *[]string, *planGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(agentDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	var booked []string
	g := &planGen{plans: plans}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
		Handlers: map[string]act.Handler{
			"agenda/marcar": func(_ context.Context, c act.Call) (any, error) {
				booked = append(booked, c.Args["slot"].(string))
				return map[string]any{}, nil
			},
			"clinica/info": func(context.Context, act.Call) (any, error) {
				return map[string]any{"estacionamento": "conveniado ao lado"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, &booked, g
}

func allTexts(r Reply) []string {
	var out []string
	for _, res := range append(r.Earlier, r.Result) {
		for _, m := range res.Outbox {
			out = append(out, m.Text)
		}
	}
	return out
}

// A Function that finishes is an observation: the planner goes on and
// replies with the rest, after the Function's own message. The input it
// gave is the Function's, not extracted again.
func TestAgentCallsThenReplies(t *testing.T) {
	h, _, g := agentHost(t, call("horarios", "dia", "quarta"), reply("E temos estacionamento conveniado ao lado."))
	r := say(t, h, "tem horário quarta? e estacionamento?")
	got := allTexts(r)
	if len(got) != 2 || got[0] != "Livre quarta às 16h." || got[1] != "E temos estacionamento conveniado ao lado." {
		t.Fatalf("the Function's message, then the reply: %q", got)
	}
	obs, _ := g.states[1]["observations"].([]any)
	if len(obs) != 1 || !strings.Contains(strings.Join(anyStringsOf(obs[0].(map[string]any)["sentToUser"]), ""), "Livre quarta") {
		t.Errorf("the second step sees what the Function sent: %v", g.states[1]["observations"])
	}
	// The next turn's history holds the message once.
	lines := h.recentLines(context.Background(), "c1", 20)
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "user: ") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("history %q", lines)
	}
}

func anyStringsOf(v any) []string {
	var out []string
	for _, x := range anyList(v) {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// Effects happen only in a Function, after its confirmation: the planner
// starting marcar ends the turn at the confirmation, and only the yes
// books, once.
func TestAgentEffectWaitsForConfirmation(t *testing.T) {
	h, booked, g := agentHost(t, call("marcar", "slot", "terça 10h"), call("horarios", "dia", "quarta"))
	r := say(t, h, "quero marcar terça 10h")
	if r.Result.Status != journal.StatusConfirming || r.Routed != "marcar" || len(*booked) != 0 {
		t.Fatalf("the turn ends at the confirmation: %+v, booked %v", r, *booked)
	}
	if len(g.states) != 1 || len(r.Earlier) != 0 {
		t.Fatalf("no step after a Function that waits: %d steps", len(g.states))
	}
	if got := allTexts(r); got[len(got)-1] != "Confirmo terça 10h?" {
		t.Fatalf("%q", got)
	}
	if r := say(t, h, "sim"); r.Result.Status != journal.StatusSucceeded || len(*booked) != 1 || (*booked)[0] != "terça 10h" {
		t.Fatalf("booked %v, %+v", *booked, r)
	}
}

// A placeholder or an unknown input is not given: the Function asks.
func TestAgentArgsArePlainValues(t *testing.T) {
	h, _, _ := agentHost(t, call("horarios", "dia", "[dia]", "hora", "10h"))
	r := say(t, h, "tem horário?")
	if r.Result.Status != journal.StatusWaitingForUser || allTexts(r)[0] != "Para qual dia?" {
		t.Fatalf("the Function asks what it was not given: %+v", r)
	}
	tool := agentTool{args: map[string]string{"n": "int", "d": "date", "b": "bool", "s": "string"}}
	got := agentArgs(tool, []any{
		map[string]any{"name": "n", "value": "2"}, map[string]any{"name": "d", "value": "15/11"},
		map[string]any{"name": "b", "value": "true"}, map[string]any{"name": "s", "value": "null"},
	}, "2026-09-28")
	if len(got) != 2 || got["n"] != int64(2) || got["b"] != true {
		t.Errorf("typed, invalid dates and placeholders dropped: %v", got)
	}
	dates := agentArgs(tool, []any{map[string]any{"name": "d", "value": "2025-11-14"}}, "2026-09-28")
	if len(dates) != 0 || agentArgs(tool, []any{map[string]any{"name": "d", "value": "2026-11-14"}}, "2026-09-28")["d"] != "2026-11-14" {
		t.Errorf("a past date is left to the Function: %v", dates)
	}
}

// A reply that claims an action no Function took is planned once more;
// if it still claims, the free answer is given instead (agency-005), and
// when that fails too, the unsure message.
func TestAgentReplyIsChecked(t *testing.T) {
	h, _, g := agentHost(t, reply("Pronto, registrei seu pedido."), reply("Já registrei."))
	r := say(t, h, "anota que eu quero terça. tem estacionamento?")
	if got := allTexts(r); len(got) != 1 || got[0] != "Temos estacionamento conveniado." {
		t.Fatalf("the free answer: %q", got)
	}
	if len(g.states) != 2 || g.states[1]["previous"] != "Pronto, registrei seu pedido." {
		t.Errorf("the second plan sees its draft: %d plans", len(g.states))
	}
	h, _, g = agentHost(t, reply("Pronto, registrei seu pedido."), reply("Já registrei."))
	g.free = "Mando para [EMAIL]."
	r = say(t, h, "anota que eu quero terça")
	if got := allTexts(r); len(got) != 1 || got[0] != h.unsure(map[string]any{}) {
		t.Fatalf("the unsure message: %q", got)
	}
}

// agency-004 (w15): a Function answered part of the message and the
// planner's reply to the rest was dropped: the rest is answered by the
// free mode, told what was already sent, or the unsure message says it is
// not; never nothing.
func TestAgentRestIsNotDropped(t *testing.T) {
	h, _, g := agentHost(t, call("horarios", "dia", "quarta"), reply("Já registrei, tem estacionamento."), reply("Já registrei."))
	r := say(t, h, "tem horário quarta? e estacionamento?")
	got := allTexts(r)
	if len(got) != 2 || got[0] != "Livre quarta às 16h." || got[1] != "Temos estacionamento conveniado." {
		t.Fatalf("the Function's message, then the free answer: %q", got)
	}
	last := g.frees[len(g.frees)-1]
	if a, _ := last["already"].(string); !strings.Contains(a, "Livre quarta") {
		t.Errorf("the free answer knows what was sent: %v", last["already"])
	}
	h, _, g = agentHost(t, call("horarios", "dia", "quarta"), reply("Já registrei, tem estacionamento."), reply("Já registrei."))
	g.free = "[EMAIL]"
	got = allTexts(say(t, h, "tem horário quarta? e estacionamento?"))
	if len(got) != 2 || got[1] != h.unsure(map[string]any{}) {
		t.Fatalf("the unsure message after the Function's: %q", got)
	}
	// A planner with nothing to add adds nothing.
	h, _, _ = agentHost(t, call("horarios", "dia", "quarta"), reply(""))
	if got := allTexts(say(t, h, "tem horário quarta?")); len(got) != 1 {
		t.Fatalf("%q", got)
	}
}

// A step that repeats a call is told so; one Function runs a turn,
// even another or with other inputs (idempotency keys assume one run per
// message); the step limit ends in a reply, and a call there is not made.
func TestAgentStepLimit(t *testing.T) {
	h, _, g := agentHost(t, call("horarios", "dia", "quarta"), call("horarios", "dia", "quarta"), call("horarios", "dia", "quinta"), call("horarios", "dia", "sexta"))
	r := say(t, h, "horários da semana?")
	if len(g.states) != 4 {
		t.Fatalf("maxSteps 3, then one reply-only step: %d steps", len(g.states))
	}
	if got := allTexts(r); len(got) != 1 || got[0] != "Livre quarta às 16h." {
		t.Fatalf("one run; no call at the reply-only step: %q", got)
	}
	h, _, _ = agentHost(t, call("horarios", "dia", "segunda"), call("horarios", "dia", "terça"), call("marcar", "slot", "terça 10h"))
	h.freeFor(map[string]any{}).agentSteps = 5
	if got := allTexts(say(t, h, "horários da semana?")); len(got) != 1 {
		t.Fatalf("one Function a turn, the same or another: %q", got)
	}
}

// With no plan (the provider is down), the router answers as without
// agent mode.
func TestAgentFallsBackToTheRouter(t *testing.T) {
	h, _, g := agentHost(t)
	g.fail = true
	r := say(t, h, "quero marcar")
	if r.Routed != "marcar" || r.Result.Status != journal.StatusWaitingForUser {
		t.Fatalf("%+v", r)
	}
	if h.AgentFallbacks() != 1 {
		t.Errorf("the fallback is counted (#72): %d", h.AgentFallbacks())
	}
}

// agency-005: after a Function answered all the message asked, the
// planner's add-on would only say it again; it is not sent. The turn
// reports the Function it ran.
func TestAgentAddsNothingRedundant(t *testing.T) {
	h, _, _ := agentHost(t, call("horarios", "dia", "quarta"), reply("Esses são os horários que te mostrei."))
	r := say(t, h, "tem horário quarta?")
	if got := allTexts(r); len(got) != 1 || got[0] != "Livre quarta às 16h." {
		t.Fatalf("only the Function's message: %q", got)
	}
	if r.Routed != "horarios" || !r.Agent {
		t.Errorf("routed %q, agent %v", r.Routed, r.Agent)
	}
	h, _, _ = agentHost(t, call("horarios", "dia", "quarta"), reply("E temos estacionamento conveniado ao lado."))
	if r := say(t, h, "tem horário quarta? e estacionamento?"); r.Routed != "horarios" || len(allTexts(r)) != 2 {
		t.Errorf("the rest answered, routed %q: %q", r.Routed, allTexts(r))
	}
}

// agency-005: the same Function with the same inputs on the next message
// would show the same thing again ("mas eu já paguei" got the status four
// times). It is not run; the planner answers from what it showed. With
// other inputs it runs.
func TestAgentDoesNotRepeatAFunction(t *testing.T) {
	h, _, g := agentHost(t, call("horarios", "dia", "quarta"), reply(""),
		call("horarios", "dia", "quarta"), reply("Como te disse, quarta às 16h está livre."),
		call("horarios", "dia", "sexta"), reply(""))
	say(t, h, "tem horário quarta?")
	r := say(t, h, "mas quarta tem mesmo?")
	if got := allTexts(r); len(got) != 1 || got[0] != "Como te disse, quarta às 16h está livre." {
		t.Fatalf("not shown again, answered: %q", got)
	}
	obs, _ := g.states[len(g.states)-1]["observations"].([]any)
	if len(obs) != 1 || !strings.Contains(obs[0].(map[string]any)["result"].(string), "not run") {
		t.Errorf("the planner is told: %v", obs)
	}
	if got := allTexts(say(t, h, "e sexta?")); len(got) != 1 || got[0] != "Livre sexta às 16h." {
		t.Fatalf("other inputs run: %q", got)
	}
}
