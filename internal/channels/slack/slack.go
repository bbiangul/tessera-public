// Package slack presents the team's approval tasks on Slack (RFC-0006,
// backoffice-003). A queue with channels: [slack] posts each approval
// task to the DM of every member who may answer it, with Aprovar /
// Recusar buttons, through the slack plugin's post-task action. A click
// comes back in a request Slack signs; the user ID in it is the identity
// (checked against the configured workspace), so no code is asked. The
// platform still checks the member's role and the task's hash. The
// message's buttons are replaced with the outcome (update-task).
package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the adapter's own bookkeeping

	"github.com/bbiangul/tessera/internal/channels/outbox"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime"
)

// Plugin is the plugin that talks to Slack's Web API.
const Plugin = "slack"

const taskActionPrefix = "task:"

// maxSkew is how old a signed request may be (Slack's recommendation):
// a captured click cannot be replayed later.
const maxSkew = 5 * time.Minute

// Adapter posts tasks and serves Slack's interactivity requests.
type Adapter struct {
	Host *host.Host
	// SigningSecret checks X-Slack-Signature on every request.
	SigningSecret string
	// TeamID is the workspace whose user IDs are the Team's: a click from
	// another workspace the app is installed in answers nothing.
	TeamID string
	// Now is the clock for the signature's age; time.Now without one.
	Now func() time.Time

	db *sql.DB
	wg sync.WaitGroup
}

