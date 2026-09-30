package email_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/channels/email"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
	"github.com/bbiangul/tessera/internal/server"
)

// E7: the raw message is RFC 822 with the member as To, the UTF-8 subject
// intact and the link in the body; a task with CR/LF and "Bcc:" adds no
// header.
func TestRawIsSafeRFC822(t *testing.T) {
	task := "Desconto de 10% para João?\r\nBcc: intruso@example.com\r\n\r\nclique aqui"
	subject := email.Subject(task)
	raw, err := email.Raw("Clínica <agente@clinica.example>", "rita@clinica.example", subject, email.Body("Rita", task, "https://x.example/tasks/tok"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.Header.Get("To"); got != "<rita@clinica.example>" {
		t.Errorf("To %q", got)
	}
	if from, err := msg.Header.AddressList("From"); err != nil || from[0].Name != "Clínica" || from[0].Address != "agente@clinica.example" {
		t.Errorf("From %v %v (raw %q)", from, err, msg.Header.Get("From"))
	}
	if len(msg.Header["Bcc"]) > 0 || len(msg.Header) != 6 {
		t.Errorf("headers: %v", msg.Header)
	}
	dec, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || dec != subject || !strings.HasPrefix(dec, "Aprovação pendente: Desconto de 10% para João? Bcc: intruso") {
		t.Errorf("subject %q (%v), want %q", dec, err, subject)
	}
	body, _ := io.ReadAll(msg.Body)
	text, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(body), "\r\n", ""))
	if err != nil || !strings.Contains(string(text), "https://x.example/tasks/tok") || !strings.Contains(string(text), "Bcc: intruso") {
		t.Errorf("body %q %v", text, err)
	}
}

const def = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: email, version: 0.1.0 }
spec:
  actions:
    send-task: { input: { to: string, subject: string, text: string, link: string, raw: string }, effect: external-message }
    send-code: { input: { to: string, text: string }, effect: external-message }
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
    - { id: rita, name: Rita, roles: [loja], email: rita@loja.example, contact: rita@loja.example }
    - { id: gil, name: Gil, roles: [loja], email: gil@loja.example, contact: gil@loja.example }
  verify: { act: email/send-code, with: { to: "${ inputs.contact }", text: "${ 'Código: ' + inputs.code }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: { roles: [loja], channels: [email] }
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

type mailbox struct {
	mu    sync.Mutex
	mails map[string][]string // to -> links
	codes map[string][]string // to -> codes
}

var linkRe = regexp.MustCompile(`https?://\S+/tasks/\S+`)

// E1–E4: the e-mail carries a link bound to its member; the page offers
// only them, a code goes only to them, only the code answers, once.
func TestApprovingByEmail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(def), 0o600); err != nil {
		t.Fatal(err)
	}
	box := &mailbox{mails: map[string][]string{}, codes: map[string][]string{}}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{
			"email/send-task": func(_ context.Context, c act.Call) (any, error) {
				box.mu.Lock()
				defer box.mu.Unlock()
				to := fmt.Sprint(c.Args["to"])
				box.mails[to] = append(box.mails[to], linkRe.FindString(fmt.Sprint(c.Args["text"])))
				return map[string]any{}, nil
			},
			"email/send-code": func(_ context.Context, c act.Call) (any, error) {
				box.mu.Lock()
				defer box.mu.Unlock()
				to := fmt.Sprint(c.Args["to"])
				box.codes[to] = append(box.codes[to], strings.TrimPrefix(fmt.Sprint(c.Args["text"]), "Código: "))
				return map[string]any{}, nil
			},
		}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	s := &server.Server{Host: h, Token: strings.Repeat("t", 32), LinkSecret: []byte("0123456789abcdef0123456789abcdef")}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	s.PublicURL = ts.URL
	a := &email.Adapter{Host: h, Link: s.MemberTaskLink}
	if err := a.Open(":memory:"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if n, err := a.DeliverTasks(ctx); err != nil || n != 2 {
		t.Fatalf("sent %d %v", n, err)
	}
	if n, _ := a.DeliverTasks(ctx); n != 0 {
		t.Error("a task is e-mailed to a member once")
	}
	link := box.mails["rita@loja.example"][0]
	if link == "" || link == box.mails["gil@loja.example"][0] {
		t.Fatalf("each member gets their own link: %v", box.mails)
	}
	post := func(form url.Values) (int, string) {
		resp, err := http.PostForm(link, form)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	get, _ := http.Get(link)
	page, _ := io.ReadAll(get.Body)
	_ = get.Body.Close()
	if !strings.Contains(string(page), ">Rita<") || strings.Contains(string(page), ">Gil<") {
		t.Fatalf("Rita's link offers only Rita: %s", page)
	}
	// E3: Gil cannot be asked a code on Rita's link.
	if code, _ := post(url.Values{"step": {"code"}, "member": {"gil"}}); code != 403 || len(box.codes["gil@loja.example"]) != 0 {
		t.Fatalf("Gil on Rita's link: %d, codes %v", code, box.codes)
	}
	// E2: whoever holds the link asks a code: it goes to Rita; a guess fails.
	if code, _ := post(url.Values{"step": {"code"}, "member": {"rita"}}); code != 200 || len(box.codes["rita@loja.example"]) != 1 {
		t.Fatalf("code: %d %v", code, box.codes)
	}
	if code, _ := post(url.Values{"step": {"answer"}, "member": {"rita"}, "code": {"000000"}, "decision": {"approve"}}); code != 403 {
		t.Fatalf("a guessed code: %d", code)
	}
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusWaitingForHuman {
		t.Fatal("answered without the code")
	}
	// E1: Rita's code answers, as Rita.
	if code, body := post(url.Values{"step": {"answer"}, "member": {"rita"}, "code": {box.codes["rita@loja.example"][0]}, "decision": {"approve"}}); code != 200 {
		t.Fatalf("Rita's answer: %d %s", code, body)
	}
	recs, _ := store.Records(ctx, res.RunID)
	var byRita bool
	for _, r := range recs {
		byRita = byRita || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"rita"`))
	}
	if run, _ := store.GetRun(ctx, res.RunID); run.Status != journal.StatusSucceeded || !byRita {
		t.Fatalf("status %s, by Rita %v", run.Status, byRita)
	}
	// E4: the link is spent.
	if code, _ := post(url.Values{"step": {"code"}, "member": {"rita"}}); code != 410 {
		t.Errorf("a spent link: %d", code)
	}
}

func TestOpenNeedsLinks(t *testing.T) {
	if err := (&email.Adapter{}).Open(":memory:"); err == nil {
		t.Error("without links an e-mail could not be answered")
	}
}
