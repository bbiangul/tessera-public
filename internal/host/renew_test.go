package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// renew-001 W1-W3: due lessons, and only those, get one renew task (with
// the team's repeated answers); approval renews and brings an expired
// lesson back; denial and roles hold.
func TestLessonsRenewedInTheChannel(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(lessonsAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(data, "lessons", "ls", "licoes.md")
	_ = os.MkdirAll(filepath.Dir(file), 0o700)
	_ = os.WriteFile(file, []byte(`# Lições

## Tem café?

<!-- lesson id=cafe kind=fact review=2026-09-01 cases=2 -->
Temos café na recepção.

## Tem estacionamento?

<!-- lesson id=estacionamento kind=price review=2026-10-08 cases=2 -->
O conveniado é o Parque Seguro.

## Tem wifi?

<!-- lesson id=wifi kind=fact review=2026-09-10 cases=2 -->
Temos wifi, a senha fica na recepção.

## Tem piscina?

<!-- lesson id=piscina kind=fact review=2026-09-01 cases=2 status=suspended -->
Temos piscina.

## Tem bebedouro?

<!-- lesson id=longe kind=fact review=2026-12-01 cases=2 -->
Temos bebedouro.
`), 0o600)
	store := journal.NewMemory()
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	answered(t, store, "r1", "tem estacionamento?", "O conveniado é o Parque Seguro.", now.Add(-time.Hour))
	answered(t, store, "r2", "onde paro? estacionamento", "O estacionamento conveniado é o Parque Seguro.", now.Add(-time.Hour))
	eng := &lessonEngines{}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "ls", Store: store, Decider: eng, Generator: eng, DataDir: data,
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	tasks := func() map[string]runtime.Task {
		ts, _ := h.Runtime.MemberTasks(ctx, "ls", "rita")
		out := map[string]runtime.Task{}
		for _, x := range ts {
			for _, id := range []string{"café", "estacionamento", "wifi", "bebedouro", "piscina"} {
				if strings.Contains(x.Text, id) {
					out[id] = x
				}
			}
		}
		return out
	}
	// The expired café lesson is not used before renewal.
	if _, err := h.Handle(ctx, "a", map[string]any{"channel": "whatsapp"}, "tem café?"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(eng.docs, "|"), "Temos café") {
		t.Fatalf("an expired lesson was used")
	}
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	ts := tasks()
	if len(ts) != 3 || ts["bebedouro"].RunID != "" || ts["piscina"].RunID != "" || !strings.HasPrefix(ts["café"].Text, "Esta lição continua valendo?") {
		t.Fatalf("renew tasks: %+v", ts)
	}
	if !strings.Contains(ts["estacionamento"].Text, "respondeu o mesmo 2 vez") || strings.Contains(ts["café"].Text, "respondeu o mesmo") {
		t.Errorf("reconfirmations: %q / %q", ts["estacionamento"].Text, ts["café"].Text)
	}
	now = now.Add(8 * 24 * time.Hour)
	_, _ = h.Tick(ctx)
	if n := len(tasks()); n != 3 {
		t.Errorf("tasks after another due tick: %d", n)
	}
	gil, _ := h.Runtime.Member("ls", "gil")
	if out, _ := h.Runtime.AnswerTask(ctx, ts["café"].RunID, ts["café"].Hash, gil, true); out != runtime.NotAllowed {
		t.Errorf("gil renewed: %v", out)
	}
	rita, _ := h.Runtime.Member("ls", "rita")
	for topic, ok := range map[string]bool{"café": true, "estacionamento": true, "wifi": false} {
		if out, err := h.Runtime.AnswerTask(ctx, ts[topic].RunID, ts[topic].Hash, rita, ok); out != runtime.Recorded || err != nil {
			t.Fatalf("%s: %v %v", topic, out, err)
		}
	}
	raw, _ := os.ReadFile(file)
	s := string(raw)
	// café (fact): now + 365 days; estacionamento (price): now + 30 days.
	if !strings.Contains(s, "id=cafe kind=fact review=2027-10-06") || !strings.Contains(s, "id=estacionamento kind=price review=2026-11-05") || !strings.Contains(s, "renewedBy=rita") {
		t.Errorf("renewed:\n%s", s)
	}
	if !strings.Contains(s, "id=wifi kind=fact review=2026-09-10 cases=2 -->") {
		t.Errorf("a denied renewal changed the lesson:\n%s", s)
	}
	eng.docs = nil
	if _, err := h.Handle(ctx, "b", map[string]any{"channel": "whatsapp"}, "tem café?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(eng.docs, "|"), "Temos café") {
		t.Errorf("the renewed lesson is not used: %v", eng.docs)
	}
	// Denied: not asked again for that review date.
	now = now.Add(8 * 24 * time.Hour)
	_, _ = h.Tick(ctx)
	if ts := tasks(); ts["wifi"].RunID != "" {
		t.Errorf("a denied renewal was asked again")
	}
}
