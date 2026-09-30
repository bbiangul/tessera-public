package runtime

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// step is a compiled Function step (RFC-0001, "Tipos de passo").
type step struct {
	id                  string
	when                *expr.Expr
	kind                string
	cel                 *expr.Expr      // compute.cel
	script              *compute.Script // compute.starlark
	fn                  string          // starlark function, or the called Function
	with                *expr.Value     // act, call, starlark arguments
	act                 *action         // act (with, key, confirm)
	batch               *batchStep
	decide              *decideStep
	gen                 *genStep
	ask                 *ask
	respond             *expr.Template
	to                  string // respond: user or queue:<name>
	esc                 *escalate
	branch              []branchCase
	waitFor             time.Duration
	until               *expr.Expr
	orReply             bool
	includeSideMessages bool
	relay               bool
	events              []waitEvent
	milestone           *milestone
	exp                 *experiment
	retrieve            *retrieveStep
	onError             []strategy
}

// retrieveStep is a compiled retrieve step.
type retrieveStep struct {
	bases []string
	query *expr.Expr
	k     int
}

// waitEvent is an event a wait listens for, and the key it must carry.
type waitEvent struct {
	name string
	key  *expr.Expr
}

type decideStep struct {
	state    *expr.Value
	question decide.Question
	min      float64
	onLow    string
}

type genStep struct {
	state        *expr.Value
	instructions string
	schema       map[string]any
}

type branchCase struct {
	when    *expr.Expr // nil for else
	call    string
	with    *expr.Value
	respond *expr.Template
	esc     *escalate
}

// compileSteps compiles a Function's steps. Scripts are loaded relative to
// the file that declared the Function.
func (p *Program) compileSteps(c *compiler, file, ns string, steps []agents.Step) []*step {
	var out []*step
	for i, st := range steps {
		path := fmt.Sprintf("spec.steps[%d]", i)
		s := &step{id: st.ID, when: c.expr(string(st.When), expr.Bool, path+".when"),
			onError:   p.strategies(st.OnError, path+".onError", c),
			milestone: p.milestone(c, ns, st.Milestone, path+".milestone")}
		switch {
		case st.Compute != nil && st.Compute.CEL != "":
			s.kind, s.cel = "cel", c.expr(string(st.Compute.CEL), expr.Any, path+".compute.cel")
		case st.Compute != nil && st.Compute.Starlark != nil:
			sl := st.Compute.Starlark
			s.kind, s.fn = "starlark", sl.Function
			s.with = c.value(sl.With, path+".compute.starlark.with")
			s.script = p.loadScript(c, filepath.Join(filepath.Dir(file), sl.Script), path+".compute.starlark.script")
		case st.Act != "":
			s.kind = "act"
			s.act = &action{
				act:     st.Act,
				spec:    p.spec(st.Act, path+".act", c),
				with:    c.value(st.With, path+".with"),
				confirm: c.template(string(st.Confirm), path+".confirm"),
				key:     c.expr(string(st.IdempotencyKey), expr.String, path+".idempotencyKey"),
			}
		case st.Call != "":
			s.kind, s.fn, s.with = "call", st.Call, c.value(st.With, path+".with")
		case st.DecideBatch != nil:
			s.kind, s.batch = "decideBatch", compileBatch(c, st.DecideBatch, path+".decideBatch")
		case st.Decide != nil:
			s.kind, s.decide = "decide", compileDecide(c, st.Decide, path+".decide")
		case st.Extract != nil:
			s.kind = "generate"
			s.gen = &genStep{state: c.value(st.Extract.State, path+".extract.state"), instructions: st.Extract.Instructions, schema: st.Extract.Schema}
		case st.Generate != nil:
			s.kind = "generate"
			s.gen = &genStep{state: c.value(st.Generate.State, path+".generate.state"), instructions: st.Generate.Instructions, schema: st.Generate.Schema}
		case st.Ask != nil:
			s.kind = "ask"
			s.ask = &ask{
				text:          c.template(string(st.Ask.Text), path+".ask.text"),
				choices:       c.expr(string(st.Ask.Choices), expr.Any, path+".ask.choices"),
				minConfidence: st.Ask.MinConfidence,
				onLow:         st.Ask.OnLowConfidence,
			}
		case st.Respond != nil:
			s.kind, s.respond, s.to = "respond", c.template(string(st.Respond.Template), path+".respond"), st.Respond.To
		case st.Escalate != nil:
			s.kind, s.esc = "escalate", compileEscalate(c, st.Escalate, path+".escalate")
		case st.Experiment != "":
			s.kind, s.exp = "experiment", p.experiment(c, ns, st.Experiment, path+".experiment")
		case st.Retrieve != nil:
			k := st.Retrieve.K
			if k == 0 {
				k = 4
			}
			s.kind, s.retrieve = "retrieve", &retrieveStep{bases: st.Retrieve.Bases, query: c.expr(string(st.Retrieve.Query), expr.String, path+".retrieve.query"), k: k}
		case st.Wait != nil:
			s.kind, s.orReply = "wait", st.Wait.OrReply
			s.includeSideMessages = st.Wait.IncludeSideMessages
			s.relay = st.Wait.Relay
			for i, ev := range st.Wait.Events {
				s.events = append(s.events, waitEvent{name: ev.Name, key: c.expr(string(ev.Key), expr.Any, fmt.Sprintf("%s.wait.events[%d].key", path, i))})
			}
			if st.Wait.For != "" {
				d, err := time.ParseDuration(st.Wait.For)
				if err != nil || d <= 0 {
					c.fail(path+".wait.for", "a positive duration such as 90m or 36h")
				}
				s.waitFor = d
			} else {
				s.until = c.expr(string(st.Wait.Until), expr.Any, path+".wait.until")
			}
		case st.Branch != nil:
			s.kind = "branch"
			for j, bc := range st.Branch {
				bp := fmt.Sprintf("%s.branch[%d]", path, j)
				cc := branchCase{when: c.expr(string(bc.When), expr.Bool, bp+".when"), call: bc.Call}
				switch {
				case bc.Call != "":
					cc.with = c.value(bc.With, bp+".with")
				case bc.Respond != nil:
					cc.respond = c.template(string(bc.Respond.Template), bp+".respond")
				case bc.Escalate != nil:
					cc.esc = compileEscalate(c, bc.Escalate, bp+".escalate")
				}
				s.branch = append(s.branch, cc)
			}
		}
		out = append(out, s)
	}
	return out
}

