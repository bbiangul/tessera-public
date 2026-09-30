package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	httpplugin "github.com/bbiangul/tessera/internal/plugins/http"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func newChatCmd() *cobra.Command {
	var (
		namespace, db, conversation string
		decideSpec, generateSpec    string
		callerPairs                 []string
		maxCost                     float64
		allowPrivate                bool
		dataDir                     string
	)
	cmd := &cobra.Command{
		Use:   "chat <files-or-dirs>...",
		Short: "Talk to an agent in the terminal",
		Long: `Loads and validates the resources, then reads messages from stdin and
prints what the agent sends. Runs are durable (SQLite) and resume after a
restart with the same --db and --conversation.

  /human <answer>   answer the human task the run waits for; JSON is parsed
                    (true, "o2", {"happened": true}), anything else is text
  /quit             leave

Engines:
  --decide typesafe:<model>             Jev (needs TYPESAFE_API_KEY)
  --decide llm:<model>[@Provider]       LLM via OpenRouter (needs OPENROUTER_API_KEY)
  --generate openrouter:<model>[@Provider]
Secrets for http plugins are read from TESSERA_SECRET_<NAME>.`,
		Example: `  tessera chat testdata/agenda --namespace agenda-exemplo \
    --decide typesafe:jev-1.13.0 --caller role=patient --caller patientId=p1`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxCost <= 0 {
				return errors.New("--max-cost-usd must be positive: model calls always run with a budget")
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
			defer func() {
				if cerr := store.Close(); cerr != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "closing the run store:", cerr)
				}
			}()
			caller := map[string]any{}
			for _, p := range callerPairs {
				k, v, ok := strings.Cut(p, "=")
				if !ok {
					return fmt.Errorf("--caller %q: want key=value", p)
				}
				caller[k] = v
			}
			h, issues, err := host.Load(host.Options{
				Paths: args, Namespace: namespace, Store: store, Decider: d, Generator: g, Secrets: httpplugin.EnvSecrets{},
				AllowPrivateNetworks: allowPrivate, DataDir: dataDir,
			})
			for _, is := range issues {
				fmt.Fprintln(cmd.ErrOrStderr(), is.String())
			}
			if err != nil {
				return err
			}
			defer func() {
				if cerr := h.Close(); cerr != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "closing plugin databases:", cerr)
				}
			}()
			var names []string
			for _, r := range h.Routes() {
				names = append(names, r.Name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "agent %s ready: %s (budget $%.2f). /quit to leave.\n", namespace, strings.Join(names, ", "), maxCost)
			err = chatLoop(cmd.Context(), h, cmd.InOrStdin(), cmd.OutOrStdout(), conversation, caller)
			tot := l.Totals()
			fmt.Fprintf(cmd.OutOrStdout(), "spent $%.6f in %d decide and %d generate calls\n", tot.CostUSD, tot.Calls["decide"], tot.Calls["generate"])
			return err
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace of the agent (required)")
	cmd.Flags().StringVar(&db, "db", "tessera-chat.db", "SQLite file for runs")
	cmd.Flags().StringVar(&conversation, "conversation", "terminal", "conversation id")
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend")
	cmd.Flags().StringVar(&generateSpec, "generate", "openrouter:deepseek/deepseek-v4.1-flash", "Generate backend, or none")
	cmd.Flags().StringArrayVar(&callerPairs, "caller", nil, "caller scope entries, key=value (repeatable)")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0.50, "hard budget for model calls in this session")
	cmd.Flags().BoolVar(&allowPrivate, "allow-private-networks", false, "let http plugins reach loopback and private addresses")
	cmd.Flags().StringVar(&dataDir, "data-dir", "tessera-data", "directory for the sql plugins' databases")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func chatLoop(ctx context.Context, h *host.Host, in io.Reader, out io.Writer, conversation string, caller map[string]any) error {
	// Runs asleep in a wait step wake here: every second the due ones run
	// and what they send is shown, as a channel would deliver it.
	out = &lockedWriter{w: out}
	tick, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-tick.Done():
				return
			case <-t.C:
				res, err := h.Tick(tick)
				if err != nil && tick.Err() == nil {
					fmt.Fprintln(out, "error waking runs:", err)
				}
				for _, r := range res {
					show(tick, h.Runtime, out, r)
				}
			}
		}
	}()
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for {
		fmt.Fprint(out, "you> ")
		if !sc.Scan() {
			fmt.Fprintln(out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			continue
		case line == "/quit":
			return nil
		case strings.HasPrefix(line, "/human"):
			res, err := answerHuman(ctx, h, conversation, strings.TrimSpace(strings.TrimPrefix(line, "/human")))
			if err != nil {
				fmt.Fprintln(out, "error:", err)
				continue
			}
			show(ctx, h.Runtime, out, res)
		default:
			r, err := h.Handle(ctx, conversation, caller, line)
			if err != nil {
				fmt.Fprintln(out, "error:", err)
				continue
			}
			for _, e := range r.Earlier {
				show(ctx, h.Runtime, out, e)
			}
			if r.Routed != "" {
				fmt.Fprintf(out, "  [started %s]\n", r.Routed)
			}
			show(ctx, h.Runtime, out, r.Result)
		}
	}
}

