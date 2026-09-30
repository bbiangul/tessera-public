package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/evals"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newTestCmd() *cobra.Command {
	var (
		decideSpec, generateSpec string
		cassette, cassMode       string
		only, out, journalPath   string
		maxCost, maxScenario     float64
		verbose                  bool
	)
	cmd := &cobra.Command{
		Use:   "test <files-or-dirs>...",
		Short: "Run EvalSuites: whole conversations against sandboxed plugins",
		Long: `Runs every scenario of every EvalSuite found in the paths (RFC-0002,
level 4). Plugins are replaced by the suites' Starlark sandboxes; an action
without a sandbox fails the scenario, so tests never reach real systems.

Model calls are live and budgeted, or replayed from a cassette:
  --cassette c.jsonl --cassette-mode record   calls the backend and records
  --cassette c.jsonl --cassette-mode replay   no network, no cost; a missing
                                             recording fails the scenario
Exit status 1 when any scenario fails.`,
		Example: `  tessera test testdata/agenda --decide typesafe:jev-1.13.0 --generate none`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxCost <= 0 {
				return errors.New("--max-cost-usd must be positive")
			}
			l := ledger.New(ledger.Budget{MaxCostUSD: maxCost})
			replay := cassette != "" && cassMode == string(decide.CassetteReplay)
			var d decide.Decider
			var err error
			if !replay {
				if d, err = buildDecider(decideSpec, l); err != nil {
					return err
				}
			}
			if cassette != "" {
				mode := decide.CassetteMode(cassMode)
				if mode != decide.CassetteRecord && mode != decide.CassetteReplay {
					return fmt.Errorf("--cassette-mode %q: record or replay", cassMode)
				}
				c, err := decide.LoadCassette(cassette, decideSpec, d, mode)
				if err != nil {
					return err
				}
				d = c
				if mode == decide.CassetteRecord {
					defer func() {
						if err := c.Save(cassette); err != nil {
							fmt.Fprintln(cmd.ErrOrStderr(), "saving the cassette:", err)
						}
					}()
				}
			}
			var g generate.Generator
			if !replay || generateSpec == "none" {
				if g, err = buildGenerator(generateSpec, l); err != nil {
					return err
				}
			}
			if cassette != "" && generateSpec != "none" {
				gc, err := generate.LoadCassette(cassette, generateSpec, g, decide.CassetteMode(cassMode))
				if err != nil {
					return err
				}
				g = gc
				if decide.CassetteMode(cassMode) == decide.CassetteRecord {
					defer func() {
						if err := gc.Save(cassette); err != nil {
							fmt.Fprintln(cmd.ErrOrStderr(), "saving the cassette:", err)
						}
					}()
				}
			}
			var keep journal.Store
			if journalPath != "" {
				j, err := journal.OpenSQLite(journalPath)
				if err != nil {
					return err
				}
				defer func() { _ = j.Close() }()
				keep = j
			}
			results, issues, err := evals.Run(cmd.Context(), evals.Options{
				Paths: args, Decider: d, Generator: g, MaxScenarioCostUSD: maxScenario, Only: only, Journal: keep,
			})
			for _, is := range issues {
				fmt.Fprintln(cmd.ErrOrStderr(), is.String())
			}
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(results) == 0 {
				return fmt.Errorf("no scenario ran (--only %q matched nothing)", only)
			}
			failed := 0
			for _, r := range results {
				mark := "PASS"
				if !r.Pass {
					mark, failed = "FAIL", failed+1
				}
				sim := ""
				if r.Simulator != nil {
					sim = fmt.Sprintf(", simulator $%.6f", r.Simulator.CostUSD)
				}
				fmt.Fprintf(w, "%s %s/%s  (%d decide, %d generate, $%.6f%s, %s)\n", mark, r.Suite, r.Scenario,
					r.Totals.Calls["decide"], r.Totals.Calls["generate"], r.Totals.CostUSD, sim, r.Duration.Round(1e6))
				for _, f := range r.Failures {
					fmt.Fprintf(w, "    - %s\n", f)
				}
				if verbose || !r.Pass {
					for _, line := range r.Transcript {
						fmt.Fprintf(w, "      %-12s %s\n", line.Who+":", strings.ReplaceAll(line.Text, "\n", " / "))
					}
				}
				if verbose {
					// The free mode's decisions, for calibrating
					// freeMode.functions' minConfidence.
					for _, hd := range r.Handoffs {
						fmt.Fprintf(w, "      handoff at turn %d: %s %v\n", hd.Turn, hd.Choice, hd.Probabilities)
					}
				}
			}
			tot := l.Totals()
			fmt.Fprintf(w, "\n%d scenarios, %d failed; spent $%.6f of $%.2f\n", len(results), failed, tot.CostUSD, maxCost)
			if out != "" {
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					return err
				}
				data, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return err
				}
				if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d scenarios failed", failed, len(results))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend (also the cassette's target)")
	cmd.Flags().StringVar(&generateSpec, "generate", "none", "Generate backend, or none")
	cmd.Flags().StringVar(&cassette, "cassette", "", "cassette file for Decide and Generate calls")
	cmd.Flags().StringVar(&cassMode, "cassette-mode", "replay", "record or replay (with --cassette)")
	cmd.Flags().StringVar(&only, "only", "", "run scenarios whose suite/scenario contains this text")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0.50, "hard budget for the whole run")
	cmd.Flags().Float64Var(&maxScenario, "max-scenario-cost-usd", 0.05, "budget of a scenario without its own")
	cmd.Flags().StringVar(&out, "out", "", "write results as JSON")
	cmd.Flags().StringVar(&journalPath, "journal", "", "keep the scenarios' runs in this SQLite journal (for tessera learn, tessera drift)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print every transcript")
	return cmd
}
