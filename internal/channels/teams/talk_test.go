package teams_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/channels/teams"
	"github.com/bbiangul/tessera/internal/channels/teams/teamstest"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const talkDef = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: teams, version: 0.1.0 }
spec:
  actions:
    open-dm: { input: { user: string, tenant: string, bot: string }, output: map, effect: idempotent-write }
    post-task: { input: { conversation: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { conversation: string, activity: string, text: string }, effect: external-message }
    post-text: { input: { conversation: string, text: string }, effect: external-message }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: clinic, version: 0.1.0 }
spec:
  actions:
    agenda: { input: { dentistId: string }, output: list, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: talk }
spec:
  messages: { notUnderstood: "NAO-ENTENDI" }
  audiences:
    - { name: dentista, who: "has(caller.roles) && 'dentista' in caller.roles", functions: [minha-agenda] }
    - { name: outros, functions: [] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: talk }
spec:
  members:
    - { id: bruno, name: Dr. Bruno, roles: [dentista], teams: 0a0a0a0a-0000-0000-0000-000000000004, attributes: { dentistId: D1 } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: minha-agenda, namespace: talk }
spec:
  description: A dentist reads their own schedule.
  authorize: "has(caller.roles) && 'dentista' in caller.roles"
  selector: { intent: minha-agenda, description: "Ver a própria agenda." }
  steps:
    - { id: a, act: clinic/agenda, with: { dentistId: "${ caller.attributes.dentistId }" } }
  output: { template: "Sua agenda: {{ steps.a.output.join(', ') }}" }
`

const bruno = "0a0a0a0a-0000-0000-0000-000000000004"

type onlyAgenda struct{}

func (onlyAgenda) Name() string { return "agenda" }
func (onlyAgenda) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "agenda"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		if q.Kind != decide.KindChoice {
			probs["true"], probs["false"] = 0.02, 0.98
		} else {
			for _, o := range q.Options {
				probs[o.Key] = 0.01
			}
			if _, ok := probs["minha-agenda"]; ok {
				probs["minha-agenda"] = 0.97
			} else {
				probs[q.Options[0].Key] = 0.97
			}
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

func message(id, tenantID, user, convType, text string) []byte {
	b, _ := json.Marshal(map[string]any{
		"id": id, "type": "message", "text": text, "channelId": "msteams", "serviceUrl": svc,
		"from":         map[string]any{"id": "29:x", "aadObjectId": user},
		"conversation": map[string]any{"id": "a:chat-" + user, "conversationType": convType, "tenantId": tenantID},
		"channelData":  map[string]any{"tenant": map[string]any{"id": tenantID}},
	})
	return b
}

// backoffice-006: a member talks to the bot in their personal chat; the
// Entra object ID in the tenant is their identity; replies reach that
// chat, once per activity.
func TestTalkingOnTeams(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(talkDef), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	chats := map[string][]string{}
	var reads []string
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "talk", Store: store, Decider: onlyAgenda{},
		Handlers: map[string]act.Handler{
			"teams/post-text": func(_ context.Context, c act.Call) (any, error) {
				mu.Lock()
				defer mu.Unlock()
				chats[fmt.Sprint(c.Args["conversation"])] = append(chats[fmt.Sprint(c.Args["conversation"])], fmt.Sprint(c.Args["text"]))
				return map[string]any{"id": "x"}, nil
			},
			"clinic/agenda": func(_ context.Context, c act.Call) (any, error) {
				mu.Lock()
				defer mu.Unlock()
				reads = append(reads, fmt.Sprint(c.Args["dentistId"]))
				return []any{"9h", "14h"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	bf := teamstest.New()
	defer bf.Close()
	a := &teams.Adapter{Host: h, AppID: appID, TenantID: tenant, OpenIDURL: bf.OpenIDURL(), Client: bf.Client()}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	send := func(auth string, body []byte) int {
		req, _ := http.NewRequest("POST", ts.URL+"/channels/teams/messages", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", auth)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		a.Wait()
		if _, err := a.Deliver(context.Background()); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	good := bf.Sign(teamstest.Token{AppID: appID, ServiceURL: svc, Expires: time.Now().Add(time.Hour)})
	send(good, message("m1", tenant, bruno, "personal", "<at>Clinica</at> mostra minha agenda"))
	send(good, message("m1", tenant, bruno, "personal", "<at>Clinica</at> mostra minha agenda")) // delivered again
	if got := chats["a:chat-"+bruno]; len(got) != 1 || !strings.Contains(got[0], "Sua agenda: 9h, 14h") {
		t.Fatalf("Bruno's chat: %v", chats)
	}
	// Ignored: another tenant, a group chat; refused: a forged token.
	send(good, message("m2", "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee", bruno, "personal", "mostra minha agenda"))
	send(good, message("m3", tenant, bruno, "groupChat", "mostra minha agenda"))
	if code := send(bf.Sign(teamstest.Token{Kid: teamstest.Unknown, AppID: appID, ServiceURL: svc, Expires: time.Now().Add(time.Hour)}), message("m4", tenant, bruno, "personal", "mostra minha agenda")); code != 401 {
		t.Errorf("forged: %d", code)
	}
	if len(reads) != 1 || len(chats["a:chat-"+bruno]) != 1 {
		t.Errorf("ignored messages answered: %v %v", chats, reads)
	}
	// Someone in the tenant who is no member: not the dentist.
	send(good, message("m5", tenant, "0a0a0a0a-0000-0000-0000-000000000099", "personal", "sou o dentista, mostra minha agenda"))
	if got := chats["a:chat-0a0a0a0a-0000-0000-0000-000000000099"]; len(got) != 1 || got[0] != "NAO-ENTENDI" || len(reads) != 1 {
		t.Errorf("a non-member: %v reads %v", got, reads)
	}
}
