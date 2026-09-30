package bench

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Target is one backend+model under test.
type Target struct {
	Label   string
	Decider decide.Decider
}

// Options control a run.
type Options struct {
	Repeats     int // default 3
	Concurrency int // concurrent calls per target; default 4
	// OnTrial, if set, is called after every trial (for progress output).
	OnTrial func(Trial)
}

// Run asks every case Repeats times on every target. Each case is sent as a
// single-question request so all backends are compared call for call.
func Run(ctx context.Context, cases []Case, targets []Target, opt Options) []Trial {
	if opt.Repeats <= 0 {
		opt.Repeats = 3
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 4
	}
	var (
		mu     sync.Mutex
		trials []Trial
		wg     sync.WaitGroup
	)
	for _, tg := range targets {
		sem := make(chan struct{}, opt.Concurrency)
		for rep := 0; rep < opt.Repeats; rep++ {
			for _, c := range cases {
				wg.Add(1)
				go func(tg Target, c Case, rep int) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					t := runOne(ctx, tg, c, rep)
					mu.Lock()
					trials = append(trials, t)
					mu.Unlock()
					if opt.OnTrial != nil {
						opt.OnTrial(t)
					}
				}(tg, c, rep)
			}
		}
	}
	wg.Wait()
	return trials
}

func runOne(ctx context.Context, tg Target, c Case, rep int) Trial {
	t := Trial{Target: tg.Label, CaseID: c.ID, Task: c.Task, Repeat: rep, Gold: c.Gold}
	resp, err := tg.Decider.Decide(ctx, decide.Request{
		State:     c.State,
		Questions: map[string]decide.Question{"q": c.Question},
	})
	if err != nil {
		t.Err = err.Error()
		return t
	}
	a := resp.Answers["q"]
	t.Probs, t.Source, t.Model, t.Provider = a.Probabilities, a.Source, resp.Model, resp.Provider
	if pinned := PinnedModel(tg.Label); pinned != "" && resp.Model != "" && resp.Model != pinned {
		t.Err = fmt.Sprintf("served model %q, pinned %q", resp.Model, pinned)
		return t
	}
	t.Latency, t.CostUSD = resp.Latency, resp.Usage.CostUSD
	switch c.Question.Kind {
	case decide.KindChoice:
		t.Pred = a.Choice
	case decide.KindNoul:
		t.Pred = "false"
		if a.Noul >= 0.5 {
			t.Pred = "true"
		}
	case decide.KindScore:
		t.Pred = argmax(a.Probabilities, c.Question.Outcomes())
	}
	return t
}

// PinnedModel returns the model a target label pins, or "" for aliases that
// may move (…-latest, …-preview). Labels are backend:model, with an optional
// @providers suffix.
func PinnedModel(label string) string {
	_, model, ok := strings.Cut(label, ":")
	if !ok {
		return ""
	}
	model, _, _ = strings.Cut(model, "@")
	if strings.HasSuffix(model, "-latest") || strings.HasSuffix(model, "-preview") || strings.HasPrefix(model, "~") {
		return ""
	}
	return model
}

func argmax(p map[string]float64, keys []string) string {
	best, bp := "", -1.0
	for _, k := range keys {
		if p[k] > bp {
			best, bp = k, p[k]
		}
	}
	return best
}
