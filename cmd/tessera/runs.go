package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/inspect"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newRunsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "runs", Short: "Inspect runs in a runs database, without a server",
		Long: `The same inspection as the console and GET /v1/runs, read straight from a
runs database (tessera chat --db, tessera serve --db): what each run did,
every decision and call explained. For people and coding agents
troubleshooting an agent; -o json for machines.`}
	var db, conversation, function, status, output string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "Runs, newest first",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			runs, err := st.Runs(cmd.Context(), journal.RunFilter{Conversation: conversation, Function: function, Status: journal.Status(status), Limit: limit})
			if err != nil {
				return err
			}
			if output == "json" {
				return writeJSON(cmd.OutOrStdout(), runs)
			}
			for _, r := range runs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-16s %-26s %-24s %s\n", r.CreatedAt.Format("2006-01-02 15:04:05"), r.Status, r.Function, r.Conversation, r.ID)
			}
			return nil
		},
	}
	list.Flags().StringVar(&conversation, "conversation", "", "only this conversation")
	list.Flags().StringVar(&function, "function", "", "only this Function")
	list.Flags().StringVar(&status, "status", "", "only this status (Failed, Indeterminate, WaitingForHuman...)")
	list.Flags().IntVar(&limit, "limit", 50, "at most this many")
	show := &cobra.Command{
		Use:   "show <run-id> [paths...]",
		Short: "One run, explained record by record",
		Long:  "With the definitions' paths, each record also says which input source, step or action it came from.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			run, err := st.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			recs, err := st.Records(cmd.Context(), run.ID)
			if err != nil {
				return err
			}
			x := inspect.New(nil)
			if len(args) > 1 {
				docs, err := loadValid(args[1:])
				if err != nil {
					return err
				}
				x = inspect.New(docs)
			}
			out, err := x.Explain(run, recs)
			if err != nil {
				return err
			}
			if output == "json" {
				return writeJSON(cmd.OutOrStdout(), out)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s  %s  %s  conversation %s\n", out.ID, out.Function, out.Status, out.Conversation)
			for _, e := range out.Timeline {
				fmt.Fprintf(w, "%3d %-9s %-50s %s\n", e.Seq, e.Kind, cut(e.What, 50), e.Summary)
			}
			return nil
		},
	}
	var namespace string
	var after int64
	var once bool
	follow := &cobra.Command{
		Use:   "follow [paths...]",
		Short: "Every record of every run as it is written (like GET /v1/activity/stream)",
		Long: `Prints each new journal record of the namespace's runs, explained, until
interrupted; the database may be in use by tessera serve or chat. Starts
after --after (a cursor it printed) or from the beginning; --once prints
what is there and stops. With the definitions' paths, records say which
step or source they came from.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			x := inspect.New(nil)
			if len(args) > 0 {
				docs, err := loadValid(args)
				if err != nil {
					return err
				}
				x = inspect.New(docs)
			}
			w := cmd.OutOrStdout()
			ctx := cmd.Context()
			for {
				changes, err := st.Changes(ctx, namespace, after, 500)
				if err != nil {
					return err
				}
				for _, c := range changes {
					run, err := st.GetRun(ctx, c.RunID)
					if err != nil {
						return err
					}
					ex, err := x.Explain(run, []journal.Record{c.Record})
					if err != nil {
						return err
					}
					e := ex.Timeline[0]
					if output == "json" {
						b, _ := json.Marshal(map[string]any{"cursor": c.Cursor, "runId": run.ID, "function": run.Function, "conversation": run.Conversation, "entry": e})
						fmt.Fprintln(w, string(b))
					} else {
						fmt.Fprintf(w, "%6d %s %-22s %-24s %-9s %s\n", c.Cursor, e.At.Format("15:04:05"), cut(run.Function, 22), cut(run.Conversation, 24), e.Kind, e.Summary)
					}
					after = c.Cursor
				}
				if len(changes) == 500 {
					continue
				}
				if once {
					return nil
				}
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(500 * time.Millisecond):
				}
			}
		},
	}
	follow.Flags().StringVar(&namespace, "namespace", "", "namespace (required)")
	follow.Flags().Int64Var(&after, "after", 0, "start after this cursor")
	follow.Flags().BoolVar(&once, "once", false, "print what is there and stop")
	_ = follow.MarkFlagRequired("namespace")
	for _, c := range []*cobra.Command{list, show, follow} {
		c.Flags().StringVar(&db, "db", "tessera-chat.db", "runs database (SQLite)")
		c.Flags().StringVarP(&output, "output", "o", "text", "text or json")
	}
	cmd.AddCommand(list, show, follow)
	return cmd
}

func cut(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
