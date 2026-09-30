package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// voiceGen rewrites drafts with a function, and counts calls.
type voiceGen struct {
	mu      sync.Mutex
	calls   int
	rewrite func(draft string, facts []string) (string, error)
	states  []map[string]any
	instr   []string
}

func (g *voiceGen) Name() string { return "voice" }
func (g *voiceGen) Generate(_ context.Context, req generate.Request) (*generate.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	st, _ := req.State.(map[string]any)
	g.states = append(g.states, st)
	g.instr = append(g.instr, req.Instructions)
	draft, _ := st["draft"].(string)
	var facts []string
	if fs, ok := st["facts"].([]string); ok {
		facts = fs
	} else if fs, ok := st["facts"].([]any); ok {
		for _, f := range fs {
			facts = append(facts, f.(string))
		}
	}
	text, err := g.rewrite(draft, facts)
	if err != nil {
		return nil, err
	}
	return &generate.Response{Text: text}, nil
}

func voiceHarness(t *testing.T, rewrite func(string, []string) (string, error)) (*harness, *voiceGen) {
	h := newHarness(t)
	g := &voiceGen{rewrite: rewrite}
	h.rt.Generator = g
	h.rt.Voice = &Voice{Persona: "Recepcionista calorosa da clínica; trata o paciente por você."}
	return h, g
}

// A rewrite that keeps every fact is sent; the confirmation still names
// the appointment and the new time, and the effect happens as before.
func TestVoiceRewritesKeepingFacts(t *testing.T) {
	h, g := voiceHarness(t, func(draft string, facts []string) (string, error) {
		return "Oi! 😊 " + draft, nil
	})
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	res := h.reply("user", "sim", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	// Only the first message keeps its greeting.
	for i, m := range h.user {
		prefix := "😊 "
		if i == 0 {
			prefix = "Oi! 😊 "
		}
		if !strings.HasPrefix(m, prefix) {
			t.Errorf("message %d: %q", i, m)
		}
	}
	if h.clin.reschedules != 1 || g.calls != len(h.user) {
		t.Errorf("reschedules %d, voice calls %d for %d messages", h.clin.reschedules, g.calls, len(h.user))
	}
	if facts, _ := g.states[1]["facts"].([]any); len(facts) == 0 {
		t.Errorf("the slot question's facts are passed: %v", g.states[1]["facts"])
	}
}

// Any failed check sends the template instead: a missing fact, a
// confirmation that stopped asking, a placeholder, an error.
func TestVoiceFallsBackToTheTemplate(t *testing.T) {
	cases := map[string]func(string, []string) (string, error){
		"drops a fact": func(d string, f []string) (string, error) {
			out := d
			for _, x := range f {
				out = strings.ReplaceAll(out, x, "o horário combinado")
			}
			return out, nil
		},
		"not a question": func(d string, _ []string) (string, error) { return strings.ReplaceAll(d, "?", "."), nil },
		"placeholder":    func(d string, _ []string) (string, error) { return d + " Escreva para [EMAIL].", nil },
		"generate fails": func(string, []string) (string, error) { return "", errors.New("down") },
	}
	for name, rw := range cases {
		t.Run(name, func(t *testing.T) {
			h, _ := voiceHarness(t, rw)
			h.start("quero remarcar a de terça")
			res := h.reply("user", "às 16h", nil)
			expectStatus(t, res, journal.StatusConfirming)
			if got := h.user[len(h.user)-1]; got != "Posso remarcar sua consulta de Ter 22/09 14:00 para Qui 24/09 16:00? Responda sim para confirmar." {
				t.Errorf("confirmation %q", got)
			}
		})
	}
}

// The rewrite is recorded once: replays and resumes do not call the model
// again and send the same words.
func TestVoiceIsRecordedOnce(t *testing.T) {
	n := 0
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) {
		n++
		return d + strings.Repeat("!", n), nil
	})
	h.start("quero remarcar a de terça")
	calls := g.calls
	for range 3 {
		res, err := h.rt.Resume(context.Background(), "run-1")
		if err != nil {
			t.Fatal(err)
		}
		expectStatus(t, res, journal.StatusWaitingForUser)
	}
	if g.calls != calls {
		t.Errorf("replay called the voice again: %d then %d", calls, g.calls)
	}
}

// The conversation the voice sees is masked like every model input.
func TestVoiceSeesAMaskedConversation(t *testing.T) {
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) { return d, nil })
	h.start("quero remarcar a de terça, meu email é ana@x.com")
	for _, st := range g.states {
		for _, l := range st["conversation"].([]any) {
			if strings.Contains(l.(string), "ana@x.com") {
				t.Errorf("personal data reached the model: %q", l)
			}
		}
	}
}

