package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
)

type contextPromiseGen struct{ seen map[string]any }

func (*contextPromiseGen) Name() string { return "context-promise" }
func (g *contextPromiseGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.seen = req.State.(map[string]any)
	text := "Oi, Ana! " + g.seen["draft"].(string)
	for _, line := range g.seen["conversation"].([]any) {
		if strings.Contains(line.(string), "limpeza") {
			text += " Consigo agendar a limpeza para você."
		}
	}
	return &generate.Response{Text: text}, nil
}

// The wording stage must not turn a side request from the transcript into
// a new capability. Its approved draft and supplied name remain available.
func TestWarmVoiceDoesNotReceiveSideRequests(t *testing.T) {
	g := &contextPromiseGen{}
	r := &Runtime{Voice: &Voice{Warm: true, Persona: "Recepcionista cordial."}, Generator: g}
	out, why, err := r.rewrite(context.Background(), rewriting{draft: "Consulta remarcada para sexta 9h.", facts: []string{"sexta 9h"}, user: "Ana",
		conversation: []string{"agent: Posso remarcar?", "user: Sim. Também consegue agendar uma limpeza? Meu email é ana@example.com."}})
	if err != nil || why != "" || strings.Contains(out, "limpeza") || strings.HasPrefix(out, "Oi") {
		t.Fatalf("unapproved request affected completion: %q %q %v", out, why, err)
	}
	history := g.seen["conversation"].([]any)
	if len(history) != 1 || history[0] != "agent: (already spoke)" {
		t.Fatalf("content leaked to wording: %v", history)
	}
	if g.seen["user"] != "Ana" || g.seen["draft"] != "Consulta remarcada para sexta 9h." {
		t.Fatalf("lost approved content: %v", g.seen)
	}
}

func TestWarmVoiceFirstMessageAndStrictHistory(t *testing.T) {
	for _, warm := range []bool{true, false} {
		g := &contextPromiseGen{}
		r := &Runtime{Voice: &Voice{Warm: warm}, Generator: g}
		out, why, err := r.rewrite(context.Background(), rewriting{draft: "Qual dia prefere?", user: "Ana", conversation: []string{"user: Quero remarcar. Meu email é ana@example.com."}})
		if err != nil || why != "" || !strings.HasPrefix(out, "Oi") {
			t.Fatalf("first greeting: %q %q %v", out, why, err)
		}
		history := g.seen["conversation"].([]any)
		if warm && len(history) != 0 {
			t.Fatalf("warm received user history: %v", history)
		}
		if !warm && (len(history) != 1 || !strings.Contains(history[0].(string), "[EMAIL]") || strings.Contains(history[0].(string), "ana@example.com")) {
			t.Fatalf("strict history must remain masked: %v", history)
		}
	}
}

type instrGen struct {
	contextPromiseGen
	instructions string
}

func (g *instrGen) Generate(ctx context.Context, req generate.Request) (*generate.Response, error) {
	g.instructions = req.Instructions
	return g.contextPromiseGen.Generate(ctx, req)
}

// With dialogue: full, the warm voice sees the masked dialogue and the
// rule that only DRAFT authorizes content (voice-008). Whether the model
// obeys is measured (docs/research/voice-008.md), not unit-tested; the
// facts, the checks and the masking are.
func TestWarmVoiceFullDialogue(t *testing.T) {
	g := &instrGen{}
	r := &Runtime{Voice: &Voice{Warm: true, WarmDialogue: true, Persona: "Recepcionista cordial."}, Generator: g}
	_, why, err := r.rewrite(context.Background(), rewriting{draft: "Consulta remarcada para sexta 9h.", facts: []string{"sexta 9h"}, user: "Ana",
		conversation: []string{"agent: Posso remarcar?", "user: Sim. Meu email é ana@example.com."}})
	if err != nil || why != "" {
		t.Fatalf("%q %v", why, err)
	}
	history := g.seen["conversation"].([]any)
	if len(history) != 2 || !strings.Contains(history[1].(string), "[EMAIL]") || strings.Contains(history[1].(string), "ana@example.com") {
		t.Fatalf("the dialogue must reach the voice, masked: %v", history)
	}
	if !strings.Contains(g.instructions, "DRAFT is the sole authority") {
		t.Errorf("the boundary rule is missing: %s", g.instructions)
	}
	// The default keeps voice-007's marker and has no boundary rule.
	d := &instrGen{}
	r = &Runtime{Voice: &Voice{Warm: true}, Generator: d}
	if _, _, err := r.rewrite(context.Background(), rewriting{draft: "Qual dia prefere?", conversation: []string{"agent: Oi!", "user: quero remarcar"}}); err != nil {
		t.Fatal(err)
	}
	if h := d.seen["conversation"].([]any); len(h) != 1 || h[0] != "agent: (already spoke)" || strings.Contains(d.instructions, "sole authority") {
		t.Errorf("the default changed: %v", h)
	}
}
