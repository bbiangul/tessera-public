package runtime

import (
	"fmt"
	"strings"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// call is one effectful call, kept so onError can repeat or verify it.
type call struct {
	site string
	act  string
	spec plugins.ActionSpec
	args map[string]any
	key  string
	// extra are body fields a repair adds (repair.go).
	extra map[string]any
}

func irreversible(effect string) bool {
	return effect == plugins.EffectIrreversible || effect == plugins.EffectExternalMessage
}

// runAction confirms if asked, performs the action and handles failures.
func (e *execution) runAction(f *frame) (any, error) {
	a := f.fn.action
	actv := e.activation(f, nil)
	args, err := a.with.Resolve(e.ctx, actv)
	if err != nil {
		return nil, e.evalFailure(f, "spec.action.with", err)
	}
	c := call{site: f.prefix + "action", act: a.act, spec: a.spec}
	c.args, _ = args.(map[string]any)
	if a.key != nil {
		k, err := a.key.Eval(e.ctx, actv)
		if err != nil {
			return nil, e.evalFailure(f, "spec.action.idempotencyKey", err)
		}
		c.key, _ = k.(string)
	}
	if a.confirm != nil {
		if err := e.confirm(f, a.confirm, f.prefix+"confirm", "spec.action.confirm"); err != nil {
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
	return e.onError(f, f.fn.onError, c, ce)
}

// confirm asks the user and continues only on a clear yes (RFC-0003,
// "confirm").
func (e *execution) confirm(f *frame, t *expr.Template, base, path string) error {
	text, err := e.render(f, t, nil)
	if err != nil {
		return e.evalFailure(f, path, err)
	}
	for round := 1; round <= 2; round++ {
		site := fmt.Sprintf("%s%d", base, round)
		reply, _, err := e.waitFor(site, journal.StatusConfirming, Message{To: "user", Text: text})
		if err != nil {
			return err
		}
		p, err := e.decideYes(site+"/decide", "Does the reply say yes to the question?",
			map[string]any{"question": text, "reply": reply})
		if err != nil {
			return err
		}
		switch {
		case p >= e.limits.ConfirmYes:
			return nil
		case p <= e.limits.ConfirmNo:
			// A no can carry a correction ("mudei de ideia, prefiro
			// terça"): the run ends without acting and starts again from
			// the reply, instead of telling the user nothing changed.
			c, err := e.decideYes(site+"/correction", "Does the reply ask for something different instead (another date, time or option), rather than only saying no?",
				map[string]any{"question": text, "reply": reply})
			if err != nil {
				return err
			}
			if c >= e.limits.ConfirmYes {
				return &finished{status: journal.StatusSucceeded, reason: "the user corrected the confirmation", restart: reply}
			}
			return e.cancelled(base)
		}
	}
	for round := 1; round <= maxHumanRounds; round++ {
		site := base + "/human"
		if round > 1 {
			site = fmt.Sprintf("%s/human/%d", base, round)
		}
		answer, v, err := e.waitFor(site, journal.StatusWaitingForHuman, Message{
			To: "queue:" + e.limits.DefaultQueue, Task: "approve",
			Text: "The user's answer to this confirmation is unclear: " + text + " Approve? Answer yes or no.",
		})
		if err != nil {
			return err
		}
		if approved, ok := readApproval(answer, v); ok {
			if approved {
				return nil
			}
			return e.cancelled(base)
		}
	}
	return e.cancelled(base) // never act on an unclear approval
}

func (e *execution) cancelled(base string) error {
	if err := e.send(base+"/cancelled", Message{To: "user", Text: e.msgs().Cancelled}); err != nil {
		return err
	}
	return &finished{status: journal.StatusSucceeded, reason: "cancelled by the user"}
}

// effect performs one call at c.site. Irreversible calls write an intent
// first; an intent with no outcome means the process stopped during the
// call, which is recorded as an ambiguous failure.
func (e *execution) effect(c call) (any, error) {
	if err := e.approveEffect(c); err != nil {
		return nil, err
	}
	do := func() (any, error) { return e.callActionExtra(c.act, c.args, c.key, c.extra) }
	if !irreversible(c.spec.Effect) {
		return e.once(c.site, do)
	}
	return e.once(c.site, func() (any, error) {
		if _, started, err := e.get(c.site, journal.KindIntent); err != nil {
			return nil, err
		} else if started {
			return nil, &callError{Message: "the process stopped during the call"}
		}
		if _, err := e.put(c.site, journal.KindIntent, map[string]any{"action": c.act, "key": c.key, "args": c.args}); err != nil {
			return nil, err
		}
		return do()
	})
}

// onError runs strategies in order (RFC-0003, "onError").
//
// Two rules hold whatever the chain says: an effect whose outcome is
// unknown is verified first when its plugin can verify it, and it reaches
// a person before the run ends (never a plain respond or fail). After a
// fallback act, later strategies act on the fallback's call.
func (e *execution) onError(f *frame, chain []strategy, c call, last *callError) (any, error) {
	cur := c
	unsafe := func() bool { return !last.Definite && irreversible(cur.spec.Effect) }
	verifyNow := func(site string) (any, bool, error) {
		out, v, err := e.verify(site, cur)
		if err != nil {
			return nil, false, err
		}
		switch v {
		case yes:
			return out, true, nil
		case no:
			last = &callError{Definite: true, Code: "NOT_HAPPENED", Message: "verification shows the effect did not happen"}
		}
		return nil, false, nil
	}
	if unsafe() && verifiable(cur.spec) && !hasProbe(chain) {
		if out, done, err := verifyNow(cur.site + "/verify"); err != nil || done {
			return out, err
		}
	}
	if !unsafe() {
		// The system said what it needs (RFC-0005 step 3): try the
		// repaired call before the author's chain.
		out, done, next, err := e.repair(f, cur, last)
		if err != nil || done {
			return out, err
		}
		last = next
	}
	for i, st := range chain {
		site := fmt.Sprintf("%s/onError[%d]", c.site, i)
		failure := map[string]any{"error": map[string]any{"code": last.Code, "message": last.Message, "definite": last.Definite}}
		if st.when != nil {
			// A condition that cannot be evaluated skips the strategy: the
			// rules below still decide what an unknown outcome gets.
			ok, err := st.when.Eval(e.ctx, e.activation(f, failure))
			if err != nil || ok != true {
				continue
			}
		}
		switch st.kind {
		case stProbe:
			if last.Definite {
				continue
			}
			if out, done, err := verifyNow(site); err != nil || done {
				return out, err
			}
		case stRetry:
			if unsafe() {
				continue // never repeat an effect that may have happened
			}
			backoff, _ := time.ParseDuration(st.backoff)
			for n := 1; n <= st.attempts; n++ {
				if backoff > 0 {
					if err := e.sleepUntil(fmt.Sprintf("%s/wait%d", site, n), backoff); err != nil {
						return nil, err
					}
				}
				rc := cur
				rc.site = fmt.Sprintf("%s/attempt%d", site, n)
				out, err := e.effect(rc)
				if err == nil {
					return out, nil
				}
				ce, ok := err.(*callError)
				if !ok {
					return nil, err
				}
				last = ce
				if unsafe() {
					// The retry itself may have happened: it is now the
					// call to verify or hand over.
					cur = rc
					break
				}
			}
		case stFallback:
			if unsafe() {
				continue // the original may have happened; a fallback could double it
			}
			out, fc, err := e.fallback(f, site, st.fallback)
			if err == nil {
				return out, nil
			}
			ce, ok := err.(*callError)
			if !ok {
				return nil, err
			}
			last = ce
			if fc != nil {
				cur = *fc
			}
		case stEscalate:
			e.toExplain(cur, last)
			return e.escalateEffect(f, site, st.escalate, cur)
		case stRespond:
			if unsafe() {
				return e.indeterminate(f, cur)
			}
			e.toExplain(cur, last)
			return nil, e.respond(f, site, st.respond, failure)
		case stFail:
			if unsafe() {
				return e.indeterminate(f, cur)
			}
			e.toExplain(cur, last)
			return nil, e.fail(f, st.message)
		}
	}
	if unsafe() {
		return e.indeterminate(f, cur)
	}
	e.toExplain(cur, last)
	return nil, e.fail(f, fmt.Sprintf("%s failed: %v", cur.act, last))
}

func verifiable(spec plugins.ActionSpec) bool {
	return spec.Verify != nil && (spec.Verify.Mode == plugins.VerifyNative || spec.Verify.Mode == plugins.VerifyProbe)
}

func hasProbe(chain []strategy) bool {
	for _, st := range chain {
		if st.kind == stProbe {
			return true
		}
	}
	return false
}

type verdict int

const (
	unknown verdict = iota
	yes
	no
)

// verify finds out whether an ambiguous call happened, using the action's
// declared verification.
func (e *execution) verify(site string, c call) (any, verdict, error) {
	mode := plugins.VerifyNone
	if c.spec.Verify != nil {
		mode = c.spec.Verify.Mode
	}
	switch mode {
	case plugins.VerifyNative:
		// Same key: the external system returns the original outcome
		// instead of acting twice.
		rc := c
		rc.site = site + "/native"
		out, err := e.effect(rc)
		if err == nil {
			return out, yes, nil
		}
		if ce, ok := err.(*callError); ok && ce.Definite {
			return nil, no, nil
		}
		if _, ok := err.(*callError); ok {
			return nil, unknown, nil
		}
		return nil, unknown, err
	case plugins.VerifyProbe:
		return e.probe(site, c)
	}
	return nil, unknown, nil
}

// probe calls the manifest's probe action and evaluates its happened and
// output expressions over sources.probe.output, with this action's
// arguments as inputs. A probe that fails or cannot be evaluated proves
// nothing: the verdict is unknown.
func (e *execution) probe(site string, c call) (any, verdict, error) {
	g := e.rt.Program.guards[c.act]
	plugin, _, _ := strings.Cut(c.act, "/")
	probeAct := plugin + "/" + c.spec.Verify.With
	actv := expr.Activation{"inputs": c.args, "caller": e.scopes["caller"], "event": e.scopes["event"], "conversation": e.scopes["conversation"]}
	args, err := g.probeArgs.Resolve(e.ctx, actv)
	if err != nil {
		return nil, unknown, nil
	}
	m, _ := args.(map[string]any)
	out, err := e.once(site+"/probe", func() (any, error) { return e.readChecked(site+"/probe", probeAct, m) })
	if err != nil {
		if _, ok := err.(*callError); ok {
			return nil, unknown, nil
		}
		return nil, unknown, err
	}
	actv["sources"] = map[string]any{"probe": map[string]any{"output": out}}
	happened, err := g.happened.EvalBool(e.ctx, actv)
	if err != nil {
		return nil, unknown, nil
	}
	if !happened {
		return nil, no, nil
	}
	if g.output == nil {
		return nil, yes, nil
	}
	v, err := g.output.Eval(e.ctx, actv)
	if err != nil {
		return nil, yes, nil // it happened; only the output is unknown
	}
	v, err = journal.Normalize(v)
	if err != nil {
		return nil, yes, nil
	}
	return v, yes, nil
}

// callAction checks the action's authorize rule and calls it. A refusal is
// a definite failure: nothing was called.
func (e *execution) callAction(action string, args map[string]any, key string) (any, error) {
	return e.callActionExtra(action, args, key, nil)
}

func (e *execution) callActionExtra(action string, args map[string]any, key string, extra map[string]any) (any, error) {
	if err := e.channelChecks(e.ctx, action, e.rt.Program.actions[action], args); err != nil {
		return nil, err
	}
	if g := e.rt.Program.guards[action]; g != nil && g.authorize != nil {
		ok, err := g.authorize.EvalBool(e.ctx, expr.Activation{"inputs": args, "caller": e.scopes["caller"], "event": e.scopes["event"], "conversation": e.scopes["conversation"]})
		if err != nil || !ok {
			return nil, &act.DefiniteError{Code: "FORBIDDEN", Message: action + ": the caller is not allowed to do this"}
		}
	}
	out, err := e.rt.Actor.Call(e.ctx, act.Call{Action: action, Args: args, IdempotencyKey: key, Caller: e.caller(), Extra: extra})
	if err != nil {
		if ce := classify(err); ce != nil {
			ce.Action = action // the journal says which action failed (tessera drift)
			return nil, ce
		}
	}
	return out, err
}

// indeterminate asks a person whether an unverifiable effect happened.
// Nothing is retried: yes continues as success, no fails the run, and an
// unclear answer is asked again.
func (e *execution) indeterminate(f *frame, c call) (any, error) {
	happened, out, err := e.askHappened(c.site+"/indeterminate", journal.StatusIndeterminate, "queue:"+e.limits.DefaultQueue, "indeterminate",
		fmt.Sprintf("Did %s happen? The call failed without a clear outcome (idempotency key %q). Answer yes or no, or {\"happened\": true|false, \"output\": ...}.", c.act, c.key))
	if err != nil {
		return nil, err
	}
	if happened {
		return out, nil
	}
	return nil, e.fail(f, c.act+" did not happen, as confirmed by a person")
}

// maxHumanRounds bounds how many times an unclear answer is asked again.
const maxHumanRounds = 3

// askHappened asks a person whether an effect happened. The answer is
// {"happened": bool, "output": any}, a bool, or text starting with a clear
// yes or no; anything else is asked again, and after maxHumanRounds the
// effect is taken as not done (the run fails; nothing is repeated).
func (e *execution) askHappened(site string, status journal.Status, to, task, text string) (bool, any, error) {
	for round := 1; round <= maxHumanRounds; round++ {
		s := site
		if round > 1 {
			s = fmt.Sprintf("%s/%d", site, round)
		}
		answer, v, err := e.waitFor(s, status, Message{To: to, Task: task, Text: text})
		if err != nil {
			return false, nil, err
		}
		if happened, out, ok := readHappened(answer, v); ok {
			return happened, out, nil
		}
	}
	return false, nil, nil
}

// readHappened interprets a person's answer about an effect.
func readHappened(text string, v any) (happened bool, out any, ok bool) {
	switch x := v.(type) {
	case map[string]any:
		if h, isBool := x["happened"].(bool); isBool {
			return h, x["output"], true
		}
	case bool:
		return x, nil, true
	}
	if b, ok := yesNo(text); ok {
		return b, nil, true
	}
	return false, nil, false
}

// yesNo reads a clear yes or no at the start of a person's text: "sim,
// liguei e remarquei" says it happened. For approvals see readApproval.
func yesNo(text string) (bool, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	word := t
	if i := strings.IndexAny(t, " ,.!;:"); i >= 0 {
		word = t[:i]
	}
	return yesNoWord(word)
}

func yesNoWord(word string) (bool, bool) {
	switch word {
	case "sim", "yes", "true", "feito", "aconteceu", "ok", "s", "y":
		return true, true
	case "não", "nao", "no", "false", "n":
		return false, true
	}
	return false, false
}

// readApproval reads an approval: the value, or a text that is one yes or
// no word as a whole ("sim", "Não."). "sim, mas só 5%" or "ok, não
// aprovo" is unclear, and an unclear answer never approves.
func readApproval(text string, v any) (approved bool, ok bool) {
	switch x := v.(type) {
	case map[string]any:
		if h, isBool := x["happened"].(bool); isBool {
			return h, true
		}
	case bool:
		return x, true
	}
	return yesNoWord(strings.Trim(strings.ToLower(strings.TrimSpace(text)), " .!?,;:"))
}

// fallback runs a fallback strategy. Failures come back as *callError,
// with the call that failed when it was an action, so later strategies act
// on it.
func (e *execution) fallback(f *frame, site string, fb *fallback) (any, *call, error) {
	switch {
	case fb.useNull:
		return nil, nil, nil
	case fb.function != "":
		out, err := e.callFunction(f, fb.function, fb.with, site, nil)
		return out, nil, err
	}
	actv := e.activation(f, nil)
	c := call{site: site, act: fb.act, spec: fb.spec}
	if fb.with != nil {
		args, err := fb.with.Resolve(e.ctx, actv)
		if err != nil {
			return nil, nil, e.evalFailure(f, site+".with", err)
		}
		c.args, _ = args.(map[string]any)
	}
	if fb.key != nil {
		k, err := fb.key.Eval(e.ctx, actv)
		if err != nil {
			return nil, nil, e.evalFailure(f, site+".idempotencyKey", err)
		}
		c.key, _ = k.(string)
	}
	out, err := e.effect(c)
	return out, &c, err
}

// escalateEffect hands a failed action to a person. The answer must say
// whether the effect happened (the person may have done it by hand): yes
// continues as success with the given output, no fails the run. Any other
// answer is asked again; a text is never taken as success.
func (e *execution) escalateEffect(f *frame, site string, es *escalate, c call) (any, error) {
	if es.whileWaiting != nil {
		text, err := e.render(f, es.whileWaiting, nil)
		if err != nil {
			return nil, e.evalFailure(f, site+".whileWaiting", err)
		}
		if err := e.send(site+"/whileWaiting", Message{To: "user", Text: text}); err != nil {
			return nil, err
		}
	}
	msg, err := e.render(f, es.message, nil)
	if err != nil {
		return nil, e.evalFailure(f, site+".message", err)
	}
	queue := e.escalationQueue(f, es, nil)
	if es.task == "takeover" {
		if _, _, err := e.waitFor(site, journal.StatusWaitingForHuman, Message{To: "queue:" + queue, Task: es.task, Text: msg}); err != nil {
			return nil, err
		}
		return nil, &finished{status: journal.StatusSucceeded, reason: "taken over by a person"}
	}
	if msg == "" {
		msg = fmt.Sprintf("%s failed.", c.act)
	}
	kind := es.task
	if kind == "approve" {
		// The question is whether it happened, never an approval: the
		// channels' Approve buttons must not answer it (admission refuses
		// approve here; definitions admitted before still run safely).
		kind = "answer"
	}
	happened, out, err := e.askHappened(site, journal.StatusWaitingForHuman, "queue:"+queue, kind,
		msg+" Did it happen (for example done by hand)? Answer yes or no, or {\"happened\": true|false, \"output\": ...}.")
	if err != nil {
		return nil, err
	}
	if happened {
		return out, nil
	}
	return nil, e.fail(f, c.act+" did not happen, as confirmed by a person")
}

// escalate hands the problem to a person and continues with their answer:
// the value they give (or their text) becomes the result. A takeover ends
// the run; the person talks to the user from then on.
func (e *execution) escalate(f *frame, site string, es *escalate, sources map[string]any) (any, error) {
	if es.whileWaiting != nil {
		text, err := e.render(f, es.whileWaiting, sources)
		if err != nil {
			return nil, e.evalFailure(f, site+".whileWaiting", err)
		}
		if err := e.send(site+"/whileWaiting", Message{To: "user", Text: text}); err != nil {
			return nil, err
		}
	}
	msg, err := e.render(f, es.message, sources)
	if err != nil {
		return nil, e.evalFailure(f, site+".message", err)
	}
	queue := e.escalationQueue(f, es, sources)
	text, v, err := e.waitFor(site, journal.StatusWaitingForHuman, Message{To: "queue:" + queue, Task: es.task, Text: msg})
	if err != nil {
		return nil, err
	}
	if es.task == "takeover" {
		return nil, &finished{status: journal.StatusSucceeded, reason: "taken over by a person"}
	}
	if es.task == "approve" {
		// A denied approval ends the Function (#53): nothing after it
		// runs. An answer that is neither yes nor no is no approval.
		if approved, ok := readApproval(text, v); !ok || !approved {
			say := e.msgs().Denied
			if es.onDeny != nil {
				if say, err = e.render(f, es.onDeny, sources); err != nil {
					return nil, e.evalFailure(f, site+".onDeny", err)
				}
			}
			if err := e.send(site+"/denied", Message{To: "user", Text: say}); err != nil {
				return nil, err
			}
			return nil, &finished{status: journal.StatusSucceeded, reason: "the approval was denied"}
		}
		return true, nil
	}
	if v != nil {
		return v, nil
	}
	return text, nil
}
