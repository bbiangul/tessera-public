package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const autonomyAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinic, version: 0.1.0 }
spec:
  actions:
    block: { input: { day: string }, effect: irreversible }
    cancel: { input: {}, effect: irreversible }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: au }
spec:
  humanInTheLoop: { queue: revisao }
  messages: { denied: "NAO-APROVADO" }
  audiences:
    - name: equipe
      who: "has(caller.roles) && size(caller.roles) > 0"
      functions: [bloquear]
      autonomy: approve-actions
      sees: [ { audience: paciente, conversation: masked } ]
    - name: paciente
      functions: [horarios, cancelar]
      autonomy: draft
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: revisao, namespace: au }
spec: {}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: au }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: au }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "horário" }
  output: { template: "Abrimos das 8h às 18h." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cancelar, namespace: au }
spec:
  description: Cancels.
  selector: { intent: cancelar, description: "cancela" }
  action: { act: clinic/cancel, with: {}, idempotencyKey: "'c'", confirm: "Posso cancelar? Responda sim." }
  output: { template: "Cancelado." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: bloquear, namespace: au }
spec:
  description: Blocks a day.
  selector: { intent: bloquear, description: "bloqueia" }
  action: { act: clinic/block, with: { day: sexta }, idempotencyKey: "'sexta'" }
  output: { template: "Bloqueado." }
