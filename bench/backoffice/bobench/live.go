package main

// Running the scenarios: deterministic fakes by default; with -live, Jev
// decides and DeepSeek extracts (backoffice-baseline-001), repeated, under
// a budget.

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/ledger"
)

type liveEngines struct {
	decider   decide.Decider
	generator generate.Generator
	ledger    *ledger.Ledger
}

// live is set by -live.
var live *liveEngines

func run() {
	defs := flag.String("defs", "bench/backoffice/agent", "the clinic agent (namespace clinica-bo)")
	out := flag.String("out", "", "JSON results")
	liveFlag := flag.Bool("live", false, "Jev decides and DeepSeek extracts (TYPESAFE_API_KEY, OPENROUTER_API_KEY)")
	only := flag.String("only", "", "comma-separated scenarios (default: all)")
	repeat := flag.Int("repeat", 1, "runs per scenario")
	maxCost := flag.Float64("max-cost", 1, "-live: budget in US$")
	flag.Parse()
	if *liveFlag {
		jev, or := os.Getenv("TYPESAFE_API_KEY"), os.Getenv("OPENROUTER_API_KEY")
		if jev == "" || or == "" {
			log.Fatal("-live needs TYPESAFE_API_KEY and OPENROUTER_API_KEY")
		}
		l := ledger.New(ledger.Budget{MaxCostUSD: *maxCost})
		live = &liveEngines{ledger: l,
			decider:   &decide.Metered{Inner: &decide.TypeSafe{APIKey: jev, Model: "jev-1.13.0"}, Ledger: l},
			generator: &generate.Metered{Inner: &generate.OpenRouter{APIKey: or, Model: "deepseek/deepseek-v4.1-flash", Providers: []string{"DeepSeek"}}, Ledger: l}}
	}
	want := map[string]bool{}
	for _, n := range strings.Split(*only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	var outcomes []Outcome
	for _, sc := range scenarios {
		if len(want) > 0 && !want[sc.name] {
			continue
		}
		for i := 1; i <= *repeat; i++ {
			before := 0.0
			if live != nil {
				before = live.ledger.Totals().CostUSD
			}
			w, err := newWorld(*defs)
			if err != nil {
				log.Fatal(err)
			}
			o := sc.run(w)
			o.Scenario = sc.name
			o.Effects = len(w.effects)
			w.close()
			if live != nil {
				o.Run, o.CostUSD = i, live.ledger.Totals().CostUSD-before
			}
			outcomes = append(outcomes, o)
			fmt.Printf("%-30s unauthorized=%d nonRole=%d stale=%d exposures=%d wrongLevel=%d unsupported=%d forged=%d unshown=%d deniedDone=%d toldApproved=%d leaks=%d voice=%d msgs=%d dup=%d effects=%d  %s\n",
				o.Scenario, o.UnauthorizedEffects, o.ApprovalsFromNonRole, o.StaleApprovals, o.Exposures, o.WrongLevel, o.Unsupported, o.AcceptedForged, o.OutcomeNotShown, o.DeniedButDone, o.ToldApproved, o.Leaks, o.VoiceMismatch, o.MessageMismatch, o.Duplicates, o.Effects, o.Note)
		}
	}
	if len(want) > 0 {
		for n := range want {
			if !slices.ContainsFunc(scenarios, func(s scenario) bool { return s.name == n }) {
				log.Fatalf("no scenario %q", n)
			}
		}
	}
	if live != nil {
		t := live.ledger.Totals()
		fmt.Printf("live: US$ %.4f, %d calls\n", t.CostUSD, t.Calls["decide"]+t.Calls["generate"])
	}
	if *out != "" {
		b, _ := json.MarshalIndent(outcomes, "", "  ")
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
