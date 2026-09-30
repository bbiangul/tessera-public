package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func batchHarness(t *testing.T, transform func(string) string) (*harness, *decidetest.Scripted) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/decide-batch/function.yaml")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if transform != nil {
		body = transform(body)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "function.yaml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	d := &decidetest.Scripted{Answers: map[string]map[string]float64{
		"weekend": {"true": .95, "false": .05},
		"period":  {"morning": .9, "afternoon": .05, "unspecified": .05},
	}}
	h.rt.Decider = d
	return h, d
}

func TestDecideBatchMasksAndReplaysOneRequest(t *testing.T) {
	h, d := batchHarness(t, func(s string) string {
		return strings.Replace(s, "  output:", "    - id: pause\n      wait: { for: 1h }\n    - id: result\n      respond: { template: \"{{ string(steps.preferences.output.value.weekend) }} {{ steps.preferences.output.value.period }}\" }\n  output:", 1)
	})
	expectStatus(t, h.startFn("preferences", "sábado de manhã, email ana@example.com"), journal.StatusWaitingForTime)
	calls := d.Calls()
	if len(calls) != 1 || len(calls[0].Questions) != 2 {
		t.Fatalf("batch calls: %+v", calls)
	}
	state := calls[0].State.(map[string]any)
	if strings.Contains(state["message"].(string), "ana@example.com") {
		t.Fatal("unmasked state")
	}
	h.rt.Clock = func() time.Time { return testNow.Add(2 * time.Hour) }
	results := wakeAll(t, h)
	if len(results) != 1 {
		t.Fatal(results)
	}
	expectStatus(t, results[0], journal.StatusSucceeded)
	if len(d.Calls()) != 1 {
		t.Fatal("replayed batch called provider")
	}
	if len(h.user) != 1 || h.user[0] != "true morning" {
		t.Fatal(h.user)
	}
}

func TestDecideBatchUncertaintyAndProviderFailure(t *testing.T) {
	for _, tc := range []struct {
		name, policy  string
		providerError bool
		want          journal.Status
	}{
		{"continue", "continue", false, journal.StatusSucceeded},
		{"fail", "fail", false, journal.StatusFailed},
		{"provider-error-even-on-continue", "continue", true, journal.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, d := batchHarness(t, func(s string) string {
				s = strings.Replace(s, "onLowConfidence: continue", "onLowConfidence: "+tc.policy, 1)
				return strings.Replace(s, "  output:", "    - id: partial\n      respond: { template: \"{{ string(steps.preferences.output.answers.weekend.value == null) }} {{ steps.preferences.output.answers.period.value }}\" }\n  output:", 1)
			})
			d.Answers["weekend"] = map[string]float64{"true": .6, "false": .4}
			if tc.providerError {
				d.Err = errors.New("private provider detail")
			}
			res := h.startFn("preferences", "talvez sábado de manhã")
			expectStatus(t, res, tc.want)
			if tc.want == journal.StatusSucceeded && (len(h.user) != 2 || h.user[1] != "true morning") {
				t.Fatal(h.user)
			}
			if strings.Contains(res.Error, "private provider") {
				t.Fatal("provider detail leaked")
			}
		})
	}
}

type brokenBatchDecider struct{ answer *decide.Response }

func (d brokenBatchDecider) Name() string { return "broken" }
func (d brokenBatchDecider) Decide(context.Context, decide.Request) (*decide.Response, error) {
	return d.answer, nil
}

func TestDecideBatchRejectsMissingAnswers(t *testing.T) {
	for _, response := range []*decide.Response{nil, {Answers: map[string]decide.Answer{}}} {
		h, _ := batchHarness(t, nil)
		h.rt.Decider = brokenBatchDecider{response}
		expectStatus(t, h.startFn("preferences", "sábado"), journal.StatusFailed)
	}
}

func TestDecideBatchRejectsMalformedProbabilities(t *testing.T) {
	q := decide.Question{Kind: decide.KindNoul}
	for _, a := range []decide.Answer{
		{Kind: decide.KindNoul},
		{Kind: decide.KindNoul, Noul: 1, Probabilities: map[string]float64{"true": .2, "false": .8}},
		{Kind: decide.KindNoul, Probabilities: map[string]float64{"true": -1, "false": 2}},
		{Kind: decide.KindNoul, Probabilities: map[string]float64{"unknown": 1}},
	} {
		if validDecisionAnswer(q, a) {
			t.Fatalf("accepted %+v", a)
		}
	}
}

func TestWeekdayHelperNeverTreatsUnknownAsUnrestricted(t *testing.T) {
	for _, tc := range []struct {
		name               string
		explicit, saturday float64
		want               string
	}{
		{"explicit Saturday", .95, .95, "true true 6"},
		{"uncertain explicitness", .55, .95, "false null "},
		{"uncertain requested day", .95, .55, "false null "},
		{"confidently absent restriction", .05, .55, "true false 0,1,2,3,4,5,6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			helper, err := os.ReadFile("../../testdata/decide-batch/weekdays.function.json")
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "helper.json"), helper, 0600); err != nil {
				t.Fatal(err)
			}
			wrapper := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: wrapper, namespace: travel }
spec:
  description: Tests the public helper.
  steps:
    - id: mask
      call: weekday-preferences
      with: { message: "${ event.message }" }
    - id: tell
      respond:
        template: "{{ string(steps.mask.output.known) }} {{ steps.mask.output.restricted == null ? 'null' : string(steps.mask.output.restricted) }} {{ steps.mask.output.allowed.map(d, string(d)).join(',') }}"
`
			if err = os.WriteFile(filepath.Join(dir, "wrapper.yaml"), []byte(wrapper), 0600); err != nil {
				t.Fatal(err)
			}
			h := newHarness(t, dir)
			d := &decidetest.Scripted{Answers: map[string]map[string]float64{}}
			for _, k := range []string{"0", "1", "2", "3", "4", "5", "6", "explicit"} {
				p := .05
				if k == "explicit" {
					p = tc.explicit
				}
				if k == "6" {
					p = tc.saturday
				}
				d.Answers[k] = map[string]float64{"true": p, "false": 1 - p}
			}
			h.rt.Decider = d
			expectStatus(t, h.startFn("wrapper", "qualquer sábado"), journal.StatusSucceeded)
			if len(h.user) != 1 || h.user[0] != tc.want {
				t.Fatalf("got %q, want %q", h.user, tc.want)
			}
			if len(d.Calls()) != 1 || len(d.Calls()[0].Questions) != 8 {
				t.Fatal("not one eight-question batch")
			}
		})
	}
}
