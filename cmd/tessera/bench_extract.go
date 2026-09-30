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

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/bench"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/ledger"
)

func newBenchExtractCmd() *cobra.Command {
	var (
		task, dataset, outDir string
		targets               []string
		repeats, concurrency  int
		maxCost               float64
		structured            bool
		baseURL               string
	)
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Benchmark Generate backends on schema extraction",
		Long: `Runs every case of an extraction dataset on every target, validates each
reply against the task's JSON Schema, compares it field by field with the
gold values, and writes report.md, summary.json and trials.jsonl to --out.

Targets:
  openrouter:<model>[@Provider[,Provider]|@any]   any chat model via OpenRouter;
                                             measured models are pinned by default
                                             (needs OPENROUTER_API_KEY)`,
		Example: `  tessera bench extract --task bench/datasets/travel-extract-v1.task.json \
    --dataset bench/datasets/travel-extract-v1.jsonl \
    --target openrouter:deepseek/deepseek-v4.1-flash --max-cost-usd 0.10`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, cases, err := bench.LoadExtract(task, dataset)
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				return errors.New("at least one --target is required")
			}
			budget := ledger.Budget{MaxCostUSD: maxCost}
			if err := budget.Validate(); err != nil {
				return err
			}
			if maxCost <= 0 {
				return errors.New("--max-cost-usd must be set: live benchmarks always run with a budget")
			}
			l := ledger.New(budget)
			key := os.Getenv("OPENROUTER_API_KEY")
			var gts []bench.GenTarget
			for _, spec := range targets {
				backend, model, ok := strings.Cut(spec, ":")
				if !ok || backend != "openrouter" || model == "" {
					return fmt.Errorf("target %q: want openrouter:<model>[@Provider]", spec)
				}
				if key == "" {
					return errors.New("OPENROUTER_API_KEY is not set")
				}
				or := &generate.OpenRouter{APIKey: key, BaseURL: baseURL, StructuredOutputs: structured}
				var defaulted bool
				or.Model, or.Providers, defaulted = generate.ParseModel(model)
				if defaulted && structured {
					// The default provider may not support structured
					// outputs (DeepSeek does not: provider-001), and a pinned
					// request has no fallback.
					or.Providers = nil
					fmt.Fprintf(cmd.ErrOrStderr(), "note: %s is not pinned to %v with --structured-outputs; name a provider that supports them, or pass --structured-outputs=false\n", spec, generate.DefaultProviders[or.Model])
				}
				gts = append(gts, bench.GenTarget{Label: spec, Generator: &generate.Metered{Inner: or, Ledger: l}})
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			trials := bench.RunExtract(ctx, t, cases, gts, repeats, concurrency)

			gold := map[string]map[string]any{}
			for _, c := range cases {
				gold[c.ID] = c.Gold
			}
			var sums []bench.ExtractSummary
			for _, g := range gts {
				sums = append(sums, bench.SummarizeExtract(g.Label, trials, gold))
			}
			if err := writeExtract(outDir, trials, sums); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprint(out, extractReport(sums))
			tot := l.Totals()
			fmt.Fprintf(out, "\nspent $%.6f of $%.2f budget in %d calls\n", tot.CostUSD, maxCost, tot.Calls["generate"])
			for _, tr := range trials {
				if strings.Contains(tr.Err, ledger.ErrBudgetExceeded.Error()) {
					return fmt.Errorf("budget exceeded: results are partial")
				}
			}
			return l.Check()
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "task file: instructions and JSON Schema (required)")
	cmd.Flags().StringVar(&dataset, "dataset", "", "cases JSONL (required)")
	cmd.Flags().StringArrayVar(&targets, "target", nil, "backend:model, repeatable")
	cmd.Flags().IntVar(&repeats, "repeats", 1, "runs per case")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "calls in flight")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0, "hard budget for the whole run (required)")
	cmd.Flags().BoolVar(&structured, "structured-outputs", true, "ask the provider to constrain decoding to the schema")
	cmd.Flags().StringVar(&outDir, "out", "out/extract", "output directory")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "OpenRouter-compatible base URL (tests)")
	_ = cmd.Flags().MarkHidden("base-url")
	_ = cmd.MarkFlagRequired("task")
	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}

// splitProviders turns "model@A,B" into the model and its pinned providers.
// splitProviders is generate.ParseModel without the flag: a model with a
// measured provider is pinned to it unless the spec says "@any" (#5).
func splitProviders(s string) (string, []string) {
	m, p, _ := generate.ParseModel(s)
	return m, p
}

func writeExtract(dir string, trials []bench.ExtractTrial, sums []bench.ExtractSummary) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "trials.jsonl"))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, t := range trials {
		if err := enc.Encode(t); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sums, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(extractReport(sums)), 0o644)
}

func extractReport(sums []bench.ExtractSummary) string {
	var b strings.Builder
	b.WriteString("| Target | Records right (95% CI) | Invalid | Repaired | p50 / p95 | $ per case |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, s := range sums {
		fmt.Fprintf(&b, "| %s | %.1f%% (%.1f–%.1f) | %d/%d | %d | %s / %s | %.6f |\n", s.Target,
			100*s.RecordAcc, 100*s.RecordLo, 100*s.RecordHi, s.Errors, s.Trials, s.Repaired,
			s.LatencyP50.Round(1e6), s.LatencyP95.Round(1e6), s.CostPerCase)
	}
	for _, s := range sums {
		fields := make([]string, 0, len(s.FieldAcc))
		for f := range s.FieldAcc {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		fmt.Fprintf(&b, "\n**%s** (%s)\n\n| Field | Accuracy |\n| --- | --- |\n", s.Target, strings.Join(s.Models, ", "))
		for _, f := range fields {
			fmt.Fprintf(&b, "| %s | %.1f%% |\n", f, 100*s.FieldAcc[f])
		}
		if len(s.FailingCases) > 0 {
			fmt.Fprintf(&b, "\nCases with a wrong or invalid trial: %s\n", strings.Join(s.FailingCases, ", "))
		}
	}
	return b.String()
}
