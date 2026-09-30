package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/channels/email"
	"github.com/bbiangul/tessera/internal/channels/slack"
	"github.com/bbiangul/tessera/internal/channels/teams"
	"github.com/bbiangul/tessera/internal/channels/whatsapp"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/mailmsg"
	httpplugin "github.com/bbiangul/tessera/internal/plugins/http"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
	"github.com/bbiangul/tessera/internal/server"
	"github.com/bbiangul/tessera/internal/tasklink"
)

func newServeCmd() *cobra.Command {
	var (
		namespace, db, dataDir, addr                           string
		decideSpec, generateSpec                               string
		publicURL                                              string
		maxCost                                                float64
		allowPrivate, dev, whatsApp, slackOn, teamsOn, emailOn bool
		inProcessScripts                                       bool
	)
	cmd := &cobra.Command{
		Use:   "serve <files-or-dirs>...",
		Short: "Run an agent as an HTTP service, with its scheduler",
		Long: `One process: the HTTP API, the scheduler (timers and schedule
Triggers, every second) and the journal (SQLite).

  POST /v1/messages           {conversation, caller, text}: a user's message
  GET  /v1/outbox             messages not acknowledged yet, by run
  POST /v1/runs/{id}/ack      {seq}: delivered up to seq
  POST /v1/runs/{id}/reply    {from: "user"|"human", text, value}
  POST /v1/hooks/{trigger}    a webhook Trigger; signed with
                              X-Tessera-Timestamp and X-Tessera-Signature:
                              sha256=<HMAC of timestamp.key.body>
  GET  /v1/agent, /v1/runs, /v1/runs/{id}, /v1/conversations/{id}
                              inspection: every decision and call, explained
  POST /v1/triggers/{name}/fire, /v1/tick, /v1/dev/clock (--dev)
  GET  /v1/metrics/funnels/{name}, /v1/metrics/experiments/{name}
  GET  /v1/openapi.json       the API, for coding agents
  GET  /                      the web console (a client of this API)
  GET/POST /tasks/{token}     a task's page, from a signed link in a
                              notification (--public-url)
  GET  /health

/v1 needs Authorization: Bearer $TESSERA_API_TOKEN (webhooks are signed
instead). The client holding the token is trusted with the caller's
identity: put a channel adapter in front, never a browser.`,
		Example: `  TESSERA_API_TOKEN=... tessera serve testdata/lojas --namespace rede --db runs.db`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := os.Getenv("TESSERA_API_TOKEN")
			if len(token) < 32 {
				return errors.New("set TESSERA_API_TOKEN (32 characters or more, random: openssl rand -hex 32): the API never runs without one")
			}
			if maxCost <= 0 {
				return errors.New("--max-cost-usd must be positive: model calls always run with a budget")
			}
			if !inProcessScripts {
				compute.Isolate(&compute.Isolation{})
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
			release, err := journal.LockRunner(db)
			if err != nil {
				return err
			}
			defer release()
			store, err := journal.OpenSQLite(db)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			var clock *server.DevClock
			var configure func(*runtime.Runtime)
			if dev {
				clock = &server.DevClock{}
				configure = func(rt *runtime.Runtime) { rt.Clock = clock.Now }
			}
			h, issues, err := host.Load(host.Options{
				Paths: args, Namespace: namespace, Store: store, Decider: d, Generator: g, Secrets: httpplugin.EnvSecrets{},
				AllowPrivateNetworks: allowPrivate, DataDir: dataDir, Configure: configure,
			})
			for _, is := range issues {
				fmt.Fprintln(cmd.ErrOrStderr(), is.String())
			}
			if err != nil {
				return err
			}
			defer func() { _ = h.Close() }()

			var wa *whatsapp.Adapter
			if whatsApp {
				if wa, err = openWhatsApp(h, dataDir); err != nil {
					return err
				}
				defer func() { _ = wa.Close() }()
			}
			var sl *slack.Adapter
			if slackOn {
				if sl, err = openSlack(h, dataDir); err != nil {
					return err
				}
				defer func() { _ = sl.Close() }()
			}
			var tm *teams.Adapter
			if teamsOn {
				if tm, err = openTeams(h, dataDir); err != nil {
					return err
				}
				defer func() { _ = tm.Close() }()
			}
			api := &server.Server{Host: h, Token: token, Dev: clock, PublicURL: publicURL}
			if publicURL != "" {
				if err := os.MkdirAll(dataDir, 0o700); err != nil {
					return err
				}
				if api.LinkSecret, err = tasklink.LoadSecret(os.Getenv("TESSERA_LINK_SECRET"), filepath.Join(dataDir, "link-secret")); err != nil {
					return fmt.Errorf("task links: %w", err)
				}
			}
			var em *email.Adapter
			if emailOn {
				if em, err = openEmail(h, api, dataDir); err != nil {
					return err
				}
				defer func() { _ = em.Close() }()
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			ticking := make(chan struct{})
			// Deferred after the adapters' Close: on the way out the ticker
			// stops and is waited for before any adapter's database closes
			// under it (#54).
			defer func() { stop(); <-ticking }()
			go func() {
				defer close(ticking)
				t := time.NewTicker(time.Second)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						// Results wait in the outbox; the channel polls it.
						if _, err := h.Tick(ctx); err != nil && ctx.Err() == nil {
							fmt.Fprintln(cmd.ErrOrStderr(), "scheduler:", err)
						}
						// People hear about tasks through their queues' notify actions.
						if _, err := h.Notify(ctx, api.TaskLink); err != nil && ctx.Err() == nil {
							fmt.Fprintln(cmd.ErrOrStderr(), "notify:", err)
						}
						if wa != nil {
							if _, err := wa.Deliver(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "whatsapp:", err)
							}
							if _, err := wa.DeliverTasks(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "whatsapp tasks:", err)
							}
						}
						if sl != nil {
							if _, err := sl.Deliver(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "slack:", err)
							}
							if _, err := sl.DeliverTasks(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "slack tasks:", err)
							}
						}
						if em != nil {
							if _, err := em.DeliverTasks(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "email tasks:", err)
							}
						}
						if tm != nil {
							if _, err := tm.Deliver(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "teams:", err)
							}
							if _, err := tm.DeliverTasks(ctx); err != nil && ctx.Err() == nil {
								fmt.Fprintln(cmd.ErrOrStderr(), "teams tasks:", err)
							}
						}
					}
				}
			}()
			var handler http.Handler = api.Handler()
			if wa != nil || sl != nil || tm != nil {
				mux := http.NewServeMux()
				if wa != nil {
					mux.Handle("/channels/whatsapp/", wa.Handler())
				}
				if sl != nil {
					mux.Handle("/channels/slack/", sl.Handler())
				}
				if tm != nil {
					mux.Handle("/channels/teams/", tm.Handler())
				}
				mux.Handle("/", handler)
				handler = mux
			}
			srv := &http.Server{Addr: addr, Handler: handler,
				ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second}
			errc := make(chan error, 1)
			go func() { errc <- srv.ListenAndServe() }()
			mode := ""
			if dev {
				mode = ", development clock"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "serving %s on http://%s (console at /, API at /v1, budget $%.2f%s)\n", namespace, addr, maxCost, mode)
			select {
			case err := <-errc:
				return err
			case <-ctx.Done():
				shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return srv.Shutdown(shut)
			}
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace to serve (required)")
	cmd.Flags().StringVar(&db, "db", "tessera-runs.db", "SQLite file for runs")
	cmd.Flags().StringVar(&dataDir, "data-dir", "tessera-data", "directory for the sql plugins' databases")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "listen address")
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend")
	cmd.Flags().StringVar(&generateSpec, "generate", "openrouter:deepseek/deepseek-v4.1-flash", "Generate backend, or none")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 5, "hard budget for model calls while the process runs")
	cmd.Flags().BoolVar(&allowPrivate, "allow-private-networks", false, "let http plugins reach private addresses")
	cmd.Flags().BoolVar(&whatsApp, "whatsapp", false, "connect the WhatsApp Cloud API: Meta's webhook at /channels/whatsapp/webhook (TESSERA_WHATSAPP_VERIFY_TOKEN, TESSERA_SECRET_WHATSAPP_APP_SECRET; TESSERA_WHATSAPP_PHONE_NUMBER_ID keeps only that number's messages), replies sent through the Agent's channel plugin")
	cmd.Flags().BoolVar(&slackOn, "slack", false, "present approval tasks of queues with channels: [slack] in the members' Slack DMs: interactivity requests at /channels/slack/interactions; with the slack plugin's post-text, members also talk to the agent in DMs (Events API at /channels/slack/events) (TESSERA_SECRET_SLACK_SIGNING_SECRET, TESSERA_SLACK_TEAM_ID), posted through the slack plugin")
	cmd.Flags().BoolVar(&teamsOn, "teams", false, "present approval tasks of queues with channels: [teams] in the members' personal chats with the bot: the Bot Framework's messaging endpoint at /channels/teams/messages (TESSERA_TEAMS_APP_ID, TESSERA_TEAMS_TENANT_ID; TESSERA_TEAMS_OPENID_URL for a sovereign cloud), sent through the teams plugin; with its post-text, members also talk to the agent in their personal chat")
	cmd.Flags().BoolVar(&emailOn, "email", false, "e-mail approval tasks of queues with channels: [email] to the members, with a link bound to each (needs --public-url; answered only through the link with a one-time code), sent through the email plugin's send-task (TESSERA_EMAIL_FROM for the raw message's From)")
	cmd.Flags().StringVar(&publicURL, "public-url", "", "the URL people reach this server at: task notifications then carry a signed link to answer the task (GET/POST /tasks/{token}, 7 days)")
	cmd.Flags().BoolVar(&inProcessScripts, "in-process-scripts", false, "run Starlark calls in the server's process: faster, but one script allocating past the memory limit kills the server (starmem-001)")
	cmd.Flags().BoolVar(&dev, "dev", false, "development: POST /v1/dev/clock moves the clock (test waits and schedules without waiting)")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