`

type keyRouter struct{}

func (keyRouter) Name() string { return "key" }
func (keyRouter) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	st, _ := req.State.(map[string]any)
	msg, _ := st["message"].(string)
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "key"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind != decide.KindChoice {
			probs["true"], probs["false"] = 0.02, 0.98
		} else {
			pick := q.Options[0].Key
			for _, o := range q.Options {
				probs[o.Key] = 0.01
				if o.Key == "none" {
					pick = "none"
				}
			}
			for _, o := range q.Options {
				if strings.Contains(msg, o.Description) {
					pick = o.Key
				}
			}
			probs[pick] = 0.95
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func loadAutonomy(t *testing.T) (*Host, journal.Store, *[]string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(autonomyAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var effects []string
	record := func(name string) act.Handler {
		return func(context.Context, act.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			effects = append(effects, name)
			return map[string]any{}, nil
		}
	}
	store := journal.NewMemory()
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "au", Store: store, Decider: keyRouter{},
		Handlers: map[string]act.Handler{"clinic/block": record("block"), "clinic/cancel": record("cancel")}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, store, &effects
}

// userTexts are the messages a run sent the user.
func userTexts(t *testing.T, store journal.Store, runID string) []string {
	t.Helper()
	recs, _ := store.Records(context.Background(), runID)
	var out []string
	for _, r := range recs {
		if v, err := journal.Decode(r.Value); err == nil && r.Kind == journal.KindSend {
			if m, _ := v.(map[string]any); m["to"] == "user" {
				out = append(out, m["text"].(string))
			}
		}
	}
	return out
}

func answer(t *testing.T, h *Host, runID string, yes bool) runtime.Result {
	t.Helper()
	task, err := h.Runtime.PendingTask(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Runtime.Deliver(context.Background(), runID, runtime.Reply{From: "human", TaskHash: task.Hash, Value: yes, Text: map[bool]string{true: "approved", false: "denied"}[yes]})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// audiences-002 (A3): under draft a run's message waits for a person's
// approval of its exact text; approved it goes out once, denied nothing
// does. A confirmation question waits too.
func TestDraftAutonomy(t *testing.T) {
	h, store, effects := loadAutonomy(t)
	ctx := context.Background()
	patient := map[string]any{"channel": "whatsapp"}
	rep, err := h.Handle(ctx, "p1", patient, "qual o horário?")
	if err != nil || rep.Result.Status != journal.StatusWaitingForHuman || len(userTexts(t, store, rep.Result.RunID)) != 0 {
		t.Fatalf("the answer must wait: %+v %v", rep.Result, err)
	}
	task, _ := h.Runtime.PendingTask(ctx, rep.Result.RunID)
	if !strings.Contains(task.Text, "Abrimos das 8h às 18h.") || task.Kind != "approve" {
		t.Fatalf("the draft's exact text: %+v", task)
	}
	answer(t, h, rep.Result.RunID, true)
	if got := userTexts(t, store, rep.Result.RunID); len(got) != 1 || got[0] != "Abrimos das 8h às 18h." {
		t.Errorf("approved, sent once: %v", got)
	}
	rep, _ = h.Handle(ctx, "p2", patient, "qual o horário?")
	answer(t, h, rep.Result.RunID, false)
	if got := userTexts(t, store, rep.Result.RunID); len(got) != 0 {
		t.Errorf("denied, nothing sent: %v", got)
	}
	// A confirmation question waits for approval before the user sees it.
	rep, _ = h.Handle(ctx, "p3", patient, "cancela minha consulta")
	task, _ = h.Runtime.PendingTask(ctx, rep.Result.RunID)
	if !strings.Contains(task.Text, "Posso cancelar?") || len(userTexts(t, store, rep.Result.RunID)) != 0 {
		t.Errorf("the question waits: %+v", task)
	}
	if len(*effects) != 0 {
		t.Errorf("effects %v", *effects)
	}
}

// audiences-002 (A1): under approve-actions an irreversible effect waits
// for the humanInTheLoop queue; approved, it happens once; denied, it does
// not, and the user hears messages.denied.
func TestApproveActionsAutonomy(t *testing.T) {
	h, store, effects := loadAutonomy(t)
	ctx := context.Background()
	rita := map[string]any{"member": "rita", "roles": []any{"recepcao"}}
	rep, err := h.Handle(ctx, "r1", rita, "bloqueia minha sexta")
	if err != nil || rep.Result.Status != journal.StatusWaitingForHuman || len(*effects) != 0 {
		t.Fatalf("the effect must wait: %+v %v %v", rep.Result, err, *effects)
	}
	task, _ := h.Runtime.PendingTask(ctx, rep.Result.RunID)
	if !strings.Contains(task.Text, "clinic/block") || !strings.Contains(task.Text, "sexta") {
		t.Errorf("the task names the action and its arguments: %q", task.Text)
	}
	answer(t, h, rep.Result.RunID, true)
	if strings.Join(*effects, ",") != "block" || strings.Join(userTexts(t, store, rep.Result.RunID), "|") != "Bloqueado." {
		t.Errorf("approved: %v %v", *effects, userTexts(t, store, rep.Result.RunID))
	}
	rep, _ = h.Handle(ctx, "r2", rita, "bloqueia minha sexta")
	answer(t, h, rep.Result.RunID, false)
	if len(*effects) != 1 || strings.Join(userTexts(t, store, rep.Result.RunID), "|") != "NAO-APROVADO" {
		t.Errorf("denied: %v %v", *effects, userTexts(t, store, rep.Result.RunID))
	}
}

// audiences-002 (V1, V3, V4): a member whose audience sees the patients'
// conversations masked sees them so, with every view journaled; a member
// of an audience without sees gets the task's text only.
func TestContextVisibility(t *testing.T) {
	h, store, _ := loadAutonomy(t)
	ctx := context.Background()
	rep, _ := h.Handle(ctx, "p1", map[string]any{"channel": "whatsapp"}, "qual o horário? meu email é ana@exemplo.com")
	task, err := h.Runtime.PendingTask(ctx, rep.Result.RunID) // the draft approval
	if err != nil {
		t.Fatal(err)
	}
	rita, _ := h.Runtime.Member("au", "rita")
	view := h.Runtime.TaskView(ctx, task, rita, "slack")
	if !strings.Contains(view, "Conversa:") || !strings.Contains(view, "[EMAIL]") || strings.Contains(view, "ana@exemplo.com") || !strings.Contains(view, "qual o horário?") {
		t.Errorf("Rita sees the conversation masked: %q", view)
	}
	h.Runtime.TaskView(ctx, task, rita, "slack") // the same view again: one audit record
	outsider := agents.Member{ID: "x", Name: "X"}
	if view := h.Runtime.TaskView(ctx, task, outsider, "slack"); view != task.Text {
		t.Errorf("no sees, only the text: %q", view)
	}
	recs, _ := store.Records(ctx, rep.Result.RunID)
	audits := 0
	for _, r := range recs {
		if strings.Contains(r.Site, "/viewed/") {
			audits++
			if !strings.Contains(r.Site, "/viewed/rita/slack") || !strings.Contains(string(r.Value), `"visibility":"masked"`) {
				t.Errorf("audit %s %s", r.Site, r.Value)
			}
		}
	}
	if audits != 1 {
		t.Errorf("one audit record for Rita's view, got %d", audits)
	}
}
