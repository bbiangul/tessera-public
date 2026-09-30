package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func sideHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	source := strings.Replace(replyCadence, "orReply: true", "orReply: true, includeSideMessages: true", 1)
	source = strings.Replace(source, "Obrigado! Você disse: {{ steps.w.output.reply }}", "{{ steps.w.output.sideMessages.join(' | ') }} / {{ steps.w.output.reply }}", 1)
	if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return newHarness(t, dir)
}

func TestSideMessagesPersistWithoutResuming(t *testing.T) {
	h := sideHarness(t)
	ctx := context.Background()
	h.startFn("follow-up", "")
	before, err := h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"sábado; tem estacionamento?", "agora prefiro quarta; qual endereço?"} {
		if err := h.rt.CaptureSideMessage(ctx, "run-1", before.Pending, nil, text); err != nil {
			t.Fatal(err)
		}
	}
	after, err := h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != journal.StatusWaitingForUser || after.Pending != before.Pending || !after.WakeAt.Equal(before.WakeAt) {
		t.Fatalf("capture changed wait: %+v", after)
	}
	restarted := &Runtime{Program: h.rt.Program, Store: h.store}
	got, err := restarted.SideMessages(ctx, "run-1", nil)
	want := []string{"sábado; tem estacionamento?", "agora prefiro quarta; qual endereço?"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("restart: %q %v", got, err)
	}
	if _, err := restarted.SideMessages(ctx, "run-1", map[string]any{"id": "another"}); err == nil {
		t.Fatal("accepted another caller")
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", "different-wait", nil, "yes"); err == nil {
		t.Fatal("accepted stale pending")
	}
	h.rt = restarted
	res := h.reply("user", "pode buscar", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if got := h.user[len(h.user)-1]; got != strings.Join(want, " | ")+" / pode buscar" {
		t.Fatalf("wait output lost context: %q", got)
	}
	if got, err := restarted.SideMessages(ctx, "run-1", nil); err != nil || got != nil {
		t.Fatalf("finished run retained active context: %v %v", got, err)
	}
}

func TestSideMessageLimitsAndOptIn(t *testing.T) {
	ctx := context.Background()
	h, _ := replyHarness(t)
	h.startFn("follow-up", "")
	run, err := h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := h.rt.SideMessages(ctx, "run-1", nil); got != nil || err != nil {
		t.Fatalf("default enabled: %v %v", got, err)
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, "no opt-in"); err == nil {
		t.Fatal("captured without opt-in")
	}
	h = sideHarness(t)
	h.startFn("follow-up", "")
	run, err = h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, strings.Repeat("x", 32769)); err == nil {
		t.Fatal("accepted oversized message")
	}
	for range 8 {
		if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, "question"); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, "ninth"); err == nil {
		t.Fatal("accepted ninth message")
	}
	got, err := h.rt.SideMessages(ctx, "run-1", nil)
	if err != nil || len(got) != 8 {
		t.Fatalf("overflow damaged history: %v %v", got, err)
	}
}

func TestSideMessagesDoNotCrossWaits(t *testing.T) {
	dir := t.TempDir()
	source := strings.Replace(replyCadence, "orReply: true", "orReply: true, includeSideMessages: true", 1)
	source = strings.Replace(source, "    - id: next", "    - { id: second, wait: { for: 48h, orReply: true, includeSideMessages: true } }\n    - id: next", 1)
	if err := os.WriteFile(filepath.Join(dir, "f.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	ctx := context.Background()
	h.startFn("follow-up", "")
	run, err := h.store.GetRun(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, "Saturday; parking?"); err != nil {
		t.Fatal(err)
	}
	h.reply("user", "continue", nil)
	got, err := h.rt.SideMessages(ctx, "run-1", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("context leaked to next wait: %v %v", got, err)
	}
	if err := h.rt.CaptureSideMessage(ctx, "run-1", run.Pending, nil, "late retry"); err == nil {
		t.Fatal("old wait accepted retry")
	}
}
