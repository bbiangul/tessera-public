package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const teamOnSlack = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: loja, namespace: shop }
spec:
  humanInTheLoop: { queue: loja }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: shop }
spec:
  members:
    - { id: rita, name: Rita, roles: [loja], slack: U0RITA }
    - { id: gil, name: Gil, roles: [gerencia], slack: U0GIL }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: { roles: [loja], channels: [slack] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: shop }
spec: { roles: [gerencia], channels: [slack] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: pedido, namespace: shop }
spec: { function: aprovar, webhook: { secret: PEDIDO } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: aprovar, namespace: shop }
spec:
  description: Asks the floor to approve a discount.
  authorize: "has(caller.trigger)"
  steps:
    - { id: human, escalate: { queue: loja, task: approve, message: "Desconto de 10% para P1?" } }
  output: { value: "steps.human.output" }
`

var now = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

type workspace struct {
	mu      sync.Mutex
	posts   []string // "to: text [ids]"
	updates []string // "channel/ts: text"
}

func (w *workspace) updated() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.updates, "\n")
}

func setup(t *testing.T) (*Adapter, *httptest.Server, *workspace, *host.Host, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(teamOnSlack), 0o600); err != nil {
		t.Fatal(err)
	}
	ws := &workspace{}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{
			"slack/post-task": func(_ context.Context, c act.Call) (any, error) {
				ws.mu.Lock()
				defer ws.mu.Unlock()
				var ids []string
				for _, b := range c.Args["buttons"].([]any) {
					ids = append(ids, b.(map[string]any)["id"].(string))
				}
				ws.posts = append(ws.posts, fmt.Sprint(c.Args["to"], ": ", c.Args["text"], " ", ids))
				return map[string]any{"ok": true}, nil
			},
			"slack/update-task": func(_ context.Context, c act.Call) (any, error) {
				ws.mu.Lock()
				defer ws.mu.Unlock()
				ws.updates = append(ws.updates, fmt.Sprint(c.Args["channel"], "/", c.Args["ts"], ": ", c.Args["text"]))
				return map[string]any{"ok": true}, nil
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
	return a, ts, ws, h, store
}

// click is a block_actions request from user in team on message ts.
func click(team, user, ts, actionID string) []byte {
	payload := fmt.Sprintf(`{"type":"block_actions","user":{"id":%q,"team_id":%q},"team":{"id":%q},
		"container":{"type":"message","channel_id":"D0DM","message_ts":%q},"actions":[{"action_id":%q,"type":"button"}]}`, user, team, team, ts, actionID)
	return []byte(url.Values{"payload": {payload}}.Encode())
}

// send posts body signed with secret at time at, and waits for the answer.
func send(t *testing.T, a *Adapter, ts *httptest.Server, body []byte, secret string, at time.Time) int {
	t.Helper()
	stamp := fmt.Sprint(at.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + stamp + ":"))
	mac.Write(body)
	req, _ := http.NewRequest("POST", ts.URL+"/channels/slack/interactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", stamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	a.Wait()
	return resp.StatusCode
}

// backoffice-003: the task reaches the member's Slack DM with buttons; only
// that member's click, signed by Slack and from the workspace, answers it,
// once; the message then shows the outcome.
func TestApprovingOnSlack(t *testing.T) {
	a, ts, ws, h, store := setup(t)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	if n, err := a.DeliverTasks(ctx); err != nil || n != 1 {
		t.Fatalf("posted %d %v", n, err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 0 {
		t.Error("a task goes to a member once")
	}
	if len(ws.posts) != 1 || !strings.HasPrefix(ws.posts[0], "U0RITA: Desconto de 10% para P1?") {
		t.Fatalf("only Rita gets the buttons: %v", ws.posts)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	approve := "task:" + task.Hash + ":approve"

	// Refused before anything: a forged signature, a replayed old request.
	if code := send(t, a, ts, click("T0SHOP", "U0RITA", "1.1", approve), "wrong", now); code != 401 {
		t.Errorf("forged: %d", code)
	}
	if code := send(t, a, ts, click("T0SHOP", "U0RITA", "1.1", approve), "signing-secret", now.Add(-6*time.Minute)); code != 401 {
		t.Errorf("replayed: %d", code)
	}
	// Signed, but not the answer: another workspace with Rita's ID, a
	// stranger, a member without the role.
	send(t, a, ts, click("T0OTHER", "U0RITA", "1.2", approve), "signing-secret", now)
	send(t, a, ts, click("T0SHOP", "U0STRANGER", "1.3", approve), "signing-secret", now)
	send(t, a, ts, click("T0SHOP", "U0GIL", "1.4", approve), "signing-secret", now)
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusWaitingForHuman {
		t.Fatalf("answered by the wrong person: %s", run.Status)
	}
	// Rita's click answers; the same click again does not.
	send(t, a, ts, click("T0SHOP", "U0RITA", "1.5", approve), "signing-secret", now)
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusSucceeded {
		t.Fatalf("Rita's click did not answer: %s", run.Status)
	}
	recs, _ := store.Records(ctx, res.RunID)
	var byRita bool
	for _, r := range recs {
		byRita = byRita || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"rita"`))
	}
	if !byRita {
		t.Error("the answer is Rita's")
	}
	send(t, a, ts, click("T0SHOP", "U0RITA", "1.6", approve), "signing-secret", now)
	got := ws.updated()
	for _, want := range []string{"D0DM/1.3: Você não está na equipe", "D0DM/1.4: Você não pode responder", "D0DM/1.5: ✅ Aprovado por Rita.", "D0DM/1.6: Esta tarefa já foi respondida"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "D0DM/1.2") {
		t.Error("a click from another workspace is not even answered")
	}
}

