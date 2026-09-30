package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/initkit"
)

func newAddCmd() *cobra.Command {
	add := &cobra.Command{Use: "add", Short: "Add a built-in plugin to an agent"}
	var provider, dir string
	webCmd := &cobra.Command{
		Use:   "web",
		Short: "Add web search for the free mode (TinyFish by default, free)",
		Long: `Writes plugins/web.yaml, a web search every provider implements the
same way. Then switch it on in the Agent:

  freeMode:
    search: { act: web/search }

Jev decides which messages need a search. A query with anything about a
person is never sent. The results reach the model as third-party pages:
cited, never obeyed (websearch-001).`,
		Example: "  tessera add web\n  tessera add web --provider searxng --dir minha-clinica",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := initkit.WriteWeb(dir, provider)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote %s (%s).\n\nSwitch it on in agent.yaml:\n  freeMode:\n    search: { act: web/search }\n", path, provider)
			if secret := webSecrets[provider]; secret != "" {
				fmt.Fprintf(out, "\nIts key is the secret %s (the environment variable TESSERA_SECRET_%s).\n", secret, secret)
			}
			if provider == "searxng" {
				fmt.Fprintln(out, "\nSet baseUrl and egress.hosts to your SearXNG instance (https, json format on).")
			}
			return nil
		},
	}
	webCmd.Flags().StringVar(&provider, "provider", "tinyfish", "one of "+strings.Join(initkit.WebProviders(), ", "))
	webCmd.Flags().StringVar(&dir, "dir", ".", "the agent's folder")
	var tenant, site string
	spCmd := &cobra.Command{
		Use:   "sharepoint",
		Short: "Add SharePoint (Microsoft Graph) as a source of knowledge documents",
		Long: `Writes plugins/sharepoint.yaml: list a folder's files and download them
through Microsoft Graph, app-only (OAuth2 client credentials). Use it as a
KnowledgeBase source:

  sources:
    - list:  { act: sharepoint/list-files, with: { folder: "Politicas" } }
      fetch: { act: sharepoint/download, with: { id: "${ inputs.id }" } }
      refresh: 24h

PDF, DOCX, Markdown and text files are read; the index is refreshed every
refresh, and a failed read keeps the documents read before (plugin-001).`,
		Example: "  tessera add sharepoint --tenant 1111-... --site contoso.sharepoint.com,2222-...,3333-...",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := initkit.WriteSharePoint(dir, tenant, site)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s.\n\nIts credentials are the secrets SHAREPOINT_CLIENT_ID and SHAREPOINT_CLIENT_SECRET\n(TESSERA_SECRET_SHAREPOINT_CLIENT_ID, TESSERA_SECRET_SHAREPOINT_CLIENT_SECRET):\nan Entra app with the application permission Sites.Selected (granted to this site) or Sites.Read.All.\n", path)
			return err
		},
	}
	spCmd.Flags().StringVar(&tenant, "tenant", "", "the Entra (Azure AD) tenant id")
	spCmd.Flags().StringVar(&site, "site", "", "the site id (GET https://graph.microsoft.com/v1.0/sites/{host}:/sites/{name})")
	spCmd.Flags().StringVar(&dir, "dir", ".", "the agent's folder")
	add.AddCommand(webCmd, spCmd)
	return add
}

var webSecrets = map[string]string{"tinyfish": "TINYFISH_API_KEY", "tavily": "TAVILY_API_KEY", "brave": "BRAVE_API_KEY"}
