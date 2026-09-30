package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/bench"
)

func newBenchGateCmd() *cobra.Command {
	var baseline, candidate, baseTarget, candTarget string
	var cfg bench.GateConfig
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Fail if a candidate run regressed against a baseline run (RFC-0002)",
		Long: `Compares two trials.jsonl files from "bench decide" on the same dataset:
accuracy regression (one-sided McNemar on cases that flipped), selective
accuracy at the routing threshold, AUROC, p95 latency, cost per question and
errors. Exits non-zero when any gate fails. When GITHUB_STEP_SUMMARY is set,
the report is appended to it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, err := pickTarget(baseline, baseTarget)
			if err != nil {
				return fmt.Errorf("baseline: %w", err)
			}
			cand, err := pickTarget(candidate, candTarget)
			if err != nil {
				return fmt.Errorf("candidate: %w", err)
			}
			// An explicit 0 means "none allowed"; the library reads 0 as the default.
			if cfg.MaxWorseCases == 0 {
				cfg.MaxWorseCases = -1
			}
			if cfg.MaxCoverageDrop == 0 {
				cfg.MaxCoverageDrop = -1
			}
			g := bench.Gate(base, cand, cfg)
			report := g.Markdown("Decide regression gate")
			fmt.Fprint(cmd.OutOrStdout(), report)
			if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
				if err != nil {
					return fmt.Errorf("write step summary: %w", err)
				}
				_, werr := f.WriteString(report + "\n")
				if cerr := f.Close(); werr == nil {
					werr = cerr
				}
				if werr != nil {
					return fmt.Errorf("write step summary: %w", werr)
				}
			}
			if !g.Pass() {
				return errors.New("regression gate failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "baseline trials.jsonl (required)")
	cmd.Flags().StringVar(&candidate, "candidate", "", "candidate trials.jsonl (required)")
	cmd.Flags().StringVar(&baseTarget, "baseline-target", "", "target label in the baseline file (default: the only one)")
	cmd.Flags().StringVar(&candTarget, "candidate-target", "", "target label in the candidate file (default: the only one)")
	cmd.Flags().Float64Var(&cfg.RoutingThreshold, "routing-threshold", 0.7, "confidence threshold for selective accuracy")
	cmd.Flags().Float64Var(&cfg.MinSelectiveAccuracy, "min-selective-accuracy", 0.99, "")
	cmd.Flags().Float64Var(&cfg.MinAUROC, "min-auroc", 0.9, "")
	cmd.Flags().Float64Var(&cfg.MaxLatencyRatio, "max-latency-ratio", 1.5, "candidate p95 / baseline p95")
	cmd.Flags().Float64Var(&cfg.MaxCostRatio, "max-cost-ratio", 1.2, "candidate / baseline cost per question")
	cmd.Flags().Float64Var(&cfg.Alpha, "alpha", 0.05, "significance level of the McNemar test")
	cmd.Flags().IntVar(&cfg.MaxWorseCases, "max-worse-cases", 2, "most cases allowed to flip from right to wrong (net)")
	cmd.Flags().Float64Var(&cfg.MaxCoverageDrop, "max-coverage-drop", 0.10, "largest allowed drop in coverage at the routing threshold")
	cmd.Flags().BoolVar(&cfg.SkipLatency, "skip-latency", false, "skip the latency gate when candidate and baseline ran in different environments")
	_ = cmd.MarkFlagRequired("baseline")
	_ = cmd.MarkFlagRequired("candidate")
	return cmd
}

func pickTarget(path, target string) ([]bench.Trial, error) {
	byTarget, err := readTrials(path)
	if err != nil {
		return nil, err
	}
	if target != "" {
		ts, ok := byTarget[target]
		if !ok {
			return nil, fmt.Errorf("target %q not in %s", target, path)
		}
		return ts, nil
	}
	if len(byTarget) != 1 {
		names := make([]string, 0, len(byTarget))
		for n := range byTarget {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%s has %d targets %v; pick one", path, len(byTarget), names)
	}
	for _, ts := range byTarget {
		return ts, nil
	}
	return nil, nil
}
