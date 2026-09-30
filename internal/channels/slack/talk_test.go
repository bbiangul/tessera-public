package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const talkAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
    post-text: { input: { to: string, text: string }, effect: external-message }
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
    - { id: bruno, name: Dr. Bruno, roles: [dentista], slack: U0BRUNO, attributes: { dentistId: D1 } }
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

type agendaRouter struct{}

func (agendaRouter) Name() string { return "agenda" }
func (agendaRouter) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
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

type dms struct {
	mu    sync.Mutex
	texts map[string][]string
	reads []string
}

func talkSetup(t *testing.T) (*Adapter, *httptest.Server, *dms, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(talkAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	box := &dms{texts: map[string][]string{}}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "talk", Store: store, Decider: agendaRouter{},
		Handlers: map[string]act.Handler{
			"slack/post-text": func(_ context.Context, c act.Call) (any, error) {
				box.mu.Lock()
				defer box.mu.Unlock()
				to := fmt.Sprint(c.Args["to"])
				box.texts[to] = append(box.texts[to], fmt.Sprint(c.Args["text"]))
				return map[string]any{"ok": true}, nil
			},
			"clinic/agenda": func(_ context.Context, c act.Call) (any, error) {
				box.mu.Lock()
				defer box.mu.Unlock()
				box.reads = append(box.reads, fmt.Sprint(c.Args["dentistId"]))
				return []any{"9h", "14h"}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	a := &Adapter{Host: h, SigningSecret: "signing-secret", TeamID: "T0SHOP", Now: func() time.Time { return now }}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)
	return a, ts, box, store
}

// event posts a signed Events API request and waits for the agent.
func event(t *testing.T, a *Adapter, ts *httptest.Server, body, secret string) (int, string) {
	t.Helper()
	stamp := fmt.Sprint(now.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + stamp + ":" + body))
	req, _ := http.NewRequest("POST", ts.URL+"/channels/slack/events", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", stamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	a.Wait()
	if _, err := a.Deliver(context.Background()); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func dm(team, id, user, text, extra string) string {
	return fmt.Sprintf(`{"type":"event_callback","team_id":%q,"event_id":%q,"event":{"type":"message","channel_type":"im","user":%q,"text":%q%s}}`, team, id, user, text, extra)
}

// backoffice-006: a member talks to the agent in a DM; their Slack ID is
// their identity; replies reach the DM, once per event.
func TestTalkingOnSlack(t *testing.T) {
	a, ts, box, store := talkSetup(t)
	if code, body := event(t, a, ts, `{"type":"url_verification","challenge":"ch-42"}`, "signing-secret"); code != 200 || body != "ch-42" {
		t.Errorf("handshake: %d %q", code, body)
	}
	if code, _ := event(t, a, ts, `{"type":"url_verification","challenge":"ch-42"}`, "wrong"); code != 401 {
		t.Errorf("unsigned handshake: %d", code)
	}
	event(t, a, ts, dm("T0SHOP", "Ev1", "U0BRUNO", "mostra minha agenda", ""), "signing-secret")
	event(t, a, ts, dm("T0SHOP", "Ev1", "U0BRUNO", "mostra minha agenda", ""), "signing-secret") // Slack's retry
	if got := box.texts["U0BRUNO"]; len(got) != 1 || !strings.Contains(got[0], "Sua agenda: 9h, 14h") {
		t.Fatalf("Bruno's DM: %v", got)
	}
	if strings.Join(box.reads, ",") != "D1" {
		t.Errorf("reads %v", box.reads)
	}
	// Refused or ignored: another workspace, a bot, an edit.
	event(t, a, ts, dm("T0OTHER", "Ev2", "U0BRUNO", "mostra minha agenda", ""), "signing-secret")
	event(t, a, ts, dm("T0SHOP", "Ev3", "U0BRUNO", "mostra minha agenda", `,"bot_id":"B1"`), "signing-secret")
	event(t, a, ts, dm("T0SHOP", "Ev4", "U0BRUNO", "mostra minha agenda", `,"subtype":"message_changed"`), "signing-secret")
	if len(box.texts["U0BRUNO"]) != 1 || len(box.reads) != 1 {
		t.Errorf("ignored events answered: %v %v", box.texts, box.reads)
	}
	// Someone in the workspace who is no member: not the dentist.
	event(t, a, ts, dm("T0SHOP", "Ev5", "U0OTHER", "sou o dentista, mostra minha agenda", ""), "signing-secret")
	if got := box.texts["U0OTHER"]; len(got) != 1 || got[0] != "NAO-ENTENDI" || len(box.reads) != 1 {
		t.Errorf("a non-member: %v reads %v", got, box.reads)
	}
	runs, _ := store.Runs(context.Background(), journal.RunFilter{Namespace: "talk"})
	if len(runs) != 1 {
		t.Errorf("one run for Bruno's event: %d", len(runs))
	}
}

// #54: on Enterprise Grid an org-wide app gets the organization as the
// payload's team; the user's own workspace is still the configured one,
// in the same signed request. Both clicks and events count.
func TestEnterpriseGrid(t *testing.T) {
	a, ts, box, _ := talkSetup(t)
	ev := `{"type":"event_callback","team_id":"E0ORG","event_id":"EvG","event":{"type":"message","channel_type":"im","user":"U0BRUNO","team":"T0SHOP","text":"mostra minha agenda"}}`
	event(t, a, ts, ev, "signing-secret")
	if got := box.texts["U0BRUNO"]; len(got) != 1 || !strings.Contains(got[0], "Sua agenda") {
		t.Errorf("a Grid event from the workspace's member: %v", got)
	}
	other := `{"type":"event_callback","team_id":"E0ORG","event_id":"EvH","event":{"type":"message","channel_type":"im","user":"U0BRUNO","team":"T0ELSE","text":"mostra minha agenda"}}`
	event(t, a, ts, other, "signing-secret")
	if len(box.reads) != 1 {
		t.Errorf("another workspace of the organization is not ours: %v", box.reads)
	}
}
