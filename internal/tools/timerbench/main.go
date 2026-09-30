// Command timerbench measures the journal as a timer store (ADR-0004
// spike): many runs asleep, how fast the due ones are found, and how fast
// they are woken end to end.
//
//	go run ./internal/tools/timerbench -runs 1000000 -db /tmp/t.db
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const cadence = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: cadence, namespace: bench }
spec:
  description: Follows up.
  steps:
    - { id: d0, respond: { template: "Olá" } }
    - { id: d1, wait: { for: 24h } }
    - { id: m1, respond: { template: "Oi de novo" } }
    - { id: d3, wait: { for: 48h } }
  output: { template: "fim" }
`

func main() {
	runs := flag.Int("runs", 100000, "sleeping runs to create")
	wakes := flag.Int("wakes", 5000, "runs to wake end to end")
	db := flag.String("db", filepath.Join(os.TempDir(), "timerbench.db"), "SQLite file (removed first)")
	flag.Parse()
	for _, p := range []string{*db, *db + "-wal", *db + "-shm"} {
		_ = os.Remove(p)
	}
	st, err := journal.OpenSQLite(*db)
	must(err)
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	base := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	// 1. Real runs through the runtime: start, sleep, wake.
	rt := program(st, &base)
	t0 := time.Now()
	for i := 0; i < *wakes; i++ {
		_, err := rt.Start(ctx, runtime.Start{Namespace: "bench", Function: "cadence", Conversation: fmt.Sprintf("w%d", i)})
		must(err)
	}
	started := time.Since(t0)
	now := base.Add(25 * time.Hour)
	rt.Clock = func() time.Time { return now }
	t0 = time.Now()
	woken := 0
	for {
		res, err := rt.WakeDue(ctx, 500)
		must(err)
		if len(res) == 0 {
			break
		}
		woken += len(res)
	}
	wakeTime := time.Since(t0)

	// 2. Many sleepers, written straight to the store with the records a
	// sleeping cadence has (a message, a wake time, a suspension).
	t0 = time.Now()
	for i := 0; i < *runs; i++ {
		id := fmt.Sprintf("s%08d", i)
		must(st.CreateRun(ctx, journal.Run{ID: id, Namespace: "bench", Function: "cadence", Generation: "g", Conversation: "c" + id,
			Scopes: []byte(`{"conversation":{"id":"c` + id + `"},"event":{"time":{"$time":"2026-09-23T12:00:00Z"}}}`),
			Status: journal.StatusResolving, CreatedAt: base, UpdatedAt: base}))
		for _, r := range []journal.Record{
			{Site: "steps.d0", Kind: journal.KindSend, Value: []byte(`{"to":"user","text":"Olá"}`), At: base},
			{Site: "steps.d1/wake", Kind: journal.KindResult, Value: []byte(`{"$time":"2026-09-24T12:00:00Z"}`), At: base},
			{Site: "steps.d1", Kind: journal.KindSleep, Value: []byte(`{"wake":{"$time":"2026-09-24T12:00:00Z"}}`), At: base},
		} {
			_, err := st.Append(ctx, id, r)
			must(err)
		}
		// Spread wake times over 30 days, so "due now" is a thin slice.
		must(st.Sleep(ctx, id, journal.StatusWaitingForTime, "steps.d1", base.Add(time.Duration(i%(30*24*60))*time.Minute+48*time.Hour)))
	}
	insert := time.Since(t0)

	measure := func(at time.Time) (time.Duration, int) {
		const n = 50
		var total time.Duration
		var got int
		for i := 0; i < n; i++ {
			t := time.Now()
			rs, err := st.DueRuns(ctx, at, "", 500)
			must(err)
			total += time.Since(t)
			got = len(rs)
		}
		return total / n, got
	}
	noneDue, n0 := measure(base.Add(time.Hour))
	someDue, n1 := measure(base.Add(48*time.Hour + 10*time.Minute))
	info, err := os.Stat(*db)
	must(err)
	wal, _ := os.Stat(*db + "-wal")
	size := info.Size()
	if wal != nil {
		size += wal.Size()
	}

	fmt.Printf("end to end: %d runs started in %v (%.0f/s), %d woken in %v (%.0f/s)\n",
		*wakes, started.Round(time.Millisecond), float64(*wakes)/started.Seconds(), woken, wakeTime.Round(time.Millisecond), float64(woken)/wakeTime.Seconds())
	fmt.Printf("sleepers: %d written in %v (%.0f/s)\n", *runs, insert.Round(time.Millisecond), float64(*runs)/insert.Seconds())
	fmt.Printf("DueRuns(limit 500): none due %v (%d); ~%d due %v (%d)\n", noneDue.Round(time.Microsecond), n0, n1, someDue.Round(time.Microsecond), n1)
	fmt.Printf("database: %.1f MB, %.0f bytes per sleeping run\n", float64(size)/1e6, float64(size)/float64(*runs+*wakes))
}

func program(st journal.Store, now *time.Time) *runtime.Runtime {
	dir, err := os.MkdirTemp("", "timerbench")
	must(err)
	must(os.WriteFile(filepath.Join(dir, "cadence.yaml"), []byte(cadence), 0o600))
	l, err := admission.NewLoader()
	must(err)
	docs, issues := l.LoadPaths([]string{dir})
	if len(issues) > 0 {
		panic(fmt.Sprint(issues))
	}
	prog, err := runtime.Build(docs)
	must(err)
	i := 0
	return &runtime.Runtime{Program: prog, Store: st, Actor: act.NewRegistry(), Clock: func() time.Time { return *now },
		NewID: func() string { i++; return fmt.Sprintf("w%08d", i) }}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
