package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const repairDocs = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: q, version: 1.0.0 }
spec:
  egress: { hosts: [x.test] }
  connection: { baseUrl: "https://x.test" }
  actions:
    move:
      input: { key: string }
      effect: irreversible
      http: { method: POST, path: /move, body: { slot: "${ inputs.key }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: f, namespace: r }
spec:
  description: repair scenario
  action:
    act: q/move
    with: { key: k1 }
    idempotencyKey: "'k1'"
  onError: [ {respond: {template: "not done"}} ]
  output: { template: "done" }
`

// proposalGen answers the repair question with a fixed proposal.
type proposalGen struct {
	mu       sync.Mutex
	proposal map[string]any
	asked    int
}

func (*proposalGen) Name() string { return "proposal" }
func (g *proposalGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if req.Instructions == repairRules {
		g.asked++
		return &generate.Response{Value: g.proposal}, nil
	}
	return &generate.Response{Text: "NONE"}, nil
}

// repairHarness: q/move is refused with detail until a call carries every
// field of want; the calls' Extra are recorded.
func repairHarness(t *testing.T, detail string, want map[string]any, proposal map[string]any) (*harness, *[]map[string]any, *proposalGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(repairDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	var extras []map[string]any
	r := act.NewRegistry()
	r.Register("q/move", func(_ context.Context, c act.Call) (any, error) {
		extras = append(extras, c.Extra)
		for k, v := range want {
			if c.Extra[k] != v {
				return nil, &act.DefiniteError{Code: "INVALID", Message: "refused", Detail: detail, Public: true}
			}
		}
		return map[string]any{"ok": true}, nil
	})
	h.rt.Actor = r
	g := &proposalGen{proposal: proposal}
	h.rt.Generator = g
	h.rt.Limits.Repair, h.rt.Limits.FailureQueue = "q/move", "recepcao"
	return h, &extras, g
}

func (h *harness) startF() Result {
	h.t.Helper()
	res, err := h.rt.Start(context.Background(), Start{Namespace: "r", Function: "f", Conversation: "c1"})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.deliver(res)
}

// A fee: the user agrees to the amount, a person approves, and the call
// runs again with the added field and its own key.
func TestRepairWithTheUsersAgreementAndApproval(t *testing.T) {
	h, extras, _ := repairHarness(t, "a fee of R$ 50 now applies: resend with acceptFee: true once the patient agrees", map[string]any{"acceptFee": true},
		map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}, "userMustAgree": "Agora há uma taxa de R$ 50 para remarcar. Você concorda? Responda sim."})
	res := h.startF()
	expectStatus(t, res, journal.StatusConfirming)
	if last := h.user[len(h.user)-1]; !strings.Contains(last, "R$ 50") {
		t.Errorf("the user is asked about the amount: %q", last)
	}
	res = h.reply("user", "sim, concordo", nil)
	expectStatus(t, res, journal.StatusWaitingForHuman)
	if q := h.queues[len(h.queues)-1]; q.Task != "approve" || !strings.Contains(q.Text, "acceptFee=true") || !strings.Contains(q.Text, "agreed") {
		t.Errorf("the person sees the repair and the user's agreement: %+v", q)
	}
	expectStatus(t, h.reply("human", "yes", nil), journal.StatusSucceeded)
	if len(*extras) != 2 || (*extras)[1]["acceptFee"] != true || (*extras)[0] != nil {
		t.Errorf("one refused call, one repaired call: %v", *extras)
	}
	if h.user[len(h.user)-1] != "done" {
		t.Errorf("the Function goes on: %v", h.user)
	}
}

// A missing value only the user knows is asked for, never invented.
func TestRepairAsksTheUserForAMissingValue(t *testing.T) {
	h, extras, _ := repairHarness(t, "field reason is required", map[string]any{"reason": "mudança de agenda"},
		map[string]any{"possible": true, "askUser": map[string]any{"field": "reason", "question": "Qual o motivo da remarcação?"}})
	expectStatus(t, h.startF(), journal.StatusWaitingForUser)
	expectStatus(t, h.reply("user", "mudança de agenda", nil), journal.StatusWaitingForHuman)
	expectStatus(t, h.reply("human", "yes", nil), journal.StatusSucceeded)
	if got := (*extras)[len(*extras)-1]["reason"]; got != "mudança de agenda" {
		t.Errorf("reason sent: %v", got)
	}
}

// Nothing runs again when the proposal changes a field already sent, hides
// the amount from the user, or the user or the person says no.
func TestRepairRefusals(t *testing.T) {
	fee := "a fee of R$ 50 now applies: resend with acceptFee: true"
	for name, c := range map[string]struct {
		proposal     map[string]any
		user, person string
		want         journal.Status
	}{
		"changes a field":   {map[string]any{"possible": true, "add": map[string]any{"slot": "other"}}, "", "", journal.StatusFailed},
		"structure":         {map[string]any{"possible": true, "add": map[string]any{"x": map[string]any{"y": 1}}}, "", "", journal.StatusFailed},
		"hides the amount":  {map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}, "userMustAgree": "Há uma taxa. Concorda?"}, "", "", journal.StatusFailed},
		"not possible":      {map[string]any{"possible": false}, "", "", journal.StatusFailed},
		"user says no":      {map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}, "userMustAgree": "Taxa de R$ 50. Concorda?"}, "não", "", journal.StatusSucceeded},
		"person refuses it": {map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}, "userMustAgree": "Taxa de R$ 50. Concorda?"}, "sim", "no", journal.StatusFailed},
	} {
		h, extras, _ := repairHarness(t, fee, map[string]any{"acceptFee": true}, c.proposal)
		res := h.startF()
		if c.user != "" {
			res = h.reply("user", c.user, nil)
		}
		if c.person != "" {
			res = h.reply("human", c.person, nil)
		}
		if res.Status != c.want || len(*extras) != 1 {
			t.Errorf("%s: status %s, calls %v", name, res.Status, *extras)
		}
	}
}

// An action not listed in improvise.repair is never repaired.
func TestOnlyListedActionsAreRepaired(t *testing.T) {
	h, extras, g := repairHarness(t, "resend with acceptFee: true", map[string]any{"acceptFee": true}, map[string]any{"possible": true, "add": map[string]any{"acceptFee": true}})
	h.rt.Limits.Repair = ""
	expectStatus(t, h.startF(), journal.StatusFailed)
	if g.asked != 0 || len(*extras) != 1 {
		t.Errorf("no proposal, one call: asked %d, calls %v", g.asked, *extras)
	}
}
