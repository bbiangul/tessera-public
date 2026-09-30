// Package cadence is the ADR-0004 spike: a sales follow-up cadence
// (D+1 at 10h on the next business day, then 48h later) as a Temporal
// workflow, to compare with the same cadence as a Tessera Function
// (internal/runtime/wait_test.go).
package cadence

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

// Send is the activity that messages the lead (a plugin call in Tessera).
func Send(text string) error { return nil }

var saoPaulo, _ = time.LoadLocation("America/Sao_Paulo")

// nextBusinessDayAt is what Tessera writes as
// atTime(addBusinessDays(event.time, 1, tz), '10:00', tz).
func nextBusinessDayAt(t time.Time, hour int) time.Time {
	l := t.In(saoPaulo)
	d := l.AddDate(0, 0, 1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, saoPaulo)
}

// Cadence sends three messages with durable sleeps between them.
func Cadence(ctx workflow.Context, lead string) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute})
	if err := workflow.ExecuteActivity(ctx, Send, "Olá! Posso ajudar?").Get(ctx, nil); err != nil {
		return err
	}
	// workflow.Now, never time.Now: the workflow is replayed from history.
	if err := workflow.Sleep(ctx, nextBusinessDayAt(workflow.Now(ctx), 10).Sub(workflow.Now(ctx))); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, Send, "Oi de novo, viu minha mensagem?").Get(ctx, nil); err != nil {
		return err
	}
	// A change to a workflow with runs in flight must be versioned by hand,
	// or those runs fail to replay. Tessera pins each run to its generation
	// instead.
	wait := 48 * time.Hour
	if v := workflow.GetVersion(ctx, "second-wait", workflow.DefaultVersion, 1); v == 1 {
		wait = 72 * time.Hour
	}
	if err := workflow.Sleep(ctx, wait); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, Send, "Última tentativa").Get(ctx, nil)
}