func TestStripGreeting(t *testing.T) {
	for in, want := range map[string]string{
		"Oi, Marina! Claro, te ajudo.":         "Claro, te ajudo.",
		"Olá Ana, sua consulta foi remarcada.": "Sua consulta foi remarcada.",
		"Oi! 😊 Pronto.":                        "😊 Pronto.",
		"Bom dia, Carlos! Qual dia?":           "Qual dia?",
		"oi, tudo certo por aqui.":             "Tudo certo por aqui.",
		"Oi, Marina!":                          "Oi, Marina!",
		"Oitenta reais a limpeza.":             "Oitenta reais a limpeza.",
		"Posso remarcar? Oi.":                  "Posso remarcar? Oi.",
		"Oi Marina 😊 qual dia?":                "Oi Marina 😊 qual dia?",
	} {
		if got := StripGreeting(in); got != want {
			t.Errorf("StripGreeting(%q) = %q, want %q", in, got, want)
		}
	}
}

// A kept word the rewrite drops sends the template: "remarcar" does not
// become "agendar".
func TestVoiceKeepsWords(t *testing.T) {
	h, _ := voiceHarness(t, func(d string, _ []string) (string, error) {
		return strings.ReplaceAll(d, "remarcar", "agendar"), nil
	})
	h.rt.Voice.Keep = []string{"Remarc"}
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	if got := h.user[len(h.user)-1]; got != "Posso remarcar sua consulta de Ter 22/09 14:00 para Qui 24/09 16:00? Responda sim para confirmar." {
		t.Errorf("confirmation %q", got)
	}
}

// Reask asks the pending question again in the voice, from its draft and
// facts, as a continuation; a rewrite that fails a check gives the
// question as sent, without its greeting.
func TestReask(t *testing.T) {
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) { return "Oi, Ana! " + d, nil })
	h.start("quero remarcar a de terça")
	res := h.reply("user", "às 16h", nil)
	expectStatus(t, res, journal.StatusConfirming)
	g.rewrite = func(d string, _ []string) (string, error) { return "Voltando: " + d, nil }
	got, err := h.rt.Reask(context.Background(), res.RunID, []string{"user: tem estacionamento?", "agent: Tem sim."})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Voltando: Posso remarcar sua consulta de Ter 22/09 14:00 para Qui 24/09 16:00? Responda sim para confirmar."; got != want {
		t.Errorf("reask %q", got)
	}
	st := g.states[len(g.states)-1]
	conv := st["conversation"].([]any)
	if last := conv[len(conv)-1]; last != "agent: Tem sim." {
		t.Errorf("the aside is not in the conversation: %v", conv)
	}
	if facts, _ := st["facts"].([]any); len(facts) == 0 {
		t.Errorf("the facts are not locked: %v", st["facts"])
	}

	g.rewrite = func(d string, _ []string) (string, error) { return "Voltando: posso remarcar?", nil }
	got, err = h.rt.Reask(context.Background(), res.RunID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Posso remarcar sua consulta de Ter 22/09 14:00 para Qui 24/09 16:00? Responda sim para confirmar."; got != want {
		t.Errorf("fallback %q", got)
	}
}

// Latitude "warm" swaps the instructions; the fact lock and the checks are
// the same (voice-004).
func TestWarmVoice(t *testing.T) {
	h, g := voiceHarness(t, func(d string, f []string) (string, error) { return "Claro! " + d, nil })
	h.rt.Voice.Warm = true
	h.start("quero remarcar a de terça")
	if !strings.Contains(g.instr[0], "friendly person") || !strings.Contains(g.instr[0], "Keep every item of FACTS") {
		t.Errorf("warm instructions: %s", g.instr[0])
	}
	h2, g2 := voiceHarness(t, func(d string, f []string) (string, error) { return d, nil })
	h2.start("quero remarcar a de terça")
	if strings.Contains(g2.instr[0], "friendly person") {
		t.Errorf("Warm false gives the strict instructions")
	}
}

// Repeated questions can omit padding, but not the options already offered.
func TestRepeatedQuestionKeepsFacts(t *testing.T) {
	h, g := voiceHarness(t, func(d string, f []string) (string, error) { return d, nil })
	h.rt.Voice.Warm = true
	h.start("quero remarcar a de terça")
	want := h.user[len(h.user)-1]
	// An attempted short rewrite loses the available slots and must fall back.
	g.rewrite = func(string, []string) (string, error) { return "Qual desses horários prefere?", nil }
	first := len(g.instr)
	h.reply("user", "tem sábado?", nil)
	if len(g.instr) <= first {
		t.Fatalf("no second rewrite")
	}
	if facts, _ := g.states[len(g.states)-1]["facts"].([]any); len(facts) == 0 {
		t.Error("the repeated question lost its facts")
	}
	if got := h.user[len(h.user)-1]; got != want {
		t.Errorf("missing-fact rewrite was sent: %q; want %q", got, want)
	}
	calls := g.calls
	if _, err := h.rt.Resume(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	if g.calls != calls {
		t.Error("replay regenerated the repeated question")
	}
}

func TestRepeatedConfirmationKeepsFacts(t *testing.T) {
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) { return d, nil })
	h.rt.Voice.Warm = true
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	want := h.user[len(h.user)-1]
	g.rewrite = func(string, []string) (string, error) { return "Posso remarcar?", nil }
	res := h.reply("user", "talvez", nil)
	expectStatus(t, res, journal.StatusConfirming)
	if got := h.user[len(h.user)-1]; got != want {
		t.Errorf("confirmation lost facts: %q; want %q", got, want)
	}
	if last := g.instr[len(g.instr)-1]; strings.Contains(last, "already asked") || strings.Contains(last, "did not choose any of its options") {
		t.Error("confirmation used the repeated-question instructions")
	}
	if h.clin.reschedules != 0 {
		t.Error("unclear confirmation performed an effect")
	}
}

