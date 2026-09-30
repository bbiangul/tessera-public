package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const restaurant = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: opening-hours, namespace: bistro }
spec:
  description: Tells when the restaurant is open.
  selector: { intent: hours }
  output: { template: "Abrimos de terça a domingo, das 12h às 23h." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: book-table, namespace: bistro }
spec:
  description: Books a table for tonight.
  selector: { intent: booking }
  steps:
    - id: time
      ask:
        text: "Qual horário?"
        choices: "['19h', '21h']"
        minConfidence: 0.7
        onLowConfidence: ask-user
    - id: human
      escalate: { queue: salao, task: approve, message: "Mesa às {{ steps.time.output }}?" }
  output: { template: "Reservado às {{ steps.time.output }}." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: internal-helper, namespace: bistro }
spec:
  description: Not routable.
  output: { template: "x" }
`

// wordDecider routes and answers by keywords, and records what it saw.
type wordDecider struct {
	mu     sync.Mutex
	states []any
}

func (d *wordDecider) Name() string { return "words" }
func (d *wordDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	d.mu.Lock()
	d.states = append(d.states, req.State)
	d.mu.Unlock()
	state, _ := req.State.(map[string]any)
	text, _ := state["message"].(string)
	if r, ok := state["reply"].(string); ok {
		text = r
	}
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "words"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		pick := "none"
		switch {
		case strings.Contains(text, "horário de funcionamento"):
			pick = "opening-hours"
		case strings.Contains(text, "mesa"):
			pick = "book-table"
		case strings.Contains(text, "talvez reservar"):
			pick = "book-table" // but unsure
		case strings.Contains(text, "21h"):
			pick = "o2"
		}
		for _, o := range q.Options {
			probs[o.Key] = 0.02
		}
		if _, ok := probs[pick]; !ok {
			pick = "none"
		}
		probs[pick] = 1 - 0.02*float64(len(q.Options)-1)
		if strings.Contains(text, "talvez reservar") {
			probs = map[string]float64{}
			for _, o := range q.Options {
				probs[o.Key] = 1 / float64(len(q.Options))
			}
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func load(t *testing.T) (*Host, *wordDecider) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(restaurant), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &wordDecider{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: d})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h, d
}

func texts(r Reply) []string {
	var out []string
	for _, m := range r.Outbox {
		out = append(out, m.To+": "+m.Text)
	}
	return out
}

func TestRoutesOnlyFunctionsWithASelector(t *testing.T) {
	h, _ := load(t)
	var names []string
	for _, r := range h.Routes() {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "book-table,opening-hours" {
		t.Errorf("routes %v", names)
	}
}

func TestConversation(t *testing.T) {
	h, d := load(t)
	ctx := context.Background()
	r, err := h.Handle(ctx, "c1", nil, "qual o horário de funcionamento?")
	if err != nil || r.Routed != "opening-hours" || r.Status != journal.StatusSucceeded {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = h.Handle(ctx, "c1", nil, "quero uma mesa, meu email é ana@x.com")
	if err != nil || r.Routed != "book-table" || r.Status != journal.StatusWaitingForUser || texts(r)[0] != "user: Qual horário?" {
		t.Fatalf("%+v %v", r, err)
	}
	if err := h.Runtime.Ack(ctx, r.RunID, r.Outbox[len(r.Outbox)-1].Seq); err != nil {
		t.Fatal(err)
	}
	r, err = h.Handle(ctx, "c1", nil, "às 21h")
	if err != nil || r.Routed != "" || r.Status != journal.StatusWaitingForHuman {
		t.Fatalf("the reply must go to the waiting run: %+v %v", r, err)
	}
	if got := texts(r); len(got) != 1 || got[0] != "queue:salao: Mesa às 21h?" {
		t.Errorf("outbox %q", got)
	}
	_ = h.Runtime.Ack(ctx, r.RunID, r.Outbox[len(r.Outbox)-1].Seq)
	r, err = h.Handle(ctx, "c1", nil, "e aí?")
	if err != nil || r.Routed != "" || len(r.Outbox) != 1 || !strings.Contains(r.Outbox[0].Text, "person") {
		t.Errorf("while a person handles the run, the user hears so: %+v %v", r, err)
	}
	for _, s := range d.states {
		if strings.Contains(strings.ToLower(strings.TrimSpace(anyString(s))), "ana@x.com") {
			t.Errorf("the router saw personal data: %v", s)
		}
	}
}

func anyString(v any) string {
	m, _ := v.(map[string]any)
	s, _ := m["message"].(string)
	return s
}

func TestUnroutedMessages(t *testing.T) {
	h, _ := load(t)
	for _, text := range []string{"bom dia!", "talvez reservar algo"} {
		r, err := h.Handle(context.Background(), "c2", nil, text)
		if err != nil || !r.Unrouted || r.RunID != "" || len(r.Outbox) != 1 {
			t.Errorf("%q: a message that fits nothing, or fits too weakly, starts no run: %+v %v", text, r, err)
		}
	}
}

func TestLoadRefusesInvalidResources(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(strings.Replace(restaurant, "description: Tells", "descripton: Tells", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}}); err != ErrAdmission || len(issues) == 0 {
		t.Errorf("want ErrAdmission with issues, got %v, %v", err, issues)
	}
	if _, _, err := Load(Options{Paths: []string{"../../testdata/agenda"}, Namespace: "nowhere", Store: journal.NewMemory(), Decider: &wordDecider{}}); err == nil {
		t.Error("a namespace with nothing routable must be refused")
	}
}

// slowRouter pauses while routing, to widen the window in which two
// first messages could both see no active run.
type slowRouter struct{ wordDecider }

func (d *slowRouter) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	if _, routing := req.Questions["route"]; routing {
		time.Sleep(30 * time.Millisecond)
	}
	return d.wordDecider.Decide(ctx, req)
}

// Review 2026-09-23: two first messages at once started two runs, and one
// was orphaned.
func TestConcurrentFirstMessagesStartOneRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(restaurant), 0o600); err != nil {
		t.Fatal(err)
	}
	h, _, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &slowRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	var started atomic.Int64
	h.Runtime.NewID = func() string { return fmt.Sprintf("run-%d", started.Add(1)) }
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.Handle(context.Background(), "same", nil, "quero uma mesa"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := started.Load(); n != 1 {
		t.Errorf("%d runs started for one conversation; the second message must reach the first run", n)
	}
}

const bistroAgent = `
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: bistro, namespace: bistro }
spec:
  messages:
    notUnderstood: "Não entendi. Pode dizer de outro jeito?"
    withAPerson: "Uma pessoa da equipe está vendo isso e responde aqui."
  humanInTheLoop: { queue: salao }
