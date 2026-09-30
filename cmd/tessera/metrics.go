package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "metrics", Short: "Business numbers from milestones (funnels)"}
	var (
		namespace, name, db, by, at, output string
	)
	funnel := &cobra.Command{
		Use:   "funnel <paths...>",
		Short: "A funnel's conversion, time per stage and stuck subjects, by cohort",
		Long: `Reads the milestones in a runs database (the --db of tessera chat, or a
host's journal) and computes a Funnel declared in the paths: for each
cohort, how many reached each stage from the previous one, how many are
still pending inside the window, how long each stage took (median and
p90), and who waits longer than expected.`,
		Example: `  tessera metrics funnel testdata/lojas --namespace rede --funnel abertura --db tessera-chat.db`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := admission.NewLoader()
			if err != nil {
				return err
			}
			c, err := admission.NewChecker()
			if err != nil {
				return err
			}
			docs, issues := l.LoadPaths(args)
			issues = append(issues, c.Check(docs)...)
			for _, is := range issues {
				if is.Severity == admission.SeverityError {
					return fmt.Errorf("resources do not validate: %s", is.String())
				}
			}
			var f *agents.Funnel
			zone := ""
			for _, d := range docs {
				if d.Funnel != nil && d.Funnel.Metadata.Namespace == namespace && d.Funnel.Metadata.Name == name {
					f = d.Funnel
				}
				if d.Agent != nil && d.Agent.Metadata.Namespace == namespace {
					zone = d.Agent.Spec.Timezone
				}
			}
			if f == nil {
				return fmt.Errorf("no Funnel %q in namespace %s", name, namespace)
			}
			if zone == "" {
				return errors.New("the namespace's Agent needs spec.timezone: cohorts start at local midnight")
			}
			loc, err := time.LoadLocation(zone)
			if err != nil {
				return err
			}
			now := time.Now()
			if at != "" {
				if now, err = time.Parse(time.RFC3339, at); err != nil {
					return fmt.Errorf("--at: %w", err)
				}
			}
			store, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			ms, err := store.Milestones(cmd.Context(), namespace, f.Spec.Subject)
			if err != nil {
				return err
			}
			rep, err := metrics.Compute(f, ms, now, loc, by)
			if err != nil {
				return err
			}
			if output == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			printFunnel(cmd.OutOrStdout(), f, rep, loc)
			return nil
		},
	}
	funnel.Flags().StringVar(&namespace, "namespace", "", "namespace of the Funnel (required)")
	funnel.Flags().StringVar(&name, "funnel", "", "Funnel name (required)")
	funnel.Flags().StringVar(&db, "db", "tessera-chat.db", "runs database (SQLite) with the milestones")
	funnel.Flags().StringVar(&by, "by", "", "cut by a Subject attribute listed in the Funnel's dimensions")
	funnel.Flags().StringVar(&at, "at", "", "compute as of this time (RFC 3339); default now")
	funnel.Flags().StringVarP(&output, "output", "o", "text", "text or json")
	_ = funnel.MarkFlagRequired("namespace")
	_ = funnel.MarkFlagRequired("funnel")
	cmd.AddCommand(funnel, newExperimentCmd())
	return cmd
}

