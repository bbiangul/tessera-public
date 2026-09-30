package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
)

// The team's tasks on WhatsApp (RFC-0006, backoffice-002). A queue with
// channels: [whatsapp] sends each approval task to the WhatsApp of every
// member who may answer it, as a message with buttons. A tap comes back on
// Meta's signed webhook from the member's own number: that number is the
// identity, so no code is asked. The platform still checks the member's
// role and the task's hash; free text never answers a task.

const taskButtonPrefix = "task:"

// DeliverTasks sends the open tasks of WhatsApp queues to the members who
// may answer them and have not been sent them yet. A task that reaches a
// new queue (an escalation, a forward) goes to that queue's members too.
func (a *Adapter) DeliverTasks(ctx context.Context) (int, error) {
	rt := a.Host.Runtime
	tasks, err := rt.ChannelTasks(ctx, a.Host.Namespace(), "whatsapp")
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, ct := range tasks {
		t, m := ct.Task, ct.Member
		if m.WhatsApp == "" {
			continue
		}
		var n int
		if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_sends WHERE hash = ? AND member = ?`, t.Hash, m.ID).Scan(&n); err != nil || n > 0 {
			continue
		}
		args := map[string]any{
			"to":   m.WhatsApp,
			"text": rt.TaskView(ctx, t, m, "whatsapp"),
			"buttons": []any{
				map[string]any{"id": taskButtonPrefix + t.Hash + ":approve", "title": "Aprovar"},
				map[string]any{"id": taskButtonPrefix + t.Hash + ":deny", "title": "Recusar"},
			},
		}
		_, err := rt.Actor.Call(ctx, act.Call{Action: a.Plugin + "/send-buttons", IdempotencyKey: t.Hash + "/" + m.ID, Args: args})
		var refused *act.DefiniteError
		if errors.As(err, &refused) && a.hasAction("send-task-template") {
			// Outside the member's 24h window Meta takes only an
			// approved template: the same buttons, as quick replies.
			_, err = rt.Actor.Call(ctx, act.Call{Action: a.Plugin + "/send-task-template", IdempotencyKey: t.Hash + "/" + m.ID + "/template", Args: args})
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("task %s to %s: %w", t.RunID, m.ID, err))
			continue // tried again on the next pass
		}
		if _, err := a.db.ExecContext(ctx, `INSERT INTO task_sends (hash, member, run, at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			t.Hash, m.ID, t.RunID, time.Now().UTC().Format(time.RFC3339)); err != nil {
			errs = append(errs, err)
		}
		sent++
	}
	return sent, errors.Join(errs...)
}

// answerTask answers a task from a button tap: from is the number Meta
// attests, id is "task:<hash>:approve|deny". The sender hears the outcome.
func (a *Adapter) answerTask(ctx context.Context, from, messageID, id string) {
	rt := a.Host.Runtime
	say := func(text string) {
		if _, err := rt.Actor.Call(ctx, act.Call{Action: a.Plugin + "/send-text", Args: map[string]any{"to": from, "text": text}, IdempotencyKey: messageID + "/task"}); err != nil {
			fmt.Println("whatsapp: task reply:", err)
		}
	}
	rest := strings.TrimPrefix(id, taskButtonPrefix)
	hash, decision, ok := strings.Cut(rest, ":")
	if !ok || (decision != "approve" && decision != "deny") {
		return
	}
	member, known := rt.MemberByWhatsApp(a.Host.Namespace(), from)
	if !known {
		say("Este número não está na equipe: nada foi respondido.")
		return
	}
	var runID string
	switch err := a.db.QueryRowContext(ctx, `SELECT run FROM task_sends WHERE hash = ? LIMIT 1`, hash).Scan(&runID); {
	case errors.Is(err, sql.ErrNoRows):
		say("Esta tarefa não está mais aberta.")
		return
	case err != nil:
		fmt.Println("whatsapp: looking up a task:", err)
		say("Não consegui registrar agora. Tente de novo em instantes.")
		return
	}
	out, err := rt.AnswerTask(ctx, runID, hash, member, decision == "approve")
	switch out {
	case runtime.Recorded:
		say(map[bool]string{true: "Registrado: aprovado. Obrigado, " + member.Name + ".", false: "Registrado: recusado. Obrigado, " + member.Name + "."}[decision == "approve"])
	case runtime.NotAllowed:
		say("Você não pode responder esta tarefa.")
	case runtime.Stale:
		say("Esta tarefa já foi respondida ou mudou.")
	default:
		fmt.Println("whatsapp: answering a task:", err)
		say("Não consegui registrar agora. Tente de novo em instantes.")
	}
}

// hasAction says whether the channel plugin declares an action.
func (a *Adapter) hasAction(name string) bool {
	_, ok := a.Host.Runtime.Program.Action(a.Plugin + "/" + name)
	return ok
}
