package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/bench"
)

func newBenchCalibrateCmd() *cobra.Command {
	var folds int
	cmd := &cobra.Command{
		Use:   "calibrate <trials.jsonl>",
		Short: "Fit temperature scaling to recorded trials, with k-fold cross-validation",
		Long: `Reads trials written by "bench decide" and, for each target, fits a
temperature that rescales its probabilities. Scores are cross-validated by
case, so the reported improvement is on cases the temperature never saw.
Scaling never changes which option wins: it can fix calibration, not
separation (AUROC). No model is called.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			byTarget, err := readTrials(args[0])
			if err != nil {
				return err
			}
			targets := make([]string, 0, len(byTarget))
			for t := range byTarget {
				targets = append(targets, t)
			}
			sort.Strings(targets)
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "| Target | Fitted T | Fold T | Log loss raw → CV | Brier raw → CV | ECE raw → CV | AUROC | ≥0.7 cov / acc raw → CV | Note |")
			fmt.Fprintln(out, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
			for _, t := range targets {
				r := bench.CrossValidateTemperature(t, byTarget[t], folds)
				note := ""
				switch {
				case !r.Scalable:
					note = "not scaled: point/vote or saturated (p = 1.0) probabilities"
				case r.AtBound:
					note = "a fold hit the grid bound: treat T as unreliable"
				}
				fmt.Fprintf(out, "| %s | %.2f | %.2f–%.2f | %.3f → %.3f | %.3f → %.3f | %.3f → %.3f | %.3f | %.0f%% / %.1f%% → %.0f%% / %.1f%% | %s |\n",
					t, r.Temperature, minF(r.FoldTemps), maxF(r.FoldTemps),
					r.Raw.LogLoss, r.CV.LogLoss, r.Raw.Brier, r.CV.Brier, r.Raw.ECE, r.CV.ECE, r.Raw.AUROC,
					100*r.Raw.Select[1].Coverage, 100*r.Raw.Select[1].Accuracy,
					100*r.CV.Select[1].Coverage, 100*r.CV.Select[1].Accuracy, note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&folds, "folds", 5, "cross-validation folds (split by case)")
	return cmd
}

func readTrials(path string) (map[string][]bench.Trial, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]bench.Trial{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for line := 1; sc.Scan(); line++ {
		var t bench.Trial
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out[t.Target] = append(out[t.Target], t)
	}
	return out, sc.Err()
}

func minF(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		m = min(m, x)
	}
	return m
}

func maxF(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		m = max(m, x)
	}
	return m
}
