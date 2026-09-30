package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// A SDR-style cadence on a channel with a 24h window and consent: the
// first contact is a template; after 48h of silence free text is refused
// and the follow-up goes as a template; a reply opens the window.
const channelFixture = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: whatsapp, version: 0.1.0 }
spec:
  channel:
    window: 24h
    consent: "has(caller.optIn) && caller.optIn == true"
  actions:
    send-template:
      input: { template: string }
      effect: external-message
    send-text:
      input: { text: string }
      effect: external-message
      freeText: true
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: sdr, namespace: travel }
spec: { channel: whatsapp }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cadence, namespace: travel }
spec:
  description: Outbound cadence.
  steps:
    - { id: first, act: whatsapp/send-template, with: { template: outbound_first_contact }, idempotencyKey: "conversation.id + ':first'" }
    - { id: w, wait: { for: 48h, orReply: true } }
    - id: next
      branch:
        - { when: "!steps.w.output.timedOut", respond: { template: "Que bom! Posso te mostrar como funciona?" } }
        - { else: true, call: nudge }
  output: { value: "steps.w.output" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: nudge, namespace: travel }
spec:
  description: A follow-up, free text if the window allows, else a template.
  steps:
    - id: text
      respond: { template: "Oi de novo! Viu minha mensagem?" }
      onError:
        - fallback: { act: whatsapp/send-template, with: { template: followup_outbound }, idempotencyKey: "conversation.id + ':followup'" }
  output: { value: "true" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: lembrete, namespace: travel }
spec:
  description: Free text through an action, to a user who may never have written.
  steps:
    - { id: texto, act: whatsapp/send-text, with: { text: "Oi de novo" }, idempotencyKey: "conversation.id + ':texto'" }
  output: { value: "true" }
`

func channelHarness(t *testing.T) (*harness, *time.Time, *[]string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(channelFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	var sent []string
	h.rt.Actor.(*act.Registry).Register("whatsapp/send-template", func(_ context.Context, c act.Call) (any, error) {
		sent = append(sent, c.Args["template"].(string))
		return map[string]any{"id": "wamid"}, nil
	})
	h.rt.Actor.(*act.Registry).Register("whatsapp/send-text", func(_ context.Context, c act.Call) (any, error) {
		sent = append(sent, "text: "+c.Args["text"].(string))
		return map[string]any{"id": "wamid"}, nil
	})
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	h.rt.Clock = func() time.Time { return now }
	return h, &now, &sent
}

func startCadence(t *testing.T, h *harness, optIn bool) Result {
	t.Helper()
	res, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: "cadence", Conversation: "lead-1", Caller: map[string]any{"optIn": optIn}})
	if err != nil {
		t.Fatal(err)
	}
	return h.deliver(res)
}

func TestChannelConsent(t *testing.T) {
	h, _, sent := channelHarness(t)
	res := startCadence(t, h, false)
	if res.Status != journal.StatusFailed || len(*sent) != 0 {
		t.Fatalf("without consent nothing is sent: %s %q", res.Status, *sent)
	}
}

func TestChannelWindowAfterSilence(t *testing.T) {
	h, now, sent := channelHarness(t)
	startCadence(t, h, true)
	*now = now.Add(49 * time.Hour)
	wakeAll(t, h)
	if run, _ := h.store.GetRun(context.Background(), "run-1"); run.Status != journal.StatusSucceeded {
		t.Fatalf("status %s", run.Status)
	}
	if len(*sent) != 2 || (*sent)[1] != "followup_outbound" {
		t.Errorf("after 48h of silence the follow-up must be a template: %q", *sent)
	}
	for _, m := range h.user {
		if m == "Oi de novo! Viu minha mensagem?" {
			t.Error("free text reached the user outside the window")
		}
	}
}

func TestChannelWindowOpenAfterReply(t *testing.T) {
	h, now, sent := channelHarness(t)
	startCadence(t, h, true)
	*now = now.Add(30 * time.Hour)
	res := h.reply("user", "tenho interesse", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[len(h.user)-1] != "Que bom! Posso te mostrar como funciona?" || len(*sent) != 1 {
		t.Errorf("a reply opens the window for free text: %q %q", h.user, *sent)
	}
}

// A freeText action is held to the window at run time too, where
// admission cannot see it (started directly for a user who never wrote):
// it is not called.
func TestFreeTextActionOutsideTheWindow(t *testing.T) {
	h, _, sent := channelHarness(t)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: "lembrete", Conversation: "lead-9", Caller: map[string]any{"optIn": true}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != journal.StatusFailed || len(*sent) != 0 {
		t.Fatalf("free text to a user who never wrote: %s %q", res.Status, *sent)
	}
}
