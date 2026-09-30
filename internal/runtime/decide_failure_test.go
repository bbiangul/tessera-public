package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type faultDecider struct {
	response *decide.Response
	err      error
}

func (d faultDecider) Name() string { return "fault" }
func (d faultDecider) Decide(context.Context, decide.Request) (*decide.Response, error) {
	return d.response, d.err
}

func TestSingleDecisionRejectsInvalidResponses(t *testing.T) {
	for _, policy := range []string{"continue", "fail", "human"} {
		for _, tc := range []struct {
			name     string
			response *decide.Response
			err      error
		}{
			{name: "nil"},
			{name: "missing", response: &decide.Response{Answers: map[string]decide.Answer{}}},
			{name: "inconsistent", response: &decide.Response{Answers: map[string]decide.Answer{"q": {Kind: decide.KindNoul, Noul: 1, Probabilities: map[string]float64{"true": .2, "false": .8}}}}},
			{name: "provider", err: errors.New("private provider detail")},
		} {
			t.Run(policy+"/"+tc.name, func(t *testing.T) {
				// Recover only to report a nil-response panic as a regression failure.
				defer func() {
					if v := recover(); v != nil {
						t.Errorf("decision panicked: %v", v)
					}
				}()
				h, b := decisionFailureHarness(t, policy)
				h.rt.Decider = faultDecider{tc.response, tc.err}
				res := h.startFn("book-flight", "urgente")
				expectStatus(t, res, journal.StatusFailed)
				if b.books != 0 || strings.Contains(strings.Join(h.user, " "), "Reservo") || strings.Contains(strings.Join(h.user, " "), "priorizar") {
					t.Fatalf("continued after invalid decision: books=%d messages=%v", b.books, h.user)
				}
				if strings.Contains(res.Error, "private provider") {
					t.Fatal("leaked provider detail")
				}
			})
		}
	}
}

func decisionFailureHarness(t *testing.T, policy string) (*harness, *booking) {
	t.Helper()
	script := strings.Replace(stepsFunction, "onLowConfidence: continue", "onLowConfidence: "+policy, 1)
	dir := t.TempDir()
	for name, body := range map[string]string{"plugin.yaml": stepsPlugin, "fn.yaml": script, "rank.star": stepsScript} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, dir)
	b := &booking{}
	h.rt.Actor = b.actor()
	return h, b
}

func TestSingleDecisionPreservesUncertaintyPolicy(t *testing.T) {
	for _, tc := range []struct {
		policy string
		want   journal.Status
	}{
		{"continue", journal.StatusConfirming},
		{"fail", journal.StatusFailed},
		{"human", journal.StatusWaitingForHuman},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			h, _ := decisionFailureHarness(t, tc.policy)
			h.rt.Decider = &yesDecider{}
			expectStatus(t, h.startFn("book-flight", "talvez viaje"), tc.want)
		})
	}
}
