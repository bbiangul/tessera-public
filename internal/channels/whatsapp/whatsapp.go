// Package whatsapp connects a host to the WhatsApp Cloud API (Meta):
// Meta's webhook brings users' messages in, and the outbox's messages to
// users go out through the channel plugin's send-text action (the plugin's
// egress and secrets). The platform's channel rules already kept free text
// inside the 24h window, so everything in the outbox may be sent.
package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the adapter's own bookkeeping

	"github.com/bbiangul/tessera/internal/channels/outbox"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime"
)

// ConversationPrefix marks conversations on this channel: "wa:" + the
// user's WhatsApp ID (E.164 without +). A CRM webhook that starts a
// cadence for a lead uses the same ID, so the lead's replies reach it.
const ConversationPrefix = "wa:"

// Adapter serves Meta's webhook and delivers the outbox.
type Adapter struct {
	Host *host.Host
	// VerifyToken answers Meta's subscription handshake.
	VerifyToken string
	// AppSecret checks X-Hub-Signature-256 on every delivery.
	AppSecret string
	// Plugin is the channel plugin (its send-text action sends free text).
	Plugin string
	// PhoneNumberID, when set, is the business number this agent answers
	// on: messages Meta delivers for the app's other numbers are ignored.
	PhoneNumberID string
	// Now is the clock for retries; time.Now without one.
	Now func() time.Time

	db *sql.DB
}

// Open prepares the adapter's database (seen message IDs, sends).
func (a *Adapter) Open(path string) error {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		dsn = "file::memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS seen (id TEXT PRIMARY KEY, at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS sends (message TEXT PRIMARY KEY, state TEXT NOT NULL, at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS task_sends (hash TEXT NOT NULL, member TEXT NOT NULL, run TEXT NOT NULL, at TEXT NOT NULL, PRIMARY KEY (hash, member))`); err != nil {
		_ = db.Close()
		return err
	}
	for _, col := range []string{"attempts INTEGER NOT NULL DEFAULT 0", "next_at TEXT NOT NULL DEFAULT ''"} {
		if _, err := db.Exec(`ALTER TABLE sends ADD COLUMN ` + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			_ = db.Close()
			return err
		}
	}
	a.db = db
	return nil
}

// Close closes the database.
func (a *Adapter) Close() error { return a.db.Close() }

// Handler serves GET (handshake) and POST (messages) on Meta's webhook.
func (a *Adapter) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /channels/whatsapp/webhook", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("hub.mode") != "subscribe" || a.VerifyToken == "" || !hmac.Equal([]byte(q.Get("hub.verify_token")), []byte(a.VerifyToken)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// Meta's challenge is a number; anything else is not echoed (the
		// console shares this origin).
		ch := q.Get("hub.challenge")
		if ch == "" || len(ch) > 64 || strings.Trim(ch, "0123456789") != "" {
			http.Error(w, "bad challenge", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.WriteString(w, ch)
	})
	mux.HandleFunc("POST /channels/whatsapp/webhook", a.receive)
	return mux
}

