package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/learn"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/mask"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newLearnCmd() *cobra.Command {
	var dbs []string
	var namespace, out, decideSpec, generateSpec, lessonsFile string
	var since time.Duration
	var min int
	var maxCost float64
	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Propose lessons from what the team answered (RFC-0008 part B)",
		Long: `Reads the journal (read-only) for escalate tasks of kind answer that a
person answered, and proposes lessons for a person to accept:
  answers about a customer's own case are rejected;
  each general answer becomes a general question and fact, with no one's data;
  the same questions are grouped; a group whose answers contradict is a conflict;
  a lesson needs --min answered cases.

Nothing changes until a person runs tessera learn accept, which writes the
accepted lessons into a KnowledgeBase file the free mode reads.`,
		Example: "  tessera learn --db tessera-runs.db --namespace clinica --out proposals.json\n  tessera learn accept --proposals proposals.json --ids estacionamento --to docs/licoes.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" || len(dbs) == 0 {
				return fmt.Errorf("--namespace and --db are required")
			}
			var stores []learn.Store
			for _, pattern := range dbs {
				files, err := filepath.Glob(pattern)
				if err != nil || len(files) == 0 {
					return fmt.Errorf("--db %s: no such file", pattern)
				}
				for _, f := range files {
					st, err := journal.OpenSQLite(f)
					if err != nil {
						return fmt.Errorf("%s: %w", f, err)
					}
					defer func() { _ = st.Close() }()
					stores = append(stores, st)
				}
			}
			l := ledger.New(ledger.Budget{MaxCostUSD: maxCost})
			d, err := buildDecider(decideSpec, l)
			if err != nil {
				return err
			}
			g, err := buildGenerator(generateSpec, l)
			if err != nil {
				return err
			}
			if g == nil {
				return fmt.Errorf("--generate is required: lessons are written by the Generate engine")
			}
			var lessons []learn.Lesson
			if lessonsFile != "" {
				if lessons, err = learn.ReadLessons(lessonsFile); err != nil {
					return err
				}
			}
			r, err := learn.Propose(context.Background(), stores, learn.Options{
				Namespace: namespace, Since: time.Now().Add(-since), Min: min, Decider: d, Generator: g, Mask: mask.Text, Lessons: lessons,
			})
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(r, "", "  ")
			if out != "" {
				if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
					return err
				}
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d answered cases: %d proposals, %d conflicts, %d rejected, %d below --min %d; spent $%.4f\n",
				r.Answered, len(r.Proposals), len(r.Conflicts), len(r.Rejected), len(r.Pending), min, l.Totals().CostUSD)
			for _, p := range r.Proposals {
				fmt.Fprintf(w, "\n[%s] (%s, %d cases) %s\n  %s\n", p.ID, p.Kind, len(p.Evidence), p.Question, p.Fact)
			}
			for _, m := range r.Contradicts {
				fmt.Fprintf(w, "\nCONTRADICTS lesson %s (%q): %s\n  accept with --supersede %s if the lesson is outdated\n", m.Lesson, m.Fact, m.Proposal.Fact, m.Lesson)
			}
			for _, m := range r.Reconfirms {
				fmt.Fprintf(w, "\nreconfirms lesson %s (%d cases): renew it with tessera learn renew --ids %s\n", m.Lesson, len(m.Proposal.Evidence), m.Lesson)
			}
			for _, c := range r.Conflicts {
				fmt.Fprintf(w, "\nCONFLICT (%d cases) %s\n", len(c.Cases), c.Question)
				for _, x := range c.Cases {
					fmt.Fprintf(w, "  - %s\n", x.Answer)
				}
			}
			for _, c := range r.Rejected {
				fmt.Fprintf(w, "\nrejected: %s\n  %q\n", c.Rejected, c.Question)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&dbs, "db", nil, "journal SQLite file (repeatable; globs allowed)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "the agent's namespace")
	cmd.Flags().DurationVar(&since, "since", 30*24*time.Hour, "look at runs started in this window")
	cmd.Flags().IntVar(&min, "min", 2, "answered cases a lesson needs")
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend")
	cmd.Flags().StringVar(&generateSpec, "generate", "openrouter:deepseek/deepseek-v4.1-flash", "Generate backend")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0.50, "hard budget")
	cmd.Flags().StringVar(&out, "out", "", "write the proposals (JSON) here")
	cmd.Flags().StringVar(&lessonsFile, "lessons", "", "the lessons file: proposals on the same question as an active lesson are reported as contradicting or reconfirming it")
	cmd.AddCommand(newLearnAcceptCmd(), newLearnReviewCmd(), newLearnSetCmd("renew"), newLearnSetCmd("retire"))
	return cmd
}

