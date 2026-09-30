package whatsapp

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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type graph struct {
	mu        sync.Mutex
	texts     []string
	templates []string
	fail      []error // the next send-text calls fail with these, in order
}

func setup(t *testing.T) (*Adapter, *httptest.Server, *graph) {
	return setupStore(t, journal.NewMemory())
}

func setupStore(t *testing.T, store journal.Store) (*Adapter, *httptest.Server, *graph) {
	t.Helper()
	g := &graph{}
	h, issues, err := host.Load(host.Options{Paths: []string{"../../../testdata/cadencia"}, Namespace: "sdr", Store: store,
		Decider: &decidetest.Scripted{Answers: map[string]map[string]float64{}},
		Handlers: map[string]act.Handler{
			"whatsapp/send-text": func(_ context.Context, c act.Call) (any, error) {
				g.mu.Lock()
				defer g.mu.Unlock()
				if len(g.fail) > 0 {
					err := g.fail[0]
					g.fail = g.fail[1:]
					return nil, err
				}
				g.texts = append(g.texts, fmt.Sprint(c.Args["to"], ": ", c.Args["text"]))
				return map[string]any{}, nil
			},
			"whatsapp/send-template": func(_ context.Context, c act.Call) (any, error) {
				g.mu.Lock()
				defer g.mu.Unlock()
				g.templates = append(g.templates, fmt.Sprint(c.Args["to"], ": ", c.Args["template"]))
				return map[string]any{}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	a := &Adapter{Host: h, VerifyToken: "vt", AppSecret: "app-secret", Plugin: "whatsapp"}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)
	return a, ts, g
}

func post(t *testing.T, ts *httptest.Server, body []byte, secret string) int {
	t.Helper()
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	req, _ := http.NewRequest("POST", ts.URL+"/channels/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func text(id, from, body string) []byte {
	return []byte(fmt.Sprintf(`{"entry":[{"changes":[{"value":{"contacts":[{"wa_id":%q,"profile":{"name":"Ana"}}],
		"messages":[{"id":%q,"from":%q,"type":"text","text":{"body":%q}}]}}]}]}`, from, id, from, body))
}

func TestHandshake(t *testing.T) {
	_, ts, _ := setup(t)
	for token, want := range map[string]int{"vt": http.StatusOK, "wrong": http.StatusForbidden} {
		resp, err := http.Get(ts.URL + "/channels/whatsapp/webhook?hub.mode=subscribe&hub.challenge=42&hub.verify_token=" + token)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("token %s: %d", token, resp.StatusCode)
		}
		if want == http.StatusOK && resp.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("content type %q", resp.Header.Get("Content-Type"))
		}
	}
	resp, err := http.Get(ts.URL + "/channels/whatsapp/webhook?hub.mode=subscribe&hub.verify_token=vt&hub.challenge=" + url.QueryEscape("<script>alert(1)</script>"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a challenge that is not a number is not echoed: %d", resp.StatusCode)
	}
}

// A lead from the CRM gets the first template; their WhatsApp reply
// reaches the waiting cadence, and the answer goes out once.
func TestReplyReachesTheCadence(t *testing.T) {
	a, ts, g := setup(t)
	ctx := context.Background()
	if _, err := a.Host.StartHook(ctx, "crm-novo-lead", map[string]any{"leadId": "L1", "phone": "5511999990000", "name": "Ana", "optIn": true}, "k1"); err != nil {
		t.Fatal(err)
	}
	if len(g.templates) != 1 {
		t.Fatalf("first contact: %q", g.templates)
	}
	if code := post(t, ts, text("wamid.1", "5511999990000", "oi, pode falar"), "wrong-secret"); code != http.StatusUnauthorized {
		t.Fatalf("a bad signature must be refused: %d", code)
	}
	for range 2 { // Meta delivers twice
		if code := post(t, ts, text("wamid.1", "5511999990000", "oi, pode falar"), "app-secret"); code != http.StatusOK {
			t.Fatalf("delivery: %d", code)
		}
	}
	n, err := a.Deliver(ctx)
	if err != nil || n != 1 {
		t.Fatalf("deliver: %d %v", n, err)
	}
	if len(g.texts) != 1 || g.texts[0] != "5511999990000: Que bom falar com você! Posso te fazer duas perguntas rápidas para entender se faz sentido para vocês?" {
		t.Errorf("sent %q", g.texts)
	}
	if n, _ := a.Deliver(ctx); n != 0 {
		t.Errorf("a delivered message is not sent again: %d", n)
	}
	runs, _ := a.Host.Runtime.Store.Runs(ctx, journal.RunFilter{Conversation: "wa:5511999990000"})
	if len(runs) != 1 || runs[0].Status != journal.StatusSucceeded {
		t.Errorf("the reply must reach the cadence's run, once: %+v", runs)
	}
}

// The same message delivered twice is handled once: a message nobody
// expected gets one "not understood", not two.
func TestDuplicateDeliveriesAreHandledOnce(t *testing.T) {
	a, ts, g := setup(t)
	for range 3 {
		if code := post(t, ts, text("wamid.9", "5511988887777", "oi"), "app-secret"); code != http.StatusOK {
			t.Fatalf("delivery: %d", code)
		}
	}
	if _, err := a.Deliver(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(g.texts) != 1 {
		t.Fatalf("handled once: %q", g.texts)
	}
}

func TestOtherNumbersAreIgnored(t *testing.T) {
	a, ts, g := setup(t)
	a.PhoneNumberID = "111"
	body := func(id, number string) []byte {
		return []byte(fmt.Sprintf(`{"entry":[{"changes":[{"value":{"metadata":{"phone_number_id":%q},
			"messages":[{"id":%q,"from":"5511988887777","type":"text","text":{"body":"oi"}}]}}]}]}`, number, id))
	}
	post(t, ts, body("wamid.a", "222"), "app-secret")
	post(t, ts, body("wamid.b", "111"), "app-secret")
	if _, err := a.Deliver(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(g.texts) != 1 {
		t.Fatalf("only the agent's own number: %q", g.texts)
	}
}

// Messages to the team's queues in a WhatsApp conversation never reach
// the user.
func TestQueueMessagesStayWithTheTeam(t *testing.T) {
	a, _, g := setup(t)
	ctx := context.Background()
	res, err := a.Host.StartHook(ctx, "crm-novo-lead", map[string]any{"leadId": "L1", "phone": "5511999990000", "name": "Ana", "optIn": true}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := journal.Encode(map[string]any{"to": "queue:vendas", "text": "dados internos do lead"})
	if _, err := a.Host.Runtime.Store.Append(ctx, res.RunID, journal.Record{Site: "manual/team", Kind: journal.KindSend, Value: v, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	if len(g.texts) != 0 {
		t.Fatalf("sent to the user: %q", g.texts)
	}
	if runs, _ := a.Host.Runtime.Store.Unacked(ctx, ConversationPrefix, 10); len(runs) != 0 {
		t.Errorf("the team's message is acknowledged, not left pending: %d runs", len(runs))
	}
}

// A send that fails is tried again later; one Meta refuses for good is
// given up; neither holds back other conversations.
func TestFailedSends(t *testing.T) {
	a, ts, g := setup(t)
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	ctx := context.Background()
	for i := 1; i <= 3; i++ { // three leads answer the cadence
		phone := fmt.Sprintf("551100000000%d", i)
		if _, err := a.Host.StartHook(ctx, "crm-novo-lead", map[string]any{"leadId": phone, "phone": phone, "name": "Ana", "optIn": true}, phone); err != nil {
			t.Fatal(err)
		}
		post(t, ts, text("wamid."+phone, phone, "oi, pode falar"), "app-secret")
	}
	g.fail = []error{&act.AmbiguousError{Message: "timeout"}, &act.DefiniteError{Code: "INVALID", Message: "blocked"}}
	n, err := a.Deliver(ctx)
	if n != 1 || err == nil || !strings.Contains(err.Error(), "given up") || !strings.Contains(err.Error(), "again in 1m") {
		t.Fatalf("one sent, one to retry, one given up: %d %v %q", n, err, g.texts)
	}
	if n, err := a.Deliver(ctx); n != 0 || err != nil {
		t.Fatalf("the retry waits: %d %v", n, err)
	}
	now = now.Add(2 * time.Minute)
	if n, err := a.Deliver(ctx); n != 1 || err != nil {
		t.Fatalf("the retry: %d %v", n, err)
	}
	if n, _ := a.Deliver(ctx); n != 0 || len(g.texts) != 2 || !strings.HasPrefix(g.texts[0], "5511000000003") && !strings.HasPrefix(g.texts[1], "5511000000001") {
		t.Fatalf("each message once: %q", g.texts)
	}
}

// A message marked as being sent when the process stopped is not sent
// again: Meta has no idempotency key, a duplicate would reach the user.
func TestNoResendAfterACrash(t *testing.T) {
	a, ts, g := setup(t)
	ctx := context.Background()
	if _, err := a.Host.StartHook(ctx, "crm-novo-lead", map[string]any{"leadId": "L1", "phone": "5511999990000", "name": "Ana", "optIn": true}, "k1"); err != nil {
		t.Fatal(err)
	}
	post(t, ts, text("wamid.1", "5511999990000", "oi, pode falar"), "app-secret")
	runs, _ := a.Host.Runtime.Store.Unacked(ctx, ConversationPrefix, 10)
	msgs, _ := a.Host.Runtime.Outbox(ctx, runs[0].ID)
	if _, err := a.db.ExecContext(ctx, `INSERT INTO sends (message, state, at) VALUES (?, 'sending', '')`, msgs[len(msgs)-1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	if len(g.texts) != 0 {
		t.Fatalf("sent again after a crash: %q", g.texts)
	}
}

// A message that could not be handled is answered 500 and forgotten, so
// Meta's retry handles it; the others in the delivery are not handled twice.
func TestHandlingFailureIsRetried(t *testing.T) {
	store := &failingStore{Store: journal.NewMemory(), down: true}
	_, ts, g := setupStore(t, store)
	if code := post(t, ts, text("wamid.7", "5511977776666", "oi"), "app-secret"); code != http.StatusInternalServerError {
		t.Fatalf("a failure must ask Meta to retry: %d", code)
	}
	store.down = false
	if code := post(t, ts, text("wamid.7", "5511977776666", "oi"), "app-secret"); code != http.StatusOK {
		t.Fatalf("the retry: %d", code)
	}
	if len(g.texts) != 1 {
		t.Fatalf("the retried message is handled: %q", g.texts)
	}
}

// failingStore fails reading conversations while down, as a database
// that is unavailable.
type failingStore struct {
	journal.Store
	down bool
}

func (f *failingStore) ActiveRun(ctx context.Context, conversation string) (journal.Run, bool, error) {
	if f.down {
		return journal.Run{}, false, fmt.Errorf("database is down")
	}
	return f.Store.ActiveRun(ctx, conversation)
}
