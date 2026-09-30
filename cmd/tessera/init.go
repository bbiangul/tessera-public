package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/initkit"
	"github.com/bbiangul/tessera/internal/project"
)

func newInitCmd() *cobra.Command {
	var namespace, image string
	cmd := &cobra.Command{
		Use:   "init <dir>",
		Short: "Create a new agent's repository: a working agent, the authoring skill, guides and a Dockerfile",
		Long: `Creates <dir> (which must not exist or be empty) with:
  a small working agent: agent.yaml, functions/, plugins/, evals/;
  AGENTS.md and .claude/skills/tessera-author, so Claude Code can build
    your agent from a plain-language request;
  .tessera/: the guides and reference examples;
  a Dockerfile on the Tessera image.`,
		Example: `  tessera init minha-clinica --namespace clinica`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				namespace = strings.ToLower(filepath.Base(filepath.Clean(args[0])))
			}
			if image == "" {
				image = defaultImage()
			}
			files, err := initkit.Write(args[0], initkit.Options{Namespace: namespace, Image: image})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Created %s (namespace %s, %d files).\n\n", args[0], namespace, len(files))
			fmt.Fprintf(out, "Next:\n  cd %s\n  %s validate .\n  claude    # then describe the agent you want\n", args[0], project.CLI)
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "the agent's namespace (default: the folder's name)")
	cmd.Flags().StringVar(&image, "image", "", "the Tessera image in the Dockerfile (default: this version's)")
	return cmd
}

// defaultImage is this binary's image; a development build has no release
// tag, so it points at latest.
func defaultImage() string {
	v := project.Version
	if v == "" || v == "dev" || strings.Contains(v, "dirty") || !strings.HasPrefix(v, "v") {
		return "ghcr.io/bbiangul/tessera:latest"
	}
	return "ghcr.io/bbiangul/tessera:" + strings.TrimPrefix(v, "v")
}
