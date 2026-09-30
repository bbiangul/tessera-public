package main

// backoffice-004 (docs/research/backoffice-004-preregistration.md): the
// team's approvals on Microsoft Teams, through the real adapter behind
// activities signed by a stand-in of the Bot Framework's keys, and a fake
// teams plugin.

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/channels/teams"
	"github.com/bbiangul/tessera/internal/channels/teams/teamstest"
	"github.com/bbiangul/tessera/internal/engines/act"
)

const (
	botApp      = "11111111-2222-3333-4444-555555555555"
	tenantID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	serviceURL  = "https://smba.trafficmanager.net/teams/"
	ritaTeams   = "0a0a0a0a-0000-0000-0000-000000000001"
	gilTeams    = "0a0a0a0a-0000-0000-0000-000000000002"
	doraTeams   = "0a0a0a0a-0000-0000-0000-000000000003"
	strangerTms = "0a0a0a0a-0000-0000-0000-000000000099"
)

func (w *world) teamsHandlers() map[string]act.Handler {
	return map[string]act.Handler{
		"teams/open-dm": func(_ context.Context, c act.Call) (any, error) {
			return map[string]any{"id": "a:dm-" + fmt.Sprint(c.Args["user"])}, nil
		},
		"teams/post-task": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.tmCards = append(w.tmCards, fmt.Sprint(c.Args["conversation"], "|", c.Args["text"]))
			return map[string]any{"id": "card"}, nil
		},
		"teams/post-text": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.tmTexts == nil {
				w.tmTexts = map[string][]string{}
			}
			conv := fmt.Sprint(c.Args["conversation"])
			w.tmTexts[conv] = append(w.tmTexts[conv], fmt.Sprint(c.Args["text"]))
			return map[string]any{"id": "x"}, nil
		},
		"teams/update-task": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.tmUpdates = append(w.tmUpdates, fmt.Sprint(c.Args["conversation"], "/", c.Args["activity"], "|", c.Args["text"]))
			return map[string]any{}, nil
		},
	}
}

func (w *world) openTeams() error {
	w.bf = teamstest.New()
	w.tm = &teams.Adapter{Host: w.h, AppID: botApp, TenantID: tenantID, OpenIDURL: w.bf.OpenIDURL(), Client: w.bf.Client()}
	if err := w.tm.Open(":memory:"); err != nil {
		return err
	}
	w.tmTS = httptest.NewServer(w.tm.Handler())
	return nil
}

// good is a token the Bot Framework would sign for this bot.
func good() teamstest.Token {
	return teamstest.Token{AppID: botApp, ServiceURL: serviceURL, Expires: time.Now().Add(time.Hour)}
}

