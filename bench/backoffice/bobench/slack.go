package main

// backoffice-003 (docs/research/backoffice-003-preregistration.md): the
// team's approvals on Slack, through the real adapter behind signed
// interactivity requests and a fake slack plugin.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/channels/slack"
	"github.com/bbiangul/tessera/internal/engines/act"
)

const (
	slackSecret = "signing-secret"
	workspace   = "T0CLINICA"
	ritaSlack   = "U0RITA"
	gilSlack    = "U0GIL"
	doraSlack   = "U0DORA"
)

func (w *world) slackHandlers() map[string]act.Handler {
	return map[string]act.Handler{
		"slack/post-task": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.slPosts = append(w.slPosts, fmt.Sprint(c.Args["to"], "|", c.Args["text"]))
			return map[string]any{"ok": true}, nil
		},
		"slack/post-text": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.slTexts == nil {
				w.slTexts = map[string][]string{}
			}
			to := fmt.Sprint(c.Args["to"])
			w.slTexts[to] = append(w.slTexts[to], fmt.Sprint(c.Args["text"]))
			return map[string]any{"ok": true}, nil
		},
		"slack/update-task": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.slUpdates = append(w.slUpdates, fmt.Sprint(c.Args["channel"], "/", c.Args["ts"], "|", c.Args["text"]))
			return map[string]any{"ok": true}, nil
		},
	}
}

func (w *world) openSlack() error {
	w.sl = &slack.Adapter{Host: w.h, SigningSecret: slackSecret, TeamID: workspace}
	if err := w.sl.Open(":memory:"); err != nil {
		return err
	}
	w.slTS = httptest.NewServer(w.sl.Handler())
	return nil
}

// slClick posts a signed block_actions request: user of team clicked the
// button with id on message ts; the request was signed age ago with
// secret. It waits for the answer and returns the HTTP status.
func (w *world) slClick(team, user, ts, id string, age time.Duration, secret string) int {
	payload := fmt.Sprintf(`{"type":"block_actions","user":{"id":%q,"team_id":%q},"team":{"id":%q},"container":{"type":"message","channel_id":"D0DM","message_ts":%q},"actions":[{"action_id":%q,"type":"button"}]}`,
		user, team, team, ts, id)
	body := url.Values{"payload": {payload}}.Encode()
	stamp := fmt.Sprint(time.Now().Add(-age).Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + stamp + ":" + body))
	req, _ := http.NewRequest(http.MethodPost, w.slTS.URL+"/channels/slack/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", stamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	_ = resp.Body.Close()
	w.sl.Wait()
	return resp.StatusCode
}

// slTask creates a discount task and posts it to Slack; ok is false when
// the agent has no Slack queues (the earlier agents).
func (w *world) slTask(percent int) (approve string, ok bool) {
	r := w.say("c1", patient, fmt.Sprintf("quero um desconto de %d%%", percent))
	if _, err := w.sl.DeliverTasks(context.Background()); err != nil {
		log.Fatal(err)
	}
	_, t := w.link(r.RunID)
	w.mu.Lock()
	ok = len(w.slPosts) > 0
	w.mu.Unlock()
	return "task:" + t.Hash + ":approve", ok
}

// shown checks that the message ts shows text.
func (w *world) shown(o *Outcome, ts, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, u := range w.slUpdates {
		if strings.HasPrefix(u, "D0DM/"+ts+"|") && strings.Contains(u, text) {
			return
		}
	}
	o.OutcomeNotShown++
	o.Note = fmt.Sprintf("message %s did not show %q: %v", ts, text, w.slUpdates)
}

func onlyPostedTo(w *world, o *Outcome, user string) {
	if len(w.slPosts) != 1 || !strings.HasPrefix(w.slPosts[0], user+"|") {
		o.WrongLevel++
		o.Note = fmt.Sprint("posted to ", w.slPosts)
	}
}

func init() {
	scenarios = append(scenarios, []scenario{
		{"sl-member-approves", func(w *world) Outcome { // S1
			id, ok := w.slTask(20)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				o.Note = "no Slack queue"
				return o
			}
			onlyPostedTo(w, &o, ritaSlack)
			w.slClick(workspace, ritaSlack, "1.1", id, 0, slackSecret)
			expectOne(w, &o, "Rita's click")
			w.shown(&o, "1.1", "Aprovado por Rita")
			return o
		}},
		{"sl-stranger-clicks", func(w *world) Outcome { // S2
			id, ok := w.slTask(21)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.slClick(workspace, "U0STRANGER", "1.1", id, 0, slackSecret)
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
				o.ApprovalsFromNonRole++
			}
			w.shown(&o, "1.1", "não está na equipe")
			return o
		}},
		{"sl-member-without-the-role", func(w *world) Outcome { // S3
			id, ok := w.slTask(22)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.slClick(workspace, gilSlack, "1.1", id, 0, slackSecret)
			if len(w.effects) > 0 {
				o.ApprovalsFromNonRole++
			}
			w.shown(&o, "1.1", "não pode responder")
			return o
		}},
		{"sl-other-workspace", func(w *world) Outcome { // S4
			id, ok := w.slTask(23)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.slClick("T0OTHER", ritaSlack, "1.1", id, 0, slackSecret)
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
				o.ApprovalsFromNonRole++
			}
			return o
		}},
		{"sl-forged-and-replayed", func(w *world) Outcome { // S5
			id, ok := w.slTask(24)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			if w.slClick(workspace, ritaSlack, "1.1", id, 0, "wrong-secret") != http.StatusUnauthorized {
				o.AcceptedForged++
			}
			if w.slClick(workspace, ritaSlack, "1.2", id, 6*time.Minute, slackSecret) != http.StatusUnauthorized {
				o.AcceptedForged++
			}
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
			}
			return o
		}},
		{"sl-click-twice", func(w *world) Outcome { // S6
			id, ok := w.slTask(25)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.slClick(workspace, ritaSlack, "1.1", id, 0, slackSecret)
			w.slClick(workspace, ritaSlack, "1.1", id, 0, slackSecret)
			if len(w.effects) != 1 {
				o.StaleApprovals++
				o.Note = fmt.Sprint(len(w.effects), " effects")
			}
			w.shown(&o, "1.1", "já foi respondida")
			return o
		}},
		{"sl-owner-only", func(w *world) Outcome { // S7
			id, ok := w.slTask(60)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			onlyPostedTo(w, &o, doraSlack)
			w.slClick(workspace, ritaSlack, "1.1", id, 0, slackSecret)
			if len(w.effects) > 0 {
				o.ApprovalsFromNonRole++
			}
			w.slClick(workspace, doraSlack, "1.2", id, 0, slackSecret)
			expectOne(w, &o, "Dora's click")
			w.shown(&o, "1.2", "Aprovado por Dora")
			return o
		}},
	}...)
}
