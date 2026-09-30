package runtime

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// A repair (RFC-0005 step 3, drift-007) answers a system that refused an
// action and said what it now needs: a new field (a reason), or the user's
// acceptance of something (a fee). The model proposes only additions to
// the binding's request body; the platform checks them, asks the user for
// a missing value or their agreement, and a person on the team approves
// before the action runs again, as an effect with its own idempotency key
// and the action's verification. Off unless the Agent lists the action in
// improvise.repair.

const repairRules = `The agent called ACTION and the system refused it: nothing was done. SYSTEM is what the system said. BODY lists the fields the request already sends. INPUTS are the call's inputs.
Propose how to send the request again so the system accepts it, by ADDING fields to the body. You cannot change the address, the method or the fields already sent.
Reply with JSON:
- "possible": false when SYSTEM does not say which field or value is missing, or when fixing it needs anything but adding fields.
- "add": the fields to add with their values, taken from SYSTEM (for example {"acceptFee": true}). Never invent a value about the user.
- "askUser": when a field needs something only the user knows (a reason), {"field": name, "question": the question to ask the user, in the language of LANGUAGE}. Otherwise null.
- "userMustAgree": when the new request commits the user to something (a fee, a charge, a condition), the question asking them to agree, in the language of LANGUAGE, naming it exactly as SYSTEM does (the amount), ending by asking for a yes. Otherwise null.`

var repairSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"possible": map[string]any{"type": "boolean"},
		"add":      map[string]any{"type": "object"},
		"askUser": map[string]any{"type": []any{"object", "null"}, "properties": map[string]any{
			"field": map[string]any{"type": "string"}, "question": map[string]any{"type": "string"}}},
		"userMustAgree": map[string]any{"type": []any{"string", "null"}},
	},
	"required": []any{"possible"},
}

var fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,40}$`)

// repairable says whether the run may repair calls of act.
func (e *execution) repairable(act string) bool {
	for _, a := range strings.Split(e.limits.Repair, ",") {
		if a == act {
			return true
		}
	}
	return false
}

// repairProposal is a checked proposal.
type repairProposal struct {
	add           map[string]any
	askField      string
	askQuestion   string
	agreeQuestion string
}

// repair tries to send a definitely refused call again, repaired. It
// returns the output and true when the repaired call succeeded; false
// with the error to continue the onError chain with (the repair's own
// failure, or last when no repair was tried or it was declined).
func (e *execution) repair(f *frame, c call, last *callError) (any, bool, *callError, error) {
	if !last.Definite || strings.TrimSpace(last.Detail) == "" || c.extra != nil || !e.repairable(c.act) || e.rt.Generator == nil || c.spec.HTTP == nil {
		return nil, false, last, nil
	}
	site := c.site + "/repair"
	v, err := e.once(site+"/proposal", func() (any, error) {
		body := []string{}
		for k := range c.spec.HTTP.Body {
			body = append(body, k)
		}
		sort.Strings(body)
		state := map[string]any{"action": c.act, "system": last.Detail, "body": body, "inputs": e.rt.Mask(c.args),
			"language": e.msgs().Failed}
		resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{Instructions: repairRules, State: state, Schema: repairSchema, MaxTokens: 400})
		if err != nil {
			return map[string]any{"action": c.act, "detail": last.Detail, "proposal": map[string]any{"possible": false}}, nil
		}
		return map[string]any{"action": c.act, "detail": last.Detail, "proposal": resp.Value}, nil
	})
	if err != nil {
		return nil, false, last, err
	}
	rec, _ := v.(map[string]any)
	p, ok := checkRepair(rec["proposal"], c.spec.HTTP.Body, last.Detail)
	if !ok {
		return nil, false, last, nil
	}
	if p.askField != "" {
		answer, _, err := e.waitFor(site+"/ask", journal.StatusWaitingForUser, Message{To: "user", Text: p.askQuestion})
		if err != nil {
			return nil, false, last, err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" || len(answer) > 300 {
			return nil, false, last, nil
		}
		p.add[p.askField] = answer
	}
	agreed := ""
	if p.agreeQuestion != "" {
		reply, _, err := e.waitFor(site+"/agree", journal.StatusConfirming, Message{To: "user", Text: p.agreeQuestion})
		if err != nil {
			return nil, false, last, err
		}
		yes, err := e.decideYes(site+"/agree/decide", "Does the reply agree to what the question asks, clearly?",
			map[string]any{"question": p.agreeQuestion, "reply": reply})
		if err != nil {
			return nil, false, last, err
		}
		if yes < e.limits.ConfirmYes {
			return nil, false, last, e.cancelled(site)
		}
		agreed = " The user agreed when asked: " + p.agreeQuestion
	}
	fields := make([]string, 0, len(p.add))
	for k, x := range p.add {
		fields = append(fields, fmt.Sprintf("%s=%v", k, x))
	}
	sort.Strings(fields)
	queue := e.limits.FailureQueue
	if queue == "" {
		queue = e.limits.DefaultQueue
	}
	text := fmt.Sprintf("The system refused %s and said: %q. Proposed repair: send it again adding %s.%s Approve this repair? Answer yes or no.",
		c.act, last.Detail, strings.Join(fields, ", "), agreed)
	answer, value, err := e.waitFor(site+"/approve", journal.StatusWaitingForHuman, Message{To: "queue:" + queue, Task: "approve", Text: text,
		Options: map[string]string{"yes": "Approve the repair", "no": "Refuse it"}})
	if err != nil {
		return nil, false, last, err
	}
	if approved, ok := readApproval(answer, value); !ok || !approved {
		return nil, false, last, nil
	}
	rc := c
	rc.site, rc.extra = site+"/call", p.add
	if rc.key != "" {
		rc.key += "/repair" // the refused request had no effect; a new key keeps a deduplicating system from replaying its refusal
	}
	out, err := e.effect(rc)
	if err == nil {
		return out, true, nil, nil
	}
	ce, ok := err.(*callError)
	if !ok {
		return nil, false, last, err
	}
	return nil, false, ce, nil
}

// checkRepair keeps a proposal only when it adds a few new scalar fields,
// and a user-facing question names every number the system gave.
func checkRepair(v any, body map[string]any, detail string) (repairProposal, bool) {
	m, _ := v.(map[string]any)
	if possible, _ := m["possible"].(bool); !possible {
		return repairProposal{}, false
	}
	p := repairProposal{add: map[string]any{}}
	add, _ := m["add"].(map[string]any)
	for k, x := range add {
		if !fieldName.MatchString(k) {
			return repairProposal{}, false
		}
		if _, taken := body[k]; taken {
			return repairProposal{}, false
		}
		switch x := x.(type) {
		case bool, float64, int64:
		case string:
			if len(x) > 200 {
				return repairProposal{}, false
			}
		default:
			return repairProposal{}, false // only scalars: a repair adds a flag or a value, not a structure
		}
		p.add[k] = x
	}
	if ask, ok := m["askUser"].(map[string]any); ok {
		p.askField, _ = ask["field"].(string)
		p.askQuestion, _ = ask["question"].(string)
		if _, taken := body[p.askField]; taken || !fieldName.MatchString(p.askField) || strings.TrimSpace(p.askQuestion) == "" {
			return repairProposal{}, false
		}
		if _, dup := p.add[p.askField]; dup {
			return repairProposal{}, false
		}
	}
	if q, ok := m["userMustAgree"].(string); ok && strings.TrimSpace(q) != "" {
		have := digitsOnly(q)
		for _, n := range digitRuns.FindAllString(detail, -1) {
			if d := digitsOnly(n); d != "" && !strings.Contains(have, d) {
				return repairProposal{}, false // the user must see the amount the system named
			}
		}
		p.agreeQuestion = q
	}
	n := len(p.add)
	if p.askField != "" {
		n++
	}
	if n == 0 || n > 3 {
		return repairProposal{}, false
	}
	return p, true
}
