package teams

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/channels/outbox"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
)

// The conversational back office on Teams (RFC-0006 step 6,
// backoffice-006). A member writes to the bot in their personal chat; the
// activity arrives with the Bot Framework's JWT, checked like a click. In
// the configured tenant, the sender's Entra object ID is a Team member's
// (their roles and attributes, so their audience) or a caller without
// roles. Replies, now and later (the run's outbox), go to that chat
// through the teams plugin's post-text.

// ConversationPrefix marks conversations on Teams: "teams:" + the
// sender's Entra object ID, lower case.
const ConversationPrefix = "teams:"

var tags = regexp.MustCompile(`<[^>]*>`)

// receive handles a message activity (already authenticated) to the bot.
func (a *Adapter) receive(ctx context.Context, in activity, tenant string) {
	user := strings.ToLower(in.From.AADObjectID)
	if !a.Talks() || tenant != a.TenantID || user == "" || in.ID == "" || in.Conversation.ID == "" {
		return // another organization, or not a person's message
	}
	if in.Conversation.Type != "" && in.Conversation.Type != "personal" {
		return // group chats and channels are not the agent's conversation
	}
	res, err := a.db.ExecContext(ctx, `INSERT INTO seen (id, at) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`, in.ID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		fmt.Println("teams: recording a message:", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return // delivered again
	}
	if _, err := a.db.ExecContext(ctx, `INSERT INTO talks (user, conversation) VALUES (?, ?) ON CONFLICT (user) DO UPDATE SET conversation = excluded.conversation`,
		user, in.Conversation.ID); err != nil {
		fmt.Println("teams: recording the chat:", err)
		return
	}
	text := strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(in.Text, "")))
	if text == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		a.talk(ctx, in.ID, user, in.Conversation.ID, text)
	}()
}

// talk hands the message to the agent and sends the platform's own
// replies now; a run's messages go out through Deliver.
func (a *Adapter) talk(ctx context.Context, activityID, user, conversation, text string) {
	rt := a.Host.Runtime
	caller := map[string]any{"channel": "teams", "aadObjectId": user}
	if m, ok := rt.MemberByTeams(a.Host.Namespace(), user); ok {
		caller = runtime.AsMember(caller, m) // the Bot Framework vouched for this user in our tenant
	}
	res, err := a.Host.Handle(ctx, ConversationPrefix+user, caller, text)
	if err != nil {
		fmt.Println("teams: handling a message:", err)
		return
	}
	for i, out := range res.Outbox {
		if out.Seq != 0 || out.To != "user" {
			continue
		}
		if _, err := rt.Actor.Call(ctx, act.Call{Action: Plugin + "/post-text", Args: map[string]any{"conversation": conversation, "text": out.Text},
			IdempotencyKey: fmt.Sprintf("%s/reply/%d", activityID, i)}); err != nil {
			fmt.Println("teams: replying:", err)
		}
	}
}

// Talks says whether the agent converses on Teams: the teams plugin
// declares post-text. Without it only approval clicks are served.
func (a *Adapter) Talks() bool {
	_, ok := a.Host.Runtime.Program.Action(Plugin + "/post-text")
	return ok
}

// Deliver sends the runs' messages to the users' personal chats, in order
// (package outbox).
func (a *Adapter) Deliver(ctx context.Context) (int, error) {
	if !a.Talks() {
		return 0, nil
	}
	d := &outbox.Delivery{Runtime: a.Host.Runtime, DB: a.db, Prefix: ConversationPrefix, Now: a.Now,
		Send: func(ctx context.Context, user string, m runtime.Message) error {
			var conv string
			if err := a.db.QueryRowContext(ctx, `SELECT conversation FROM talks WHERE user = ?`, user).Scan(&conv); err != nil {
				return fmt.Errorf("no chat with %s: %w", user, err)
			}
			_, err := a.Host.Runtime.Actor.Call(ctx, act.Call{Action: Plugin + "/post-text", Args: map[string]any{"conversation": conv, "text": m.Text}, IdempotencyKey: m.ID})
			return err
		}}
	return d.Deliver(ctx)
}
