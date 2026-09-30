package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
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

// ADVREV-4: any database error while looking up the task (here: the
// adapter's DB closed during shutdown) is reported as "no longer open"
// and the member's buttons are replaced, while the task is still open.
func TestAdvrevDBErrorRemovesButtonsOfOpenTask(t *testing.T) {
	a, ts, ws, h, store := setup(t)
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	_, _ = a.DeliverTasks(ctx)
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	_ = a.db.Close() // e.g. Close() ran after srv.Shutdown timed out with a handler in flight
	send(t, a, ts, click("T0SHOP", "U0RITA", "9.1", "task:"+task.Hash+":approve"), "signing-secret", now)
	run, _ := store.GetRun(ctx, res.RunID)
	t.Logf("run status %s; updates: %q", run.Status, ws.updated())
	if run.Status == journal.StatusWaitingForHuman && strings.Contains(ws.updated(), "9.1: Esta tarefa não está mais aberta") {
		t.Error("an open task's buttons were replaced with 'no longer open' because of a DB error")
	}
}

// ADVREV-5: ChannelTasks reads only the 500 newest waiting runs. A task
// that becomes deliverable later (forwarded / escalated) on an older run
// is never presented once 500 newer runs wait for a person.
func TestAdvrevOldTaskStarvedBy500NewerRuns(t *testing.T) {
	a, _, ws, h, _ := setup(t)
	ctx := context.Background()
	old, _ := h.StartHook(ctx, "pedido", map[string]any{}, "old")
	_, _ = a.DeliverTasks(ctx) // Rita gets it
	for i := 0; i < 500; i++ {
		if _, err := h.StartHook(ctx, "pedido", map[string]any{}, fmt.Sprint("k", i)); err != nil {
			t.Fatal(err)
		}
	}
	n0, _ := a.DeliverTasks(ctx)
	t.Logf("posted for the 500 newer runs: %d", n0)
	if _, err := h.Runtime.Forward(ctx, old.RunID, "gerencia", "rita"); err != nil {
		t.Fatal(err)
	}
	n, _ := a.DeliverTasks(ctx)
	var gil int
	for _, p := range ws.posts {
		if strings.HasPrefix(p, "U0GIL:") {
			gil++
		}
	}
	t.Logf("after forwarding the old run to gerencia: sent %d, posts to Gil %d", n, gil)
	if gil == 0 {
		t.Error("the forwarded task never reaches the manager: its run is beyond ChannelTasks' 500-run page")
	}
}

// failOnce fails the first Append after a resume record (the run's
// execution after the answer), as a transient store error would.
type failOnce struct {
	*journal.Memory
	mu      sync.Mutex
	armed   bool
	tripped bool
}

func (f *failOnce) Append(ctx context.Context, runID string, r journal.Record) (int, error) {
	f.mu.Lock()
	if r.Kind == journal.KindResume {
		f.armed = true
	} else if f.armed && !f.tripped {
		f.mu.Unlock()
		return 0, errors.New("disk I/O error (injected)")
	}
	f.mu.Unlock()
	return f.Memory.Append(ctx, runID, r)
}

// ADVREV-6: member A's approval is journaled but the execution after it
// fails (A's buttons stay). Member B then clicks Aprovar: Deliver hits
// ErrDuplicate, sameReply ignores who answered, and B is told "Aprovado
// por B" while the journal (audit) says A approved.
func TestAdvrevSecondApproverMisattributed(t *testing.T) {
	dir := t.TempDir()
	def := strings.Replace(teamOnSlack, "- { id: gil, name: Gil, roles: [gerencia], slack: U0GIL }",
		"- { id: gil, name: Gil, roles: [gerencia], slack: U0GIL }\n    - { id: bia, name: Bia, roles: [loja], slack: U0BIA }", 1)
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(def), 0o600); err != nil {
		t.Fatal(err)
	}
	ws := &workspace{}
	store := &failOnce{Memory: journal.NewMemory()}
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: store,
		Handlers: map[string]act.Handler{
			"slack/post-task": func(_ context.Context, c act.Call) (any, error) { return map[string]any{"ok": true}, nil },
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
	defer func() { _ = a.Close() }()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	if n, err := a.DeliverTasks(ctx); n != 2 {
		t.Fatalf("posted %d %v", n, err)
	}
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	approve := "task:" + task.Hash + ":approve"
	send(t, a, ts, click("T0SHOP", "U0RITA", "r.1", approve), "signing-secret", now) // journaled, execution fails
	store.mu.Lock()
	store.tripped = true // the store recovers
	store.mu.Unlock()
	if r, _ := store.GetRun(ctx, res.RunID); true {
		t.Logf("after Rita's click: run %s, updates %q", r.Status, ws.updated())
	}
	send(t, a, ts, click("T0SHOP", "U0BIA", "b.1", approve), "signing-secret", now)
	run, _ := store.GetRun(ctx, res.RunID)
	recs, _ := store.Records(ctx, res.RunID)
	var by string
	for _, r := range recs {
		if r.Kind == journal.KindResume {
			by = string(r.Value)
		}
	}
	t.Logf("run %s; resume record %s; updates:\n%s", run.Status, by, ws.updated())
	if strings.Contains(ws.updated(), "b.1: ✅ Aprovado por Bia.") && strings.Contains(by, `"by":"rita"`) {
		t.Error("Bia is told her approval was recorded, but the journal attributes the approval to Rita")
	}
}

func (f *failOnce) UpdateRun(ctx context.Context, id string, status journal.Status, pending string) error {
	f.mu.Lock()
	fail := f.armed && !f.tripped
	f.mu.Unlock()
	if fail {
		return errors.New("disk I/O error (injected)")
	}
	return f.Memory.UpdateRun(ctx, id, status, pending)
}
