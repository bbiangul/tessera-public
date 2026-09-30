// Package evals runs EvalSuites (RFC-0002, level 4): whole conversations
// against an agent whose plugins are replaced by stateful sandboxes, with
// per-turn and final expectations, invariants every agent must keep, and a
// budget per scenario.
package evals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Options configures a test run.
type Options struct {
	// Paths hold the agent's resources and the suites.
	Paths []string
	// Decider and Generator are shared by all scenarios (live, or a
	// cassette); each scenario meters them against its own budget.
	Decider   decide.Decider
	Generator generate.Generator
	// Simulator plays the persona of llm scenarios; nil uses Generator.
	// Its cost is reported apart (Result.Simulator).
	Simulator generate.Generator
	// MaxScenarioCostUSD bounds a scenario without its own budget.
	MaxScenarioCostUSD float64
	// Only runs scenarios whose "suite/scenario" contains this text.
	Only string
	// Journal, when set, keeps every scenario's runs (tessera learn and
	// tessera drift read them). Each scenario has its own conversation,
	// so its checks see only its runs. Nil: a memory journal per scenario.
	Journal journal.Store
}

// Line is one entry of a transcript.
type Line struct {
	Who  string `json:"who"` // user, human, agent, queue:<name>
	Text string `json:"text"`
}

// Call is one plugin call a sandbox served.
type Call struct {
	Action  string         `json:"action"`
	Args    map[string]any `json:"args"`
	Key     string         `json:"key,omitempty"`
	Outcome string         `json:"outcome"` // ok, definite, ambiguous, no-sandbox, script-error
	// Output is what the call returned, so a judge can check the agent's
	// facts against it.
	Output any `json:"output,omitempty"`
	// Error is the failure's message: the sandbox's own for definite and
	// ambiguous failures, the Starlark error for a bug in the script.
	Error string `json:"error,omitempty"`
}

// Result is one scenario's outcome.
type Result struct {
	Suite      string   `json:"suite"`
	Scenario   string   `json:"scenario"`
	Pass       bool     `json:"pass"`
	Failures   []string `json:"failures,omitempty"`
	Transcript []Line   `json:"transcript"`
	// Handoffs are the free mode's decisions, per user turn that asked
	// one: what calibrating freeMode.functions' minConfidence needs.
	Handoffs []Handoff     `json:"handoffs,omitempty"`
	Calls    []Call        `json:"calls"`
	Totals   ledger.Totals `json:"totals"`
	// Simulator is what the llm simulator spent, apart from the agent.
	Simulator *ledger.Totals `json:"simulator,omitempty"`
	// TeamAnswers counts the tasks the simulated team answered from its
	// facts (EvalSuite spec.team).
	TeamAnswers int           `json:"teamAnswers,omitempty"`
	Duration    time.Duration `json:"duration"`
}