func compileDecide(c *compiler, d *agents.DecideStep, path string) *decideStep {
	ds := &decideStep{state: c.value(d.State, path+".state"), min: d.MinConfidence, onLow: d.OnLowConfidence}
	q := decide.Question{Instructions: d.Instructions}
	switch {
	case d.Choice != nil:
		q.Kind = decide.KindChoice
		for _, k := range sortedKeys(d.Choice) {
			q.Options = append(q.Options, decide.Option{Key: k, Description: d.Choice[k]})
		}
	case d.YesNo != nil:
		q.Kind, q.IfTrue, q.IfFalse = decide.KindNoul, d.YesNo.IfTrue, d.YesNo.IfFalse
	default:
		q.Kind, q.Levels = decide.KindScore, d.Score
	}
	ds.question = q
	return ds
}

func compileEscalate(c *compiler, e *agents.Escalate, path string) *escalate {
	es := &escalate{queue: e.Queue, task: e.Task, message: c.template(string(e.Message), path+".message")}
	if e.WhileWaiting != nil {
		es.whileWaiting = c.template(string(e.WhileWaiting.Template), path+".whileWaiting")
	}
	if e.OnDeny != nil {
		es.onDeny = c.template(string(e.OnDeny.Template), path+".onDeny")
	}
	for i, r := range e.Route {
		es.route = append(es.route, escalateRoute{when: c.expr(string(r.When), expr.Bool, fmt.Sprintf("%s.route[%d].when", path, i)), queue: r.Queue})
	}
	return es
}

// escalationQueue is the queue a task goes to: the first route whose rule
// holds, else the escalation's queue, else the default. A rule that
// cannot be evaluated does not route.
func (e *execution) escalationQueue(f *frame, es *escalate, sources map[string]any) string {
	for _, r := range es.route {
		if ok, err := r.when.EvalBool(e.ctx, e.activation(f, sources)); err == nil && ok {
			return r.queue
		}
	}
	if es.queue != "" {
		return es.queue
	}
	return e.limits.DefaultQueue
}

