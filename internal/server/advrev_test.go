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

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func advSetup(t *testing.T) (*Server, *host.Host, journal.Store, string, *[]map[string]any) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shop.yaml"), []byte(memberDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	sent := &[]map[string]any{}
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{"msg/code": func(_ context.Context, c act.Call) (any, error) {
			*sent = append(*sent, c.Args)
			return map[string]any{}, nil
		}}})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	s := &Server{Host: h, Token: "tok", LinkSecret: []byte("0123456789abcdef0123456789abcdef")}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	s.PublicURL = ts.URL + "/"
	res, err := h.StartHook(context.Background(), "pedido", map[string]any{}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	return s, h, store, res.RunID, sent
}

func post(t *testing.T, link string, form url.Values) (int, string) {
	resp, err := http.PostForm(link, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// ADV-1: the code allowance is per link token, not per (task, member):
// every distinct token for the same task (notification created/remind/
// escalate links, the e-mail link, any re-signing a second later) brings a
// fresh 3 codes x 5 tries for the same member within the same hour.
func TestAdvRationingIsPerTokenNotPerMember(t *testing.T) {
	s, h, _, runID, sent := advSetup(t)
	task, _ := h.Runtime.PendingTask(context.Background(), runID)
	links := []string{s.TaskLink(task), s.MemberTaskLink(task, "rita")}
	time.Sleep(1100 * time.Millisecond) // Exp is in seconds: a new second, a new token
	links = append(links, s.TaskLink(task), s.MemberTaskLink(task, "rita"))
	for _, l := range links {
		for {
			code, _ := post(t, l, url.Values{"step": {"code"}, "member": {"rita"}})
			if code != 200 {
				break
			}
		}
	}
	t.Logf("distinct tokens: %d; codes sent to rita for ONE task within one hour: %d (codesPerMember=%d) => %d guesses/hour",
		len(links), len(*sent), codesPerMember, len(*sent)*codeTries)
	if len(*sent) > codesPerMember {
		t.Errorf("DEFECT: rationing per member is bypassed by distinct tokens: %d codes to rita", len(*sent))
	}
}

// ADV-2: the code is consumed before the form is validated: a POST with
// the right code but no decision (double-click, a missing button value,
// a proxy stripping it) burns the code.
func TestAdvCodeBurnedByInvalidDecision(t *testing.T) {
	s, h, _, runID, sent := advSetup(t)
	task, _ := h.Runtime.PendingTask(context.Background(), runID)
	link := s.MemberTaskLink(task, "rita")
	if code, _ := post(t, link, url.Values{"step": {"code"}, "member": {"rita"}}); code != 200 {
		t.Fatal(code)
	}
	c := strings.TrimPrefix((*sent)[0]["text"].(string), "Seu código: ")
	code1, _ := post(t, link, url.Values{"step": {"answer"}, "member": {"rita"}, "code": {c}})
	code2, body := post(t, link, url.Values{"step": {"answer"}, "member": {"rita"}, "code": {c}, "decision": {"approve"}})
	t.Logf("no decision -> %d; then same code with decision -> %d", code1, code2)
	if code2 != 200 {
		t.Errorf("DEFECT: a request refused for a missing decision consumed the valid code: %d %s", code2, body[strings.Index(body, "<h1>"):])
	}
}

// ADV-3: rationing and code state is never pruned for links that are
// not used again: keys of every link ever used stay in memory.
func TestAdvCodeStateNeverPruned(t *testing.T) {
	var c codes
	for i := 0; i < 1000; i++ {
		l := "link" + string(rune('A'+i%26)) + strings.Repeat("x", i)
		c.allow(l, "rita")
		c.put(l+"|rita", "123456")
	}
	// simulate the hour passing for all of them
	for k, v := range c.issued {
		for i := range v {
			v[i] = v[i].Add(-2 * time.Hour)
		}
		c.issued[k] = v
	}
	for k, v := range c.m {
		v.exp = v.exp.Add(-2 * time.Hour)
		c.m[k] = v
	}
	c.swept = c.swept.Add(-2 * time.Minute) // the sweep runs at most once a minute
	c.allow("fresh", "rita")
	t.Logf("after all windows/TTLs passed: issued keys=%d, codes=%d", len(c.issued), len(c.m))
	if len(c.issued) > 2 || len(c.m) > 0 {
		t.Errorf("DEFECT: expired rationing/code entries are never removed (%d, %d)", len(c.issued), len(c.m))
	}
}

// Held-up checks: a member-bound link cannot pick another member, the
// answer is recorded as the link's member, the no-member typed-name form
// is refused on a roles queue even for a member link.
func TestAdvMemberLinkHolds(t *testing.T) {
	s, h, store, runID, sent := advSetup(t)
	ctx := context.Background()
	task, _ := h.Runtime.PendingTask(ctx, runID)
	link := s.MemberTaskLink(task, "rita")
	if code, _ := post(t, link, url.Values{"step": {"code"}, "member": {"carlos"}}); code != 403 || len(*sent) != 0 {
		t.Fatalf("carlos on rita's link: %d", code)
	}
	if code, _ := post(t, link, url.Values{"by": {"x"}, "decision": {"approve"}}); code != 403 {
		t.Fatalf("typed name: %d", code)
	}
	// forged: another member name via token tamper
	bad := link[:len(link)-2] + "AA"
	if code, _ := post(t, bad, url.Values{"step": {"code"}, "member": {"rita"}}); code != 410 {
		t.Fatalf("tampered: %d", code)
	}
	post(t, link, url.Values{"step": {"code"}, "member": {"rita"}})
	c := strings.TrimPrefix((*sent)[0]["text"].(string), "Seu código: ")
	// Codes belong to the task and the member, not to a link (ADV-1): Rita's
	// code does not answer for Carlos on the open link, and answers for her
	// on any of the task's links.
	other := s.TaskLink(task)
	if code, _ := post(t, other, url.Values{"step": {"answer"}, "member": {"carlos"}, "code": {c}, "decision": {"approve"}}); code != 403 {
		t.Fatalf("Rita's code for Carlos: %d", code)
	}
	if code, _ := post(t, other, url.Values{"step": {"answer"}, "member": {"rita"}, "code": {c}, "decision": {"approve"}}); code != 200 {
		t.Fatalf("right code: %d", code)
	}
	recs, _ := store.Records(ctx, runID)
	ok := false
	for _, r := range recs {
		ok = ok || (r.Kind == journal.KindResume && strings.Contains(string(r.Value), `"by":"rita"`))
	}
	if !ok {
		t.Fatal("not recorded as rita")
	}
	if code, _ := post(t, link, url.Values{"step": {"code"}, "member": {"rita"}}); code != 410 {
		t.Fatalf("link after answer: %d", code)
	}
}
