package teams_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/channels/teams"
	"github.com/bbiangul/tessera/internal/channels/teams/teamstest"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Adversarial review: the issuer is the one the OpenID configuration
// names, so a sovereign cloud's tokens (iss https://api.botframework.us)
// are accepted there, and only there.
func TestSovereignCloudIssuer(t *testing.T) {
	a, ts, bf, _, h, _ := setup(t)
	bf.Issuer = "https://api.botframework.us"
	ctx := context.Background()
	res, _ := h.StartHook(ctx, "pedido", map[string]any{}, "k1")
	_, _ = a.DeliverTasks(ctx)
	task, _ := h.Runtime.PendingTask(ctx, res.RunID)
	public := bf.Sign(teamstest.Token{AppID: appID, ServiceURL: svc, Expires: time.Now().Add(time.Hour)})
	if code := deliver(t, a, ts, public, teamstest.Click(svc, tenant, rita, "c0", "task:"+task.Hash+":approve")); code != 401 {
		t.Errorf("a public-cloud token where the configuration names the gov issuer: %d", code)
	}
	tok := bf.Sign(teamstest.Token{AppID: appID, ServiceURL: svc, Issuer: "https://api.botframework.us", Expires: time.Now().Add(time.Hour)})
	if code := deliver(t, a, ts, tok, teamstest.Click(svc, tenant, rita, "c1", "task:"+task.Hash+":approve")); code != 200 {
		t.Errorf("gov-cloud token -> HTTP %d", code)
	}
}

// ADVREV-3: the personal-chat cache is keyed by member ID only. After a
// member's teams: ID is corrected (or reassigned), their tasks keep going
// to the chat opened with the OLD Entra user.
func TestAdvrevStaleDMAfterTeamsIDChange(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "teams.db")
	store := journal.NewMemory()
	var cards []string
	load := func(ritaID string) *host.Host {
		d := t.TempDir()
		def := strings.Replace(teamsDef, "teams: 0A0A0A0A-0000-0000-0000-000000000001", "teams: "+ritaID, 1)
		if err := os.WriteFile(filepath.Join(d, "a.yaml"), []byte(def), 0o600); err != nil {
			t.Fatal(err)
		}
		h, issues, err := host.Load(host.Options{Paths: []string{d}, Namespace: "shop", Store: store,
			Handlers: map[string]act.Handler{
				"teams/open-dm": func(_ context.Context, c act.Call) (any, error) {
					return map[string]any{"id": "a:dm-" + fmt.Sprint(c.Args["user"])}, nil
				},
				"teams/post-task": func(_ context.Context, c act.Call) (any, error) {
					cards = append(cards, fmt.Sprint(c.Args["conversation"]))
					return map[string]any{"id": "card"}, nil
				},
				"teams/update-task": func(_ context.Context, c act.Call) (any, error) { return map[string]any{}, nil },
			}})
		if err != nil {
			t.Fatalf("%v %v", err, issues)
		}
		return h
	}
	bf := teamstest.New()
	defer bf.Close()
	ctx := context.Background()
	wrongID := "0a0a0a0a-0000-0000-0000-00000000dead" // typo'd / someone else
	h1 := load(wrongID)
	a1 := &teams.Adapter{Host: h1, AppID: appID, TenantID: tenant, OpenIDURL: bf.OpenIDURL(), Client: bf.Client()}
	if err := a1.Open(dbPath); err != nil {
		t.Fatal(err)
	}
	_, _ = h1.StartHook(ctx, "pedido", map[string]any{}, "k1")
	_, _ = a1.DeliverTasks(ctx)
	_ = a1.Close()

	// The operator fixes Rita's ID and restarts.
	h2 := load(rita)
	a2 := &teams.Adapter{Host: h2, AppID: appID, TenantID: tenant, OpenIDURL: bf.OpenIDURL(), Client: bf.Client()}
	if err := a2.Open(dbPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a2.Close() }()
	_, _ = h2.StartHook(ctx, "pedido", map[string]any{}, "k2")
	_, _ = a2.DeliverTasks(ctx)
	t.Logf("cards posted to: %v", cards)
	if last := cards[len(cards)-1]; last != "a:dm-"+rita {
		t.Errorf("after the fix, Rita's new task went to %s (the old user's chat), not a:dm-%s", last, rita)
	}
}
