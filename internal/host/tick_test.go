package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const report = `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: bistro, namespace: bistro }
spec: { timezone: America/Sao_Paulo }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: daily-report, namespace: bistro }
spec:
  description: Tells the team the day started.
  steps:
    - { id: say, respond: { template: "Bom dia, equipe ({{ event.trigger }})" } }
  output: { template: "{{ formatDate(event.time, 'America/Sao_Paulo') }} {{ string(event.time) }}" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: morning, namespace: bistro }
spec:
  function: daily-report
  schedule: { cron: "0 9 * * 1-5", maxDelay: 2h }
`

func tickHost(t *testing.T, store journal.Store, now *time.Time) *Host {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bistro.yaml"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "bistro", Store: store, Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return *now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	return h
}

func TestScheduleTrigger(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	store := journal.NewMemory()
	now := time.Date(2026, 9, 28, 8, 59, 0, 0, sp)             // Monday
	a, b := tickHost(t, store, &now), tickHost(t, store, &now) // two schedulers, one store
	ctx := context.Background()
	fire := func() (texts []string) {
		for _, h := range []*Host{a, b} {
			res, err := h.Tick(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range res {
				for _, m := range r.Outbox {
					texts = append(texts, m.Text)
				}
			}
		}
		return texts
	}
	if got := fire(); len(got) != 0 {
		t.Fatalf("fired before 9h: %q", got)
	}
	now = time.Date(2026, 9, 28, 9, 0, 30, 0, sp)
	got := fire()
	if len(got) != 2 || got[0] != "Bom dia, equipe (morning)" || !strings.HasPrefix(got[1], "2026-09-28 2026-09-28T09:00:00-03:00") {
		t.Fatalf("at 9h, once, with event.time = the scheduled time: %q", got)
	}
	now = now.Add(30 * time.Minute)
	if got := fire(); len(got) != 0 {
		t.Errorf("the same slot fired again: %q", got)
	}
	// Down from Monday 9h30 to Tuesday 10h30: Tuesday's 9h is 1h30 late,
	// within maxDelay (2h), so it runs once.
	now = time.Date(2026, 9, 29, 10, 30, 0, 0, sp)
	if got := fire(); len(got) != 2 {
		t.Errorf("a slot within maxDelay must run after downtime: %q", got)
	}
	// Down until Wednesday 11h30: 2h30 late, skipped.
	now = time.Date(2026, 9, 30, 11, 30, 0, 0, sp)
	if got := fire(); len(got) != 0 {
		t.Errorf("a slot older than maxDelay must be skipped: %q", got)
	}
}

// A store waiting past 1.5x the expected time starts the funnel's
// onStuck Function once: the team gets one task, however many checks.
func TestFunnelOnStuck(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	store := journal.NewMemory()
	ctx := context.Background()
	for i, m := range []struct {
		name string
		at   time.Time
	}{{"contrato-assinado", time.Date(2026, 1, 5, 10, 0, 0, 0, sp)}, {"ponto-aprovado", time.Date(2026, 1, 20, 10, 0, 0, 0, sp)}} {
		id := fmt.Sprintf("seed%d", i)
		if err := store.CreateRun(ctx, journal.Run{ID: id, Namespace: "rede", Function: "registrar-etapa", Generation: "g", Conversation: id, Scopes: []byte("{}"), Status: journal.StatusSucceeded, CreatedAt: m.at, UpdatedAt: m.at}); err != nil {
			t.Fatal(err)
		}
		v, _ := journal.Encode(map[string]any{"namespace": "rede", "subject": "loja", "id": "12", "name": m.name, "at": m.at, "attributes": map[string]any{}})
		if _, err := store.Append(ctx, id, journal.Record{Site: "s/milestone", Kind: journal.KindMilestone, Value: v, At: m.at}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 2, 10, 10, 0, 0, 0, sp) // project expected in 20d: 21 days, not stuck yet (30)
	h, issues, err := Load(Options{Paths: []string{"../../testdata/lojas"}, Namespace: "rede", Store: store, Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	tasks := func() []string {
		res, err := h.Tick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range res {
			for _, m := range r.Outbox {
				out = append(out, m.To+": "+m.Text)
			}
			if n := len(r.Outbox); n > 0 { // delivered, as a channel would
				if err := h.Runtime.Ack(ctx, r.RunID, r.Outbox[n-1].Seq); err != nil {
					t.Fatal(err)
				}
			}
		}
		return out
	}
	if got := tasks(); len(got) != 0 {
		t.Fatalf("not stuck yet: %q", got)
	}
	now = time.Date(2026, 2, 25, 10, 0, 0, 0, sp) // 36 days since the site was approved
	got := tasks()
	if len(got) != 1 || got[0] != "queue:implantacao: A loja 12 está há 36 dias esperando a etapa projeto. O que houve?" {
		t.Fatalf("one task for the team: %q", got)
	}
	now = now.Add(24 * time.Hour)
	// A day later: no second task for the same store; the open task's
	// queue SLA (a working day) reminds and escalates it instead.
	for _, m := range tasks() {
		if !strings.Contains(m, "⏰") && !strings.Contains(m, "⤴") {
			t.Errorf("the same stuck store started another task: %q", m)
		}
	}
	runs, _ := store.Runs(ctx, journal.RunFilter{Function: "avisar-atraso"})
	if len(runs) != 1 {
		t.Errorf("one task run per stuck episode: %d", len(runs))
	}
}

// Monday 9h: the report Function reads the funnel through the built-in
// metrics plugin and the team's queue gets the numbers, once.
func TestWeeklyReport(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	store := journal.NewMemory()
	ctx := context.Background()
	seed := func(id, name string, at time.Time) {
		run := id + "/" + name
		if err := store.CreateRun(ctx, journal.Run{ID: run, Namespace: "rede", Function: "registrar-etapa", Generation: "g", Conversation: run, Scopes: []byte("{}"), Status: journal.StatusSucceeded, CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
		v, _ := journal.Encode(map[string]any{"namespace": "rede", "subject": "loja", "id": id, "name": name, "at": at, "attributes": map[string]any{}})
		if _, err := store.Append(ctx, run, journal.Record{Site: "s/milestone", Kind: journal.KindMilestone, Value: v, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 10, 0, 0, 0, sp) }
	for i, name := range []string{"contrato-assinado", "ponto-aprovado", "projeto-aprovado", "obra-concluida", "equipe-treinada", "loja-aberta"} {
		seed("1", name, d(1, 5).AddDate(0, 0, []int{0, 20, 40, 90, 100, 105}[i]))
	}
	seed("2", "contrato-assinado", d(1, 12))
	seed("2", "ponto-aprovado", d(2, 21))
	now := time.Date(2026, 6, 1, 9, 0, 30, 0, sp) // a Monday
	h, issues, err := Load(Options{Paths: []string{"../../testdata/lojas"}, Namespace: "rede", Store: store, Decider: &wordDecider{},
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	var notes []string
	for range 2 {
		res, err := h.Tick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			for _, m := range r.Outbox {
				if strings.HasPrefix(m.Text, "Abertura de lojas") {
					notes = append(notes, m.To+" | "+m.Text)
				}
			}
		}
	}
	if len(notes) != 1 {
		t.Fatalf("one report, to the team: %q", notes)
	}
	t.Log(notes[0])
	for _, want := range []string{"queue:implantacao | Abertura de lojas: 2 em implantação.", "- ponto: 2 concluíram", "coorte 2026-01-01: 105 dias do contrato à inauguração", "loja 2 parada antes de projeto"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("report lacks %q:\n%s", want, notes[0])
		}
	}
}
