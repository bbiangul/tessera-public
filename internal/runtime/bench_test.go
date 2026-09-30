package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// BenchmarkReplay measures re-executing a run from its journal with no new
// work (RFC-0003, "Measurements": under 1 ms for about 50 records).
func BenchmarkReplay(b *testing.B) {
	t := &testing.T{}
	h := newHarness(t)
	h.start("quero remarcar")
	h.reply("user", "a de terça", nil)
	h.reply("user", "às 16h", nil) // waiting at the confirmation
	recs, _ := h.store.Records(context.Background(), "run-1")
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		if _, err := h.rt.Resume(ctx, "run-1"); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(recs)), "records") // after ResetTimer, which clears metrics
}

// BenchmarkReplay50 replays a synthetic Function with 50 recorded reads.
func BenchmarkReplay50(b *testing.B) {
	dir := b.TempDir()
	var fn strings.Builder
	fn.WriteString("apiVersion: agents.tessera.dev/v1alpha1\nkind: Function\nmetadata: { name: wide, namespace: bench }\nspec:\n  description: fifty reads\n  inputs:\n")
	for i := range 50 {
		fmt.Fprintf(&fn, "    in%02d:\n      type: string\n      resolve:\n        - { id: get, fromAction: kv/get, with: { key: \"k%d\" } }\n", i, i)
	}
	fn.WriteString("  output:\n    template: \"{{ inputs.in00 }} {{ inputs.in49 }}\"\n")
	plugin := "apiVersion: plugins.tessera.dev/v1alpha1\nkind: PluginManifest\nmetadata: { name: kv, version: 1.0.0 }\nspec:\n  actions:\n    get: { input: { key: string }, output: string, effect: read }\n"
	for name, body := range map[string]string{"fn.yaml": fn.String(), "plugin.yaml": plugin} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	t := &testing.T{}
	reg := act.NewRegistry()
	reg.Register("kv/get", func(_ context.Context, c act.Call) (any, error) { return "v-" + c.Args["key"].(string), nil })
	store := journal.NewMemory()
	rt := &Runtime{Program: admit(t, dir), Store: store, Actor: reg, Decider: &readerDecider{}, NewID: func() string { return "r" }}
	ctx := context.Background()
	res, err := rt.Start(ctx, Start{Namespace: "bench", Function: "wide", Conversation: "c"})
	if err != nil || res.Status != journal.StatusSucceeded {
		b.Fatalf("%+v %v", res, err)
	}
	// Execute the finished run again directly: Resume would return at once
	// for a finished run and measure nothing.
	run, _ := store.GetRun(ctx, "r")
	recs, _ := store.Records(ctx, "r")
	b.ResetTimer()
	for range b.N {
		if res, err := rt.execute(ctx, run); err != nil || res.Status != journal.StatusSucceeded {
			b.Fatal(res, err)
		}
	}
	b.ReportMetric(float64(len(recs)), "records")
}
