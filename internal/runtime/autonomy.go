package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Autonomy levels (RFC-0001; per audience, RFC-0006 and audiences-002).
const (
	AutonomyAutonomous     = "autonomous"
	AutonomyApproveActions = "approve-actions"
	AutonomyDraft          = "draft"
)

// AutonomyFor is how much the agent does alone for a caller: their
// audience's level, else the Agent's, else autonomous.
func (r *Runtime) AutonomyFor(caller map[string]any) string {
	if a, ok := r.audienceOf(caller); ok && a.Autonomy != "" {
		return a.Autonomy
	}
	if r.Autonomy != "" {
		return r.Autonomy
	}
	return AutonomyAutonomous
}

func (e *execution) autonomy() string { return e.rt.AutonomyFor(e.caller()) }

// approveEffect asks the humanInTheLoop queue before an irreversible or
// external-message effect, under approve-actions and draft. The action and
// its arguments are the task's text, so the task's hash binds the
// approval to them. A denial ends the Function (#53).
func (e *execution) approveEffect(c call) error {
	if e.autonomy() == AutonomyAutonomous || !irreversible(c.spec.Effect) {
		return nil
	}
	args, _ := json.Marshal(c.args)
	text := fmt.Sprintf("Aprovar %s com %s?", c.act, args)
	return e.personApproves(c.site+"/autonomy", text, true)
}

// approveDraft holds a message to the user until a person approves its
// exact text, under draft. A denial ends the run with nothing sent.
func (e *execution) approveDraft(site, text string) error {
	if e.autonomy() != AutonomyDraft {
		return nil
	}
	return e.personApproves(site+"/draft", "Enviar ao usuário esta mensagem?\n\n"+text, false)
}

// personApproves waits for the humanInTheLoop queue's approval at site;
// anything but a yes ends the run. tell: the user hears messages.denied
// (an effect not approved); a draft not approved sends nothing.
func (e *execution) personApproves(site, text string, tell bool) error {
	queue := e.limits.DefaultQueue
	if queue == "" { // admission requires humanInTheLoop for these levels
		return fmt.Errorf("autonomy %s needs Agent.spec.humanInTheLoop: nobody to approve", e.autonomy())
	}
	answer, v, err := e.waitFor(site, journal.StatusWaitingForHuman, Message{To: "queue:" + queue, Task: "approve", Text: text})
	if err != nil {
		return err
	}
	if approved, ok := readApproval(answer, v); ok && approved {
		return nil
	}
	if tell {
		if err := e.send(site+"/denied", Message{To: "user", Text: e.msgs().Denied}); err != nil {
			return err
		}
	}
	return &finished{status: journal.StatusSucceeded, reason: "a person did not approve (autonomy " + e.autonomy() + ")"}
}