func newExperimentCmd() *cobra.Command {
	var namespace, name, db, at, output string
	cmd := &cobra.Command{
		Use:   "experiment <paths...>",
		Short: "An A/B test's arms, evidence and verdict",
		Long: `Reads the exposures and milestones in a runs database and compares the
variants of an Experiment on its primary Funnel stage. The evidence is a
sequential test (mSPRT): the verdict stays "inconclusive" until each
variant has the minimum sample and the evidence passes 1/alpha, and
reading it every day does not raise false positives. A low SRM p-value
means the split itself is broken: do not trust the verdict.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			docs, err := loadValid(args)
			if err != nil {
				return err
			}
			var x *agents.Experiment
			funnels := map[string]*agents.Funnel{}
			zone := "UTC"
			for _, d := range docs {
				if d.Experiment != nil && d.Experiment.Metadata.Namespace == namespace && d.Experiment.Metadata.Name == name {
					x = d.Experiment
				}
				if d.Funnel != nil && d.Funnel.Metadata.Namespace == namespace {
					funnels[d.Funnel.Metadata.Name] = d.Funnel
				}
				if d.Agent != nil && d.Agent.Metadata.Namespace == namespace && d.Agent.Spec.Timezone != "" {
					zone = d.Agent.Spec.Timezone
				}
			}
			if x == nil {
				return fmt.Errorf("no Experiment %q in namespace %s", name, namespace)
			}
			loc, err := time.LoadLocation(zone)
			if err != nil {
				return err
			}
			now := time.Now()
			if at != "" {
				if now, err = time.Parse(time.RFC3339, at); err != nil {
					return fmt.Errorf("--at: %w", err)
				}
			}
			store, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			ms, err := store.Milestones(cmd.Context(), namespace, x.Spec.Subject)
			if err != nil {
				return err
			}
			rep, err := metrics.AnalyzeExperiment(x, funnels[x.Spec.Primary.Funnel], ms, now, loc)
			if err != nil {
				return err
			}
			if _, err := store.GetRun(cmd.Context(), runtime.ExperimentStopRun(namespace, x.Metadata.Name)); err == nil {
				rep.Stopped = true
			} else if !errors.Is(err, journal.ErrNotFound) {
				return err
			}
			w := cmd.OutOrStdout()
			if output == "json" {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			fmt.Fprintf(w, "Experiment %s: %s reached, among the exposed\n\n", x.Metadata.Name, x.Spec.Primary.Stage)
			fmt.Fprintf(w, "  %-16s %8s %8s %10s %8s %16s %10s\n", "variant", "exposed", "decided", "converted", "rate", "95% interval", "vs control")
			for i, a := range rep.Verdict.Arms {
				diff := "control"
				if i > 0 {
					diff = fmt.Sprintf("%+.1f pts", 100*a.Difference)
				}
				fmt.Fprintf(w, "  %-16s %8d %8d %10d %7.1f%% %7.1f–%-6.1f%% %10s\n", a.Name, a.Exposed, a.Decided, a.Converted, 100*a.Rate, 100*a.Low, 100*a.High, diff)
			}
			for _, c := range rep.Verdict.Comparisons {
				fmt.Fprintf(w, "\n  %s: evidence %.1f (significant at %.1f)\n", c.Variant, c.Evidence, 1/alphaOf(x))
			}
			fmt.Fprintf(w, "\nverdict: %s\n", rep.Verdict.Decision)
			for _, g := range rep.Guardrails {
				fmt.Fprintf(w, "\nguardrail %s:", g.Name)
				for _, a := range g.Arms {
					fmt.Fprintf(w, " %s %.1f%% (%d/%d)", a.Variant, 100*a.Rate, a.Hit, a.Decided)
				}
				if g.Breach != "" {
					fmt.Fprintf(w, " BREACHED by %s (%+.1f pts)", g.Breach, 100*g.Difference)
				}
				fmt.Fprintln(w)
			}
			if rep.Stopped {
				fmt.Fprintln(w, "STOPPED by a guardrail: subjects exposed since get the control and are left out of this comparison")
			}
			if rep.SRM < 0.001 {
				fmt.Fprintf(w, "WARNING: sample ratio mismatch (p=%.2g): the split or the exposure logging is broken; do not trust the verdict\n", rep.SRM)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (required)")
	cmd.Flags().StringVar(&name, "experiment", "", "Experiment name (required)")
	cmd.Flags().StringVar(&db, "db", "tessera-chat.db", "runs database (SQLite)")
	cmd.Flags().StringVar(&at, "at", "", "compute as of this time (RFC 3339); default now")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "text or json")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("experiment")
	return cmd
}

func alphaOf(x *agents.Experiment) float64 {
	if x.Spec.Alpha == 0 {
		return 0.05
	}
	return x.Spec.Alpha
}

// loadValid loads resources and refuses them when admission finds errors.
func loadValid(args []string) ([]admission.Doc, error) {
	l, err := admission.NewLoader()
	if err != nil {
		return nil, err
	}
	c, err := admission.NewChecker()
	if err != nil {
		return nil, err
	}
	docs, issues := l.LoadPaths(args)
	issues = append(issues, c.Check(docs)...)
	for _, is := range issues {
		if is.Severity == admission.SeverityError {
			return nil, fmt.Errorf("resources do not validate: %s", is.String())
		}
	}
	return docs, nil
}

func days(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < 48*time.Hour {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

func printFunnel(w io.Writer, f *agents.Funnel, rep metrics.Report, loc *time.Location) {
	fmt.Fprintf(w, "Funnel %s (%s), as of %s\n", f.Metadata.Name, f.Spec.Subject, rep.At.In(loc).Format("2006-01-02 15:04"))
	if len(rep.Cohorts) == 0 {
		fmt.Fprintln(w, "  no subject has entered yet")
		return
	}
	for _, c := range rep.Cohorts {
		group := ""
		if c.Group != "" {
			group = " · " + c.Group
		}
		fmt.Fprintf(w, "\ncohort %s%s: %d entered (%s)\n", c.Start.In(loc).Format("2006-01-02"), group, c.Entered, f.Spec.Stages[0].Name)
		fmt.Fprintf(w, "  %-14s %8s %8s %6s %11s %8s %8s  %s\n", "stage", "reached", "pending", "lost", "conversion", "median", "p90", "stuck")
		for _, s := range c.Stages {
			conv := "-"
			if s.Reached+s.Lost > 0 {
				conv = fmt.Sprintf("%.0f%%", 100*s.Conversion)
			}
			stuck := ""
			if len(s.Stuck) > 0 {
				ids := make([]string, len(s.Stuck))
				for i, x := range s.Stuck {
					ids[i] = x.ID
				}
				stuck = fmt.Sprintf("%d (%s)", len(s.Stuck), strings.Join(ids, ", "))
			}
			fmt.Fprintf(w, "  %-14s %8d %8d %6d %11s %8s %8s  %s\n", s.Name, s.Reached, s.Pending, s.Lost, conv, days(s.Median), days(s.P90), stuck)
		}
		fmt.Fprintf(w, "  %-14s %8d %35s %8s %8s\n", "end to end", c.Total.Reached, "", days(c.Total.Median), days(c.Total.P90))
	}
}