// runSteps runs the frame's steps in order. Each result is visible to later
// steps, the action and the output as steps.<id>.
func (e *execution) runSteps(f *frame) error {
	for _, s := range f.fn.steps {
		site := f.prefix + "steps." + s.id
		if s.when != nil {
			ok, err := s.when.EvalBool(e.ctx, e.activation(f, nil))
			if err != nil {
				return e.evalFailure(f, site+".when", err)
			}
			if !ok {
				f.steps[s.id] = map[string]any{"status": "skipped", "output": nil}
				continue
			}
		}
		out, err := e.runStep(f, s, site)
		if err != nil {
			return err
		}
		if s.milestone != nil {
			if err := e.recordMilestone(f, s.milestone, site); err != nil {
				return err
			}
		}
		f.steps[s.id] = map[string]any{"status": "succeeded", "output": out}
	}
	return nil
}

func (e *execution) runStep(f *frame, s *step, site string) (any, error) {
	actv := e.activation(f, nil)
	switch s.kind {
	case "cel":
		v, err := s.cel.Eval(e.ctx, actv)
		if err != nil {
			return nil, e.evalFailure(f, site, err)
		}
		return normalizeOrFail(e, f, site, v)
	case "starlark":
		args, err := s.with.Resolve(e.ctx, actv)
		if err != nil {
			return nil, e.evalFailure(f, site+".with", err)
		}
		m, _ := args.(map[string]any)
		v, err := s.script.Call(e.ctx, s.fn, m, compute.Limits{})
		if err != nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return e.stepFailed(f, s, site, err.Error())
		}
		return normalizeOrFail(e, f, site, v)
	case "act":
		return e.runActStep(f, s, site)
	case "call":
		return e.callFunction(f, s.fn, s.with, site, nil)
	case "decideBatch":
		return e.runBatchStep(f, s, site)
	case "decide":
		return e.runDecideStep(f, s, site)
	case "generate":
		return e.runGenerateStep(f, s, site)
	case "retrieve":
		q, err := s.retrieve.query.Eval(e.ctx, actv)
		if err != nil {
			return nil, e.evalFailure(f, site+".query", err)
		}
		if e.rt.Retrieve == nil {
			return e.stepFailed(f, s, site, "no knowledge bases are served")
		}
		query, _ := q.(string)
		v, err := e.once(site, func() (any, error) {
			ps, err := e.rt.Retrieve(e.ctx, e.caller(), s.retrieve.bases, query, s.retrieve.k)
			if err != nil {
				if e.ctx.Err() != nil {
					return nil, e.ctx.Err()
				}
				return map[string]any{"failed": true, "reason": err.Error()}, nil
			}
			return orEmptyList(ps), nil
		})
		if err != nil {
			return nil, err
		}
		if m, ok := v.(map[string]any); ok && m["failed"] == true {
			return e.stepFailed(f, s, site, fmt.Sprint(m["reason"]))
		}
		return v, nil
	case "ask":
		v, ok, err := e.ask(f, &source{ask: s.ask}, site, nil)
		if err != nil || ok {
			return v, err
		}
		return e.stepFailed(f, s, site, "the user's answer did not match any choice")
	case "respond":
		text, err := e.render(f, s.respond, nil)
		if err != nil {
			return nil, e.evalFailure(f, site, err)
		}
		to := s.to
		if to == "" {
			to = "user"
		}
		var wc *windowClosed
		if err := e.send(site, Message{To: to, Text: text}); errors.As(err, &wc) {
			return e.stepFailed(f, s, site, wc.reason) // onError: a template
		} else if err != nil {
			return nil, err
		}
		return text, nil
	case "escalate":
		return e.escalate(f, site, s.esc, nil)
	case "wait":
		return e.waitWithSideMessages(f, s, site)
	case "experiment":
		return e.expose(f, s.exp, site)
	case "branch":
		for i, bc := range s.branch {
			if bc.when != nil {
				ok, err := bc.when.EvalBool(e.ctx, actv)
				if err != nil {
					return nil, e.evalFailure(f, fmt.Sprintf("%s.branch[%d].when", site, i), err)
				}
				if !ok {
					continue
				}
			}
			cs := fmt.Sprintf("%s/case%d", site, i)
			switch {
			case bc.call != "":
				return e.callFunction(f, bc.call, bc.with, cs, nil)
			case bc.respond != nil:
				text, err := e.render(f, bc.respond, nil)
				if err != nil {
					return nil, e.evalFailure(f, cs, err)
				}
				return text, e.send(cs, Message{To: "user", Text: text})
			default:
				return e.escalate(f, cs, bc.esc, nil)
			}
		}
		return nil, nil // unreachable after admission: a branch ends with else
	}
	return nil, fmt.Errorf("runtime: unknown step kind %q", s.kind)
}

