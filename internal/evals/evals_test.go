package evals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
)

// keywords answers like a careful model would, from words in the state,
// so the runner is tested without a network.
type keywords struct{}

func (keywords) Name() string { return "keywords" }
func (keywords) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	state, _ := req.State.(map[string]any)
	text, _ := state["message"].(string)
	if r, ok := state["reply"].(string); ok {
		text = r
	}
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Model: "keywords"}
	for id, q := range req.Questions {
		probs := map[string]float64{}
		switch q.Kind {
		case decide.KindNoul:
			probs["true"], probs["false"] = 0.03, 0.97
			if strings.Contains(text, "sim") {
				probs["true"], probs["false"] = 0.97, 0.03
			}
		default:
			pick := "none"
			for _, o := range q.Options {
				switch {
				case o.Key == "reschedule-appointment" && strings.Contains(text, "remarcar"),
					strings.Contains(text, "terça") && strings.HasPrefix(o.Description, "Ter"),
					strings.Contains(text, "segunda") && strings.HasPrefix(o.Description, "Seg"),
					strings.Contains(text, "16h") && strings.HasSuffix(o.Description, "16h"),
					strings.Contains(text, "8h") && strings.HasSuffix(o.Description, "8h"):
					pick = o.Key
				}
			}
			for _, o := range q.Options {
				probs[o.Key] = 0.01
			}
			probs[pick] = 1 - 0.01*float64(len(q.Options)-1)
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

// copyExample copies testdata/agenda so a test can plant defects.
func copyExample(t *testing.T) string { return copyNamed(t, "agenda") }

// copyNamed copies testdata/<name>.
func copyNamed(t *testing.T, name string) string {
	t.Helper()
	dst := t.TempDir()
	src := "../../testdata/" + name
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dst, filepath.Dir(rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func edit(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, new, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string) map[string]Result {
	t.Helper()
	results, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	out := map[string]Result{}
	for _, r := range results {
		out[r.Scenario] = r
	}
	return out
}

func TestExampleSuitePasses(t *testing.T) {
	for id, r := range run(t, "../../testdata/agenda") {
		if !r.Pass {
			t.Errorf("%s: %v\n%v", id, r.Failures, r.Transcript)
		}
		if r.Duration <= 0 {
			t.Errorf("%s: duration not recorded", id)
		}
	}
}

// A runner that cannot fail proves nothing: each case plants a defect and
// checks that the scenario fails for that reason.
func TestRunnerCatchesDefects(t *testing.T) {
	cases := []struct {
		name, file, old, new, scenario, want string
	}{
		{"missing confirmation", "reschedule-appointment.yaml",
			"    confirm: \"Posso remarcar sua consulta de {{ inputs.appointment.label }} para {{ inputs.newSlot.label }}? Responda sim para confirmar.\"\n", "",
			"happy-path", "confirm-before-irreversible"},
		{"sandbox bug", "evals/agenda.sandbox.star", "a[\"slotId\"] = slotId\n            if", "a[\"slotId\"] = slotId + 1\n            if",
			"happy-path", "fix the script"},
		{"wrong expected effect count", "evals/reschedule.evalsuite.yaml", "count: 1", "count: 2", "happy-path", "succeeded 1 times, want 2"},
		{"wrong turn expectation", "evals/reschedule.evalsuite.yaml", "says: [\"Pronto\"]", "says: [\"Até logo\"]", "happy-path", "no message says \"Até logo\""},
		{"budget", "evals/reschedule.evalsuite.yaml", "decideCalls: 8", "decideCalls: 2", "happy-path", "Decide calls, at most 2"},
		{"milestone not recorded", "reschedule-appointment.yaml", "    milestone: { name: remarcou, subject: paciente }", "", "happy-path", "milestone paciente/remarcou recorded 0 times, want 1"},
		{"plugin without a sandbox", "evals/reschedule.evalsuite.yaml", "    - plugin: agenda\n      script: agenda.sandbox.star\n",
			"    - plugin: agenda\n      script: other.star\n", "happy-path", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := copyExample(t)
			edit(t, filepath.Join(dir, c.file), c.old, c.new)
			if c.want == "" { // the suite itself must be refused
				if _, _, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1}); err == nil {
					t.Error("a suite pointing at a missing sandbox script must not run")
				}
				return
			}
			r := run(t, dir)[c.scenario]
			if r.Pass {
				t.Fatalf("the defect went unnoticed:\n%v", r.Transcript)
			}
			if !strings.Contains(strings.Join(r.Failures, "\n"), c.want) {
				t.Errorf("want a failure about %q, got %v", c.want, r.Failures)
			}
		})
	}
}

func TestNoRealSystemIsReached(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/agenda.sandbox.star"), "def reschedule(", "def not_reschedule(")
	r := run(t, dir)["happy-path"]
	if r.Pass || !strings.Contains(strings.Join(r.Failures, "\n"), "has no sandbox") {
		t.Errorf("an action without a sandbox must fail the scenario, not call anything: %v", r.Failures)
	}
}

// Review 2026-09-23: one confirmation, or any human answer, counted as
// approval for every later effect.
func TestConfirmBaseIsPerEffect(t *testing.T) {
	for site, want := range map[string]string{
		"action":                        "confirm",
		"action/onError[1]/attempt1":    "confirm",
		"action/verify/native":          "confirm",
		"steps.pay":                     "steps.pay/confirm",
		"steps.pay/onError[0]/attempt2": "steps.pay/confirm",
		"steps.plan/steps.pay":          "steps.plan/steps.pay/confirm",
		"inputs.x/said/action":          "inputs.x/said/confirm",
	} {
		if got := confirmBase(site); got != want {
			t.Errorf("confirmBase(%q) = %q, want %q", site, got, want)
		}
	}
}

// A scenario can change a sandbox's state (an outage, an empty agenda)
// without changing the suite's: here the patient has no appointments, and
// the other scenarios still see theirs.
func TestScenarioState(t *testing.T) {
	dir := copyExample(t)
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "  scenarios:\n", `  scenarios:
    - id: nothing-booked
      caller: { role: patient, patientId: p1, appointmentIds: [] }
      state: { agenda: { appointments: [] } }
      simulator: scripted
      turns:
        - user: "oi, preciso remarcar minha consulta"
          expect: { routed: reschedule-appointment, says: ["Não encontrei consultas futuras"] }
`)
	results := run(t, dir)
	for _, id := range []string{"nothing-booked", "happy-path"} {
		if r := results[id]; !r.Pass {
			t.Errorf("%s: %v\n%v", id, r.Failures, r.Transcript)
		}
	}
	edit(t, filepath.Join(dir, "evals/reschedule.evalsuite.yaml"), "state: { agenda:", "state: { agendap:")
	if _, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1}); err == nil {
		t.Errorf("state for a plugin without a sandbox must be refused: %v", issues)
	}
}