func answerHuman(ctx context.Context, h *host.Host, conversation, answer string) (runtime.Result, error) {
	run, ok, err := h.Runtime.Store.ActiveRun(ctx, conversation)
	if err != nil {
		return runtime.Result{}, err
	}
	if !ok {
		return runtime.Result{}, errors.New("no run is waiting")
	}
	var v any
	if err := json.Unmarshal([]byte(answer), &v); err != nil {
		v = nil // plain text answer
	}
	// The person at the terminal reads the task as it is now and answers it.
	task, err := h.Runtime.PendingTask(ctx, run.ID)
	if err != nil {
		return runtime.Result{}, err
	}
	return h.Runtime.Deliver(ctx, run.ID, runtime.Reply{From: "human", Text: answer, Value: v, TaskHash: task.Hash, By: "terminal"})
}

// show prints the outbox and acknowledges it, as a channel would.
func show(ctx context.Context, rt *runtime.Runtime, out io.Writer, res runtime.Result) {
	for _, m := range res.Outbox {
		if m.To == "user" {
			fmt.Fprintf(out, "agent> %s\n", m.Text)
			continue
		}
		fmt.Fprintf(out, "  [%s, task %s] %s\n", m.To, m.Task, m.Text)
		for k, v := range m.Options {
			fmt.Fprintf(out, "      %s: %s\n", k, v)
		}
	}
	if n := len(res.Outbox); n > 0 && res.Outbox[n-1].Seq > 0 {
		if err := rt.Ack(ctx, res.RunID, res.Outbox[n-1].Seq); err != nil {
			fmt.Fprintln(out, "error:", err)
		}
	}
	switch {
	case res.Status.Final():
		fmt.Fprintf(out, "  [run %s]\n", res.Status)
		if res.Error != "" {
			fmt.Fprintf(out, "  [reason: %s]\n", res.Error)
		}
	case res.WaitingFor == "human":
		fmt.Fprintln(out, "  [waiting for a person: answer with /human <answer>]")
	case res.Status == journal.StatusWaitingForTime:
		fmt.Fprintf(out, "  [asleep until %s]\n", res.WakeAt.Local().Format("2006-01-02 15:04"))
	}
}

func buildDecider(spec string, l *ledger.Ledger) (decide.Decider, error) {
	backend, model, ok := strings.Cut(spec, ":")
	if !ok || model == "" {
		return nil, fmt.Errorf("--decide %q: want backend:model", spec)
	}
	var d decide.Decider
	switch backend {
	case "typesafe":
		key := os.Getenv("TYPESAFE_API_KEY")
		if key == "" {
			return nil, errors.New("TYPESAFE_API_KEY is not set")
		}
		d = &decide.TypeSafe{APIKey: key, Model: model}
	case "llm":
		key := os.Getenv("OPENROUTER_API_KEY")
		if key == "" {
			return nil, errors.New("OPENROUTER_API_KEY is not set")
		}
		m, providers := splitProviders(model)
		d = &decide.LLMConstrained{APIKey: key, Model: m, Strategy: decide.StrategyLogprobs, Providers: providers}
	default:
		return nil, fmt.Errorf("--decide %q: unknown backend (typesafe or llm)", spec)
	}
	return &decide.Metered{Inner: d, Ledger: l}, nil
}

func buildGenerator(spec string, l *ledger.Ledger) (generate.Generator, error) {
	if spec == "none" || spec == "" {
		return nil, nil
	}
	backend, model, ok := strings.Cut(spec, ":")
	if !ok || backend != "openrouter" || model == "" {
		return nil, fmt.Errorf("--generate %q: want openrouter:<model>[@Provider] or none", spec)
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, errors.New("OPENROUTER_API_KEY is not set (or use --generate none)")
	}
	m, providers := splitProviders(model)
	return &generate.Metered{Inner: &generate.OpenRouter{APIKey: key, Model: m, Providers: providers}, Ledger: l}, nil
}

// lockedWriter lets the scheduler and the prompt write without interleaving.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
