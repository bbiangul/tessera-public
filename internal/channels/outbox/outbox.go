// Package outbox delivers the runs' messages to users on a channel
// (WhatsApp, Slack, Teams), conversation by conversation, in order. A
// message is marked before it is sent, so a crash after the send is not
// followed by a second one (the channels have no idempotency key). A send
// the channel refused for good is recorded as failed and skipped; one that
// failed otherwise is tried again with a growing delay, up to MaxAttempts,
// and holds back only its own conversation's later messages.
package outbox

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

// MaxAttempts bounds the sends of one message that fail without the
// channel refusing it for good (timeouts, 5xx, rate limits).
const MaxAttempts = 8

// Schema is the sends table (the adapter's database runs it).
const Schema = `CREATE TABLE IF NOT EXISTS sends (message TEXT PRIMARY KEY, state TEXT NOT NULL, at TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_at TEXT NOT NULL DEFAULT '')`

// ErrSend means the channel did not take a message.
var ErrSend = errors.New("send failed")

// Delivery sends the outbox of the conversations with Prefix.
type Delivery struct {
	Runtime *runtime.Runtime
	DB      *sql.DB
	// Prefix marks the channel's conversations ("wa:", "slack:").
	Prefix string
	// Send sends one message to the conversation's rest (after Prefix).
	Send func(ctx context.Context, to string, m runtime.Message) error
	// Now is the clock for retries; time.Now without one.
	Now func() time.Time
}

// Deliver sends the outbox's messages to users and acknowledges them.
// Errors are returned together; every conversation is tried.
func (d *Delivery) Deliver(ctx context.Context) (int, error) {
	runs, err := d.Runtime.Store.Unacked(ctx, d.Prefix, 100)
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, run := range runs {
		to := strings.TrimPrefix(run.Conversation, d.Prefix)
		msgs, err := d.Runtime.Outbox(ctx, run.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		last := 0
		for _, m := range msgs {
			if m.To == "user" { // messages to queues are the team's, never the user's
				done, err := d.send(ctx, to, m)
				if err != nil {
					errs = append(errs, fmt.Errorf("run %s: %w", run.ID, err))
				}
				if !done {
					break // later messages wait for this one
				}
				if err == nil {
					sent++
				}
			}
			last = m.Seq
		}
		if last > 0 {
			if err := d.Runtime.Ack(ctx, run.ID, last); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return sent, errors.Join(errs...)
}

func (d *Delivery) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// send sends one message unless it was sent or given up before; done means
// the message needs nothing more (sent, or failed for good).
func (d *Delivery) send(ctx context.Context, to string, m runtime.Message) (done bool, err error) {
	now := d.now().UTC()
	var state, nextAt string
	var attempts int
	err = d.DB.QueryRowContext(ctx, `SELECT state, attempts, next_at FROM sends WHERE message = ?`, m.ID).Scan(&state, &attempts, &nextAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, err
	case state == "retry":
		if at, _ := time.Parse(time.RFC3339Nano, nextAt); now.Before(at) {
			return false, nil
		}
	default:
		return true, nil // sent, failed, or sending when the process stopped
	}
	if _, err := d.DB.ExecContext(ctx, `INSERT INTO sends (message, state, at, attempts) VALUES (?, 'sending', ?, ?)
		ON CONFLICT (message) DO UPDATE SET state = 'sending', at = excluded.at, attempts = excluded.attempts`,
		m.ID, now.Format(time.RFC3339), attempts+1); err != nil {
		return false, err
	}
	callErr := d.Send(ctx, to, m)
	if callErr == nil {
		_, err = d.DB.ExecContext(ctx, `UPDATE sends SET state = 'sent' WHERE message = ?`, m.ID)
		return err == nil, err
	}
	var def *act.DefiniteError
	final := errors.As(callErr, &def) && def.Code != "RATE_LIMITED" || attempts+1 >= MaxAttempts
	if final {
		_, err = d.DB.ExecContext(ctx, `UPDATE sends SET state = 'failed' WHERE message = ?`, m.ID)
		if err != nil {
			return false, err
		}
		return true, fmt.Errorf("%w (given up): %v", ErrSend, callErr)
	}
	delay := min(time.Minute<<attempts, time.Hour)
	if _, err := d.DB.ExecContext(ctx, `UPDATE sends SET state = 'retry', next_at = ? WHERE message = ?`, now.Add(delay).Format(time.RFC3339Nano), m.ID); err != nil {
		return false, err
	}
	return false, fmt.Errorf("%w (again in %s): %v", ErrSend, delay, callErr)
}
