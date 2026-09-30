// Package teams presents the team's approval tasks on Microsoft Teams
// (RFC-0006, backoffice-004). A queue with channels: [teams] sends each
// approval task to the personal chat of every member who may answer it,
// as an Adaptive Card with Aprovar / Recusar, through the teams plugin
// (the Bot Framework's REST API). A click comes back as an activity with
// a JWT the Bot Framework signs (auth.go); the sender's Microsoft Entra
// object ID, in the configured tenant, is the identity. The platform
// still checks the member's role and the task's hash. The card is then
// replaced with the outcome.
package teams

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the adapter's own bookkeeping

	"github.com/bbiangul/tessera/internal/channels/outbox"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime"
)

// Plugin is the plugin that talks to the Bot Framework.
const Plugin = "teams"

// Actions are the plugin's actions the adapter uses.
var Actions = []string{"open-dm", "post-task", "update-task"}

const taskPrefix = "task:"

// Adapter sends tasks and serves the Bot Framework's messaging endpoint.
type Adapter struct {
	Host *host.Host
	// AppID is the bot's Microsoft app ID: tokens for any other bot are
	// refused.
	AppID string
	// TenantID is the organization whose members are the Team's: a click
	// from another tenant answers nothing.
	TenantID string
	// OpenIDURL is where the signing keys are published; DefaultOpenIDURL
	// without one.
	OpenIDURL string
	// Client fetches the keys; http.DefaultClient without one.
	Client *http.Client
	// Now is the clock for token validity; time.Now without one.
	Now func() time.Time

	keys *keySet
	db   *sql.DB
	wg   sync.WaitGroup
}

// Open prepares the adapter's database (personal chats, tasks sent).
func (a *Adapter) Open(path string) error {
	if a.AppID == "" || a.TenantID == "" {
		return errors.New("teams: the bot's app ID and the tenant ID are required")
	}
	if a.OpenIDURL == "" {
		a.OpenIDURL = DefaultOpenIDURL
	}
	if !strings.HasPrefix(a.OpenIDURL, "https://") {
		return errors.New("teams: the OpenID configuration URL must be https")
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	a.keys = &keySet{openID: a.OpenIDURL, client: client, now: a.now}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS dms (member TEXT PRIMARY KEY, conversation TEXT NOT NULL, user TEXT NOT NULL DEFAULT '');
		CREATE TABLE IF NOT EXISTS task_posts (hash TEXT NOT NULL, member TEXT NOT NULL, run TEXT NOT NULL, at TEXT NOT NULL, PRIMARY KEY (hash, member));
		CREATE TABLE IF NOT EXISTS seen (id TEXT PRIMARY KEY, at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS talks (user TEXT PRIMARY KEY, conversation TEXT NOT NULL);` + outbox.Schema); err != nil {
		_ = db.Close()
		return err
	}
	if _, err := db.Exec(`ALTER TABLE dms ADD COLUMN user TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		_ = db.Close()
		return err
	}
	a.db = db
	return nil
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Wait waits for the answers being recorded.
func (a *Adapter) Wait() { a.wg.Wait() }

// Close waits for the answers being recorded and closes the database.
func (a *Adapter) Close() error {
	a.wg.Wait()
	return a.db.Close()
}

// Handler serves the bot's messaging endpoint.
func (a *Adapter) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /channels/teams/messages", a.messages)
	return mux
}

// activity is the part of a Bot Framework activity read here.
type activity struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Type       string `json:"type"`
	ChannelID  string `json:"channelId"`
	ServiceURL string `json:"serviceUrl"`
	ReplyToID  string `json:"replyToId"`
	From       struct {
		AADObjectID string `json:"aadObjectId"`
	} `json:"from"`
	Conversation struct {
		ID       string `json:"id"`
		TenantID string `json:"tenantId"`
		Type     string `json:"conversationType"`
	} `json:"conversation"`
	ChannelData struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	} `json:"channelData"`
	Value map[string]any `json:"value"`
}

