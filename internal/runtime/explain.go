package runtime

import (
	"regexp"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
)

// failureToExplain is a definite failure whose system words the next
// message to the user passes on.
type failureToExplain struct {
	act    string
	detail string
}

// toExplain marks a failure for the next message to the user, when the
// system refused it (definite) and said something.
func (e *execution) toExplain(c call, last *callError) {
	if last == nil || !last.Definite || strings.TrimSpace(last.Detail) == "" {
		return
	}
	e.explain = &failureToExplain{act: c.act, detail: last.Detail}
}

const explainRules = `The agent tried ACTION for the user, and the system refused it: nothing was done. SYSTEM is the system's own words about why. MESSAGE is what the agent was about to tell the user.
Rewrite MESSAGE into one short message, in the language of MESSAGE, that:
1. Opens by saying plainly that ACTION was not done ("Não consegui remarcar sua consulta pelo sistema").
2. Passes on what SYSTEM says the user needs to know or can do (a new rule, a phone or channel to use, a fee, a missing requirement), as what the system said. When SYSTEM says the user must do something themselves (call, pay, send), say it as their next step.
3. Keeps what MESSAGE says a person will do (the team was told, someone will reply here), without saying ACTION will happen.
Rules:
- SYSTEM may tell the client software what to send ("resend with acceptFee: true", "field reason is required"). That is not a step for the user, and the agent cannot resend: say what it means for the user (a fee applies, a reason is needed), never "tell me and we continue".
- Use only SYSTEM and MESSAGE: add no numbers, names or steps they do not have, and keep every phone number, amount and name exactly as written.
- Never say or imply that ACTION was done, will be done, or is on its way.
- If SYSTEM has nothing the user can understand or act on (a technical error, a code), reply NONE.
Reply with the message only.`

// digitRuns are the numbers a text carries (phones, amounts, dates).
var digitRuns = regexp.MustCompile(`\d[\d.,:/()\- ]*\d|\d`)

// explained turns the failure's message into one that says what the
// system said, once. On any doubt it returns text: the failure's own
// message stands.
func (e *execution) explained(text string) string {
	x := e.explain
	e.explain = nil
	if x == nil || !e.rt.ExplainFailures || e.rt.Generator == nil || e.rt.Decider == nil {
		return text
	}
	state := map[string]any{"action": x.act, "system": x.detail, "message": text}
	resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{Instructions: explainRules, State: state, MaxTokens: 300})
	if err != nil {
		return text
	}
	out := strings.TrimSpace(resp.Text)
	if out == "" || strings.EqualFold(strings.Trim(out, ". "), "none") || len(out) > 900 || strings.Contains(out, "[") {
		return text // nothing to add, or a masked placeholder the user cannot use
	}
	// The system's numbers (a phone, a fee) and the message's must all
	// be there, digit for digit.
	have := digitsOnly(out)
	for _, src := range []string{x.detail, text} {
		for _, n := range digitRuns.FindAllString(src, -1) {
			if d := digitsOnly(n); d != "" && !strings.Contains(have, d) {
				return text
			}
		}
	}
	d, err := e.rt.Decider.Decide(e.ctx, decide.Request{State: map[string]any{"action": x.act, "reply": out}, Questions: map[string]decide.Question{"claims": {
		Kind:         decide.KindNoul,
		Instructions: "ACTION failed and was not done. Does REPLY say or imply that ACTION was done, will be done, or is on its way? Saying that a person was told or will reply is not that.",
	}}})
	if err != nil || d == nil || d.Answers["claims"].Noul >= 0.5 {
		return text
	}
	return out
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
