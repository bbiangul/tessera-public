package inspect

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// A real clinic conversation, explained: the router's choice, the reads,
// Jev's decisions with the question and the options, the confirmation,
// the effect and its milestone, and the transcript.
func TestExplainClinicRun(t *testing.T) {
	store := journal.NewMemory()
	d := &decidetest.Scripted{Answers: map[string]map[string]float64{
		"route": {"reschedule-appointment": 0.97, "none": 0.03},
		"pick":  {"o2": 0.95, "o1": 0.03, "none": 0.02},
		"yes":   {"true": 0.98, "false": 0.02},
	}}
	h, issues, err := host.Load(host.Options{Paths: []string{"../../testdata/agenda"}, Namespace: "agenda-exemplo", Store: store, Decider: d,
		Handlers: clinic()})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	caller := map[string]any{"role": "patient", "patientId": "p1", "appointmentIds": []any{"a1", "a2"}}
	// The scripted Decider picks from the first message: the run goes
	// straight to the confirmation.
	for _, msg := range []string{"quero remarcar a de terça às 16h", "sim"} {
		if _, err := h.Handle(ctx, "c1", caller, msg); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := store.Runs(ctx, journal.RunFilter{Conversation: "c1"})
	if len(runs) != 1 {
		t.Fatalf("runs %v", runs)
	}
	recs, _ := store.Records(ctx, runs[0].ID)
	l, _ := admission.NewLoader()
	docs, _ := l.LoadPaths([]string{"../../testdata/agenda"})
	run, err := New(docs).Explain(runs[0], recs)
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, e := range run.Timeline {
		all = append(all, e.What+" | "+e.Summary)
	}
	text := strings.Join(all, "\n")
	t.Log("\n" + text)
	for _, want := range []string{
		`router | routed to "reschedule-appointment"`,
		"input appointment, read agenda/list-upcoming-appointments (source upcoming) | result:",
		`input appointment, asked (source pick), Jev decision | Jev chose o2 (p=0.95) for "Qual consulta você quer remarcar?"`,
		"confirmation, Jev decision | Jev: yes with p=0.98",
		`action agenda/reschedule | about to call agenda/reschedule {"appointmentId":"a2","slotId":"s2"}`,
		"action agenda/reschedule, milestone | milestone remarcou for paciente p1",
		"the Function's output | message to user: Pronto!",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("timeline lacks %q:\n%s", want, text)
		}
	}
	lines := Transcript(runs, map[string][]journal.Record{runs[0].ID: recs})
	var tr []string
	for _, l := range lines {
		tr = append(tr, l.From+": "+l.Text)
	}
	if got := strings.Join(tr, " / "); !strings.HasPrefix(got, "user: quero remarcar a de terça às 16h / agent: Posso remarcar") || !strings.Contains(got, "reply: sim / agent: Pronto!") {
		t.Errorf("transcript: %s", got)
	}
}

// clinic fakes the clinic's system for the host.
func clinic() map[string]act.Handler {
	return map[string]act.Handler{
		"agenda/list-upcoming-appointments": func(context.Context, act.Call) (any, error) {
			return []any{
				map[string]any{"id": "a1", "dentistId": "d1", "slotId": "s0", "label": "Seg 21/09 09:00"},
				map[string]any{"id": "a2", "dentistId": "d2", "slotId": "s0", "label": "Ter 22/09 14:00"},
			}, nil
		},
		"agenda/find-slots": func(_ context.Context, c act.Call) (any, error) {
			from, _ := c.Args["from"].(time.Time)
			day := from.AddDate(0, 0, 1)
			return []any{
				map[string]any{"id": "s1", "label": "Qui 24/09 10:00", "start": time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, from.Location())},
				map[string]any{"id": "s2", "label": "Qui 24/09 16:00", "start": time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, from.Location())},
			}, nil
		},
		"agenda/reschedule": func(_ context.Context, c act.Call) (any, error) {
			return map[string]any{"id": c.Args["appointmentId"], "slotId": c.Args["slotId"]}, nil
		},
	}
}