// Handoff is the free mode's decision at one turn: each option's
// probability, and what it did.
type Handoff struct {
	Turn          int                `json:"turn"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// Run loads the resources and runs every scenario of every suite.
func Run(ctx context.Context, opts Options) ([]Result, []admission.Issue, error) {
	l, err := admission.NewLoader()
	if err != nil {
		return nil, nil, err
	}
	c, err := admission.NewChecker()
	if err != nil {
		return nil, nil, err
	}
	docs, issues := l.LoadPaths(opts.Paths)
	issues = append(issues, c.Check(docs)...)
	for _, is := range issues {
		if is.Severity == admission.SeverityError {
			return nil, issues, host.ErrAdmission
		}
	}
	manifests := map[string]*plugins.PluginManifest{}
	var suites []admission.Doc
	for _, d := range docs {
		if d.Plugin != nil {
			manifests[d.Plugin.Metadata.Name] = d.Plugin
		}
		if d.EvalSuite != nil {
			suites = append(suites, d)
		}
	}
	if len(suites) == 0 {
		return nil, issues, errors.New("no EvalSuite found")
	}
	sort.Slice(suites, func(i, j int) bool { return suites[i].ID() < suites[j].ID() })
	var results []Result
	for _, sd := range suites {
		s := sd.EvalSuite
		for _, sc := range s.Spec.Scenarios {
			name := s.Metadata.Name + "/" + sc.ID
			if opts.Only != "" && !strings.Contains(name, opts.Only) {
				continue
			}
			r := runScenario(ctx, opts, sd, sc, manifests)
			results = append(results, r)
		}
	}
	return results, issues, nil
}

type scenarioRun struct {
	consented map[string]bool // external-message actions whose channel checks consent
	res       Result
	mu        sync.Mutex
	runIDs    map[string]bool
	lastRun   string
	teamFacts string // EvalSuite spec.team's facts, "" without a team
	// refused counts model calls the budget refused. The runtime degrades
	// a failed Decide call to asking a person, so without this count an
	// exhausted budget would pass unnoticed.
	refused atomic.Int64
}

type countingDecider struct {
	decide.Decider
	refused *atomic.Int64
}

func (c countingDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	resp, err := c.Decider.Decide(ctx, req)
	if errors.Is(err, ledger.ErrBudgetExceeded) {
		c.refused.Add(1)
	}
	return resp, err
}

type countingGenerator struct {
	generate.Generator
	refused *atomic.Int64
}

func (c countingGenerator) Generate(ctx context.Context, req generate.Request) (*generate.Response, error) {
	resp, err := c.Generator.Generate(ctx, req)
	if errors.Is(err, ledger.ErrBudgetExceeded) {
		c.refused.Add(1)
	}
	return resp, err
}

func (r *scenarioRun) fail(format string, args ...any) {
	r.res.Failures = append(r.res.Failures, fmt.Sprintf(format, args...))
}

// replayer preserves reset behavior for legacy adapters without scope support.
type replayer interface{ ResetReplay() }

type scopedRecorder interface{ SetScope(string) }

func runScenario(ctx context.Context, opts Options, sd admission.Doc, sc agents.Scenario, manifests map[string]*plugins.PluginManifest) (out Result) {
	s := sd.EvalSuite
	for _, engine := range []any{opts.Decider, opts.Generator} {
		if r, ok := engine.(scopedRecorder); ok {
			r.SetScope(s.Metadata.Namespace + "/" + s.Metadata.Name + "/" + sc.ID)
			defer r.SetScope("")
		} else if r, ok := engine.(replayer); ok {
			r.ResetReplay()
		}
	}
	start := time.Now()
	r := &scenarioRun{res: Result{Suite: s.Metadata.Name, Scenario: sc.ID}, runIDs: map[string]bool{}}
	// The named result is set here, after every return: returning r.res
	// directly would copy it before Pass and Duration are filled in.
	defer func() {
		r.res.Duration = time.Since(start)
		r.res.Pass = len(r.res.Failures) == 0
		out = r.res
	}()

	nowText := sc.Now
	if nowText == "" {
		nowText = s.Spec.Now
	}
	now, err := time.Parse(time.RFC3339, nowText)
	if err != nil {
		r.fail("clock: %v", err)
		return r.res
	}
	r.consented = map[string]bool{}
	for name, m := range manifests {
		if m.Spec.Channel == nil || m.Spec.Channel.Consent == "" {
			continue
		}
		for a, spec := range m.Spec.Actions {
			if spec.Effect == plugins.EffectExternalMessage {
				r.consented[name+"/"+a] = true // authorized by the recipient's consent, checked at every send
			}
		}
	}
	handlers, err := r.sandboxes(sd, sc, manifests)
	if err != nil {
		r.fail("sandbox: %v", err)
		return r.res
	}
	budget := ledger.Budget{MaxCostUSD: opts.MaxScenarioCostUSD}
	var want *agents.ScenarioBudget
	if sc.Expect != nil && sc.Expect.Budget != nil {
		want = sc.Expect.Budget
		if want.CostUSD > 0 {
			budget.MaxCostUSD = want.CostUSD
		}
	}
	led := ledger.New(budget)
	var d decide.Decider = countingDecider{&decide.Metered{Inner: opts.Decider, Ledger: led}, &r.refused}
	var g generate.Generator
	if opts.Generator != nil {
		g = countingGenerator{&generate.Metered{Inner: opts.Generator, Ledger: led}, &r.refused}
	}
	var store journal.Store = journal.NewMemory()
	if opts.Journal != nil {
		store = opts.Journal
	}
	h, _, err := host.Load(host.Options{
		Paths: opts.Paths, Namespace: s.Metadata.Namespace, Store: store, Decider: d, Generator: g,
		Handlers: handlers,
		Observe:  r.observe,
		Configure: func(rt *runtime.Runtime) {
			rt.Clock = func() time.Time { return now }
			rt.Sleep = func(context.Context, time.Duration) error { return nil }
		},
	})
	if err != nil {
		r.fail("load: %v", err)
		return r.res
	}
	defer func() {
		if err := h.Close(); err != nil {
			r.fail("closing: %v", err)
		}
	}()
	conversation := s.Metadata.Name + "/" + sc.ID
	played := true
	for i, t := range sc.Turns {
		if err := r.turn(ctx, h, store, &conversation, &now, sc.Caller, i, t); err != nil {
			r.fail("turn %d: %v", i+1, err)
			played = false
			break
		}
	}
	if played && sc.Simulator == "llm" {
		if t := sd.EvalSuite.Spec.Team; t != nil {
			b, err := os.ReadFile(filepath.Join(filepath.Dir(sd.File), t.Facts))
			if err != nil {
				r.fail("team: %v", err)
				return r.res
			}
			r.teamFacts = string(b)
		}
		sim := opts.Simulator
		if sim == nil {
			sim = opts.Generator
		}
		var metered generate.Generator
		sled := simulatorLedger(opts.MaxScenarioCostUSD)
		if sim != nil {
			metered = &generate.Metered{Inner: sim, Ledger: sled}
		}
		if err := r.simulate(ctx, metered, h, store, &conversation, &now, sc); err != nil {
			r.fail("simulator: %v", err)
		}
		t := sled.Totals()
		r.res.Simulator = &t
	}
	r.res.Totals = led.Totals()
	r.final(ctx, store, sd.EvalSuite.Metadata.Namespace, sc, want)
	return r.res
}

// sandboxes builds a handler for every action of every plugin: the
// sandbox's function when there is one, otherwise a refusal, so a test can
// never reach a real system.
func (r *scenarioRun) sandboxes(sd admission.Doc, sc agents.Scenario, manifests map[string]*plugins.PluginManifest) (map[string]act.Handler, error) {
	boxes := map[string]*compute.Sandbox{}
	for _, sb := range sd.EvalSuite.Spec.Sandboxes {
		src, err := os.ReadFile(filepath.Join(filepath.Dir(sd.File), sb.Script))
		if err != nil {
			return nil, err
		}
		script, err := compute.CompileSandbox(filepath.Base(sb.Script), src)
		if err != nil {
			return nil, err
		}
		state := map[string]any{}
		for k, v := range sb.State {
			state[k] = v
		}
		for k, v := range sc.State[sb.Plugin] {
			state[k] = v
		}
		box, err := compute.NewSandbox(script, state)
		if err != nil {
			return nil, err
		}
		boxes[sb.Plugin] = box
	}
	handlers := map[string]act.Handler{}
	for name, m := range manifests {
		if boxes[name] == nil && (m.Spec.SQL != nil || name == admission.MetricsPlugin || name == admission.LessonsPlugin) {
			// A sql plugin without a sandbox runs for real on a fresh
			// in-memory database per scenario: it is local and
			// deterministic, so faking it would test less.
			continue
		}
		for a := range m.Spec.Actions {
			full, box, action := name+"/"+a, boxes[name], a
			handlers[full] = func(ctx context.Context, c act.Call) (any, error) {
				if box == nil || !box.Has(action) {
					return nil, &act.DefiniteError{Code: "NO_SANDBOX", Message: full + " has no sandbox in this suite"}
				}
				out, err := box.Call(ctx, action, c.Args)
				var f *compute.SandboxFailure
				switch {
				case err == nil:
					return out, nil
				case errors.As(err, &f) && f.Definite:
					return nil, &act.DefiniteError{Code: f.Code, Message: f.Message}
				case errors.As(err, &f):
					return nil, &act.AmbiguousError{Message: f.Message}
				}
				return nil, &act.DefiniteError{Code: "SANDBOX_BUG", Message: err.Error()}
			}
		}
	}
	return handlers, nil
}

// observe records every plugin call, sandboxed or real (sql plugins).
func (r *scenarioRun) observe(c act.Call, out any, err error) {
	call := Call{Action: c.Action, Args: c.Args, Key: c.IdempotencyKey, Outcome: "ok", Output: out}
	var d *act.DefiniteError
	var a *act.AmbiguousError
	switch {
	case err == nil:
	case errors.As(err, &d) && d.Code == "NO_SANDBOX":
		call.Outcome = "no-sandbox"
	case errors.As(err, &d) && d.Code == "SANDBOX_BUG":
		call.Outcome, call.Error = "script-error", d.Message
	case errors.As(err, &d):
		call.Outcome, call.Error = "definite", d.Error()
	case errors.As(err, &a):
		call.Outcome, call.Error = "ambiguous", a.Error()
	default:
		call.Outcome, call.Error = "ambiguous", err.Error()
	}
	r.mu.Lock()
	r.res.Calls = append(r.res.Calls, call)
	r.mu.Unlock()
}

// turn plays one turn and checks its expectations.
func (r *scenarioRun) turn(ctx context.Context, h *host.Host, store journal.Store, conv *string, now *time.Time, caller map[string]any, i int, t agents.Turn) error {
	conversation := *conv
	var res runtime.Result
	routed, handoff := "", ""
	var extra []runtime.Result    // more results of this turn (runs woken or started together)
	var earlier []runtime.Message // sent before the turn's result (recovered runs, agent mode's Functions)
	switch {
	case t.Advance != "":
		d, err := time.ParseDuration(t.Advance)
		if err != nil {
			return err
		}
		*now = now.Add(d)
		r.res.Transcript = append(r.res.Transcript, Line{Who: "clock", Text: "+" + t.Advance})
		results, err := h.Tick(ctx)
		if err != nil {
			return err
		}
		extra = results
	case t.Trigger != nil:
		r.res.Transcript = append(r.res.Transcript, Line{Who: "trigger", Text: t.Trigger.Name})
		var err error
		if t.Trigger.Payload != nil {
			payload, nerr := journal.Normalize(t.Trigger.Payload)
			if nerr != nil {
				return nerr
			}
			res, err = h.StartHook(ctx, t.Trigger.Name, payload.(map[string]any), fmt.Sprintf("turn-%d", i+1))
		} else {
			res, err = h.Fire(ctx, t.Trigger.Name)
		}
		if err != nil {
			return err
		}
		if res.RunID == "" {
			break // an event nobody waited for
		}
		if run, gerr := store.GetRun(ctx, res.RunID); gerr == nil {
			*conv = run.Conversation // later user turns reply in the run's conversation
		}
	case t.User != "":
		r.res.Transcript = append(r.res.Transcript, Line{Who: "user", Text: t.User})
		fallbacks := h.AgentFallbacks()
		rep, err := h.Handle(ctx, conversation, caller, t.User)
		if err != nil {
			return err
		}
		if h.AgentFallbacks() > fallbacks {
			r.fail("turn %d: agent mode could not plan and the router answered (a provider error, or a cassette to record again)", i+1)
		}
		res, routed = rep.Result, rep.Routed
		if rep.Handoff != nil {
			handoff = "answer"
			if rep.Routed != "" {
				handoff = rep.Routed
			}
			r.res.Handoffs = append(r.res.Handoffs, Handoff{Turn: i + 1, Choice: handoff, Probabilities: rep.Handoff})
		}
		if rep.Agent && routed != "" && handoff == "" {
			// Agent mode decides with its planner, not a handoff question:
			// the Function it started is its choice.
			handoff = routed
		}
		for _, e := range rep.Earlier {
			r.runIDs[e.RunID] = true
			for _, m := range e.Outbox {
				r.res.Transcript = append(r.res.Transcript, Line{Who: agentOrQueue(m.To), Text: m.Text})
				earlier = append(earlier, m)
			}
			if n := len(e.Outbox); n > 0 && e.Outbox[n-1].Seq > 0 {
				if err := h.Runtime.Ack(ctx, e.RunID, e.Outbox[n-1].Seq); err != nil {
					return err
				}
			}
		}
	default:
		r.res.Transcript = append(r.res.Transcript, Line{Who: "human", Text: fmt.Sprint(t.Human)})
		run, ok, err := store.ActiveRun(ctx, conversation)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("a human answer, but no run is waiting")
		}
		task, err := h.Runtime.PendingTask(ctx, run.ID)
		if err != nil {
			return err
		}
		// A queue with roles is answered by a member holding one: the
		// first by ID, so the run is the same every time.
		by := "evalsuite"
		if members, open, err := h.Runtime.Answerers(ctx, run.ID); err == nil && !open && len(members) > 0 {
			by = members[0].ID
			for _, m := range members[1:] {
				by = min(by, m.ID)
			}
		}
		if res, err = h.Runtime.Deliver(ctx, run.ID, runtime.Reply{From: "human", Text: fmt.Sprint(t.Human), Value: t.Human, TaskHash: task.Hash, By: by}); err != nil {
			return err
		}
	}
	all := extra
	if res.RunID != "" || len(res.Outbox) > 0 {
		all = append([]runtime.Result{res}, extra...)
	}
	if len(extra) > 0 {
		res = extra[len(extra)-1]
	}
	said, queues := turnSaid(earlier, all)
	for _, x := range all {
		if x.RunID != "" {
			r.runIDs[x.RunID] = true
			r.lastRun = x.RunID
		}
		for _, m := range x.Outbox {
			r.res.Transcript = append(r.res.Transcript, Line{Who: agentOrQueue(m.To), Text: m.Text})
		}
		if n := len(x.Outbox); n > 0 && x.Outbox[n-1].Seq > 0 {
			if err := h.Runtime.Ack(ctx, x.RunID, x.Outbox[n-1].Seq); err != nil {
				return err
			}
		}
	}
	e := t.Expect
	if e == nil {
		return nil
	}
	if e.Status != "" && string(res.Status) != e.Status {
		r.fail("turn %d: status %s, want %s%s", i+1, res.Status, e.Status, reason(res))
	}
	if e.Routed != "" && routed != e.Routed {
		r.fail("turn %d: routed to %q, want %q", i+1, routed, e.Routed)
	}
	if e.Handoff != "" && handoff != e.Handoff {
		got := handoff
		if got == "" {
			got = "no free-mode decision"
		}
		r.fail("turn %d: free mode chose %q, want %q", i+1, got, e.Handoff)
	}
	for _, want := range e.Says {
		if !containsFold(said, want) {
			r.fail("turn %d: no message says %q (got %q)", i+1, want, said)
		}
	}
	for _, bad := range e.NotSays {
		if containsFold(said, bad) {
			r.fail("turn %d: a message says %q", i+1, bad)
		}
	}
	if e.Queue != "" && !queues[e.Queue] {
		r.fail("turn %d: no task went to queue %s", i+1, e.Queue)
	}
	return nil
}

func reason(res runtime.Result) string {
	if res.Error != "" {
		return " (" + res.Error + ")"
	}
	return ""
}

func agentOrQueue(to string) string {
	if to == "user" {
		return "agent"
	}
	return to
}

func containsFold(msgs []string, want string) bool {
	w := strings.ToLower(want)
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m), w) {
			return true
		}
	}
	return false
}

// final checks the scenario's expectations and the invariants.
func (r *scenarioRun) final(ctx context.Context, store journal.Store, namespace string, sc agents.Scenario, budget *agents.ScenarioBudget) {
	e := sc.Expect
	if e == nil {
		e = &agents.Expectation{}
	}
	if e.Status != "" {
		status := "none"
		if r.lastRun != "" {
			if run, err := store.GetRun(ctx, r.lastRun); err == nil {
				status = string(run.Status)
			}
		}
		if status != e.Status {
			r.fail("final status %s, want %s", status, e.Status)
		}
	}
	ok := map[string]int{}
	called := map[string]bool{}
	for _, c := range r.res.Calls {
		called[c.Action] = true
		if c.Outcome == "ok" {
			ok[c.Action]++
		}
		if c.Outcome == "no-sandbox" {
			r.fail("%s was called but has no sandbox", c.Action)
		}
		if c.Outcome == "script-error" {
			r.fail("the sandbox for %s failed, fix the script: %s", c.Action, c.Error)
		}
	}
	for _, ef := range e.Effects {
		if ef.Where == "" {
			if ok[ef.Action] != ef.Count {
				r.fail("%s succeeded %d times, want %d", ef.Action, ok[ef.Action], ef.Count)
			}
			continue
		}
		n, sent, err := r.matching(ctx, ef)
		if err != nil {
			r.fail("%s where %s: %v", ef.Action, ef.Where, err)
			continue
		}
		if n != ef.Count {
			r.fail("%s succeeded %d times where %s, want %d; it was sent %v", ef.Action, n, ef.Where, ef.Count, sent)
		}
	}
	for _, want := range e.Milestones {
		got, err := store.Milestones(ctx, namespace, want.Subject)
		if err != nil {
			r.fail("milestones: %v", err)
			continue
		}
		n := 0
		for _, m := range got {
			if m.Name == want.Name {
				n++
			}
		}
		if n != want.Count {
			r.fail("milestone %s/%s recorded %d times, want %d", want.Subject, want.Name, n, want.Count)
		}
	}
	for _, a := range e.Never {
		if called[a] {
			r.fail("%s must never be called", a)
		}
	}
	var said []string
	for _, l := range r.res.Transcript {
		if l.Who == "agent" {
			said = append(said, l.Text)
		}
	}
	for _, want := range e.Says {
		if !containsFold(said, want) {
			r.fail("no message says %q", want)
		}
	}
	for _, bad := range e.NotSays {
		if containsFold(said, bad) {
			r.fail("a message says %q", bad)
		}
	}
	skip := map[string]bool{}
	for _, s := range e.SkipInvariants {
		skip[s] = true
	}
	if !skip["confirm-before-irreversible"] {
		r.confirmBeforeIrreversible(ctx, store)
	}
	if !skip["no-duplicate-effects"] {
		r.noDuplicateEffects()
	}
	if !skip["no-lost-messages"] {
		r.noLostMessages(ctx, store)
	}
	if budget != nil {
		t := r.res.Totals
		if budget.DecideCalls > 0 && t.Calls["decide"] > budget.DecideCalls {
			r.fail("budget: %d Decide calls, at most %d", t.Calls["decide"], budget.DecideCalls)
		}
		if budget.GenerateCalls > 0 && t.Calls["generate"] > budget.GenerateCalls {
			r.fail("budget: %d Generate calls, at most %d", t.Calls["generate"], budget.GenerateCalls)
		}
	}
	if r.refused.Load() > 0 {
		r.fail("budget: %d model calls were refused because the scenario's budget was spent", r.refused.Load())
	}
}

// confirmSite matches the answer to a confirmation question (confirm1,
// confirm2) or a person's approval of an unclear one (confirm/human...).
var confirmSite = regexp.MustCompile(`confirm(\d+|/human(/\d+)?)$`)

// matching counts the successful calls of an action whose arguments meet
// the expectation's condition, and lists every call's arguments for the
// failure message.
func (r *scenarioRun) matching(ctx context.Context, ef agents.EffectCount) (int, []map[string]any, error) {
	e, err := expr.New()
	if err != nil {
		return 0, nil, err
	}
	x, err := e.Compile(ef.Where, expr.Bool)
	if err != nil {
		return 0, nil, err
	}
	n := 0
	var sent []map[string]any
	for _, c := range r.res.Calls {
		if c.Action != ef.Action || c.Outcome != "ok" {
			continue
		}
		sent = append(sent, c.Args)
		yes, err := x.EvalBool(ctx, expr.Activation{"inputs": c.Args})
		if err != nil {
			return 0, sent, err
		}
		if yes {
			n++
		}
	}
	return n, sent, nil
}

// intentAction is the action an intent record is about.
func intentAction(v []byte) string {
	var m struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(v, &m)
	return m.Action
}

// turnSaid is what a turn told the user, in order, and the queues it
// wrote to: the messages sent before its result (agent mode's Functions,
// recovered runs) and its results'.
func turnSaid(earlier []runtime.Message, results []runtime.Result) ([]string, map[string]bool) {
	var said []string
	queues := map[string]bool{}
	add := func(m runtime.Message) {
		if m.To == "user" {
			said = append(said, m.Text)
		} else {
			queues[strings.TrimPrefix(m.To, "queue:")] = true
		}
	}
	for _, m := range earlier {
		add(m)
	}
	for _, x := range results {
		for _, m := range x.Outbox {
			add(m)
		}
	}
	return said, queues
}
