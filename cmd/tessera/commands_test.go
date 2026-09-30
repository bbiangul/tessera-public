package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestValidateExamples(t *testing.T) {
	out, err := run(t, "validate", "../../examples")
	if err != nil {
		t.Fatalf("examples must validate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 errors") {
		t.Errorf("output = %q", out)
	}
}

func TestValidateReportsErrors(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("apiVersion: agents.tessera.dev/v1alpha1\nkind: Function\nmetadata: {name: x}\nspec: {nope: 1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "validate", bad)
	if err == nil {
		t.Fatalf("an invalid file must fail:\n%s", out)
	}
	if !strings.Contains(out, "UNKNOWN_FIELD") || !strings.Contains(out, "3 errors") {
		t.Errorf("output = %q", out)
	}
}

func TestExplainWithoutArgumentListsKinds(t *testing.T) {
	out, err := run(t, "explain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Kinds:") || !strings.Contains(out, "Function") {
		t.Errorf("output = %q", out)
	}
	if _, err := run(t, "explain", "Function.spec", "extra"); err == nil {
		t.Error("two arguments must be refused")
	}
}

func TestGatePassesOnItselfAndFailsOnRegression(t *testing.T) {
	const baseline = "../../bench/baselines/example-travel-v1/trials.jsonl"
	out, err := run(t, "bench", "gate", "--baseline", baseline, "--candidate", baseline, "--max-worse-cases", "0", "--max-coverage-drop", "0")
	if err != nil {
		t.Fatalf("the baseline must pass against itself, even with zero tolerance: %v\n%s", err, out)
	}

	// Flip every correct answer of the first few cases to a wrong one.
	f, err := os.Open(baseline)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf bytes.Buffer
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	broken := map[string]bool{}
	for sc.Scan() {
		var tr map[string]any
		if err := json.Unmarshal(sc.Bytes(), &tr); err != nil {
			t.Fatal(err)
		}
		id := tr["caseId"].(string)
		if len(broken) < 6 || broken[id] {
			broken[id] = true
			if tr["pred"] == tr["gold"] {
				tr["pred"] = "WRONG"
				tr["probs"] = map[string]any{"WRONG": 0.99}
			}
		}
		b, _ := json.Marshal(tr)
		buf.Write(append(b, '\n'))
	}
	cand := filepath.Join(t.TempDir(), "trials.jsonl")
	if err := os.WriteFile(cand, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, "bench", "gate", "--baseline", baseline, "--candidate", cand); err == nil {
		t.Errorf("a regression must fail the gate:\n%s", out)
	}
}

// The routing benchmark, replayed: every message reaches the host's own
// router, and the report says what each threshold would do.
func TestBenchRouteReplay(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, "bench", "route", "../../examples/traveler", "--namespace", "traveler",
		"--dataset", "../../bench/datasets/traveler-routing-v1.jsonl", "--max-cost-usd", "1",
		"--cassette", "../../bench/cassettes/traveler-routing-v1.jsonl", "--out", dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"80 messages, 80 trials", "| 0.6 |", "| set-preferences | 15 |"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "trials.jsonl")); err != nil {
		t.Error(err)
	}
}

