package evals

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
)

// runTeamBench answers every task three times with DeepSeek V4.1 Flash
// and writes one JSON line per answer.
func runTeamBench(t *testing.T, tasksPath, factsPath, out string) {
	var tasks []struct{ ID, Kind, Text string }
	b, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &tasks); err != nil {
		t.Fatal(err)
	}
	facts, err := os.ReadFile(factsPath)
	if err != nil {
		t.Fatal(err)
	}
	g := &generate.OpenRouter{APIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))}
	g.Model, g.Providers, _ = generate.ParseModel("deepseek/deepseek-v4.1-flash")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, task := range tasks {
		for rep := 1; rep <= 3; rep++ {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				a, err := teamAnswer(context.Background(), g, string(facts), runtime.Task{Kind: task.Kind, Text: task.Text}, nil)
				line := map[string]any{"id": task.ID, "repeat": rep, "answer": a}
				if err != nil {
					line["error"] = err.Error()
				}
				j, _ := json.Marshal(line)
				mu.Lock()
				_, _ = f.Write(append(j, '\n'))
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
}