func normalizeOrFail(e *execution, f *frame, site string, v any) (any, error) {
	n, err := journal.Normalize(v)
	if err != nil {
		return nil, e.fail(f, fmt.Sprintf("%s: %v", site, err))
	}
	return n, nil
}

func (e *execution) runActStep(f *frame, s *step, site string) (any, error) {
	a := s.act
	actv := e.activation(f, nil)
	args, err := a.with.Resolve(e.ctx, actv)
	if err != nil {
		return nil, e.evalFailure(f, site+".with", err)
	}
	c := call{site: site, act: a.act, spec: a.spec}
	c.args, _ = args.(map[string]any)
	if a.key != nil {
		k, err := a.key.Eval(e.ctx, actv)
		if err != nil {
			return nil, e.evalFailure(f, site+".idempotencyKey", err)
		}
		c.key, _ = k.(string)
	}
	if a.confirm != nil {
		if err := e.confirm(f, a.confirm, site+"/confirm", site+".confirm"); err != nil {
			return nil, err
		}
	}
	out, err := e.effect(c)
	if err == nil {
		return out, nil
	}
	ce, ok := err.(*callError)
	if !ok {
		return nil, err
	}
	return e.onError(f, s.onError, c, ce)
}

// stepFailed runs the step's onError strategies that apply to a failure
// that is not a plugin call (fallback, escalate, respond, fail).
func (e *execution) stepFailed(f *frame, s *step, site, reason string) (any, error) {
	for i, st := range s.onError {
		ss := fmt.Sprintf("%s/onError[%d]", site, i)
		switch st.kind {
		case stFallback:
			out, _, err := e.fallback(f, ss, st.fallback)
			if _, failed := err.(*callError); failed {
				continue
			}
			return out, err
		case stEscalate:
			return e.escalate(f, ss, st.escalate, nil)
		case stRespond:
			return nil, e.respond(f, ss, st.respond, nil)
		case stFail:
			return nil, e.fail(f, st.message)
		}
	}
	return nil, e.fail(f, site+": "+reason)
}

// runDecideStep asks one question once and applies onLowConfidence.
func (e *execution) runDecideStep(f *frame, s *step, site string) (any, error) {
	d := s.decide
	state, err := d.state.Resolve(e.ctx, e.activation(f, nil))
	if err != nil {
		return nil, e.evalFailure(f, site+".state", err)
	}
	v, err := e.once(site, func() (any, error) {
		resp, err := e.rt.Decider.Decide(e.ctx, decide.Request{
			State:     e.rt.Masker.Mask(state),
			Questions: map[string]decide.Question{"q": d.question},
		})
		if err != nil || resp == nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return map[string]any{"value": nil, "probability": 0.0, "error": "decide failed"}, nil
		}
		a, ok := resp.Answers["q"]
		if !ok || !validDecisionAnswer(d.question, a) {
			return map[string]any{"error": "decision answer missing or malformed"}, nil
		}
		return answerValue(d.question, a, resp.Model), nil
	})
	if err != nil {
		return nil, err
	}
	out, _ := v.(map[string]any)
	if reason, ok := out["error"].(string); ok {
		return e.stepFailed(f, s, site, reason)
	}
	if p, _ := out["probability"].(float64); p >= d.min {
		return out, nil
	}
	switch d.onLow {
	case "continue":
		low := map[string]any{"value": nil, "probability": out["probability"], "probabilities": out["probabilities"]}
		return low, nil
	case "human":
		opts := map[string]string{}
		for _, o := range humanOptions(d.question) {
			opts[o[0]] = o[1]
		}
		_, hv, err := e.waitFor(site+"/human", journal.StatusWaitingForHuman, Message{
			To: "queue:" + e.limits.DefaultQueue, Task: "answer", Text: d.question.Instructions, Options: opts,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"value": hv, "probability": 1.0, "decidedBy": "human"}, nil
	}
	return e.stepFailed(f, s, site, "the decision was below its confidence threshold")
}

