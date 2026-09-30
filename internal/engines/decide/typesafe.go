package decide

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bbiangul/tessera/internal/httpx"
)

// TypeSafe is the Decide backend for TypeSafe's System One API (Jev).
// API reference: https://docs.typesafe.ai/api
type TypeSafe struct {
	APIKey  string
	Model   string // e.g. "jev-1.13.0"; pin a version in production
	BaseURL string // default https://api.typesafe.ai
	// PricePerInputToken in USD; output tokens are free. Default 0.042/MTok.
	PricePerInputToken float64
	Client             *httpx.Client
}

const typesafeDefaultPrice = 0.042e-6

func (t *TypeSafe) Name() string { return "typesafe" }

type tsQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type tsAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Noul          *float64           `json:"noul"`
	Score         *float64           `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
}

type tsResponse struct {
	Model   string              `json:"model"`
	Answers map[string]tsAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// orderedCriteria marshals choice options as a JSON object in option order.
// TypeSafe's criteria is a map; Go maps would lose the order the author wrote.
type orderedCriteria []Option

func (o orderedCriteria) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, opt := range o {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendQuote(buf, opt.Key)
		buf = append(buf, ':')
		if opt.Description == "" {
			buf = append(buf, "null"...)
		} else {
			buf = strconv.AppendQuote(buf, opt.Description)
		}
	}
	return append(buf, '}'), nil
}

func toTypeSafe(q Question) tsQuestion {
	out := tsQuestion{Type: string(q.Kind), Instructions: q.Instructions}
	switch q.Kind {
	case KindChoice:
		out.Criteria = orderedCriteria(q.Options)
	case KindNoul:
		if q.IfTrue != "" || q.IfFalse != "" {
			c := map[string]string{}
			if q.IfTrue != "" {
				c["true"] = q.IfTrue
			}
			if q.IfFalse != "" {
				c["false"] = q.IfFalse
			}
			out.Criteria = c
		}
	case KindScore:
		out.Criteria = q.Levels
	}
	return out
}

// Decide sends all questions in one request; TypeSafe evaluates them in parallel.
func (t *TypeSafe) Decide(ctx context.Context, req Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if t.APIKey == "" {
		return nil, errors.New("typesafe: API key is empty")
	}
	base, model := t.BaseURL, t.Model
	if base == "" {
		base = "https://api.typesafe.ai"
	}
	if model == "" {
		model = "jev-latest"
	}
	price := t.PricePerInputToken
	if price == 0 {
		price = typesafeDefaultPrice
	}
	client := t.Client
	if client == nil {
		client = &httpx.Client{}
	}

	qs := make(map[string]tsQuestion, len(req.Questions))
	for id, q := range req.Questions {
		qs[id] = toTypeSafe(q)
	}
	body := map[string]any{"state": req.State, "model": model, "questions": qs}

	var out tsResponse
	start := time.Now()
	st, err := client.PostJSON(ctx, base+"/v1/systemone",
		map[string]string{"Authorization": "Bearer " + t.APIKey}, body, &out)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("typesafe: %w", err)
	}

	resp := &Response{
		Answers: make(map[string]Answer, len(req.Questions)),
		Backend: t.Name(),
		Model:   out.Model,
		Latency: latency,
		Usage: Usage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
			CostUSD:      float64(out.Usage.InputTokens) * price,
			// Earlier attempts that may have been billed (#7).
			UnreportedCalls: st.MaybeBilled,
		},
	}
	for id, q := range req.Questions {
		raw, ok := out.Answers[id]
		if !ok {
			resp.Answers = nil
			return resp, fmt.Errorf("typesafe: missing answer for %q", id)
		}
		a, err := fromTypeSafe(q, raw)
		if err != nil {
			resp.Answers = nil
			return resp, fmt.Errorf("typesafe: answer %q: %w", id, err) // resp carries the billed usage
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

// fromTypeSafe converts and checks an answer. A malformed answer is an error,
// never silently patched: the whole point of this backend is type safety.
func fromTypeSafe(q Question, raw tsAnswer) (Answer, error) {
	if raw.Type != string(q.Kind) {
		return Answer{}, fmt.Errorf("type %q, want %q", raw.Type, q.Kind)
	}
	var probs map[string]float64
	switch q.Kind {
	case KindNoul:
		if raw.Noul == nil {
			return Answer{}, errors.New("noul value missing")
		}
		p := *raw.Noul
		if p < 0 || p > 1 {
			return Answer{}, fmt.Errorf("noul %v out of [0,1]", p)
		}
		probs = map[string]float64{"true": p, "false": 1 - p}
	default:
		var ok bool
		probs, ok = normalize(raw.Probabilities, q.Outcomes())
		if !ok {
			return Answer{}, errors.New("no probability mass on known outcomes")
		}
	}
	a := answerFromProbs(q, probs, ProbNative)
	if q.Kind == KindChoice && raw.Choice != "" && raw.Choice != a.Choice && probs[raw.Choice] < probs[a.Choice] {
		return Answer{}, fmt.Errorf("choice %q is not the most probable option", raw.Choice)
	}
	if q.Kind == KindScore && raw.Score != nil {
		a.Score = *raw.Score
	}
	return a, nil
}