func TestBenchRouteRefusesUnknownLabels(t *testing.T) {
	ds := filepath.Join(t.TempDir(), "d.jsonl")
	if err := os.WriteFile(ds, []byte(`{"id":"a","message":"oi","want":"book-hotel"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, "bench", "route", "../../examples/traveler", "--namespace", "traveler", "--dataset", ds,
		"--max-cost-usd", "1", "--cassette", "../../bench/cassettes/traveler-routing-v1.jsonl", "--out", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a routable Function") {
		t.Errorf("a label that is not a route must be refused before any call: %v", err)
	}
}

func TestRouteOutcome(t *testing.T) {
	for _, c := range []struct {
		want, choice string
		p, th        float64
		out          string
	}{
		{"search-hotels", "search-hotels", 0.9, 0.6, "right"},
		{"search-hotels", "search-hotels", 0.5, 0.6, "unrouted"},
		{"search-hotels", "search-flights", 0.9, 0.6, "wrong"},
		{"search-hotels", "search-flights", 0.5, 0.6, "unrouted"},
		{"none", "flight-details", 0.7, 0.6, "false-route"},
		{"none", "flight-details", 0.5, 0.6, "right"},
		{"none", "none", 0.9, 0.6, "right"},
	} {
		if got := routeOutcome(routeTrial{Want: c.want, Choice: c.choice, P: c.p}, c.th); got != c.out {
			t.Errorf("%+v: %s", c, got)
		}
	}
}

// The funnel command on a history of months: days to open, time per
// phase, a stuck store, the cut by region.
func TestMetricsFunnel(t *testing.T) {
	db := filepath.Join(t.TempDir(), "runs.db")
	st, err := journal.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	at := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 10, 0, 0, 0, sp) }
	n := 0
	add := func(unit, region, name string, when time.Time) {
		n++
		id := fmt.Sprintf("r%d", n)
		if err := st.CreateRun(ctx, journal.Run{ID: id, Namespace: "rede", Function: "registrar-etapa", Generation: "g", Conversation: id,
			Scopes: []byte("{}"), Status: journal.StatusSucceeded, CreatedAt: when, UpdatedAt: when}); err != nil {
			t.Fatal(err)
		}
		v, _ := journal.Encode(map[string]any{"namespace": "rede", "subject": "loja", "id": unit, "name": name, "at": when,
			"attributes": map[string]any{"regiao": region}})
		if _, err := st.Append(ctx, id, journal.Record{Site: "steps.x/milestone", Kind: journal.KindMilestone, Value: v, At: when}); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct {
		unit, region string
		steps        []time.Time
	}{
		{"1", "sul", []time.Time{at(1, 5), at(1, 25), at(2, 14), at(4, 5), at(4, 15), at(4, 20)}},
		{"2", "nordeste", []time.Time{at(1, 12), at(2, 21), at(3, 3), at(4, 30), at(5, 10), at(5, 22)}},
		{"3", "sul", []time.Time{at(1, 20), at(2, 9)}},
	} {
		names := []string{"contrato-assinado", "ponto-aprovado", "projeto-aprovado", "obra-concluida", "equipe-treinada", "loja-aberta"}
		for i, when := range s.steps {
			add(s.unit, s.region, names[i], when)
		}
	}
	_ = st.Close()
	out, err := run(t, "metrics", "funnel", "../../testdata/lojas", "--namespace", "rede", "--funnel", "abertura",
		"--db", db, "--at", "2026-06-01T12:00:00-03:00")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"cohort 2026-01-01: 3 entered", "end to end            2", "1 (3)", "105.0d"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	out, _ = run(t, "metrics", "funnel", "../../testdata/lojas", "--namespace", "rede", "--funnel", "abertura",
		"--db", db, "--at", "2026-06-01T12:00:00-03:00", "--by", "regiao")
	if !strings.Contains(out, "· nordeste: 1 entered") || !strings.Contains(out, "· sul: 2 entered") {
		t.Errorf("cut by region:\n%s", out)
	}
}

// The experiment command on a database written by real runs of an
// experiment step, plus the outcome milestones.
func TestMetricsExperiment(t *testing.T) {
	dir := t.TempDir()
	src := `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: lead, namespace: ab }
spec: { id: "conversation.id" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Funnel
metadata: { name: prosp, namespace: ab }
spec:
  subject: lead
  stages: [ { name: contato, milestone: contatado }, { name: reuniao, milestone: reuniao-marcada, within: 14d } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Experiment
metadata: { name: msg, namespace: ab }
spec:
  subject: lead
  variants: [ { name: controle, weight: 50 }, { name: nova, weight: 50 } ]
  primary: { funnel: prosp, stage: reuniao }
  minSamplePerVariant: 50
`
	if err := os.WriteFile(filepath.Join(dir, "ab.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "runs.db")
	st, err := journal.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	put := func(run, lead, name, variant string, at time.Time) {
		if err := st.CreateRun(ctx, journal.Run{ID: run, Namespace: "ab", Function: "f", Generation: "g", Conversation: lead, Scopes: []byte("{}"), Status: journal.StatusSucceeded, CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
		attrs := map[string]any{}
		if variant != "" {
			attrs["variant"] = variant
		}
		v, _ := journal.Encode(map[string]any{"namespace": "ab", "subject": "lead", "id": lead, "name": name, "at": at, "attributes": attrs})
		if _, err := st.Append(ctx, run, journal.Record{Site: "s/m", Kind: journal.KindMilestone, Value: v, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 400; i++ {
		lead := fmt.Sprintf("lead-%d", i)
		v := metrics.Assign("msg", lead, []metrics.Variant{{Name: "controle", Weight: 50}, {Name: "nova", Weight: 50}})
		at := base.Add(time.Duration(i) * time.Hour)
		put(lead+"/x", lead, "experiment/msg", v, at)
		put(lead+"/c", lead, "contatado", "", at)
		if (v == "nova" && i%3 == 0) || (v == "controle" && i%10 == 0) {
			put(lead+"/r", lead, "reuniao-marcada", "", at.Add(48*time.Hour))
		}
	}
	_ = st.Close()
	out, err := run(t, "metrics", "experiment", dir, "--namespace", "ab", "--experiment", "msg", "--db", db, "--at", "2026-10-01T00:00:00Z")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "verdict: nova better than controle") || strings.Contains(out, "WARNING") {
		t.Errorf("report:\n%s", out)
	}
}

// tessera runs reads a runs database without a server: the list, and one
// run explained with the definitions.
func TestRunsCommand(t *testing.T) {
	db := filepath.Join(t.TempDir(), "runs.db")
	st, err := journal.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if err := st.CreateRun(ctx, journal.Run{ID: "r1", Namespace: "rede", Function: "registrar-etapa", Generation: "g", Conversation: "equipe",
		Scopes: []byte(`{"event":{"message":"a loja 21 assinou"}}`), Status: journal.StatusSucceeded, CreatedAt: at, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		site string
		kind journal.Kind
		v    any
	}{
		{"router", journal.KindResult, map[string]any{"choice": "registrar-etapa", "probabilities": map[string]any{"registrar-etapa": 0.99}, "threshold": 0.6}},
		{"steps.contrato/milestone", journal.KindMilestone, map[string]any{"namespace": "rede", "subject": "loja", "id": "21", "name": "contrato-assinado", "at": at, "attributes": map[string]any{}}},
		{"output", journal.KindSend, map[string]any{"to": "user", "text": "Registrado."}},
	} {
		v, _ := journal.Encode(r.v)
		if _, err := st.Append(ctx, "r1", journal.Record{Site: r.site, Kind: r.kind, Value: v, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	out, err := run(t, "runs", "list", "--db", db, "--status", "Succeeded")
	if err != nil || !strings.Contains(out, "registrar-etapa") || !strings.Contains(out, "r1") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err = run(t, "runs", "show", "r1", "../../testdata/lojas", "--db", db)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{`routed to "registrar-etapa"`, "step contrato: compute, milestone", "milestone contrato-assinado for loja 21", "message to user: Registrado."} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if out, _ := run(t, "runs", "show", "r1", "--db", db, "-o", "json"); !strings.Contains(out, `"timeline"`) {
		t.Errorf("json:\n%s", out)
	}
	out, err = run(t, "runs", "follow", "--namespace", "rede", "--db", db, "--once")
	if err != nil || strings.Count(out, "\n") != 3 || !strings.Contains(out, "milestone contrato-assinado for loja 21") {
		t.Fatalf("follow --once prints every record:\n%s %v", out, err)
	}
	if out, _ := run(t, "runs", "follow", "--namespace", "rede", "--db", db, "--once", "--after", "2"); strings.Count(out, "\n") != 1 || !strings.Contains(out, "Registrado.") {
		t.Errorf("after a cursor, only what is newer:\n%s", out)
	}
	// Following (not --once) keeps polling: each record is printed once.
	ctx2, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	root := newRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"runs", "follow", "--namespace", "rede", "--db", db})
	if err := root.ExecuteContext(ctx2); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 3 {
		t.Errorf("following prints each record once, got %d lines:\n%s", n, buf.String())
	}
	if out, _ := run(t, "runs", "follow", "--namespace", "other", "--db", db, "--once"); out != "" {
		t.Errorf("another namespace's records are not printed:\n%s", out)
	}
}