func newLearnAcceptCmd() *cobra.Command {
	var proposals, to string
	var ids, supersede []string
	cmd := &cobra.Command{
		Use:   "accept",
		Short: "Write accepted lessons into a KnowledgeBase file",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(proposals)
			if err != nil {
				return err
			}
			var r learn.Report
			if err := json.Unmarshal(raw, &r); err != nil {
				return fmt.Errorf("%s: %w", proposals, err)
			}
			all := r.Proposals
			for _, m := range append(r.Contradicts, r.Reconfirms...) {
				all = append(all, m.Proposal)
			}
			wrote, err := learn.AcceptSuperseding(to, all, ids, supersede, time.Now())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %d lesson(s) to %s: %s\n", len(wrote), to, strings.Join(wrote, ", "))
			return err
		},
	}
	cmd.Flags().StringVar(&proposals, "proposals", "", "the proposals file (tessera learn --out)")
	cmd.Flags().StringSliceVar(&ids, "ids", nil, "proposal ids to accept")
	cmd.Flags().StringVar(&to, "to", "", "the lessons Markdown file (a KnowledgeBase source)")
	cmd.Flags().StringSliceVar(&supersede, "supersede", nil, "lesson ids the accepted ones replace (retired, superseded)")
	_ = cmd.MarkFlagRequired("proposals")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("ids")
	return cmd
}

func newLearnReviewCmd() *cobra.Command {
	var dbs []string
	var namespace, lessonsFile, decideSpec, out, nowFlag string
	var apply bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Report lessons that are due, dead or harmful; --apply suspends the harmful ones (RFC-0008 part C)",
		Long: `Reads the journal (read-only) and the lessons file:
  due: the review date is within 14 days or past (past: the agent no longer uses it);
  dead: not used for 90 days (retire it with tessera learn retire);
  harmful: disputed by the user right after being used, at least twice and in
    at least 30% of its uses. --apply suspends these in the file.
Renew a lesson that still holds with tessera learn renew.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" || len(dbs) == 0 || lessonsFile == "" {
				return fmt.Errorf("--namespace, --db and --lessons are required")
			}
			now := time.Now()
			if nowFlag != "" {
				t, err := time.Parse("2006-01-02", nowFlag)
				if err != nil {
					return fmt.Errorf("--now: %w", err)
				}
				now = t
			}
			var stores []learn.Store
			for _, pattern := range dbs {
				files, err := filepath.Glob(pattern)
				if err != nil || len(files) == 0 {
					return fmt.Errorf("--db %s: no such file", pattern)
				}
				for _, f := range files {
					st, err := journal.OpenSQLite(f)
					if err != nil {
						return fmt.Errorf("%s: %w", f, err)
					}
					defer func() { _ = st.Close() }()
					stores = append(stores, st)
				}
			}
			lessons, err := learn.ReadLessons(lessonsFile)
			if err != nil {
				return err
			}
			l := ledger.New(ledger.Budget{MaxCostUSD: 0.5})
			d, err := buildDecider(decideSpec, l)
			if err != nil {
				return err
			}
			rs, err := learn.Review(context.Background(), stores, lessons, learn.ReviewOptions{Namespace: namespace, Now: now, Decider: d, Mask: mask.Text})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for i, r := range rs {
				flags := []string{r.Status()}
				if r.Expired {
					flags = append(flags, "EXPIRED")
				} else if r.Due {
					flags = append(flags, "due")
				}
				if r.Dead {
					flags = append(flags, "DEAD")
				}
				if r.Harmful {
					flags = append(flags, "HARMFUL")
				}
				fmt.Fprintf(w, "%s [%s] used %d, disputed %d, review %s\n  %s\n", r.ID, strings.Join(flags, " "), len(r.Uses), r.Disputes, r.Meta["review"], r.Question)
				if apply && r.Harmful && r.Status() == "active" {
					if _, err := learn.SetMeta(lessonsFile, []string{r.ID}, map[string]string{"status": "suspended", "reason": "harmful", "on": now.Format("2006-01-02")}); err != nil {
						return err
					}
					rs[i].Applied = "suspended"
					fmt.Fprintf(w, "  -> suspended (harmful)\n")
				}
			}
			if out != "" {
				b, _ := json.MarshalIndent(rs, "", "  ")
				return os.WriteFile(out, append(b, '\n'), 0o644)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&dbs, "db", nil, "journal SQLite file (repeatable; globs allowed)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "the agent's namespace")
	cmd.Flags().StringVar(&lessonsFile, "lessons", "", "the lessons file")
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend (reads disputes)")
	cmd.Flags().BoolVar(&apply, "apply", false, "suspend the harmful lessons in the file")
	cmd.Flags().StringVar(&out, "out", "", "write the review (JSON) here")
	cmd.Flags().StringVar(&nowFlag, "now", "", "review as of this date (YYYY-MM-DD); default today")
	return cmd
}

// newLearnSetCmd is renew or retire.
func newLearnSetCmd(action string) *cobra.Command {
	var file, reason string
	var ids []string
	short := "Set lessons' review date to today plus their validity"
	if action == "retire" {
		short = "Retire lessons (they stay in the file, unused)"
	}
	cmd := &cobra.Command{
		Use:   action,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			var done []string
			var err error
			if action == "renew" {
				done, err = learn.Renew(file, ids, time.Now())
			} else {
				done, err = learn.SetMeta(file, ids, map[string]string{"status": "retired", "reason": reason, "on": time.Now().Format("2006-01-02")})
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", action, strings.Join(done, ", "))
			return err
		},
	}
	cmd.Flags().StringVar(&file, "lessons", "", "the lessons file")
	cmd.Flags().StringSliceVar(&ids, "ids", nil, "lesson ids")
	if action == "retire" {
		cmd.Flags().StringVar(&reason, "reason", "dead", "why (no spaces)")
	}
	_ = cmd.MarkFlagRequired("lessons")
	_ = cmd.MarkFlagRequired("ids")
	return cmd
}
