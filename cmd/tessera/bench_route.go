package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// routeCase is one labelled message: the Function it must reach, or
// "none", given the agent's last message.
type routeCase struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	AgentSaid string `json:"agentSaid,omitempty"`
	After     string `json:"after,omitempty"` // the Function of the conversation's last run
	Want      string `json:"want"`
}

type routeTrial struct {
	ID            string             `json:"id"`
	Repeat        int                `json:"repeat"`
	Want          string             `json:"want"`
	Choice        string             `json:"choice"`
	P             float64            `json:"p"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Error         string             `json:"error,omitempty"`
}

// routeThresholds are the cut-offs the report compares.
var routeThresholds = []float64{0, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}

func newBenchRouteCmd() *cobra.Command {
	var (
		namespace, dataset, decideSpec, outDir string
		cassettePath, cassMode                 string
		repeats, concurrency                   int
		maxCost                                float64
	)
	cmd := &cobra.Command{
		Use:   "route <paths...>",
		Short: "Measure the router of a namespace on labelled messages",
		Long: `Sends every message of a dataset through the host's own router (the
Functions' selector.description, the agent's last message as context) and
reports accuracy, the confusions, and what each confidence threshold would
do: route correctly, route to the wrong Function, or not route.

Dataset: JSONL of {"id", "message", "agentSaid"?, "after"?, "want"}, where
after is the Function of the conversation's last run (it decides which
follow-ups are options, selector.after) and want is a routable Function or
"none".`,
		Example: `  tessera bench route examples/traveler --namespace traveler \
    --dataset bench/datasets/traveler-routing-v1.jsonl --repeats 3 --max-cost-usd 0.5`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxCost <= 0 {
				return errors.New("--max-cost-usd must be positive")
			}
			cases, err := loadRouteCases(dataset)
			if err != nil {
				return fmt.Errorf("dataset: %w", err)
			}
			l := ledger.New(ledger.Budget{MaxCostUSD: maxCost})
			var d decide.Decider
			replay := cassettePath != "" && cassMode == string(decide.CassetteReplay)
			if !replay {
				if d, err = buildDecider(decideSpec, l); err != nil {
					return err
				}
			}
			if cassettePath != "" {
				c, err := decide.LoadCassette(cassettePath, decideSpec, d, decide.CassetteMode(cassMode))
				if err != nil {
					return err
				}
				d = c
				if decide.CassetteMode(cassMode) == decide.CassetteRecord {
					defer func() {
						if err := c.Save(cassettePath); err != nil {
							fmt.Fprintln(cmd.ErrOrStderr(), "saving the cassette:", err)
						}
					}()
				}
			}
			dataDir, err := os.MkdirTemp("", "tessera-bench-route-")
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(dataDir) }()
			h, issues, err := host.Load(host.Options{Paths: args, Namespace: namespace, Store: journal.NewMemory(), Decider: d, DataDir: dataDir})
			for _, is := range issues {
				fmt.Fprintln(cmd.ErrOrStderr(), is.String())
			}
			if err != nil {
				return err
			}
			defer func() { _ = h.Close() }()
			valid := map[string]bool{"none": true}
			for _, r := range h.Routes() {
				valid[r.Name] = true
			}
			for _, c := range cases {
				if !valid[c.Want] {
					return fmt.Errorf("case %s: want %q is not a routable Function of %s", c.ID, c.Want, namespace)
				}
			}

			trials := make([]routeTrial, 0, len(cases)*repeats)
			var mu sync.Mutex
			sem := make(chan struct{}, max(concurrency, 1))
			var wg sync.WaitGroup
			for rep := 1; rep <= repeats; rep++ {
				for _, c := range cases {
					wg.Add(1)
					sem <- struct{}{}
					go func() {
						defer func() { <-sem; wg.Done() }()
						t := routeTrial{ID: c.ID, Repeat: rep, Want: c.Want}
						if l.Exceeded() {
							t.Error = "budget exceeded"
						} else if r, err := h.Route(cmd.Context(), c.Message, c.AgentSaid, c.After); err != nil {
							t.Error = err.Error()
						} else {
							t.Choice, t.P, t.Probabilities = r.Choice, r.Probabilities[r.Choice], r.Probabilities
						}
						mu.Lock()
						trials = append(trials, t)
						mu.Unlock()
					}()
				}
			}
			wg.Wait()
			sort.Slice(trials, func(i, j int) bool {
				if trials[i].ID != trials[j].ID {
					return trials[i].ID < trials[j].ID
				}
				return trials[i].Repeat < trials[j].Repeat
			})
			report := routeReport(cases, trials, decideSpec, l.Totals().CostUSD)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(outDir, "report.md"), []byte(report), 0o600); err != nil {
				return err
			}
			var b strings.Builder
			for _, t := range trials {
				line, _ := json.Marshal(t)
				b.Write(line)
				b.WriteByte('\n')
			}
			if err := os.WriteFile(filepath.Join(outDir, "trials.jsonl"), []byte(b.String()), 0o600); err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), report)
			for _, t := range trials {
				if t.Error != "" {
					return fmt.Errorf("%s: %s", t.ID, t.Error)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace whose Functions are routed (required)")
	cmd.Flags().StringVar(&dataset, "dataset", "", "labelled messages, JSONL (required)")
	cmd.Flags().StringVar(&decideSpec, "decide", "typesafe:jev-1.13.0", "Decide backend (also the cassette's target)")
	cmd.Flags().IntVar(&repeats, "repeats", 1, "runs per message")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "calls in flight")
	cmd.Flags().Float64Var(&maxCost, "max-cost-usd", 0, "hard budget for the whole run (required)")
	cmd.Flags().StringVar(&cassettePath, "cassette", "", "cassette file")
	cmd.Flags().StringVar(&cassMode, "cassette-mode", "replay", "record or replay (with --cassette)")
	cmd.Flags().StringVar(&outDir, "out", "out/route", "output directory")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}

func loadRouteCases(path string) ([]routeCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []routeCase
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c routeCase
		d := json.NewDecoder(strings.NewReader(line))
		d.DisallowUnknownFields()
		if err := d.Decode(&c); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if c.ID == "" || c.Message == "" || c.Want == "" || seen[c.ID] {
			return nil, fmt.Errorf("line %d: id, message and want are required, ids unique", n)
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.New("no cases")
	}
	return out, sc.Err()
}

// routeOutcome is what the host would do at a threshold.
func routeOutcome(t routeTrial, threshold float64) string {
	got := t.Choice
	if got != "none" && t.P < threshold {
		got = "none"
	}
	switch {
	case got == t.Want:
		return "right"
	case got == "none":
		return "unrouted" // the user is told it was not understood
	case t.Want == "none":
		return "false-route" // something out of scope started a Function
	}
	return "wrong" // the wrong Function started
}

func routeReport(cases []routeCase, trials []routeTrial, target string, cost float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Routing benchmark\n\nTarget %s, %d messages, %d trials, $%.6f.\n\n", target, len(cases), len(trials), cost)
	b.WriteString("## By threshold\n\nright: the expected outcome; wrong: another Function started; false-route: an out-of-scope message started one; unrouted: an in-scope message was not routed (the user is asked again).\n\n")
	b.WriteString("| threshold | right | wrong | false-route | unrouted |\n| --- | --- | --- | --- | --- |\n")
	for _, th := range routeThresholds {
		counts := map[string]int{}
		for _, t := range trials {
			if t.Error == "" {
				counts[routeOutcome(t, th)]++
			}
		}
		n := float64(len(trials))
		fmt.Fprintf(&b, "| %.1f | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n", th,
			100*float64(counts["right"])/n, 100*float64(counts["wrong"])/n, 100*float64(counts["false-route"])/n, 100*float64(counts["unrouted"])/n)
	}
	b.WriteString("\n## Per expected route (threshold 0.6)\n\n| want | n | right | got instead |\n| --- | --- | --- | --- |\n")
	byWant := map[string][]routeTrial{}
	for _, t := range trials {
		byWant[t.Want] = append(byWant[t.Want], t)
	}
	wants := make([]string, 0, len(byWant))
	for w := range byWant {
		wants = append(wants, w)
	}
	sort.Strings(wants)
	for _, w := range wants {
		right, instead := 0, map[string]int{}
		for _, t := range byWant[w] {
			if routeOutcome(t, 0.6) == "right" {
				right++
				continue
			}
			got := t.Choice
			if got != "none" && t.P < 0.6 {
				got = fmt.Sprintf("none (%s at %.2f)", t.Choice, t.P)
			}
			instead[got]++
		}
		var parts []string
		for k, v := range instead {
			parts = append(parts, fmt.Sprintf("%s ×%d", k, v))
		}
		sort.Strings(parts)
		fmt.Fprintf(&b, "| %s | %d | %.1f%% | %s |\n", w, len(byWant[w]), 100*float64(right)/float64(len(byWant[w])), strings.Join(parts, ", "))
	}
	b.WriteString("\n## Misses at 0.6\n\n")
	msg := map[string]routeCase{}
	for _, c := range cases {
		msg[c.ID] = c
	}
	misses := 0
	for _, t := range trials {
		if t.Error != "" || routeOutcome(t, 0.6) == "right" {
			continue
		}
		misses++
		c := msg[t.ID]
		ctx := ""
		if c.AgentSaid != "" {
			ctx = " (after the agent said something)"
		}
		fmt.Fprintf(&b, "- %s%s: %q want %s, got %s %.2f\n", t.ID, ctx, c.Message, t.Want, t.Choice, t.P)
	}
	if misses == 0 {
		b.WriteString("None.\n")
	}
	return b.String()
}