// The keep instruction lists the draft's own words, never a bare stem
// ("remarc"), which the model sometimes copied (voice-008).
func TestKeepInstructionUsesDraftWords(t *testing.T) {
	if got := keepWords("Posso remarcar? A remarcação e a Remarcação ficam salvas.", []string{"Remarc"}); strings.Join(got, ",") != "remarcar,remarcação" {
		t.Errorf("keepWords = %v", got)
	}
	if got := keepWords("Qual dia prefere?", []string{"remarc"}); len(got) != 0 {
		t.Errorf("no keep word in the draft: %v", got)
	}
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) { return d, nil })
	h.rt.Voice.Keep = []string{"remarc"}
	h.start("quero remarcar a de terça")
	h.reply("user", "às 16h", nil)
	var found bool
	for _, in := range g.instr {
		if strings.Contains(in, "Keep these words") {
			found = true
			if strings.Contains(in, "writes them: remarc.") || !strings.Contains(in, "remarcar") {
				t.Errorf("keep instruction: %s", in)
			}
		}
	}
	if !found {
		t.Error("no keep instruction for a draft that has the word")
	}
}

// A question asked again after a reply that picked nothing acknowledges
// that reply and keeps every option (voice-008).
func TestRepeatedQuestionAcknowledgesTheReply(t *testing.T) {
	h, g := voiceHarness(t, func(d string, _ []string) (string, error) { return d, nil })
	h.rt.Voice.Warm = true
	h.start("quero remarcar a de terça")
	first := len(g.instr)
	if strings.Contains(g.instr[first-1], "did not choose any of its options") {
		t.Fatal("the first question was marked as repeated")
	}
	h.reply("user", "tem sábado?", nil)
	if len(g.instr) <= first {
		t.Fatal("no second rewrite")
	}
	if !strings.Contains(g.instr[len(g.instr)-1], "did not choose any of its options") {
		t.Error("the repeated question does not acknowledge the reply")
	}
	if facts, _ := g.states[len(g.states)-1]["facts"].([]any); len(facts) == 0 {
		t.Error("the repeated question lost its facts")
	}
}

// beyondDecider answers the voice's claims question by keyword and
// passes everything else on.
type beyondDecider struct{ decide.Decider }

func (d beyondDecider) Decide(ctx context.Context, req decide.Request) (*decide.Response, error) {
	q, ok := req.Questions["beyond"]
	if !ok {
		return d.Decider.Decide(ctx, req)
	}
	rewrite, _ := req.State.(map[string]any)["rewrite"].(string)
	yes := 0.05
	if strings.Contains(rewrite, "pedi para") {
		yes = 0.95
	}
	a, err := decide.AnswerFor(q, map[string]float64{"true": yes, "false": 1 - yes}, decide.ProbNative)
	if err != nil {
		return nil, err
	}
	return &decide.Response{Answers: map[string]decide.Answer{"beyond": a}}, nil
}

// agency-001: the warm voice wrote "já pedi para a agente verificar"
// around a question that asked nothing of anyone. A rewrite that claims an
// action the draft does not is dropped, and the draft goes out.
func TestWarmVoiceNeverClaimsAnAction(t *testing.T) {
	h, _ := voiceHarness(t, func(d string, f []string) (string, error) {
		return "Perfeito, já pedi para a equipe verificar. " + d, nil
	})
	h.rt.Voice.Warm = true
	h.rt.Decider = beyondDecider{h.rt.Decider}
	h.start("quero remarcar a de terça")
	for _, m := range h.user {
		if strings.Contains(m, "pedi para") {
			t.Errorf("a claimed action reached the user: %q", m)
		}
	}
	h2, _ := voiceHarness(t, func(d string, f []string) (string, error) { return "Claro! " + d, nil })
	h2.rt.Voice.Warm = true
	h2.rt.Decider = beyondDecider{h2.rt.Decider}
	h2.start("quero remarcar a de terça")
	if len(h2.user) == 0 || !strings.HasPrefix(h2.user[0], "Claro! ") {
		t.Errorf("a rewrite without claims is kept: %q", h2.user)
	}
}
