package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/bench"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/ledger"
)

func newBenchCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "bench", Short: "Measure engines against labelled datasets"}
	cmd.AddCommand(newBenchDecideCmd(), newBenchExtractCmd(), newBenchRouteCmd(), newBenchCalibrateCmd(), newBenchGateCmd())
	return cmd
}

func newBenchDecideCmd() *cobra.Command {
	var (
		dataset, outDir, title string
		targets                []string
		repeats, concurrency   int
		requireLogprobs        bool
		providers              []string
		cassettePath, cassMode string
		maxCost                float64
	)
	cmd := &cobra.Command{
		Use:   "decide",
		Short: "Benchmark Decide backends on a JSONL dataset",
		Long: `Runs every case of a dataset on every target and writes report.md,
summary.json and trials.jsonl to --out.

Targets:
  typesafe:<model>   TypeSafe System One (needs TYPESAFE_API_KEY), e.g. typesafe:jev-latest
  llm:<model>        OpenAI-compatible chat via OpenRouter (needs OPENROUTER_API_KEY),
                     probabilities from first-token logprobs,
                     e.g. llm:deepseek/deepseek-v4.1-flash
  llm-vote:<model>   same model, probabilities from 5 samples at temperature 0.8
  llm-verbal:<model> same model, probability stated by the model`,
		Example: `  tessera bench decide --dataset bench/datasets/travel-decisions-v1.jsonl \
    --target typesafe:jev-latest --target llm:deepseek/deepseek-v4.1-flash --repeats 3`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cases, err := bench.LoadDataset(dataset)
			if err != nil {
				return fmt.Errorf("dataset: %w", err)
			}
			tgs, err := buildTargets(targets, requireLogprobs, providers, cassMode == string(decide.CassetteReplay))
			if err != nil {
				return err
			}
			// Metering wraps the real backend, inside the cassette: replayed
			// answers cost nothing and are not charged against the budget.
			var budget *ledger.Ledger
			if maxCost != 0 {
				b := ledger.Budget{MaxCostUSD: maxCost}
				if err := b.Validate(); err != nil {
					return fmt.Errorf("--max-cost-usd: %w", err)
				}
				budget = ledger.New(b)
				if cassMode != string(decide.CassetteReplay) || cassettePath == "" {
					for i := range tgs {
						tgs[i].Decider = &decide.Metered{Inner: tgs[i].Decider, Ledger: budget}
					}
				}
			}
			var cassettes []*decide.Cassette
			if cassettePath != "" {
				for i, tg := range tgs {
					c, err := decide.LoadCassette(cassettePath, tg.Label, tg.Decider, decide.CassetteMode(cassMode))
					if err != nil {
						return fmt.Errorf("cassette: %w", err)
					}
					cassettes = append(cassettes, c)
					tgs[i].Decider = c
				}
			}
			if outDir == "" {
				outDir = filepath.Join("bench", "results", time.Now().UTC().Format("20060102-150405"))
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			total := int64(len(cases) * len(tgs) * repeats)
			var done atomic.Int64
			errOut := cmd.ErrOrStderr()
			trials := bench.Run(ctx, cases, tgs, bench.Options{
				Repeats: repeats, Concurrency: concurrency,
				OnTrial: func(t bench.Trial) {
					n := done.Add(1)
					if t.Err != "" {
						fmt.Fprintf(errOut, "  error %s %s: %s\n", t.Target, t.CaseID, t.Err)
					}
					if n%25 == 0 || n == total {
						fmt.Fprintf(errOut, "  %d/%d trials\n", n, total)
					}
				},
			})
			if cassMode == string(decide.CassetteRecord) {
				for _, c := range cassettes {
					if err := c.Save(cassettePath); err != nil {
						return err
					}
				}
			}
			if err := writeResults(outDir, title, dataset, len(cases), repeats, tgs, trials, cmd); err != nil {
				return err
			}
			if budget != nil {
				tot := budget.Totals()
				fmt.Fprintf(errOut, "  spent $%.6f of $%.2f budget in %d calls\n", tot.CostUSD, maxCost, tot.Calls["decide"])
				if err := budget.Check(); err != nil {
					return err
				}
				for _, t := range trials {
					if strings.Contains(t.Err, ledger.ErrBudgetExceeded.Error()) {
						return fmt.Errorf("%w: the run stopped before every case was answered", ledger.ErrBudgetExceeded)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dataset, "dataset", "", "JSONL dataset (required)")
	cmd.Flags().StringArrayVar(&targets, "target", nil, "backend:model to test (repeatable, required)")
	cmd.Flags().IntVar(&repeats, "repeats", 3, "times each case is asked per target")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "concurrent calls per target")
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default bench/results/<timestamp>)")
	cmd.Flags().StringVar(&title, "title", "Decide benchmark", "report title")
	cmd.Flags().BoolVar(&requireLogprobs, "require-logprobs", true, "llm targets: route only to providers that return logprobs")
	cmd.Flags().StringVar(&cassettePath, "cassette", "", "record or replay model calls in this file (RFC-0002, level 2)")
	cmd.Flags().StringVar(&cassMode, "cassette-mode", "replay", "record | replay")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0, "stop calling models once this much is spent (0 = no limit)")
	cmd.Flags().StringSliceVar(&providers, "provider", nil, "llm targets: pin upstream providers in order, no fallback (e.g. DeepSeek)")
	_ = cmd.MarkFlagRequired("dataset")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// buildTargets turns backend:model specs into Deciders. With replayOnly no
// API keys are needed: the cassette answers every call.
func buildTargets(specs []string, requireLogprobs bool, providers []string, replayOnly bool) ([]bench.Target, error) {
	var out []bench.Target
	for _, spec := range specs {
		backend, model, ok := strings.Cut(spec, ":")
		if !ok || model == "" {
			return nil, fmt.Errorf("target %q: want backend:model", spec)
		}
		var d decide.Decider
		switch backend {
		case "typesafe":
			key := os.Getenv("TYPESAFE_API_KEY")
			if key == "" && !replayOnly {
				return nil, errors.New("TYPESAFE_API_KEY is not set")
			}
			d = &decide.TypeSafe{APIKey: key, Model: model}
		case "llm", "llm-vote", "llm-verbal":
			key := os.Getenv("OPENROUTER_API_KEY")
			if key == "" && !replayOnly {
				return nil, errors.New("OPENROUTER_API_KEY is not set")
			}
			strategy := map[string]string{"llm": decide.StrategyLogprobs, "llm-vote": decide.StrategyVote, "llm-verbal": decide.StrategyVerbal}[backend]
			d = &decide.LLMConstrained{APIKey: key, Model: model, Strategy: strategy, RequireLogprobs: requireLogprobs, Providers: providers}
			if len(providers) > 0 {
				spec += "@" + strings.Join(providers, ",")
			}
		default:
			return nil, fmt.Errorf("target %q: unknown backend %q (want typesafe, llm, llm-vote or llm-verbal)", spec, backend)
		}
		out = append(out, bench.Target{Label: spec, Decider: d})
	}
	return out, nil
}

func writeResults(dir, title, dataset string, cases, repeats int, tgs []bench.Target, trials []bench.Trial, cmd *cobra.Command) error {
	sort.Slice(trials, func(i, j int) bool {
		a, b := trials[i], trials[j]
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.CaseID != b.CaseID {
			return a.CaseID < b.CaseID
		}
		return a.Repeat < b.Repeat
	})
	byTarget := map[string][]bench.Trial{}
	for _, t := range trials {
		byTarget[t.Target] = append(byTarget[t.Target], t)
	}
	var sums []bench.Summary
	for _, tg := range tgs {
		sums = append(sums, bench.Summarize(tg.Label, byTarget[tg.Label]))
	}

	tf, err := os.Create(filepath.Join(dir, "trials.jsonl"))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tf)
	for _, t := range trials {
		if err := enc.Encode(t); err != nil {
			tf.Close()
			return err
		}
	}
	if err := tf.Close(); err != nil {
		return err
	}

	sj, err := json.MarshalIndent(sums, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), sj, 0o644); err != nil {
		return err
	}

	rf, err := os.Create(filepath.Join(dir, "report.md"))
	if err != nil {
		return err
	}
	if err := bench.WriteMarkdown(rf, title, filepath.Base(dataset), cases, repeats, sums); err != nil {
		rf.Close()
		return err
	}
	if err := rf.Close(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "results written to %s\n", dir)
	return nil
}