// answerValue shapes a Decide answer as a step output.
func answerValue(q decide.Question, a decide.Answer, model string) map[string]any {
	probs := map[string]any{}
	for k, p := range a.Probabilities {
		probs[k] = p
	}
	out := map[string]any{"probabilities": probs, "model": model}
	switch q.Kind {
	case decide.KindChoice:
		out["value"], out["probability"] = a.Choice, a.Probabilities[a.Choice]
	case decide.KindNoul:
		yes := a.Noul >= 0.5
		out["value"] = yes
		if yes {
			out["probability"] = a.Noul
		} else {
			out["probability"] = 1 - a.Noul
		}
	case decide.KindScore:
		best, bestP := "", -1.0
		keys := make([]string, 0, len(a.Probabilities))
		for k := range a.Probabilities {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if p := a.Probabilities[k]; p > bestP {
				best, bestP = k, p
			}
		}
		idx := 0
		_, _ = fmt.Sscanf(best, "%d", &idx)
		if idx >= 0 && idx < len(q.Levels) {
			out["value"] = q.Levels[idx]
		}
		out["probability"] = bestP
	}
	return out
}

// humanOptions lists what a person can answer to a question: key, label.
func humanOptions(q decide.Question) [][2]string {
	switch q.Kind {
	case decide.KindChoice:
		out := make([][2]string, 0, len(q.Options))
		for _, o := range q.Options {
			out = append(out, [2]string{o.Key, o.Description})
		}
		return out
	case decide.KindNoul:
		return [][2]string{{"true", "yes " + q.IfTrue}, {"false", "no " + q.IfFalse}}
	}
	out := make([][2]string, 0, len(q.Levels))
	for _, l := range q.Levels {
		out = append(out, [2]string{l, l})
	}
	return out
}

// runGenerateStep asks the Generate engine once per site. A reply that is
// refused (invalid after the repair) fails the step, and its onError chain
// decides what happens; the run never continues with an unvalidated value.
func (e *execution) runGenerateStep(f *frame, s *step, site string) (any, error) {
	g := s.gen
	state, err := g.state.Resolve(e.ctx, e.activation(f, nil))
	if err != nil {
		return nil, e.evalFailure(f, site+".state", err)
	}
	if e.rt.Generator == nil {
		return e.stepFailed(f, s, site, "no Generate backend is configured")
	}
	v, err := e.once(site, func() (any, error) {
		resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{
			Instructions: g.instructions, State: e.rt.Masker.Mask(state), Schema: g.schema,
		})
		if err != nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			// Recorded, so replay agrees; the reason has no model output.
			return map[string]any{"failed": true, "reason": generateFailure(err)}, nil
		}
		if g.schema != nil {
			return map[string]any{"value": resp.Value, "model": resp.Model}, nil
		}
		return map[string]any{"value": resp.Text, "model": resp.Model}, nil
	})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	if failed, _ := m["failed"].(bool); failed {
		reason, _ := m["reason"].(string)
		return e.stepFailed(f, s, site, reason)
	}
	return m["value"], nil
}

func generateFailure(err error) string {
	switch {
	case errors.Is(err, generate.ErrInvalidOutput):
		return "the model's reply did not match the schema"
	case errors.Is(err, ledger.ErrBudgetExceeded):
		return "the model budget is spent"
	}
	return "the Generate backend failed"
}

func orEmptyList(xs []any) []any {
	if xs == nil {
		return []any{}
	}
	return xs
}