// Forwarded to the manager's queue, the task reaches the manager's DM, and
// the manager's denial is recorded as theirs.
func TestAForwardedTaskReachesTheManagersDM(t *testing.T) {
	a, ts, ws, h, store := setup(t)
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if _, err := a.DeliverTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Runtime.Forward(ctx, res.RunID, "gerencia", "rita"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 1 || !strings.HasPrefix(ws.posts[len(ws.posts)-1], "U0GIL:") {
		t.Fatalf("the manager gets it: %d %v", n, ws.posts)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	send(t, a, ts, click("T0SHOP", "U0GIL", "2.1", "task:"+task.Hash+":deny"), "signing-secret", now)
	run, _ := store.GetRun(ctx, res.RunID)
	if run.Status == journal.StatusWaitingForHuman {
		t.Fatalf("Gil's denial did not answer")
	}
	if !strings.Contains(ws.updated(), "D0DM/2.1: ❌ Recusado por Gil.") {
		t.Errorf("updates: %s", ws.updated())
	}
}

func TestOpenNeedsTheWorkspace(t *testing.T) {
	if err := (&Adapter{SigningSecret: "s"}).Open(":memory:"); err == nil {
		t.Error("without the team ID any workspace's user IDs would count")
	}
}

// #54: a click on Enterprise Grid names the organization as team and the
// user's workspace as user.team_id: it answers.
func TestEnterpriseGridClick(t *testing.T) {
	a, ts, _, h, store := setup(t)
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if _, err := a.DeliverTasks(ctx); err != nil {
		t.Fatal(err)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	payload := fmt.Sprintf(`{"type":"block_actions","user":{"id":"U0RITA","team_id":"T0SHOP"},"team":{"id":"E0ORG"},
		"container":{"type":"message","channel_id":"D0DM","message_ts":"g.1"},"actions":[{"action_id":%q}]}`, "task:"+task.Hash+":approve")
	send(t, a, ts, []byte(url.Values{"payload": {payload}}.Encode()), "signing-secret", now)
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusSucceeded {
		t.Errorf("a Grid click from the workspace's member: %s", run.Status)
	}
}
