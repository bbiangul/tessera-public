package evals

import (
	"context"
	"os"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
)

type teamGen struct{ value map[string]any }

func (teamGen) Name() string { return "team" }
func (g teamGen) Generate(context.Context, generate.Request) (*generate.Response, error) {
	return &generate.Response{Value: g.value}, nil
}

// team-001: a question gets the team's text, a value task its value; an
// answer of the wrong kind is an error, never a silent default.
func TestTeamAnswerByKind(t *testing.T) {
	ctx := context.Background()
	g := teamGen{map[string]any{"text": " Sim, R$ 120 por trecho. ", "value": nil}}
	if a, err := teamAnswer(ctx, g, "facts", runtime.Task{Kind: "answer"}, nil); err != nil || a != "Sim, R$ 120 por trecho." {
		t.Errorf("%v %v", a, err)
	}
	if _, err := teamAnswer(ctx, g, "facts", runtime.Task{Kind: "input"}, nil); err == nil {
		t.Error("a value task needs a value")
	}
	g = teamGen{map[string]any{"text": nil, "value": map[string]any{"reserved": true}}}
	if a, err := teamAnswer(ctx, g, "facts", runtime.Task{Kind: "input"}, nil); err != nil || a.(map[string]any)["reserved"] != true {
		t.Errorf("%v %v", a, err)
	}
	if _, err := teamAnswer(ctx, nil, "facts", runtime.Task{Kind: "answer"}, nil); err == nil {
		t.Error("no backend")
	}
}

// team-001, live (TESSERA_LIVE=1, OPENROUTER_API_KEY): each task of the
// pre-registered set three times, answers to TEAM_OUT for the judge.
func TestTeamLive(t *testing.T) {
	if os.Getenv("TESSERA_LIVE") != "1" || os.Getenv("TEAM_OUT") == "" {
		t.Skip("live: set TESSERA_LIVE=1 and TEAM_OUT")
	}
	runTeamBench(t, "../../bench/agency/team-001/tasks.json", "../../bench/agency/world/team-facts.md", os.Getenv("TEAM_OUT"))
}