// inbound is the part of Meta's webhook payload the adapter reads.
type inbound struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []struct {
					ID          string                 `json:"id"`
					From        string                 `json:"from"`
					Type        string                 `json:"type"`
					Text        *struct{ Body string } `json:"text"`
					Button      *buttonReply           `json:"button"`
					Interactive *interactive           `json:"interactive"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func (a *Adapter) receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	mac := hmac.New(sha256.New, []byte(a.AppSecret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if a.AppSecret == "" || !hmac.Equal([]byte(r.Header.Get("X-Hub-Signature-256")), []byte(want)) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var in inbound
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	// Meta retries until it gets a 200: every message is handled once. A
	// message that could not be handled is forgotten and the delivery
	// answered 500, so Meta brings it again; the others are not handled
	// twice.
	failed := false
	for _, e := range in.Entry {
		for _, c := range e.Changes {
			if a.PhoneNumberID != "" && c.Value.Metadata.PhoneNumberID != a.PhoneNumberID {
				continue // another number of the same Meta app
			}
			names := map[string]string{}
			for _, ct := range c.Value.Contacts {
				names[ct.WaID] = ct.Profile.Name
			}
			for _, m := range c.Value.Messages {
				text := messageText(m.Type, m.Text, m.Button, m.Interactive)
				if text == "" || m.From == "" || m.ID == "" {
					continue // media and reactions: not understood here
				}
				fresh, err := a.firstTime(r.Context(), m.ID)
				if err != nil {
					failed = true
					continue
				}
				if !fresh {
					continue
				}
				if id := replyID(m.Button, m.Interactive); strings.HasPrefix(id, taskButtonPrefix) {
					// A tap on a task's button is the team's answer, never
					// a message to the agent (tasks.go).
					a.answerTask(r.Context(), m.From, m.ID, id)
					continue
				}
				caller := map[string]any{"channel": "whatsapp", "waId": m.From}
				if n := names[m.From]; n != "" {
					caller["name"] = n
				}
				if member, ok := a.Host.Runtime.MemberByWhatsApp(a.Host.Namespace(), m.From); ok {
					// Meta attests the number: a Team member writes with
					// their roles and attributes (audiences-001).
					caller = runtime.AsMember(caller, member)
				}
				res, err := a.Host.Handle(r.Context(), ConversationPrefix+m.From, caller, text)
				if err != nil {
					fmt.Println("whatsapp: handling a message:", err)
					if _, derr := a.db.ExecContext(r.Context(), `DELETE FROM seen WHERE id = ?`, m.ID); derr == nil {
						failed = true
					}
					continue
				}
				// The platform's own replies outside a run (not understood,
				// a person has it, not allowed) are not in any outbox: they
				// go out now. The reply window is open, the user just wrote.
				for i, out := range res.Outbox {
					if out.Seq != 0 || out.To != "user" {
						continue
					}
					if _, err := a.Host.Runtime.Actor.Call(r.Context(), act.Call{Action: a.Plugin + "/send-text",
						Args: map[string]any{"to": m.From, "text": out.Text}, IdempotencyKey: fmt.Sprintf("%s/reply/%d", m.ID, i)}); err != nil {
						fmt.Println("whatsapp: replying:", err)
					}
				}
			}
		}
	}
	if failed {
		http.Error(w, "not handled, send it again", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// buttonReply is a template's quick-reply button; interactive, a reply to
// an interactive message. Their ids carry a task's answer (tasks.go).
type buttonReply struct {
	Text    string `json:"text"`
	Payload string `json:"payload"`
}

type interactive struct {
	ButtonReply *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"button_reply"`
	ListReply *struct{ Title string } `json:"list_reply"`
}

// replyID is the id of a tapped button, if the message is one.
func replyID(button *buttonReply, inter *interactive) string {
	switch {
	case inter != nil && inter.ButtonReply != nil:
		return inter.ButtonReply.ID
	case button != nil:
		return button.Payload
	}
	return ""
}

func messageText(typ string, text *struct{ Body string }, button *buttonReply, inter *interactive) string {
	switch {
	case typ == "text" && text != nil:
		return text.Body
	case typ == "button" && button != nil:
		return button.Text
	case typ == "interactive" && inter != nil && inter.ButtonReply != nil:
		return inter.ButtonReply.Title
	case typ == "interactive" && inter != nil && inter.ListReply != nil:
		return inter.ListReply.Title
	}
	return ""
}

func (a *Adapter) firstTime(ctx context.Context, id string) (bool, error) {
	res, err := a.db.ExecContext(ctx, `INSERT INTO seen (id, at) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`, id, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Deliver sends the outbox's messages to users of this channel and
// acknowledges them, conversation by conversation, in order (package
// outbox: marked before sent, refusals final, other failures retried with
// a growing delay).
func (a *Adapter) Deliver(ctx context.Context) (int, error) {
	d := &outbox.Delivery{Runtime: a.Host.Runtime, DB: a.db, Prefix: ConversationPrefix, Now: a.Now,
		Send: func(ctx context.Context, to string, m runtime.Message) error {
			_, err := a.Host.Runtime.Actor.Call(ctx, act.Call{Action: a.Plugin + "/send-text", Args: map[string]any{"to": to, "text": m.Text}, IdempotencyKey: m.ID})
			return err
		}}
	return d.Deliver(ctx)
}

// ErrSend means Meta refused a message.
var ErrSend = outbox.ErrSend