// An effect expectation can check what was sent: only calls whose
// arguments meet where count, and a miss names what was sent.
func TestEffectWhere(t *testing.T) {
	dir := copyExample(t)
	suite := filepath.Join(dir, "evals/reschedule.evalsuite.yaml")
	edit(t, suite, "effects: [ { action: agenda/reschedule, count: 1 } ]",
		`effects: [ { action: agenda/reschedule, count: 1, where: "inputs.appointmentId == 'a2' && inputs.slotId == 's2'" } ]`)
	if r := run(t, dir)["happy-path"]; !r.Pass {
		t.Fatalf("the right arguments must pass: %v", r.Failures)
	}
	edit(t, suite, "inputs.slotId == 's2'", "inputs.slotId == 's1'")
	r := run(t, dir)["happy-path"]
	if r.Pass || !strings.Contains(strings.Join(r.Failures, "\n"), "succeeded 0 times where") || !strings.Contains(strings.Join(r.Failures, "\n"), "s2") {
		t.Errorf("wrong arguments must fail and show what was sent: %v", r.Failures)
	}
	edit(t, suite, "inputs.slotId == 's1'", "caller.patientId == 'p1'")
	if _, issues, err := Run(context.Background(), Options{Paths: []string{dir}, Decider: keywords{}, MaxScenarioCostUSD: 1}); err == nil || !strings.Contains(fmt.Sprint(issues), "SCOPE_NOT_ALLOWED") {
		t.Errorf("where may only read the call's arguments: %v %v", err, issues)
	}
}

// The SDR suite catches the channel's rules going missing: without the
// window, free text reaches a lead who went silent; without the consent
// rule, a lead without opt-in is messaged.
func TestSDRSuiteCatchesChannelDefects(t *testing.T) {
	for id, r := range run(t, "../../testdata/cadencia") {
		if !r.Pass {
			t.Fatalf("%s: %v", id, r.Failures)
		}
	}
	for _, c := range []struct{ name, old, scenario, want string }{
		{"window removed", "    window: 24h                                   # outside it, only approved templates\n", "silence-then-templates", "conseguiu ver"},
		{"consent removed", "    consent: \"has(event.payload) && has(event.payload.optIn) && event.payload.optIn == true\"\n", "no-consent", "whatsapp/send-template"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := copyNamed(t, "cadencia")
			edit(t, filepath.Join(dir, "plugins.yaml"), c.old, "")
			r := run(t, dir)[c.scenario]
			if r.Pass {
				t.Fatalf("the defect went unnoticed:\n%v", r.Transcript)
			}
			if !strings.Contains(strings.Join(r.Failures, "\n"), c.want) {
				t.Errorf("want a failure about %q, got %v", c.want, r.Failures)
			}
		})
	}
}