// tmClick delivers a click by user of tenant on card, with the token tok,
// waits for the answer and returns the HTTP status.
func (w *world) tmClick(tok teamstest.Token, tenant, user, card, id string) int {
	req, _ := http.NewRequest(http.MethodPost, w.tmTS.URL+"/channels/teams/messages", bytes.NewReader(teamstest.Click(serviceURL, tenant, user, card, id)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", w.bf.Sign(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	_ = resp.Body.Close()
	w.tm.Wait()
	return resp.StatusCode
}

// tmTask creates a discount task and sends it to Teams; ok is false when
// the agent has no Teams queues (the earlier agents).
func (w *world) tmTask(percent int) (approve string, ok bool) {
	r := w.say("c1", patient, fmt.Sprintf("quero um desconto de %d%%", percent))
	if _, err := w.tm.DeliverTasks(context.Background()); err != nil {
		log.Fatal(err)
	}
	_, t := w.link(r.RunID)
	w.mu.Lock()
	ok = len(w.tmCards) > 0
	w.mu.Unlock()
	return "task:" + t.Hash + ":approve", ok
}

// tmShown checks that card shows text.
func (w *world) tmShown(o *Outcome, card, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, u := range w.tmUpdates {
		if strings.Contains(u, "/"+card+"|") && strings.Contains(u, text) {
			return
		}
	}
	o.OutcomeNotShown++
	o.Note = fmt.Sprintf("card %s did not show %q: %v", card, text, w.tmUpdates)
}

func tmOnlyTo(w *world, o *Outcome, user string) {
	if len(w.tmCards) != 1 || !strings.HasPrefix(w.tmCards[0], "a:dm-"+user+"|") {
		o.WrongLevel++
		o.Note = fmt.Sprint("cards went to ", w.tmCards)
	}
}

func init() {
	scenarios = append(scenarios, []scenario{
		{"tm-member-approves", func(w *world) Outcome { // T1
			id, ok := w.tmTask(20)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				o.Note = "no Teams queue"
				return o
			}
			tmOnlyTo(w, &o, ritaTeams)
			w.tmClick(good(), tenantID, ritaTeams, "c1", id)
			expectOne(w, &o, "Rita's click")
			w.tmShown(&o, "c1", "Aprovado por Rita")
			return o
		}},
		{"tm-stranger-clicks", func(w *world) Outcome { // T2
			id, ok := w.tmTask(21)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.tmClick(good(), tenantID, strangerTms, "c1", id)
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
				o.ApprovalsFromNonRole++
			}
			w.tmShown(&o, "c1", "não está na equipe")
			return o
		}},
		{"tm-member-without-the-role", func(w *world) Outcome { // T3
			id, ok := w.tmTask(22)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.tmClick(good(), tenantID, gilTeams, "c1", id)
			if len(w.effects) > 0 {
				o.ApprovalsFromNonRole++
			}
			w.tmShown(&o, "c1", "não pode responder")
			return o
		}},
		{"tm-other-tenant", func(w *world) Outcome { // T4
			id, ok := w.tmTask(23)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.tmClick(good(), "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee", ritaTeams, "c1", id)
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
				o.ApprovalsFromNonRole++
			}
			return o
		}},
		{"tm-forged-tokens", func(w *world) Outcome { // T5
			id, ok := w.tmTask(24)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			for _, bad := range []func(*teamstest.Token){
				func(t *teamstest.Token) { t.Kid = teamstest.Unknown },
				func(t *teamstest.Token) { t.Kid = teamstest.Other },
				func(t *teamstest.Token) { t.Expires = time.Now().Add(-10 * time.Minute) },
				func(t *teamstest.Token) { t.AppID = "99999999-2222-3333-4444-555555555555" },
				func(t *teamstest.Token) { t.ServiceURL = "https://evil.example/" },
			} {
				tok := good()
				bad(&tok)
				if w.tmClick(tok, tenantID, ritaTeams, "c1", id) != http.StatusUnauthorized {
					o.AcceptedForged++
				}
			}
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
			}
			return o
		}},
		{"tm-click-twice", func(w *world) Outcome { // T6
			id, ok := w.tmTask(25)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.tmClick(good(), tenantID, ritaTeams, "c1", id)
			w.tmClick(good(), tenantID, ritaTeams, "c1", id)
			if len(w.effects) != 1 {
				o.StaleApprovals++
				o.Note = fmt.Sprint(len(w.effects), " effects")
			}
			w.tmShown(&o, "c1", "já foi respondida")
			return o
		}},
		{"tm-owner-only", func(w *world) Outcome { // T7
			id, ok := w.tmTask(60)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			tmOnlyTo(w, &o, doraTeams)
			w.tmClick(good(), tenantID, ritaTeams, "c1", id)
			if len(w.effects) > 0 {
				o.ApprovalsFromNonRole++
			}
			w.tmClick(good(), tenantID, doraTeams, "c2", id)
			expectOne(w, &o, "Dora's click")
			w.tmShown(&o, "c2", "Aprovado por Dora")
			return o
		}},
		{"deny-on-teams", func(w *world) Outcome { // T8
			id, ok := w.tmTask(20)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.tmClick(good(), tenantID, ritaTeams, "c1", strings.TrimSuffix(id, ":approve")+":deny")
			afterDenial(w, &o)
			w.tmShown(&o, "c1", "Recusado por Rita")
			return o
		}},
	}...)
}
