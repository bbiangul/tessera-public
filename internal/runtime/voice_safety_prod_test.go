//go:build integration

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/ledger"
)

// TestVoiceSafetyProduction runs voice-007's safety cases through the
// production rewrite, in both warm dialogue modes (voice-008): "marker"
// (voice-007's C, the default) and "full" (the dialogue with the boundary
// rule). Explicitly opted in, budgeted and synthetic. The rows use
// bench/voice-safety/judge.py's format.
//
//	VOICE_SAFETY_DATA=bench/voice-safety/replication.json VOICE_SAFETY_OUT=... \
//	VOICE_SAFETY_BUDGET=.1 go test -tags integration ./internal/runtime -run TestVoiceSafetyProduction
func TestVoiceSafetyProduction(t *testing.T) {
	dataPath, out := os.Getenv("VOICE_SAFETY_DATA"), os.Getenv("VOICE_SAFETY_OUT")
	if dataPath == "" || out == "" || os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("explicit dataset/output and provider key required")
	}
	budget := 0.1
	if v := os.Getenv("VOICE_SAFETY_BUDGET"); v != "" {
		b, err := strconv.ParseFloat(v, 64)
		if err != nil || b <= 0 || b > 0.4 {
			t.Fatal("budget must be positive and at most 0.4")
		}
		budget = b
	}
	var cases []voiceExperimentCase
	b, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	global := ledger.New(ledger.Budget{MaxCostUSD: budget})
	gen := &generate.OpenRouter{APIKey: os.Getenv("OPENROUTER_API_KEY"), Model: "deepseek/deepseek-v4.1-flash", Providers: []string{"DeepSeek"}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	for _, c := range cases {
		for rep := 1; rep <= 3; rep++ {
			for _, mode := range []string{"marker", "full"} {
				sem <- struct{}{}
				wg.Add(1)
				go func(c voiceExperimentCase, rep int, mode string) {
					defer wg.Done()
					defer func() { <-sem }()
					local := ledger.New(ledger.Budget{MaxCostUSD: 0.03})
					rt := &Runtime{Voice: &Voice{Persona: c.Persona, Warm: true, WarmDialogue: mode == "full"},
						Generator: &generate.Metered{Inner: &generate.Metered{Inner: gen, Ledger: global}, Ledger: local}}
					start := time.Now()
					text, why, e := rt.rewrite(context.Background(), rewriting{draft: c.Draft, facts: c.Facts, confirmation: c.Confirmation, conversation: c.Conversation, user: c.User})
					row := voiceExperimentRow{Case: c.ID, Variant: mode, Repeat: rep, Text: text, Fallback: why,
						Cost: local.Totals().CostUSD, Latency: time.Since(start).Milliseconds()}
					if e != nil {
						row.Error, row.Fallback = e.Error(), "rewrite error"
					}
					if row.Fallback != "" {
						row.Text = c.Draft
					}
					mu.Lock()
					defer mu.Unlock()
					if err := json.NewEncoder(f).Encode(row); err != nil {
						t.Error(err)
					}
				}(c, rep, mode)
			}
		}
	}
	wg.Wait()
	fmt.Printf("voice safety (production): %d cases x3 x2 modes, $%.6f\n", len(cases), global.Totals().CostUSD)
	if strings.TrimSpace(out) == "" {
		t.Fatal("no output")
	}
}
