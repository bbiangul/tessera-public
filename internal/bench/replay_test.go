package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// Level 2 of RFC-0002: replay a recorded baseline (the example one) through the real
// runner and gate. Free, offline and deterministic. It fails when a question
// in the dataset changes without re-recording (cassette miss), or when the
// runner or metrics stop reproducing the committed baseline.
const (
	replayTarget   = exampleTarget
	replayDataset  = exampleDataset
	replayCassette = exampleCassette
	replayBaseline = exampleBaseline
)

func TestReplayReproducesBaseline(t *testing.T) {
	cases, err := LoadDataset(replayDataset)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := decide.LoadCassette(replayCassette, replayTarget, nil, decide.CassetteReplay)
	if err != nil {
		t.Fatal(err)
	}
	trials := Run(context.Background(), cases, []Target{{Label: replayTarget, Decider: cas}}, Options{Repeats: 3})
	for _, tr := range trials {
		if tr.Err != "" {
			t.Fatalf("case %s: %s\nIf the question changed on purpose, re-record: see bench/README.md", tr.CaseID, tr.Err)
		}
	}

	baseline := loadTrials(t, replayBaseline)
	g := Gate(baseline, trials, GateConfig{})
	if !g.Pass() {
		t.Fatalf("replay does not reproduce the baseline:\n%s", g.Markdown("replay"))
	}

	// Passing the gate is not enough: the replay must reproduce the exact
	// answers. Recordings are assigned to repeats in arrival order, so
	// compare, per case, the multiset of (prediction, probabilities).
	if got, want := answersByCase(trials), answersByCase(baseline); !reflect.DeepEqual(got, want) {
		for id := range want {
			if !reflect.DeepEqual(got[id], want[id]) {
				t.Errorf("case %s: replay %v, baseline %v", id, got[id], want[id])
			}
		}
	}
	// Sums run in arrival order, so allow floating-point rounding only.
	bs, rs := Summarize("x", baseline), Summarize("x", trials)
	same := func(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
	if !same(bs.CaseAccuracy, rs.CaseAccuracy) || !same(bs.Brier, rs.Brier) || !same(bs.AUROC, rs.AUROC) ||
		!same(bs.ECE, rs.ECE) || bs.LatencyP95 != rs.LatencyP95 {
		t.Errorf("summary differs:\nbaseline %+v\nreplay   %+v", bs, rs)
	}
}

// answersByCase returns, per case, the sorted list of answers as strings.
func answersByCase(ts []Trial) map[string][]string {
	out := map[string][]string{}
	for _, t := range ts {
		keys := make([]string, 0, len(t.Probs))
		for k := range t.Probs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(t.Pred + "|" + t.Err)
		for _, k := range keys {
			fmt.Fprintf(&b, "|%s=%.9f", k, t.Probs[k])
		}
		out[t.CaseID] = append(out[t.CaseID], b.String())
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}

func loadTrials(t *testing.T, path string) []Trial {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []Trial
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var tr Trial
		if err := json.Unmarshal(sc.Bytes(), &tr); err != nil {
			t.Fatal(err)
		}
		out = append(out, tr)
	}
	return out
}
