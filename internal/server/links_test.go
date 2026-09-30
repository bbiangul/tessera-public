package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
	"github.com/bbiangul/tessera/internal/tasklink"
)

const approvals = `apiVersion: agents.tessera.dev/v1alpha1
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
    - { id: human, escalate: { queue: loja, task: approve, message: "Desconto de <b>10%</b>?" } }
    - { id: frete, escalate: { queue: loja, task: approve, message: "Frete grátis?" } }
  output: { template: "{{ steps.human.output }}" }
`

func TestTaskLinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shop.yaml"), []byte(approvals), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	s := &Server{Host: h, Token: "tok", LinkSecret: []byte("0123456789abcdef0123456789abcdef")}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	s.PublicURL = ts.URL + "/"
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil || res.Status != journal.StatusWaitingForHuman {
		t.Fatalf("%+v %v", res, err)
	}
	task, err := h.Runtime.PendingTask(ctx, res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	link := s.TaskLink(task)
	if !strings.HasPrefix(link, ts.URL+"/tasks/") {
		t.Fatalf("link %q", link)
	}
	get := func(u string) (int, string) {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	post := func(u string, form url.Values) (int, string) {
		resp, err := http.PostForm(u, form)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	code, body := get(link)
	if code != 200 || !strings.Contains(body, "Desconto de &lt;b&gt;10%&lt;/b&gt;?") || !strings.Contains(body, `value="approve"`) {
		t.Fatalf("page %d %s", code, body)
	}
	if r, _ := store.GetRun(ctx, res.RunID); r.Status != journal.StatusWaitingForHuman {
		t.Fatal("opening the page must not answer the task")
	}
	if code, _ := get(link[:len(link)-2] + "xx"); code != http.StatusGone {
		t.Errorf("tampered link: %d", code)
	}
	if code, _ := post(link, url.Values{"decision": {"approve"}}); code != http.StatusBadRequest {
		t.Errorf("an answer needs a name: %d", code)
	}
	if code, body := post(link, url.Values{"decision": {"approve"}, "by": {"Ana"}}); code != 200 || !strings.Contains(body, "Recorded") {
		t.Fatalf("answer %d %s", code, body)
	}
	r, _ := store.GetRun(ctx, res.RunID)
	if r.Status != journal.StatusWaitingForHuman || r.Pending == task.Site {
		t.Fatalf("the run moved to its next task: %s %s", r.Status, r.Pending)
	}
	recs, _ := store.Records(ctx, res.RunID)
	found := false
	for _, rec := range recs {
		if rec.Kind == journal.KindResume && strings.Contains(string(rec.Value), "link: Ana") {
			found = true
		}
	}
	if !found {
		t.Error("the resume records who answered")
	}
	if code, _ := post(link, url.Values{"decision": {"deny"}, "by": {"Bia"}}); code != http.StatusGone {
		t.Errorf("a link answers once: %d", code)
	}
	if code, body := get(link); code != http.StatusGone || strings.Contains(body, "Frete") {
		t.Errorf("an answered task's link must not show the run's next task: %d", code)
	}
	resp, _ := http.Get(link)
	_ = resp.Body.Close()
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("cache %q", resp.Header.Get("Cache-Control"))
	}

	// Links last 7 days.
	next, _ := h.Runtime.PendingTask(ctx, res.RunID)
	tok := strings.TrimPrefix(s.TaskLink(next), s.PublicURL+"tasks/")
	if _, err := tasklink.Verify(s.LinkSecret, tok, time.Now().Add(LinkTTL-time.Minute)); err != nil {
		t.Errorf("a link works for 7 days: %v", err)
	}
	if _, err := tasklink.Verify(s.LinkSecret, tok, time.Now().Add(LinkTTL+time.Minute)); err == nil {
		t.Error("and not after")
	}
	if LinkTTL != 7*24*time.Hour {
		t.Errorf("LinkTTL %s", LinkTTL)
	}
	// A server without a link secret refuses every link, even one signed
	// with an empty key.
	bare := httptest.NewServer((&Server{Host: h, Token: "tok"}).Handler())
	defer bare.Close()
	forged := tasklink.Sign(nil, tasklink.Claims{RunID: next.RunID, Hash: next.Hash, Exp: time.Now().Add(time.Hour).Unix()})
	if code, _ := post(bare.URL+"/tasks/"+forged, url.Values{"decision": {"approve"}, "by": {"x"}}); code != http.StatusGone {
		t.Errorf("a link signed with an empty key: %d", code)
	}
	if (&Server{Host: h}).TaskLink(runtime.Task{}) != "" {
		t.Error("no link without a public URL")
	}
}
