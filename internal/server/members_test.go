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

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const memberDocs = approvals + `---
apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: shop }
spec:
  members:
    - { id: rita, name: Rita, roles: [loja], contact: "+5511999990001" }
    - { id: carlos, name: Carlos, roles: [estoque], contact: "+5511999990002" }
  verify: { act: msg/code, with: { to: "${ inputs.contact }", text: "${ 'Seu código: ' + inputs.code }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: loja, namespace: shop }
spec: { roles: [loja] }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: msg, version: 0.1.0 }
spec:
  actions:
    code: { input: { to: string, text: string }, effect: external-message }
`

// RFC-0006: on a queue with roles, a task page answers only with the
// one-time code sent to the member chosen; a forwarded link, a wrong or
// reused code, or a non-member, answer nothing.
func TestTaskPagesAuthenticateMembers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shop.yaml"), []byte(memberDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	var sent []map[string]any
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{"msg/code": func(_ context.Context, c act.Call) (any, error) {
			sent = append(sent, c.Args)
			return map[string]any{}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	s := &Server{Host: h, Token: "tok", LinkSecret: []byte("0123456789abcdef0123456789abcdef")}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	s.PublicURL = ts.URL + "/"
	ctx := context.Background()
	res, err := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	link := s.TaskLink(task)
	do := func(form url.Values) (int, string) {
		var resp *http.Response
		var err error
		if form == nil {
			resp, err = http.Get(link)
		} else {
			resp, err = http.PostForm(link, form)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := do(nil); code != 200 || !strings.Contains(body, `value="rita"`) || strings.Contains(body, "carlos") {
		t.Fatalf("the page lists the queue's members only: %d %s", code, body)
	}
	if code, _ := do(url.Values{"decision": {"approve"}, "by": {"Rita"}}); code != http.StatusForbidden {
		t.Errorf("a typed name answers nothing: %d", code)
	}
	if code, _ := do(url.Values{"step": {"code"}, "member": {"carlos"}}); code != http.StatusForbidden || len(sent) != 0 {
		t.Errorf("a non-member gets no code: %d %v", code, sent)
	}
	if code, _ := do(url.Values{"step": {"code"}, "member": {"rita"}}); code != 200 || len(sent) != 1 || sent[0]["to"] != "+5511999990001" {
		t.Fatalf("the code goes to Rita's contact: %d %v", code, sent)
	}
	codeSent := strings.TrimPrefix(sent[0]["text"].(string), "Seu código: ")
	if code, _ := do(url.Values{"step": {"answer"}, "member": {"rita"}, "code": {"000000"}, "decision": {"approve"}}); code != http.StatusForbidden {
		t.Errorf("a wrong code: %d", code)
	}
	if code, body := do(url.Values{"step": {"answer"}, "member": {"rita"}, "code": {codeSent}, "decision": {"approve"}}); code != 200 || !strings.Contains(body, "Rita") {
		t.Fatalf("the right code answers: %d %s", code, body)
	}
	recs, _ := store.Records(ctx, res.RunID)
	var byRita bool
	for _, r := range recs {
		byRita = byRita || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"rita"`))
	}
	if !byRita {
		t.Error("the answer is recorded as the member's id")
	}
}

// Codes are rationed per task: a link holder cannot ask for codes without
// end, however many links the task has.
func TestCodeRequestsAreRationed(t *testing.T) {
	var c codes
	for i := 0; i < codesPerMember; i++ {
		if !c.allow("task", "rita") {
			t.Fatalf("request %d refused", i)
		}
	}
	if c.allow("task", "rita") {
		t.Error("a member's allowance per task is bounded")
	}
	n := codesPerMember
	for _, m := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		if c.allow("task", m) {
			n++
		}
	}
	if n != codesPerTask {
		t.Errorf("the task's allowance is bounded: %d codes", n)
	}
	if !c.allow("other-task", "rita") {
		t.Error("another task has its own allowance")
	}
}