func (a *Adapter) messages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var in activity
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "bad activity", http.StatusBadRequest)
		return
	}
	// The token is checked before anything in the activity is believed.
	if err := a.keys.verify(r.Context(), r.Header.Get("Authorization"), a.AppID, in.ChannelID, in.ServiceURL); err != nil || in.ChannelID != "msteams" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Answered at once; recording the answer runs the rest of the run.
	w.WriteHeader(http.StatusOK)
	id, _ := in.Value["tessera"].(string)
	tenant := in.ChannelData.Tenant.ID
	if tenant == "" {
		tenant = in.Conversation.TenantID
	}
	if in.Type == "message" && id == "" && in.Text != "" {
		a.receive(r.Context(), in, tenant) // a message to the agent (talk.go)
		return
	}
	if in.Type != "message" || !strings.HasPrefix(id, taskPrefix) {
		return // installs, other activities: not tasks
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		a.answer(ctx, tenant, in.From.AADObjectID, in.Conversation.ID, in.ReplyToID, id)
	}()
}

// DeliverTasks sends the open tasks of Teams queues to the personal chats
// of the members who may answer them and have not been sent them yet.
func (a *Adapter) DeliverTasks(ctx context.Context) (int, error) {
	rt := a.Host.Runtime
	tasks, err := rt.ChannelTasks(ctx, a.Host.Namespace(), "teams")
	if err != nil {
		return 0, err
	}
	sent := 0
	var errs []error
	for _, ct := range tasks {
		t, m := ct.Task, ct.Member
		if m.Teams == "" {
			continue
		}
		var n int
		if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_posts WHERE hash = ? AND member = ?`, t.Hash, m.ID).Scan(&n); err != nil || n > 0 {
			continue
		}
		conv, err := a.dm(ctx, m.ID, m.Teams)
		if err != nil {
			errs = append(errs, fmt.Errorf("personal chat with %s: %w", m.ID, err))
			continue
		}
		args := map[string]any{
			"conversation": conv,
			"text":         rt.TaskView(ctx, t, m, "teams"),
			"buttons": []any{
				map[string]any{"id": taskPrefix + t.Hash + ":approve", "title": "Aprovar", "style": "positive"},
				map[string]any{"id": taskPrefix + t.Hash + ":deny", "title": "Recusar", "style": "destructive"},
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

// dm is the member's personal conversation with the bot, created once per
// Entra user: when the member's teams ID is corrected, a new chat is
// opened with the right person.
func (a *Adapter) dm(ctx context.Context, member, user string) (string, error) {
	var conv string
	if err := a.db.QueryRowContext(ctx, `SELECT conversation FROM dms WHERE member = ? AND user = ?`, member, strings.ToLower(user)).Scan(&conv); err == nil {
		return conv, nil
	}
	out, err := a.Host.Runtime.Actor.Call(ctx, act.Call{Action: Plugin + "/open-dm",
		Args: map[string]any{"user": user, "tenant": a.TenantID, "bot": "28:" + a.AppID}, IdempotencyKey: "dm/" + member + "/" + strings.ToLower(user)})
	if err != nil {
		return "", err
	}
	conv = conversationID(out)
	if conv == "" {
		return "", errors.New("open-dm returned no conversation id")
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO dms (member, conversation, user) VALUES (?, ?, ?) ON CONFLICT (member) DO UPDATE SET conversation = excluded.conversation, user = excluded.user`, member, conv, strings.ToLower(user))
	return conv, err
}

// conversationID reads open-dm's output: the id, or {id: …}.
func conversationID(out any) string {
	switch v := out.(type) {
	case string:
		return v
	case map[string]any:
		s, _ := v["id"].(string)
		return s
	}
	return ""
}

// answer records a click: user is the Entra object ID in tenant that the
// signed activity carries; id is "task:<hash>:approve|deny". The card
// (activity cardID in conversation) then shows the outcome.
func (a *Adapter) answer(ctx context.Context, tenant, user, conversation, cardID, id string) {
	rt := a.Host.Runtime
	show := func(text string) {
		if conversation == "" || cardID == "" {
			return
		}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/update-task", IdempotencyKey: conversation + "/" + cardID + "/" + text,
			Args: map[string]any{"conversation": conversation, "activity": cardID, "text": text}}); err != nil {
			fmt.Println("teams: showing the outcome:", err)
		}
	}
	hash, decision, ok := strings.Cut(strings.TrimPrefix(id, taskPrefix), ":")
	if !ok || (decision != "approve" && decision != "deny") {
		return
	}
	if tenant != a.TenantID || user == "" {
		return // another organization: its users are not the Team's
	}
	member, known := rt.MemberByTeams(a.Host.Namespace(), user)
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
		fmt.Println("teams: looking up a task:", err)
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
		fmt.Println("teams: answering a task:", err)
	}
}
