package runtime

import (
	"fmt"
	"math"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/expr"
)

type batchStep struct {
	state     *expr.Value
	questions map[string]decide.Question
	min       float64
	onLow     string
}

func compileBatch(c *compiler, b *agents.DecideBatchStep, path string) *batchStep {
	out := &batchStep{state: c.value(b.State, path+".state"), questions: map[string]decide.Question{}, min: b.MinConfidence, onLow: b.OnLowConfidence}
	for _, id := range sortedKeys(b.Questions) {
		q := b.Questions[id]
		out.questions[id] = compileDecide(c, &agents.DecideStep{Instructions: q.Instructions, Choice: q.Choice, YesNo: q.YesNo, Score: q.Score}, path+".questions."+id).question
	}
	return out
}

func (e *execution) runBatchStep(f *frame, s *step, site string) (any, error) {
	b := s.batch
	state, err := b.state.Resolve(e.ctx, e.activation(f, nil))
	if err != nil {
		return nil, e.evalFailure(f, site+".state", err)
	}
	v, err := e.once(site, func() (any, error) {
		resp, err := e.rt.Decider.Decide(e.ctx, decide.Request{State: e.rt.Masker.Mask(state), Questions: b.questions})
		if err != nil || resp == nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return map[string]any{"error": "batch decision failed"}, nil
		}
		answers := map[string]any{}
		for _, id := range sortedKeys(b.questions) {
			q := b.questions[id]
			a, ok := resp.Answers[id]
			if !ok || !validDecisionAnswer(q, a) {
				return map[string]any{"error": fmt.Sprintf("batch answer %s missing or malformed", id)}, nil
			}
			answers[id] = answerValue(q, a, resp.Model)
		}
		return map[string]any{"answers": answers}, nil
	})
	if err != nil {
		return nil, err
	}
	recorded, _ := v.(map[string]any)
	if reason, ok := recorded["error"].(string); ok {
		return e.stepFailed(f, s, site, reason)
	}
	raw, _ := recorded["answers"].(map[string]any)
	answers, values := map[string]any{}, map[string]any{}
	low := false
	for _, id := range sortedKeys(b.questions) {
		a, _ := raw[id].(map[string]any)
		copy := map[string]any{}
		for k, v := range a {
			copy[k] = v
		}
		p, _ := copy["probability"].(float64)
		if p < b.min {
			low = true
			copy["value"] = nil
		}
		answers[id] = copy
		values[id] = copy["value"]
	}
	if low && b.onLow == "fail" {
		return e.stepFailed(f, s, site, "a batch answer was below its confidence threshold")
	}
	var value any = values
	if low {
		value = nil
	}
	return map[string]any{"answers": answers, "value": value}, nil
}

// validDecisionAnswer rejects malformed provider data rather than granting a default.
func validDecisionAnswer(q decide.Question, a decide.Answer) bool {
	if a.Kind != q.Kind {
		return false
	}
	sum := 0.0
	for _, p := range a.Probabilities {
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return false
		}
		sum += p
	}
	if math.Abs(sum-1) > 1e-6 {
		return false
	}
	checked, err := decide.AnswerFor(q, a.Probabilities, a.Source)
	if err != nil {
		return false
	}
	switch q.Kind {
	case decide.KindNoul:
		return !math.IsNaN(a.Noul) && math.Abs(checked.Noul-a.Noul) <= 1e-6
	case decide.KindChoice:
		return checked.Choice == a.Choice
	default:
		return true // score output uses the validated distribution's most likely level
	}
}