// openSlack prepares the Slack task surface: the slack plugin posts, the
// signing secret and the workspace check the clicks.
func openSlack(h *host.Host, dataDir string) (*slack.Adapter, error) {
	for _, a := range []string{"post-task", "update-task"} {
		if _, ok := h.Runtime.Program.Action(slack.Plugin + "/" + a); !ok {
			return nil, fmt.Errorf("--slack needs a plugin named %s with a %s action", slack.Plugin, a)
		}
	}
	secret, err := (httpplugin.EnvSecrets{}).Secret("SLACK_SIGNING_SECRET")
	team := os.Getenv("TESSERA_SLACK_TEAM_ID")
	if err != nil || team == "" {
		return nil, errors.New("--slack needs TESSERA_SECRET_SLACK_SIGNING_SECRET and TESSERA_SLACK_TEAM_ID (the workspace whose user IDs are the Team's)")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	a := &slack.Adapter{Host: h, SigningSecret: secret, TeamID: team}
	return a, a.Open(filepath.Join(dataDir, "slack.db"))
}

// openEmail prepares the e-mail task surface: the email plugin sends,
// the server's member-bound links answer.
func openEmail(h *host.Host, api *server.Server, dataDir string) (*email.Adapter, error) {
	if _, ok := h.Runtime.Program.Action(email.Plugin + "/send-task"); !ok {
		return nil, fmt.Errorf("--email needs a plugin named %s with a send-task action", email.Plugin)
	}
	if api.PublicURL == "" {
		return nil, errors.New("--email needs --public-url: the e-mail's link is the only way to answer")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	from := os.Getenv("TESSERA_EMAIL_FROM")
	if from != "" {
		if _, err := mailmsg.Address(from); err != nil {
			return nil, fmt.Errorf("TESSERA_EMAIL_FROM: %w", err)
		}
	}
	a := &email.Adapter{Host: h, Link: api.MemberTaskLink, From: from}
	return a, a.Open(filepath.Join(dataDir, "email.db"))
}

// openTeams prepares the Teams task surface: the teams plugin sends, the
// Bot Framework's keys, the bot's app ID and the tenant check the clicks.
func openTeams(h *host.Host, dataDir string) (*teams.Adapter, error) {
	for _, a := range teams.Actions {
		if _, ok := h.Runtime.Program.Action(teams.Plugin + "/" + a); !ok {
			return nil, fmt.Errorf("--teams needs a plugin named %s with a %s action", teams.Plugin, a)
		}
	}
	app, tenant := os.Getenv("TESSERA_TEAMS_APP_ID"), os.Getenv("TESSERA_TEAMS_TENANT_ID")
	if app == "" || tenant == "" {
		return nil, errors.New("--teams needs TESSERA_TEAMS_APP_ID (the bot's Microsoft app ID) and TESSERA_TEAMS_TENANT_ID (the organization whose users are the Team's)")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	a := &teams.Adapter{Host: h, AppID: app, TenantID: tenant, OpenIDURL: os.Getenv("TESSERA_TEAMS_OPENID_URL")}
	return a, a.Open(filepath.Join(dataDir, "teams.db"))
}

// openWhatsApp prepares the WhatsApp adapter for a host whose Agent's
// channel plugin sends free text (send-text).
func openWhatsApp(h *host.Host, dataDir string) (*whatsapp.Adapter, error) {
	plugin := h.Channel()
	if plugin == "" {
		return nil, errors.New("--whatsapp: the Agent needs spec.channel (the WhatsApp plugin)")
	}
	verify := os.Getenv("TESSERA_WHATSAPP_VERIFY_TOKEN")
	secret, err := (httpplugin.EnvSecrets{}).Secret("WHATSAPP_APP_SECRET")
	if verify == "" || err != nil {
		return nil, errors.New("--whatsapp needs TESSERA_WHATSAPP_VERIFY_TOKEN and TESSERA_SECRET_WHATSAPP_APP_SECRET")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	a := &whatsapp.Adapter{Host: h, VerifyToken: verify, AppSecret: secret, Plugin: plugin,
		PhoneNumberID: os.Getenv("TESSERA_WHATSAPP_PHONE_NUMBER_ID")}
	return a, a.Open(filepath.Join(dataDir, "whatsapp.db"))
}
