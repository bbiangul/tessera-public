package admission

import (
	"fmt"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
)

// checkErrorChain validates an onError chain and the safety rules for
// repeating an effect whose outcome is unknown.
func (f *fnCheck) checkErrorChain(chain []agents.ErrorStrategy, path string, action *plugins.ActionSpec, skipEffectRules, readOnly bool, sc scope) {
	// Inside onError, sources.error is the failure (#30).
	withErr := sc
	withErr.sources = map[string]bool{"error": true}
	for k, v := range sc.sources {
		withErr.sources[k] = v
	}
	risky := action != nil && irreversible(action.Effect)
	native := action != nil && action.Verify != nil && action.Verify.Mode == plugins.VerifyNative
	probed := false
	for i, s := range chain {
		p := fmt.Sprintf("%s[%d]", path, i)
		set := 0
		for _, b := range []bool{s.Probe != nil, s.Retry != nil, s.Fallback != nil, s.Escalate != nil, s.Compensate != nil, s.Respond != nil, s.Fail != nil} {
			if b {
				set++
			}
		}
		if set != 1 {
			f.err("ONE_STRATEGY", p, fmt.Sprintf("each onError item sets exactly one strategy; found %d", set), "split it into separate items")
			continue
		}
		if s.When != "" {
			f.expression(string(s.When), expr.Bool, p+".when", withErr)
		}
		switch {
		case s.Probe != nil:
			if !skipEffectRules && (action == nil || action.Verify == nil || action.Verify.Mode == plugins.VerifyNone) {
				f.err("PROBE_UNSUPPORTED", p+".probe", "the action's plugin declares no way to verify the effect", "remove probe, or add verify (probe or native) to the plugin manifest")
			}
			// A conditional probe may not run: it does not make a later
			// retry or fallback safe.
			probed = probed || s.When == ""
		case s.Retry != nil:
			if risky && !native && !probed {
				f.err("UNSAFE_RETRY", p+".retry", "retrying an irreversible action whose outcome is unknown can repeat the effect", "put a probe before the retry, or escalate instead")
			}
		case s.Fallback != nil:
			fb := s.Fallback
			n := 0
			for _, b := range []bool{fb.Function != "", fb.Act != "", fb.UseNull} {
				if b {
					n++
				}
			}
			if n != 1 {
				f.err("ONE_STRATEGY", p+".fallback", "fallback sets exactly one of function, act, useNull", "")
			}
			if risky && !probed && (fb.Function != "" || fb.Act != "") {
				f.err("UNSAFE_FALLBACK", p+".fallback", "switching path after an irreversible action whose outcome is unknown can do the effect twice", "put a probe before the fallback, or escalate instead")
			}
			if fb.With != nil && fb.Act == "" && fb.Function == "" {
				f.err("UNUSED_FIELD", p+".fallback.with", "with only applies to act or function", "remove it")
			}
			if fb.Act != "" {
				if a := f.lookupAction(fb.Act, p+".fallback.act"); a != nil {
					if readOnly && a.Effect != plugins.EffectRead {
						f.err("EFFECT_NOT_ALLOWED", p+".fallback.act", fmt.Sprintf("resolving an input may only call read actions; %s is %s", fb.Act, a.Effect), "escalate or respond instead")
					}
					f.checkArgs(fb.With, a.Input, p+".fallback.with", sc)
					if fb.IdempotencyKey != "" {
						f.expression(string(fb.IdempotencyKey), expr.String, p+".fallback.idempotencyKey", sc)
					}
					if irreversible(a.Effect) && fb.IdempotencyKey == "" {
						f.err("IDEMPOTENCY_REQUIRED", p+".fallback.idempotencyKey", fmt.Sprintf("%s is %s: it needs an idempotency key", fb.Act, a.Effect), "set fallback.idempotencyKey")
					}
					// Later items handle failures of the fallback action.
					action, skipEffectRules = a, false
					risky = irreversible(a.Effect)
					native = a.Verify != nil && a.Verify.Mode == plugins.VerifyNative
					probed = false
				}
			}
			if fb.Function != "" {
				f.checkFunctionRef(fb.Function, fb.With, p+".fallback.function", p+".fallback.with", sc)
				if readOnly {
					if e := f.effectOf(fb.Function, map[string]bool{}); e != "" {
						f.err("EFFECT_NOT_ALLOWED", p+".fallback.function", fmt.Sprintf("resolving an input must not change the world, and function %s can (%s)", fb.Function, e), "escalate or respond instead")
					}
				}
			}
		case s.Escalate != nil:
			f.checkEscalate(s.Escalate, p+".escalate", sc)
			if action != nil && s.Escalate.Task == "approve" {
				// Here a person is asked whether the failed effect happened
				// (done by hand): an Approve button would read as "it
				// happened" and report an effect that never ran.
				f.err("INVALID_FIELD", p+".escalate.task", "an escalate in an action's onError asks whether the effect happened, not for approval: Approve would be read as done", "use task: answer, or approve before the action with a step")
			}
		case s.Respond != nil:
			f.template(string(s.Respond.Template), p+".respond.template", withErr)
		}
	}
}
