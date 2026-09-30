package teams_test

import (
	"bytes"
	"context"
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
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const (
	appID    = "11111111-2222-3333-4444-555555555555"
	tenant   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	rita     = "0a0a0a0a-0000-0000-0000-000000000001"
	gil      = "0a0a0a0a-0000-0000-0000-000000000002"
	svc      = "https://smba.trafficmanager.net/teams/"
	teamsDef = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: teams, version: 0.1.0 }
spec:
  actions:
    open-dm: { input: { user: string, tenant: string, bot: string }, output: map, effect: idempotent-write }
    post-task: { input: { conversation: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { conversation: string, activity: string, text: string }, effect: external-message }
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
    - { id: rita, name: Rita, roles: [loja], teams: 0A0A0A0A-0000-0000-0000-000000000001 }
    - { id: gil, name: Gil, roles: [gerencia], teams: 0a0a0a0a-0000-0000-0000-000000000002 }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: { roles: [loja], channels: [teams] }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: gerencia, namespace: shop }
spec: { roles: [gerencia], channels: [teams] }
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
)

type bot struct {
	mu      sync.Mutex
	dms     []string // users a chat was opened with
	cards   []string // "conversation: text [ids]"
	updates []string // "conversation/activity: text"
}

func (b *bot) updated() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.updates, "\n")
}

