package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/drift"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newDriftCmd() *cobra.Command {
	var dbs []string
	var namespace, out string
	var since time.Duration
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "drift <files-or-dirs>...",
		Short: "Report signs that the agent's systems changed, with proposals (RFC-0005)",
		Long: `Reads the journal (read-only) and reports, per plugin action:
  refusals where the system said what it now wants (a new field, a fee, a rule);
  reads whose answer broke the plugin's declared type (CONTRACT);
  renamed fields repaired (improvise.repairReads) and request repairs (improvise.repair).

Proposals follow for a person or the authoring skill to review, never applied:
  a binding output that maps renamed fields back, once seen more than once;
  a note for the author when a request was repaired (new inputs, fees in the confirm).`,
		Example: "  tessera drift agent/ --db tessera-runs.db --namespace clinica --since 168h",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" || len(dbs) == 0 {
				return fmt.Errorf("--namespace and --db are required")
			}
			loader, err := admission.NewLoader()
			if err != nil {
				return err
			}
			docs, _ := loader.LoadPaths(args)
			manifests := map[string]*plugins.PluginManifest{}
			for _, d := range docs {
				if d.Plugin != nil {
					manifests[d.Plugin.Metadata.Name] = d.Plugin
				}
			}
			var stores []drift.Store
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
			r, err := drift.Analyze(context.Background(), stores, namespace, time.Now().Add(-since), manifests)
			if err != nil {
				return err
			}
			text := r.Markdown()
			if asJSON {
				b, _ := json.MarshalIndent(r, "", "  ")
				text = string(b) + "\n"
			}
			if out != "" {
				return os.WriteFile(out, []byte(text), 0o644)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), text)
			return err
		},
	}
	cmd.Flags().StringArrayVar(&dbs, "db", nil, "journal SQLite file (repeatable; globs allowed)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "the agent's namespace")
	cmd.Flags().DurationVar(&since, "since", 7*24*time.Hour, "look at runs started in this window")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON instead of Markdown")
	cmd.Flags().StringVar(&out, "out", "", "write here instead of stdout")
	return cmd
}
