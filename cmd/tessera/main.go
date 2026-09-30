// Command tessera is the CLI for people and coding agents.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/project"
)

func main() {
	compute.RunWorkerIfRequested() // an isolated script call (serve), then exit
	if err := newRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           project.CLI,
		Short:         project.Name + " — a declarative platform for agents that act on behalf of companies",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newVersionCmd(), newBenchCmd(), newValidateCmd(), newExplainCmd(), newChatCmd(), newTestCmd(), newMetricsCmd(), newServeCmd(), newRunsCmd(), newInitCmd(), newAddCmd(), newDriftCmd(), newLearnCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", project.CLI, project.Version)
		},
	}
}
