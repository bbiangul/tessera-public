package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type slotFactsGen struct{ states []map[string]any }

func (*slotFactsGen) Name() string { return "slots" }
func (g *slotFactsGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	st := req.State.(map[string]any)
	g.states = append(g.states, st)
	raw, err := json.Marshal(st["facts"])
	if err != nil {
		return nil, err
	}
	text := "Não consigo ver os horários por aqui."
	if strings.Contains(string(raw), "terça 10h") && strings.Contains(string(raw), "quarta 16h") {
		text = "Tenho terça 10h e quarta 16h."
	}
	return &generate.Response{Text: text}, nil
}

// The user asks across days before choosing one. No fresh read or effect
// is needed to answer from the already-authorized slot results (#31).
func TestSideQuestionUsesDeclaredRunReads(t *testing.T) {
	for _, share := range []bool{false, true} {
		t.Run(map[bool]string{false: "private-by-default", true: "declared-read"}[share], func(t *testing.T) {
			docs := strings.Replace(freeDocs, "    marcar:", "    slots: { input: {}, output: list<string>, effect: read }\n    marcar:", 1)
			docs = strings.Replace(docs, "        - id: pick", "        - { id: slots, fromAction: agenda/slots }\n        - id: pick", 1)
			if share {
				docs += "    runReads: [agenda/slots]\n"
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(docs), 0o600); err != nil {
				t.Fatal(err)
			}
			reads, effects := 0, 0
			g := &slotFactsGen{}
			h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "clinica", Store: journal.NewMemory(), Decider: turnDecider{}, Generator: g,
				Handlers: map[string]act.Handler{
					"agenda/slots": func(context.Context, act.Call) (any, error) {
						reads++
						return []any{"terça 10h", "quarta 16h"}, nil
					},
					"clinica/info":  func(context.Context, act.Call) (any, error) { return map[string]any{}, nil },
					"agenda/marcar": func(context.Context, act.Call) (any, error) { effects++; return map[string]any{}, nil },
				}})
			if err != nil {
				t.Fatalf("%v %v", err, issues)
			}
			t.Cleanup(func() {
				if err := h.Close(); err != nil {
					t.Error(err)
				}
			})
			r := say(t, h, "quero marcar")
			if r.Status != journal.StatusWaitingForUser {
				t.Fatalf("%+v", r)
			}
			r = say(t, h, "E quais horários específicos vocês têm nesses dias?")
			if len(r.Outbox) != 2 || strings.Contains(r.Outbox[0].Text, "terça 10h") != share {
				t.Fatalf("share=%v: %+v", share, r.Outbox)
			}
			if reads != 1 || effects != 0 || r.Status != journal.StatusWaitingForUser {
				t.Fatalf("reads=%d effects=%d status=%s", reads, effects, r.Status)
			}
			if r = say(t, h, "terça"); r.Status != journal.StatusConfirming || effects != 0 {
				t.Fatalf("must still confirm: %+v, effects=%d", r, effects)
			}
			if r = say(t, h, "sim"); r.Status != journal.StatusSucceeded || effects != 1 || reads != 1 {
				t.Fatalf("%+v reads=%d effects=%d", r, reads, effects)
			}
		})
	}
}