// Open prepares the adapter's database (tasks posted).
func (a *Adapter) Open(path string) error {
	if a.SigningSecret == "" || a.TeamID == "" {
		return errors.New("slack: a signing secret and the workspace's team ID are required")
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_posts (hash TEXT NOT NULL, member TEXT NOT NULL, run TEXT NOT NULL, at TEXT NOT NULL, PRIMARY KEY (hash, member));
		CREATE TABLE IF NOT EXISTS seen (id TEXT PRIMARY KEY, at TEXT NOT NULL);` + outbox.Schema); err != nil {
		_ = db.Close()
		return err
	}
	a.db = db
	return nil
}

// Wait waits for the answers being recorded.
func (a *Adapter) Wait() { a.wg.Wait() }

// Close waits for the answers being recorded and closes the database.
func (a *Adapter) Close() error {
	a.wg.Wait()
	return a.db.Close()
}

// Handler serves Slack's interactivity requests.
func (a *Adapter) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /channels/slack/interactions", a.interaction)
	mux.HandleFunc("POST /channels/slack/events", a.events)
	return mux
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// verify checks Slack's v0 signature and the request's age.
func (a *Adapter) verify(r *http.Request, body []byte) bool {
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if age := a.now().Sub(time.Unix(sec, 0)); age > maxSkew || age < -maxSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(a.SigningSecret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(r.Header.Get("X-Slack-Signature")), []byte(want))
}

// interactionPayload is the part of a block_actions payload read here.
type interactionPayload struct {
	Type string `json:"type"`
	User struct {
		ID     string `json:"id"`
		TeamID string `json:"team_id"`
	} `json:"user"`
	Team *struct {
		ID string `json:"id"`
	} `json:"team"`
	Container struct {
		ChannelID string `json:"channel_id"`
		MessageTS string `json:"message_ts"`
	} `json:"container"`
	Actions []struct {
		ActionID string `json:"action_id"`
	} `json:"actions"`
}

func (a *Adapter) interaction(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !a.verify(r, body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	var p interactionPayload
	if err := json.Unmarshal([]byte(form.Get("payload")), &p); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	// The click is ours when the workspace is the configured one, as the
	// payload's team or as the user's own workspace: an org-wide app on
	// Enterprise Grid may name the organization as team (#54). Both come
	// in the request Slack signed.
	team := p.User.TeamID
	if p.Team != nil && p.Team.ID == a.TeamID {
		team = p.Team.ID
	}
	// Slack wants its answer within 3 seconds; recording the answer runs
	// the rest of the run, so it goes on after the reply.
	w.WriteHeader(http.StatusOK)
	if p.Type != "block_actions" || len(p.Actions) == 0 || !strings.HasPrefix(p.Actions[0].ActionID, taskActionPrefix) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		a.answer(ctx, team, p.User.ID, p.Container.ChannelID, p.Container.MessageTS, p.Actions[0].ActionID)
	}()
}

// DeliverTasks posts the open tasks of Slack queues to the DMs of the
// members who may answer them and have not been sent them yet.
func (a *Adapter) DeliverTasks(ctx context.Context) (int, error) {
	rt := a.Host.Runtime
	tasks, err := rt.ChannelTasks(ctx, a.Host.Namespace(), "slack")
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, ct := range tasks {
		t, m := ct.Task, ct.Member
		if m.Slack == "" {
			continue
		}
		var n int
		if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_posts WHERE hash = ? AND member = ?`, t.Hash, m.ID).Scan(&n); err != nil || n > 0 {
			continue
		}
		args := map[string]any{
			"to":   m.Slack,
			"text": rt.TaskView(ctx, t, m, "slack"),
			"buttons": []any{
				map[string]any{"id": taskActionPrefix + t.Hash + ":approve", "title": "Aprovar", "style": "primary"},
				map[string]any{"id": taskActionPrefix + t.Hash + ":deny", "title": "Recusar", "style": "danger"},
			},
		}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/post-task", IdempotencyKey: t.Hash + "/" + m.ID, Args: args}); err != nil {
			errs = append(errs, fmt.Errorf("task %s to %s: %w", t.RunID, m.ID, err))
			continue // tried again on the next pass
		}
		if _, err := a.db.ExecContext(ctx, `INSERT INTO task_posts (hash, member, run, at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			t.Hash, m.ID, t.RunID, time.Now().UTC().Format(time.RFC3339)); err != nil {
			errs = append(errs, err)
		}
		sent++
	}
	return sent, errors.Join(errs...)
}

// answer records a click: user is the Slack user ID Slack signed for,
// id is "task:<hash>:approve|deny". The message then shows the outcome
// instead of the buttons.
func (a *Adapter) answer(ctx context.Context, team, user, channel, ts, id string) {
	rt := a.Host.Runtime
	show := func(text string) {
		if channel == "" || ts == "" {
			return
		}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/update-task", IdempotencyKey: channel + "/" + ts + "/" + text,
			Args: map[string]any{"channel": channel, "ts": ts, "text": text}}); err != nil {
			fmt.Println("slack: showing the outcome:", err)
		}
	}
	hash, decision, ok := strings.Cut(strings.TrimPrefix(id, taskActionPrefix), ":")
	if !ok || (decision != "approve" && decision != "deny") {
		return
	}
	if team != a.TeamID {
		return // another workspace: its user IDs are not the Team's
	}
	member, known := rt.MemberBySlack(a.Host.Namespace(), user)
	if !known {
		show("Você não está na equipe: nada foi respondido.")
		return
	}
	var runID string
	switch err := a.db.QueryRowContext(ctx, `SELECT run FROM task_posts WHERE hash = ? LIMIT 1`, hash).Scan(&runID); {
	case errors.Is(err, sql.ErrNoRows):
		show("Esta tarefa não está mais aberta.")
		return
	case err != nil:
		// The task may well be open: the buttons stay, to click again.
		fmt.Println("slack: looking up a task:", err)
		return
	}
	approve := decision == "approve"
	out, err := rt.AnswerTask(ctx, runID, hash, member, approve)
	switch out {
	case runtime.Recorded:
		if approve {
			show("✅ Aprovado por " + member.Name + ".")
		} else {
			show("❌ Recusado por " + member.Name + ".")
		}
	case runtime.NotAllowed:
		show("Você não pode responder esta tarefa.")
	case runtime.Stale:
		show("Esta tarefa já foi respondida ou mudou.")
	default:
		// The buttons stay: the member may click again.
		fmt.Println("slack: answering a task:", err)
	}
}
