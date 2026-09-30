package metrics

import (
	"testing"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

var sp, _ = time.LoadLocation("America/Sao_Paulo")

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 10, 0, 0, 0, sp) }

func ms(id, name string, at time.Time, attrs ...string) journal.Milestone {
	a := map[string]string{}
	for i := 0; i+1 < len(attrs); i += 2 {
		a[attrs[i]] = attrs[i+1]
	}
	return journal.Milestone{Namespace: "ns", Subject: "paciente", ID: id, Name: name, At: at, Attributes: a}
}

func clinic() *agents.Funnel {
	return &agents.Funnel{Metadata: agents.ObjectMeta{Name: "contato-a-presenca"}, Spec: agents.FunnelSpec{
		Subject: "paciente", Cohort: "week",
		Stages: []agents.FunnelStage{
			{Name: "contato", Milestone: "conversa-iniciada"},
			{Name: "agendamento", Milestone: "agendou", Within: "14d"},
			{Name: "presenca", Milestone: "compareceu", Within: "30d"},
		}}}
}

// The same people from contact to attendance, by the week they arrived:
// a slow booking falls out of the window, a recent contact is pending,
// not lost.
func TestClinicFunnel(t *testing.T) {
	data := []journal.Milestone{
		ms("a", "conversa-iniciada", day(9, 7), "unidade", "centro"), ms("a", "agendou", day(9, 8)), ms("a", "compareceu", day(9, 15)),
		ms("b", "conversa-iniciada", day(9, 8), "unidade", "zona-sul"), ms("b", "agendou", day(9, 20)),
		ms("c", "conversa-iniciada", day(9, 9), "unidade", "centro"), ms("c", "agendou", day(9, 30)), // 21 days: too late
		ms("d", "conversa-iniciada", day(9, 10), "unidade", "centro"),
		ms("e", "conversa-iniciada", day(10, 25), "unidade", "centro"),
		ms("x", "agendou", day(9, 8)), // never entered: not counted
	}
	rep, err := Compute(clinic(), data, day(10, 30), sp, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Cohorts) != 2 {
		t.Fatalf("cohorts %+v", rep.Cohorts)
	}
	sep := rep.Cohorts[0]
	if !sep.Start.Equal(time.Date(2026, 9, 7, 0, 0, 0, 0, sp)) || sep.Entered != 4 {
		t.Fatalf("first cohort %+v", sep)
	}
	book, show := sep.Stages[0], sep.Stages[1]
	if book.Reached != 2 || book.Lost != 2 || book.Pending != 0 || book.Conversion != 0.5 || book.Median != 24*time.Hour || book.P90 != 12*24*time.Hour {
		t.Errorf("booking %+v", book)
	}
	if show.Reached != 1 || show.Lost != 1 || show.Conversion != 0.5 || show.Median != 7*24*time.Hour {
		t.Errorf("attendance %+v", show)
	}
	if sep.Total.Reached != 1 || sep.Total.Median != 8*24*time.Hour {
		t.Errorf("total %+v", sep.Total)
	}
	oct := rep.Cohorts[1]
	if oct.Entered != 1 || oct.Stages[0].Pending != 1 || oct.Stages[0].Lost != 0 || oct.Stages[0].Conversion != 0 {
		t.Errorf("a recent contact is pending, not lost: %+v", oct)
	}
	// Cut by unit: the same subjects, grouped.
	rep, _ = Compute(clinic(), data, day(10, 30), sp, "unidade")
	groups := map[string]int{}
	for _, c := range rep.Cohorts {
		groups[c.Group] += c.Entered
	}
	if groups["centro"] != 4 || groups["zona-sul"] != 1 {
		t.Errorf("groups %v", groups)
	}
}

// A retail chain: days from contract to opening, time per phase, and the
// stores stuck longer than expected.
func TestFranchiseFunnel(t *testing.T) {
	f := &agents.Funnel{Metadata: agents.ObjectMeta{Name: "abertura"}, Spec: agents.FunnelSpec{Subject: "loja", Cohort: "month",
		Stages: []agents.FunnelStage{
			{Name: "contrato", Milestone: "contrato-assinado"},
			{Name: "ponto", Milestone: "ponto-aprovado", Expected: "30d"},
			{Name: "projeto", Milestone: "projeto-aprovado", Expected: "20d"},
			{Name: "inauguracao", Milestone: "loja-aberta", Expected: "60d"},
		}}}
	data := []journal.Milestone{
		ms("f1", "contrato-assinado", day(1, 5)), ms("f1", "ponto-aprovado", day(1, 25)), ms("f1", "projeto-aprovado", day(2, 14)), ms("f1", "loja-aberta", day(4, 15)),
		ms("f2", "contrato-assinado", day(1, 12)), ms("f2", "ponto-aprovado", day(2, 21)), ms("f2", "projeto-aprovado", day(3, 3)), ms("f2", "loja-aberta", day(5, 2)),
		ms("f3", "contrato-assinado", day(1, 20)), ms("f3", "ponto-aprovado", day(2, 9)), // no project for 67 days by April 17
	}
	rep, err := Compute(f, data, day(4, 17), sp, "")
	if err != nil {
		t.Fatal(err)
	}
	jan := rep.Cohorts[0]
	if jan.Entered != 3 || jan.Total.Reached != 1 {
		t.Fatalf("january %+v", jan)
	}
	if jan.Total.Median != 100*24*time.Hour {
		t.Errorf("days to open: %v, want 100d", jan.Total.Median)
	}
	ponto, projeto := jan.Stages[0], jan.Stages[1]
	if ponto.Reached != 3 || ponto.Median != 20*24*time.Hour || ponto.P90 != 40*24*time.Hour {
		t.Errorf("site phase %+v", ponto)
	}
	if projeto.Reached != 2 || projeto.Pending != 1 || len(projeto.Stuck) != 1 || projeto.Stuck[0].ID != "f3" || !projeto.Stuck[0].Since.Equal(day(2, 9)) {
		t.Errorf("project phase: f3 is stuck (67 days, expected 20): %+v", projeto)
	}
	// f2 opens on May 2, after "now": as of April 17 it has not, and a
	// report at a time must not see the future (found by this test).
	if inaug := jan.Stages[2]; inaug.Reached != 1 || inaug.Pending != 1 || len(inaug.Stuck) != 0 {
		t.Errorf("opening %+v", inaug)
	}
}

func TestParseDuration(t *testing.T) {
	for s, want := range map[string]time.Duration{"14d": 14 * 24 * time.Hour, "36h": 36 * time.Hour, "90m": 90 * time.Minute} {
		if got, err := ParseDuration(s); err != nil || got != want {
			t.Errorf("%s: %v %v", s, got, err)
		}
	}
	for _, bad := range []string{"", "0d", "-3d", "d", "1w", "-1h"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
