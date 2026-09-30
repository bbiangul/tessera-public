package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// execution is one pass over a run's journal.
type execution struct {
	rt     *Runtime
	ctx    context.Context
	run    journal.Run
	recs   map[string]journal.Record // site + "|" + kind
	scopes map[string]any            // caller, conversation, event
	// limits are the run's own, recorded when it started, so a changed
	// setting cannot change the path of a run already under way.
	limits Limits
	// facts are the values each rendered text interpolated, by text: what
	// the voice must keep word for word.
	facts map[string][]string
	// explain is a definite failure whose system words the next message to
	// the user passes on (explain.go).
	explain *failureToExplain
}

// frame is one Function being executed; fromFunction calls nest frames.
type frame struct {
	fn     *function
	prefix string // site prefix, "" at the top
	inputs map[string]any
	steps  map[string]any // id -> {status, output}
	depth  int
}

// Control flow. A suspension or a finish unwinds the whole execution.
type suspended struct {
	status journal.Status
	site   string
	wake   time.Time // WaitingForTime only
}

func (s *suspended) Error() string { return "suspended at " + s.site }

type finished struct {
	status journal.Status
	reason string
	// restart is the user's message when they corrected a confirmation:
	// the Function starts again from it.
	restart string
}

func (f *finished) Error() string { return string(f.status) + ": " + f.reason }

