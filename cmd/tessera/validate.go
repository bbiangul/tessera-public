package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/explain"
)

func newValidateCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "validate <file-or-dir>...",
		Short: "Check resources before they run: schema, expressions, references, effect rules",
		Long: `Loads every .yaml, .yml and .json file given (directories are walked) and
checks them together: each document against the JSON Schema of its kind,
then expressions, references between inputs and sources, plugin actions and
their parameters, and the effect rules (idempotency keys, safe retries).
Plugin manifests must be among the files for the Functions that use them.

Exit code 0 when there are no errors (warnings are allowed), 1 otherwise.`,
		Example: "  tessera validate testdata/agenda\n  tessera validate -o json functions/ plugins/",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := admission.NewLoader()
			if err != nil {
				return err
			}
			checker, err := admission.NewChecker()
			if err != nil {
				return err
			}
			docs, issues := loader.LoadPaths(args)
			issues = append(issues, checker.Check(docs)...)

			errs := 0
			for _, is := range issues {
				if is.Severity == admission.SeverityError {
					errs++
				}
			}
			out := cmd.OutOrStdout()
			switch output {
			case "json":
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if issues == nil {
					issues = []admission.Issue{}
				}
				if err := enc.Encode(map[string]any{"resources": len(docs), "errors": errs, "issues": issues}); err != nil {
					return err
				}
			default:
				for _, is := range issues {
					fmt.Fprintln(out, is.String())
				}
				n := 0
				for _, d := range docs {
					if d.File != "<built-in>" {
						n++
					}
				}
				fmt.Fprintf(out, "%d resources, %d errors, %d warnings\n", n, errs, len(issues)-errs)
			}
			if errs > 0 {
				cmd.SilenceErrors = true
				return errors.New("validation failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "text | json")
	return cmd
}

func newExplainCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "explain <Kind>[.field.path]",
		Short: "Document any field of any kind",
		Long: `Shows the type, syntax and description of a field, and its sub-fields.
Map keys and list positions can be skipped: Function.spec.inputs.resolve
works without naming an input.`,
		Example: "  tessera explain Function\n  tessera explain Function.spec.action.idempotencyKey\n  tessera explain PluginManifest.spec.actions.effect",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Kinds: "+strings.Join(explain.Kinds(), ", "))
				fmt.Fprintln(cmd.OutOrStdout(), "Try: tessera explain Function.spec")
				return nil
			}
			f, err := explain.Explain(args[0])
			if err != nil {
				return err
			}
			if output == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(f)
			}
			fmt.Fprint(cmd.OutOrStdout(), f.Text())
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "text | json")
	return cmd
}
