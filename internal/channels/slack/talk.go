package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bbiangul/tessera/internal/channels/outbox"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
)

// The conversational back office on Slack (RFC-0006 step 6,
// backoffice-006). A member writes to the app in a direct message; the
// Events API delivers it, signed like the clicks. The Slack user ID is a
// Team member's (their roles and attributes, so their audience) or a
// caller without roles. Replies, now and later (the run's outbox), go to
// the user's DM through the slack plugin's post-text.

// ConversationPrefix marks conversations on Slack: "slack:" + user ID.
const ConversationPrefix = "slack:"

// eventEnvelope is the part of an Events API request read here.
type eventEnvelope struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	TeamID    string `json:"team_id"`
	EventID   string `json:"event_id"`
	Event     struct {
		Type        string `json:"type"`
		ChannelType string `json:"channel_type"`
		User        string `json:"user"`
		Text        string `json:"text"`
		BotID       string `json:"bot_id"`
		Subtype     string `json:"subtype"`
		Team        string `json:"team"` // the user's workspace
	} `json:"event"`
}

func (a *Adapter) events(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !a.verify(r, body) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var in eventEnvelope
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	if in.Type == "url_verification" {
		// Slack's handshake when the Events URL is set, signed like the rest.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, in.Challenge)
		return
	}
	e := in.Event
	ours := in.TeamID == a.TeamID || e.Team == a.TeamID // Enterprise Grid names the org as team_id (#54)
	if !a.Talks() || in.Type != "event_callback" || !ours || e.Type != "message" || e.ChannelType != "im" ||
		e.BotID != "" || e.Subtype != "" || e.User == "" || e.Text == "" || in.EventID == "" {
		w.WriteHeader(http.StatusOK) // not a member's direct message to the app
		return
	}
	fresh, err := a.firstTime(r.Context(), in.EventID)
	if err != nil {
		http.Error(w, "try again", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK) // Slack wants an answer within 3 seconds
	if !fresh {
		return // Slack retried an event it had already delivered
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		a.talk(ctx, in.EventID, e.User, e.Text)
	}()
}

func (a *Adapter) firstTime(ctx context.Context, id string) (bool, error) {
	res, err := a.db.ExecContext(ctx, `INSERT INTO seen (id, at) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`, id, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// talk hands a direct message to the agent and sends the platform's own
// replies (not understood, not allowed, a free-mode answer) now; a run's
// messages go out through Deliver.
func (a *Adapter) talk(ctx context.Context, eventID, user, text string) {
	rt := a.Host.Runtime
	caller := map[string]any{"channel": "slack", "slackUser": user}
	if m, ok := rt.MemberBySlack(a.Host.Namespace(), user); ok {
		caller = runtime.AsMember(caller, m) // Slack signed the request from this user
	}
	res, err := a.Host.Handle(ctx, ConversationPrefix+user, caller, text)
	if err != nil {
		fmt.Println("slack: handling a message:", err)
		return
	}
	for i, out := range res.Outbox {
		if out.Seq != 0 || out.To != "user" {
			continue
		}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/post-text", Args: map[string]any{"to": user, "text": out.Text},
			IdempotencyKey: fmt.Sprintf("%s/reply/%d", eventID, i)}); err != nil {
			fmt.Println("slack: replying:", err)
		}
	}
}

// Talks says whether the agent converses on Slack: the slack plugin
// declares post-text. Without it only approval clicks are served.
func (a *Adapter) Talks() bool {
	_, ok := a.Host.Runtime.Program.Action(Plugin + "/post-text")
	return ok
}

// Deliver sends the runs' messages to Slack users' DMs, in order (package
// outbox).
func (a *Adapter) Deliver(ctx context.Context) (int, error) {
	if !a.Talks() {
		return 0, nil
	}
	d := &outbox.Delivery{Runtime: a.Host.Runtime, DB: a.db, Prefix: ConversationPrefix, Now: a.Now,
		Send: func(ctx context.Context, to string, m runtime.Message) error {
			_, err := a.Host.Runtime.Actor.Call(ctx, act.Call{Action: Plugin + "/post-text", Args: map[string]any{"to": to, "text": m.Text}, IdempotencyKey: m.ID})
			return err
		}}
	return d.Deliver(ctx)
}