// callError is a recorded failure of a plugin call.
type callError struct {
	Definite bool   `json:"definite"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	// Detail is what the system said, masked unless Public (drift-002).
	Detail string `json:"detail,omitempty"`
	Public bool   `json:"public,omitempty"`
	// Action is the plugin/action that failed, for the drift report.
	Action string `json:"action,omitempty"`
}

func (c *callError) Error() string {
	if c.Definite {
		return fmt.Sprintf("%s: %s", c.Code, c.Message)
	}
	return "outcome unknown: " + c.Message
}

func (r *Runtime) execute(ctx context.Context, run journal.Run) (Result, error) {
	if run.Generation != r.Program.Generation() {
		return Result{}, fmt.Errorf("%w: run %s has %s, this runtime %s", ErrGeneration, run.ID, run.Generation, r.Program.Generation())
	}
	recs, err := r.Store.Records(ctx, run.ID)
	if err != nil {
		return Result{}, err
	}
	sc, err := journal.Decode(run.Scopes)
	if err != nil {
		return Result{}, fmt.Errorf("run %s scopes: %w", run.ID, err)
	}
	e := &execution{rt: r, ctx: ctx, run: run, recs: map[string]journal.Record{}, scopes: sc.(map[string]any), limits: r.Limits}
	if l, ok := e.scopes["limits"].(map[string]any); ok {
		e.limits = limitsFrom(l, r.Limits)
	}
	for _, rec := range recs {
		e.recs[rec.Site+"|"+string(rec.Kind)] = rec
	}
	fn := r.Program.functions[run.Namespace+"/"+run.Function]
	if fn == nil {
		return Result{}, fmt.Errorf("%w: %s/%s", ErrUnknownFunction, run.Namespace, run.Function)
	}
	given := map[string]any{}
	if g, ok := e.scopes["given"].(map[string]any); ok {
		for k, v := range g {
			if v != nil {
				given[k] = v
			}
		}
	}
	_, err = e.runFunction(&frame{fn: fn, inputs: given})

	res := Result{RunID: run.ID}
	outbox, perr := e.pending(err)
	if perr != nil {
		return Result{}, perr
	}
	res.Outbox = outbox
	var s *suspended
	var f *finished
	var wc *windowClosed
	switch {
	case errors.As(err, &s):
		res.Status, res.WaitingFor = s.status, waitingFor(s.status)
		if !s.wake.IsZero() {
			res.WakeAt = s.wake
			return res, r.Store.Sleep(ctx, run.ID, s.status, s.site, s.wake)
		}
		return res, r.Store.UpdateRun(ctx, run.ID, s.status, s.site)
	case errors.As(err, &wc):
		// Free text would have reached the user outside the channel's
		// window and nothing handled it: the run fails, silently to the
		// user (the team sees why in the inspector).
		res.Status, res.Error = journal.StatusFailed, wc.reason
		return res, r.finish(ctx, run.ID, journal.StatusFailed)
	case errors.As(err, &f):
		res.Status, res.Restart = f.status, f.restart
		if f.status == journal.StatusFailed {
			res.Error = f.reason
		}
		return res, r.finish(ctx, run.ID, f.status)
	case err == nil:
		return Result{}, errors.New("runtime: a run ended without finishing")
	default:
		// Store failures and cancellations: nothing is lost, the run
		// continues from its journal on the next Resume.
		return Result{}, err
	}
}

func (e *execution) get(site string, kind journal.Kind) (any, bool, error) {
	rec, ok := e.recs[site+"|"+string(kind)]
	if !ok {
		return nil, false, nil
	}
	v, err := journal.Decode(rec.Value)
	return v, true, err
}

// put records v and returns it as later executions will see it.
func (e *execution) put(site string, kind journal.Kind, v any) (any, error) {
	data, err := journal.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", site, err)
	}
	rec := journal.Record{Site: site, Kind: kind, Value: data, At: e.rt.Clock()}
	if rec.Seq, err = e.rt.Store.Append(e.ctx, e.run.ID, rec); err != nil {
		return nil, err
	}
	e.recs[site+"|"+string(kind)] = rec
	return journal.Decode(data)
}

// once runs f the first time the site is reached and returns the recorded
// outcome afterwards. Plugin failures (act errors) are recorded; any other
// error is returned without a record, so it is retried on resume.
func (e *execution) once(site string, f func() (any, error)) (any, error) {
	if v, ok, err := e.get(site, journal.KindResult); ok || err != nil {
		return v, err
	}
	if v, ok, err := e.get(site, journal.KindError); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return nil, decodeCallError(v)
	}
	v, err := f()
	if err != nil {
		ce := classify(err)
		if ce == nil {
			return nil, err
		}
		if ce.Detail != "" && !ce.Public {
			ce.Detail, _ = e.rt.Mask(ce.Detail).(string)
		}
		if _, perr := e.put(site, journal.KindError, ce.record()); perr != nil {
			return nil, perr
		}
		return nil, ce
	}
	out, perr := e.put(site, journal.KindResult, v)
	if errors.Is(perr, journal.ErrUnsupported) {
		// A plugin in Go may return typed values ([]string, structs). Their
		// JSON form is what callers can use; if even that fails, the value
		// is recorded as unusable instead of wedging the run on every
		// resume (for an effect, the call still happened).
		if plain, jerr := viaJSON(v); jerr == nil {
			return e.put(site, journal.KindResult, plain)
		}
		ce := &callError{Definite: true, Code: "UNSUPPORTED_OUTPUT", Message: "the output cannot be recorded"}
		if _, rerr := e.put(site, journal.KindError, ce.record()); rerr != nil {
			return nil, rerr
		}
		return nil, ce
	}
	return out, perr
}

// viaJSON converts any JSON-encodable value to plain values (int64,
// float64, string, bool, nil, []any, map[string]any).
func viaJSON(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var out any
	if err := d.Decode(&out); err != nil {
		return nil, err
	}
	return journal.Normalize(numbersOf(out))
}

func numbersOf(v any) any {
	switch x := v.(type) {
	case json.Number:
		return expr.Number(x)
	case []any:
		for i := range x {
			x[i] = numbersOf(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = numbersOf(x[k])
		}
	}
	return v
}

func classify(err error) *callError {
	var d *act.DefiniteError
	var a *act.AmbiguousError
	var c *callError
	switch {
	case errors.As(err, &c):
		return c
	case errors.As(err, &d):
		return &callError{Definite: true, Code: d.Code, Message: d.Message, Detail: d.Detail, Public: d.Public}
	case errors.As(err, &a):
		return &callError{Message: a.Message}
	case errors.Is(err, context.Canceled):
		return nil // our own cancellation: not the plugin's outcome
	}
	// Unclassified plugin errors are ambiguous. Their text may carry data,
	// so only the type is kept.
	return &callError{Message: fmt.Sprintf("unclassified error (%T)", err)}
}

func (c *callError) record() map[string]any {
	m := map[string]any{"definite": c.Definite, "code": c.Code, "message": c.Message}
	if c.Action != "" {
		m["action"] = c.Action
	}
	if c.Detail != "" {
		m["detail"], m["public"] = c.Detail, c.Public
	}
	return m
}

func decodeCallError(v any) *callError {
	m, _ := v.(map[string]any)
	d, _ := m["definite"].(bool)
	code, _ := m["code"].(string)
	msg, _ := m["message"].(string)
	detail, _ := m["detail"].(string)
	public, _ := m["public"].(bool)
	action, _ := m["action"].(string)
	return &callError{Definite: d, Code: code, Message: msg, Detail: detail, Public: public, Action: action}
}

// send queues a message once per site.
func (e *execution) send(site string, m Message) error {
	if _, ok, err := e.get(site, journal.KindSend); ok || err != nil {
		if m.To == "user" {
			e.explain = nil // replayed: the explanation, if any, went with it
		}
		return err
	}
	if m.To == "user" {
		if err := e.userWindow(site, e.replyWindow()); err != nil {
			return err
		}
		text, err := e.voiced(site, m.Text, false)
		if err != nil {
			return err
		}
		m.Text = e.explained(text)
		if err := e.approveDraft(site, m.Text); err != nil {
			return err
		}
	}
	_, err := e.put(site, journal.KindSend, messageValue(m))
	return err
}

// waitFor returns the reply recorded for site, or sends m once and
// suspends the run.
func (e *execution) waitFor(site string, status journal.Status, m Message) (text string, value any, err error) {
	if v, ok, err := e.get(site, journal.KindResume); err != nil {
		return "", nil, err
	} else if ok {
		rm, _ := v.(map[string]any)
		text, _ = rm["text"].(string)
		return text, rm["value"], nil
	}
	if _, ok, err := e.get(site, journal.KindSuspend); err != nil {
		return "", nil, err
	} else if !ok {
		if m.To == "user" {
			if err := e.userWindow(site, e.replyWindow()); err != nil {
				return "", nil, err
			}
			text, err := e.voiced(site, m.Text, status == journal.StatusConfirming)
			if err != nil {
				return "", nil, err
			}
			m.Text = text
			if err := e.approveDraft(site, m.Text); err != nil {
				return "", nil, err
			}
		}
		if _, err := e.put(site, journal.KindSuspend, messageValue(m)); err != nil {
			return "", nil, err
		}
	}
	if (status == journal.StatusWaitingForHuman || status == journal.StatusIndeterminate) && strings.HasPrefix(m.To, "queue:") {
		return e.humanWait(site, status, m)
	}
	return "", nil, &suspended{status: status, site: site}
}

// pendingMessages returns a run's unacknowledged messages without
// executing it.
func (r *Runtime) pendingMessages(ctx context.Context, run journal.Run) ([]Message, error) {
	recs, err := r.Store.Records(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	e := &execution{rt: r, ctx: ctx, run: run, recs: map[string]journal.Record{}}
	for _, rec := range recs {
		e.recs[rec.Site+"|"+string(rec.Kind)] = rec
	}
	return e.pending(nil)
}

// pending returns the messages not acknowledged yet, in record order. After
// an execution that failed (a store error, not a finish or a suspension) it
// returns none: they wait for the next execution that completes.
func (e *execution) pending(execErr error) ([]Message, error) {
	var s *suspended
	var f *finished
	if execErr != nil && !errors.As(execErr, &s) && !errors.As(execErr, &f) {
		return nil, nil
	}
	var recs []journal.Record
	for _, rec := range e.recs {
		if (rec.Kind == journal.KindSend || rec.Kind == journal.KindSuspend) && rec.Seq > e.run.Acked {
			recs = append(recs, rec)
		}
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Seq < recs[j].Seq })
	out := make([]Message, 0, len(recs))
	for _, rec := range recs {
		v, err := journal.Decode(rec.Value)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		msg := Message{ID: e.run.ID + "/" + rec.Site, Seq: rec.Seq}
		msg.To, _ = m["to"].(string)
		msg.Text, _ = m["text"].(string)
		msg.Task, _ = m["task"].(string)
		if opts, ok := m["options"].(map[string]any); ok {
			msg.Options = map[string]string{}
			for k, x := range opts {
				msg.Options[k], _ = x.(string)
			}
		}
		out = append(out, msg)
	}
	return out, nil
}

func messageValue(m Message) map[string]any {
	v := map[string]any{"to": m.To, "text": m.Text}
	if m.Task != "" {
		v["task"] = m.Task
	}
	if len(m.Options) > 0 {
		opts := map[string]any{}
		for k, x := range m.Options {
			opts[k] = x
		}
		v["options"] = opts
	}
	return v
}

func (e *execution) activation(f *frame, sources map[string]any) expr.Activation {
	return expr.Activation{
		"inputs":       f.inputs,
		"sources":      sources,
		"steps":        f.steps,
		"event":        e.scopes["event"],
		"caller":       e.scopes["caller"],
		"conversation": e.scopes["conversation"],
	}
}

// voice and msgs are the run's audience's voice and messages.
func (e *execution) voice() *Voice  { return e.rt.VoiceFor(e.caller()) }
func (e *execution) msgs() Messages { return e.rt.MessagesFor(e.caller()) }

func (e *execution) caller() map[string]any {
	m, _ := e.scopes["caller"].(map[string]any)
	return m
}

// fail ends the run as Failed, telling the user once. messages.failed
// promises a person, so with a default queue the run first hands the case
// to it as a takeover task and waits until someone takes it; it then ends
// Failed. Without a queue there is nobody to hand it to (#52, drift-004).
func (e *execution) fail(f *frame, reason string) error {
	// The reason is journaled (masked), with or without a queue to tell:
	// runs show and the review read it (#67).
	if _, ok, err := e.get(f.prefix+"failed/reason", journal.KindResult); err != nil {
		return err
	} else if !ok {
		if _, err := e.put(f.prefix+"failed/reason", journal.KindResult, map[string]any{"reason": e.rt.Masker.Mask(reason)}); err != nil {
			return err
		}
	}
	var wc *windowClosed
	told := true
	if err := e.send(f.prefix+"failed", Message{To: "user", Text: e.msgs().Failed}); err != nil {
		if !errors.As(err, &wc) {
			return err
		}
		told = false // outside the window or without consent: nothing was promised
	}
	if q := e.limits.FailureQueue; q != "" && told {
		text := "The process failed and the user was told a person will follow up in this conversation. Reason: " + reason
		if _, _, err := e.waitFor(f.prefix+"failed/handover", journal.StatusWaitingForHuman, Message{To: "queue:" + q, Task: "takeover", Text: text}); err != nil {
			return err
		}
		return &finished{status: journal.StatusFailed, reason: reason + " (a person took it over)"}
	}
	return &finished{status: journal.StatusFailed, reason: reason}
}

// evalFailure turns an expression error into a run failure. Expression
// errors carry no data (expr.ErrEval), so the reason is safe to keep.
func (e *execution) evalFailure(f *frame, path string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return e.fail(f, fmt.Sprintf("%s: %v", path, err))
}

func (e *execution) render(f *frame, t *expr.Template, sources map[string]any) (string, error) {
	if t == nil {
		return "", nil
	}
	text, facts, err := t.RenderFacts(e.ctx, e.activation(f, sources), e.rt.Escaper)
	if err == nil && len(facts) > 0 {
		if e.facts == nil {
			e.facts = map[string][]string{}
		}
		e.facts[text] = facts
	}
	return text, err
}

// runFunction executes a Function. At the top it ends the run (a *finished
// error); nested, it returns the function's value: the action's output when
// it has an action, otherwise its rendered output.
func (e *execution) runFunction(f *frame) (any, error) {
	fn := f.fn
	if f.steps == nil {
		f.steps = map[string]any{}
	}
	if fn.authorize != nil {
		ok, err := fn.authorize.EvalBool(e.ctx, e.activation(f, nil))
		if err != nil {
			return nil, e.evalFailure(f, "spec.authorize", err)
		}
		if !ok {
			if err := e.send(f.prefix+"authorize", Message{To: "user", Text: e.msgs().NotAllowed}); err != nil {
				return nil, err
			}
			return nil, &finished{status: journal.StatusFailed, reason: "not authorized"}
		}
	}
	extracted, err := e.prefetchExtracts(f)
	if err != nil {
		return nil, err
	}
	for _, in := range fn.inputs {
		if v, preset := f.inputs[in.name]; preset && v != nil {
			continue
		}
		v, err := e.resolveInput(f, in, extracted)
		if err != nil {
			return nil, err
		}
		f.inputs[in.name] = v
	}
	for i, pc := range fn.preconditions {
		ok, err := pc.cel.EvalBool(e.ctx, e.activation(f, nil))
		if err != nil {
			return nil, e.evalFailure(f, fmt.Sprintf("spec.preconditions[%d]", i), err)
		}
		if !ok {
			msg, err := e.render(f, pc.message, nil)
			if err != nil {
				return nil, e.evalFailure(f, fmt.Sprintf("spec.preconditions[%d].message", i), err)
			}
			if err := e.send(fmt.Sprintf("%spreconditions[%d]", f.prefix, i), Message{To: "user", Text: msg}); err != nil {
				return nil, err
			}
			return nil, &finished{status: journal.StatusFailed, reason: fmt.Sprintf("precondition %d is false", i)}
		}
	}
	if err := e.runSteps(f); err != nil {
		return nil, err
	}
	var out any
	if fn.action != nil {
		var err error
		if out, err = e.runAction(f); err != nil {
			return nil, err
		}
		if fn.actionMilestone != nil {
			if err := e.recordMilestone(f, fn.actionMilestone, f.prefix+"action"); err != nil {
				return nil, err
			}
		}
	}
	text, err := e.render(f, fn.output, nil)
	if err != nil {
		return nil, e.evalFailure(f, "spec.output", err)
	}
	if f.depth > 0 {
		if fn.outputValue != nil {
			v, err := fn.outputValue.Eval(e.ctx, e.activation(f, nil))
			if err != nil {
				return nil, e.evalFailure(f, "spec.output.value", err)
			}
			return normalizeOrFail(e, f, f.prefix+"output.value", v)
		}
		if fn.action != nil {
			return out, nil
		}
		return text, nil
	}
	if text != "" {
		if err := e.send("output", Message{To: "user", Text: text}); err != nil {
			return nil, err
		}
	}
	return nil, &finished{status: journal.StatusSucceeded}
}

// callFunction runs another Function nested in this run. Its with values
// become its inputs.
func (e *execution) callFunction(f *frame, name string, with *expr.Value, site string, sources map[string]any) (any, error) {
	if f.depth+1 > e.limits.MaxCallDepth {
		return nil, e.fail(f, fmt.Sprintf("%s: calls nested deeper than %d", site, e.limits.MaxCallDepth))
	}
	target := e.rt.Program.functions[e.run.Namespace+"/"+name]
	if target == nil {
		return nil, e.fail(f, fmt.Sprintf("%s: unknown function %s", site, name))
	}
	args := map[string]any{}
	if with != nil {
		v, err := with.Resolve(e.ctx, e.activation(f, sources))
		if err != nil {
			return nil, e.evalFailure(f, site+".with", err)
		}
		args, _ = v.(map[string]any)
	}
	norm, err := journal.Normalize(args)
	if err != nil {
		return nil, e.fail(f, fmt.Sprintf("%s: %v", site, err))
	}
	// A null argument gives nothing: the callee resolves that input from
	// its own sources (and may ask the user for it).
	given := map[string]any{}
	for k, v := range norm.(map[string]any) {
		if v == nil {
			continue
		}
		for _, in := range target.inputs {
			if in.name == k && !conforms(in.typ, v) {
				return nil, e.fail(f, fmt.Sprintf("%s: %s gets %s, the input is %s", site, k, describe(v), in.typ))
			}
		}
		given[k] = v
	}
	return e.runFunction(&frame{fn: target, prefix: site + "/", inputs: given, depth: f.depth + 1})
}

// sleepUntil waits for a recorded deadline, so a resumed run does not wait
// the whole backoff again.
func (e *execution) sleepUntil(site string, d time.Duration) error {
	v, err := e.once(site, func() (any, error) { return e.rt.Clock().Add(d), nil })
	if err != nil {
		return err
	}
	deadline, _ := v.(time.Time)
	if rest := deadline.Sub(e.rt.Clock()); rest > 0 {
		return e.rt.Sleep(e.ctx, rest)
	}
	return nil
}

// finish records a run's final status and forgets its event watches.
func (r *Runtime) finish(ctx context.Context, runID string, status journal.Status) error {
	if err := r.Store.UpdateRun(ctx, runID, status, ""); err != nil {
		return err
	}
	return r.Store.Unwatch(ctx, runID)
}