func setup(t *testing.T) (*teams.Adapter, *httptest.Server, *teamstest.BotFramework, *bot, *host.Host, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(teamsDef), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &bot{}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{
			"teams/open-dm": func(_ context.Context, c act.Call) (any, error) {
				b.mu.Lock()
				defer b.mu.Unlock()
				b.dms = append(b.dms, fmt.Sprint(c.Args["user"]))
				return map[string]any{"id": "a:dm-" + fmt.Sprint(c.Args["user"])}, nil
			},
			"teams/post-task": func(_ context.Context, c act.Call) (any, error) {
				b.mu.Lock()
				defer b.mu.Unlock()
				var ids []string
				for _, x := range c.Args["buttons"].([]any) {
					ids = append(ids, x.(map[string]any)["id"].(string))
				}
				b.cards = append(b.cards, fmt.Sprint(c.Args["conversation"], ": ", c.Args["text"], " ", ids))
				return map[string]any{"id": "card-1"}, nil
			},
			"teams/update-task": func(_ context.Context, c act.Call) (any, error) {
				b.mu.Lock()
				defer b.mu.Unlock()
				b.updates = append(b.updates, fmt.Sprint(c.Args["conversation"], "/", c.Args["activity"], ": ", c.Args["text"]))
				return map[string]any{}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	bf := teamstest.New()
	t.Cleanup(bf.Close)
	a := &teams.Adapter{Host: h, AppID: appID, TenantID: tenant, OpenIDURL: bf.OpenIDURL(), Client: bf.Client()}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)
	return a, ts, bf, b, h, store
}

func deliver(t *testing.T, a *teams.Adapter, ts *httptest.Server, auth string, activity []byte) int {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/channels/teams/messages", bytes.NewReader(activity))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	a.Wait()
	return resp.StatusCode
}

// backoffice-004: the task reaches the member's personal chat as a card;
// only a click the Bot Framework signs, for this bot, from the member in
// the tenant, answers it, once; the card then shows the outcome.
func TestApprovingOnTeams(t *testing.T) {
	a, ts, bf, b, h, store := setup(t)
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	if n, err := a.DeliverTasks(ctx); err != nil || n != 1 {
		t.Fatalf("sent %d %v", n, err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 0 {
		t.Error("a task goes to a member once")
	}
	if len(b.cards) != 1 || !strings.HasPrefix(b.cards[0], "a:dm-0A0A0A0A-0000-0000-0000-000000000001: Desconto de 10% para P1?") {
		t.Fatalf("only Rita gets the card: %v (chats %v)", b.cards, b.dms)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	approve := "task:" + task.Hash + ":approve"
	good := teamstest.Token{AppID: appID, ServiceURL: svc, Expires: time.Now().Add(time.Hour)}
	with := func(f func(*teamstest.Token)) string {
		tok := good
		f(&tok)
		return bf.Sign(tok)
	}
	click := func(user, tn, card string) []byte { return teamstest.Click(svc, tn, user, card, approve) }

	// Refused before anything (T5).
	for name, auth := range map[string]string{
		"no token":            "",
		"unknown key":         with(func(t *teamstest.Token) { t.Kid = teamstest.Unknown }),
		"key not for msteams": with(func(t *teamstest.Token) { t.Kid = teamstest.Other }),
		"expired":             with(func(t *teamstest.Token) { t.Expires = time.Now().Add(-10 * time.Minute) }),
		"another bot":         with(func(t *teamstest.Token) { t.AppID = "99999999-2222-3333-4444-555555555555" }),
		"another issuer":      with(func(t *teamstest.Token) { t.Issuer = "https://evil.example" }),
		"another serviceUrl":  with(func(t *teamstest.Token) { t.ServiceURL = "https://evil.example/" }),
	} {
		if code := deliver(t, a, ts, auth, click(rita, tenant, "c0")); code != 401 {
			t.Errorf("%s: %d", name, code)
		}
	}
	// Signed, but not the answer: another tenant with Rita's ID, a
	// stranger, a member without the role (T2–T4).
	deliver(t, a, ts, bf.Sign(good), click(rita, "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee", "c1"))
	deliver(t, a, ts, bf.Sign(good), click("0a0a0a0a-0000-0000-0000-000000000099", tenant, "c2"))
	deliver(t, a, ts, bf.Sign(good), click(gil, tenant, "c3"))
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusWaitingForHuman {
		t.Fatalf("answered by the wrong person: %s", run.Status)
	}
	// Rita's click (her ID in lower case) answers; again, it does not.
	deliver(t, a, ts, bf.Sign(good), click(rita, tenant, "c4"))
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
	deliver(t, a, ts, bf.Sign(good), click(rita, tenant, "c5"))
	got := b.updated()
	for _, want := range []string{"/c2: Você não está na equipe", "/c3: Você não pode responder", "/c4: ✅ Aprovado por Rita.", "/c5: Esta tarefa já foi respondida"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "/c1:") || strings.Contains(got, "/c0:") {
		t.Errorf("refused clicks are not answered: %s", got)
	}
}

// A forwarded task reaches the manager's chat; the manager's denial ends
// the run without the Function going on (#53).
func TestAForwardedTaskReachesTheManagersChat(t *testing.T) {
	a, ts, bf, b, h, store := setup(t)
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if _, err := a.DeliverTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Runtime.Forward(ctx, res.RunID, "gerencia", "rita"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 1 || !strings.HasPrefix(b.cards[len(b.cards)-1], "a:dm-"+gil+":") {
		t.Fatalf("the manager gets it: %d %v", n, b.cards)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	tok := bf.Sign(teamstest.Token{AppID: appID, ServiceURL: svc, Expires: time.Now().Add(time.Hour)})
	deliver(t, a, ts, tok, teamstest.Click(svc, tenant, gil, "g1", "task:"+task.Hash+":deny"))
	if run, _ := store.GetRun(ctx, res.RunID); run.Status == journal.StatusWaitingForHuman {
		t.Fatal("Gil's denial did not answer")
	}
	if !strings.Contains(b.updated(), "/g1: ❌ Recusado por Gil.") {
		t.Errorf("updates: %s", b.updated())
	}
	if len(b.dms) != 2 {
		t.Errorf("one chat opened per member: %v", b.dms)
	}
}

func TestOpenNeedsTheBotAndTheTenant(t *testing.T) {
	if err := (&teams.Adapter{AppID: appID}).Open(":memory:"); err == nil {
		t.Error("without the tenant any organization's users would count")
	}
	if err := (&teams.Adapter{AppID: appID, TenantID: tenant, OpenIDURL: "http://keys.example"}).Open(":memory:"); err == nil {
		t.Error("keys over plain http")
	}
}
