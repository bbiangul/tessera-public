// Package email sends the team's approval tasks by e-mail (RFC-0006,
// backoffice-005). A queue with channels: [email] sends each approval task
// to the e-mail of every member who may answer it, with a link bound to
// that member, through the email plugin's send-task (Outlook through
// Microsoft Graph, Gmail through its API). The e-mail is never read back:
// an e-mail's sender is easily forged. Only the link answers, and only
// with a one-time code the Team's verify sends to the member, so a
// forwarded e-mail approves nothing.
package email

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // the adapter's own bookkeeping

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/mailmsg"
	"github.com/bbiangul/tessera/internal/runtime"
)

// Plugin is the plugin that sends e-mail.
const Plugin = "email"

// Adapter sends tasks by e-mail.
type Adapter struct {
	Host *host.Host
	// Link is the task's page bound to a member ("" without a public URL).
	Link func(t runtime.Task, member string) string
	// From is the sender's address for the raw message (Gmail sends as the
	// authorized mailbox either way); may be empty.
	From string

	db *sql.DB
}

// Open prepares the adapter's database (tasks sent).
func (a *Adapter) Open(path string) error {
	if a.Link == nil {
		return errors.New("email: task links are required (a public URL)")
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_mails (hash TEXT NOT NULL, member TEXT NOT NULL, run TEXT NOT NULL, at TEXT NOT NULL, PRIMARY KEY (hash, member))`); err != nil {
		_ = db.Close()
		return err
	}
	a.db = db
	return nil
}

// Close closes the database.
func (a *Adapter) Close() error { return a.db.Close() }

// DeliverTasks e-mails the open tasks of e-mail queues to the members who
// may answer them and have not been sent them yet.
func (a *Adapter) DeliverTasks(ctx context.Context) (int, error) {
	rt := a.Host.Runtime
	tasks, err := rt.ChannelTasks(ctx, a.Host.Namespace(), "email")
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, ct := range tasks {
		t, m := ct.Task, ct.Member
		if m.Email == "" {
			continue
		}
		var n int
		if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_mails WHERE hash = ? AND member = ?`, t.Hash, m.ID).Scan(&n); err != nil || n > 0 {
			continue
		}
		link := a.Link(t, m.ID)
		if link == "" {
			return sent, errors.New("email: no task link (tessera serve needs --public-url)")
		}
		subject := Subject(t.Text)
		text := Body(m.Name, rt.TaskView(ctx, t, m, "email"), link)
		raw, err := Raw(a.From, m.Email, subject, text)
		if err != nil {
			errs = append(errs, fmt.Errorf("task %s to %s: %w", t.RunID, m.ID, err))
			continue
		}
		args := map[string]any{"to": m.Email, "subject": subject, "text": text, "link": link, "raw": raw}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/send-task", IdempotencyKey: t.Hash + "/" + m.ID, Args: args}); err != nil {
			errs = append(errs, fmt.Errorf("task %s to %s: %w", t.RunID, m.ID, err))
			continue // tried again on the next pass
		}
		if _, err := a.db.ExecContext(ctx, `INSERT INTO task_mails (hash, member, run, at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			t.Hash, m.ID, t.RunID, time.Now().UTC().Format(time.RFC3339)); err != nil {
			errs = append(errs, err)
		}
		sent++
	}
	return sent, errors.Join(errs...)
}

// Subject is the e-mail's subject: the task's start, on one line.
func Subject(task string) string {
	s := mailmsg.OneLine(task)
	if utf8.RuneCountInString(s) > 70 {
		s = string([]rune(s)[:69]) + "…"
	}
	return "Aprovação pendente: " + s
}

// Body is the e-mail's text.
func Body(name, task, link string) string {
	return "Olá, " + name + ".\n\n" + task + "\n\n" +
		"Para aprovar ou recusar, abra o link abaixo. Pediremos um código, enviado só para você. " +
		"Responder a este e-mail não tem efeito.\n\n" + link + "\n"
}

// Raw is mailmsg.Raw: the RFC 822 message, base64url-encoded.
func Raw(from, to, subject, text string) (string, error) { return mailmsg.Raw(from, to, subject, text) }