`

// The namespace's Agent speaks the users' language: the platform's own
// messages come from it, and escalations without a queue go to its queue.
func TestAgentMessages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(restaurant+bistroAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r, err := h.Handle(context.Background(), "c1", nil, "bom dia!")
	if err != nil || len(r.Outbox) != 1 || r.Outbox[0].Text != "Não entendi. Pode dizer de outro jeito?" {
		t.Errorf("unrouted: %+v %v", r, err)
	}
	if h.opts.MinRouteConfidence != 0.6 {
		t.Errorf("without routing in the Agent the threshold is the default: %v", h.opts.MinRouteConfidence)
	}
	if h.Runtime.Limits.DefaultQueue != "salao" {
		t.Errorf("default queue %q", h.Runtime.Limits.DefaultQueue)
	}
	if got := h.Runtime.Messages.Failed; got != "" && !strings.HasPrefix(got, "Something") {
		t.Errorf("a message the Agent leaves empty keeps the default: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "again.yaml"), []byte(strings.TrimPrefix(bistroAgent, "\n---\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(restaurant+bistroAgent+"  routing: { minConfidence: 0.5 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if h, _, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}}); err == nil {
		t.Errorf("two Agents must be refused, got threshold %v", h.opts.MinRouteConfidence)
	}
	if _, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}}); err != ErrAdmission || !strings.Contains(fmt.Sprint(issues), "at most one Agent") {
		t.Errorf("two Agents in a namespace must be refused: %v %v", err, issues)
	}
}

// The namespace's Agent sets its routing threshold; explicit options win.
func TestAgentRoutingThreshold(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(restaurant+bistroAgent+"  routing: { minConfidence: 0.45 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}})
	if err != nil || h.opts.MinRouteConfidence != 0.45 {
		t.Fatalf("threshold %v: %v %v", h.opts.MinRouteConfidence, err, issues)
	}
	h, _, err = Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}, MinRouteConfidence: 0.8})
	if err != nil || h.opts.MinRouteConfidence != 0.8 {
		t.Errorf("an explicit option wins: %v %v", h.opts.MinRouteConfidence, err)
	}
}

// A follow-up (selector.after) is an option only after its Functions:
// without a previous run the router never sees it.
func TestFollowUpsOnlyAfterTheirFunctions(t *testing.T) {
	dir := t.TempDir()
	followUp := strings.Replace(restaurant, "  selector: { intent: hours }", "  selector: { intent: hours, after: [book-table] }", 1)
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(followUp), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &optionsSeen{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: d})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	if _, err := h.Route(ctx, "que horas abre?", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Route(ctx, "que horas abre?", "Reservado às 21h.", "book-table"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.seen, " | ") != "book-table,none | book-table,opening-hours,none" {
		t.Errorf("options per call: %q", d.seen)
	}
	bad := strings.Replace(restaurant, "  selector: { intent: hours }", "  selector: { intent: hours, after: [internal-helper] }", 1)
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: d}); err != ErrAdmission || !strings.Contains(fmt.Sprint(issues), "UNKNOWN_FUNCTION") {
		t.Errorf("after must name routable Functions: %v %v", err, issues)
	}
}

type optionsSeen struct{ seen []string }

func (d *optionsSeen) Name() string { return "options" }
func (d *optionsSeen) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	var keys []string
	for _, o := range req.Questions["route"].Options {
		keys = append(keys, o.Key)
	}
	d.seen = append(d.seen, strings.Join(keys, ","))
	probs := map[string]float64{"none": 1}
	for _, k := range keys {
		if k != "none" {
			probs[k] = 0
		}
	}
	a, err := decide.AnswerFor(req.Questions["route"], probs, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"route": a}, Model: "options"}, nil
}

// An agent with nothing routable (outbound only) answers "not understood"
// without asking Decide, which needs two options.
func TestNothingToRoute(t *testing.T) {
	dir := t.TempDir()
	doc := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: internal-helper, namespace: bistro }
spec:
  description: Not routable.
  output: { template: "x" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: hook, namespace: bistro }
spec: { function: internal-helper, webhook: { secret: HOOK } }
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: journal.NewMemory(), Decider: &wordDecider{}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	r, err := h.Handle(context.Background(), "c1", nil, "oi")
	if err != nil || r.Routed != "" || len(r.Outbox) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

// A channel's retry (the same message ID) is not handled twice.
func TestHandleOnce(t *testing.T) {
	h, _ := load(t)
	ctx := context.Background()
	r, err := h.HandleOnce(ctx, "c1", nil, "qual o horário de funcionamento?", "m1")
	if err != nil || r.Routed != "opening-hours" {
		t.Fatalf("%+v %v", r, err)
	}
	again, err := h.HandleOnce(ctx, "c1", nil, "qual o horário de funcionamento?", "m1")
	if err != nil || !again.Duplicate || again.Routed != "" || len(again.Outbox) != 0 {
		t.Fatalf("a retry runs nothing: %+v %v", again, err)
	}
	if other, _ := h.HandleOnce(ctx, "c2", nil, "qual o horário de funcionamento?", "m1"); other.Duplicate {
		t.Error("the same ID in another conversation is another message")
	}
	runs, _ := h.Runtime.Store.Runs(ctx, journal.RunFilter{Conversation: "c1"})
	if len(runs) != 1 {
		t.Errorf("runs %d", len(runs))
	}
}
