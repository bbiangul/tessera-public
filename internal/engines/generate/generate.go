// Package generate implements the Generate engine: text, or an object that
// must match a JSON Schema (RFC-0001, "Os quatro motores").
//
// A model's output is never trusted. When a schema is given, the reply is
// parsed and validated locally, whatever the provider claims about
// structured outputs; a reply that does not validate is sent back once with
// the validation error, and then refused.
package generate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bbiangul/tessera/internal/httpx"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/bbiangul/tessera/internal/ledger"
)

// Request asks for text or an object.
type Request struct {
	// Instructions say what to produce.
	Instructions string `json:"instructions"`
	// State is the input, already filtered and masked by the caller.
	State any `json:"state,omitempty"`
	// Schema, when set, is a JSON Schema the output must match. The output
	// is then Response.Value instead of Response.Text.
	Schema map[string]any `json:"schema,omitempty"`
	// MaxTokens bounds the reply; default 1024.
	MaxTokens int `json:"maxTokens,omitempty"`
}

// Usage reports what a call consumed, across repair attempts.
type Usage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
	// UnreportedCalls counts attempts that may have been billed without a
	// usage report (a gateway failure before the answer that counted); the
	// ledger charges each at the estimate (#7).
	UnreportedCalls int `json:"unreportedCalls,omitempty"`
}

// Response is a Generate result.
type Response struct {
	Text string `json:"text,omitempty"`
	// Value is the decoded, validated object when a schema was given.
	// Integers are int64, other numbers float64.
	Value    any           `json:"value,omitempty"`
	Model    string        `json:"model"`
	Provider string        `json:"provider,omitempty"`
	Usage    Usage         `json:"usage"`
	Latency  time.Duration `json:"latency"`
	// Attempts is 1, or 2 when the first reply had to be repaired.
	Attempts int `json:"attempts"`
}

// Generator is the Generate engine contract. Like Decide, a call that fails
// after the provider billed it returns the Response with its Usage together
// with the error.
type Generator interface {
	Generate(ctx context.Context, req Request) (*Response, error)
	Name() string
}

// ErrInvalidOutput means the reply did not match the schema, even after
// the repair attempt.
var ErrInvalidOutput = errors.New("output does not match the schema")

// ErrInvalidRequest wraps request validation failures.
var ErrInvalidRequest = errors.New("invalid generate request")

// Validate checks a request.
func (r Request) Validate() error {
	if strings.TrimSpace(r.Instructions) == "" {
		return fmt.Errorf("%w: no instructions", ErrInvalidRequest)
	}
	if r.Schema != nil {
		if _, err := compileSchema(r.Schema); err != nil {
			return fmt.Errorf("%w: schema: %v", ErrInvalidRequest, err)
		}
	}
	return nil
}

// schemaCache avoids recompiling the same schema on every call.
var schemaCache sync.Map // canonical JSON -> *jsonschema.Schema

func compileSchema(s map[string]any) (*jsonschema.Schema, error) {
	key, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if v, ok := schemaCache.Load(string(key)); ok {
		return v.(*jsonschema.Schema), nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(key))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	// A definition's schema is self-contained: a $ref to a file or a URL
	// would make its meaning depend on the host, and would let a schema
	// probe which files exist.
	c.UseLoader(refuseLoader{})
	if err := c.AddResource("mem://output.json", doc); err != nil {
		return nil, err
	}
	compiled, err := c.Compile("mem://output.json")
	if err != nil {
		return nil, err
	}
	schemaCache.Store(string(key), compiled)
	return compiled, nil
}

type refuseLoader struct{}

func (refuseLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema references are not allowed (%s); keep the schema in one document", url)
}

// ParseObject extracts a JSON value from a model reply (optionally inside a
// ```json fence) and validates it. The error says what is wrong in terms a
// model can fix; it quotes schema locations, not the reply.
func ParseObject(reply string, schema map[string]any) (any, error) {
	text := strings.TrimSpace(reply)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		if i := strings.LastIndex(text, "```"); i >= 0 {
			text = text[:i]
		}
		text = strings.TrimSpace(text)
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	var raw any
	if err := d.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: not JSON", ErrInvalidOutput)
	}
	if d.More() {
		return nil, fmt.Errorf("%w: text after the JSON value", ErrInvalidOutput)
	}
	compiled, err := compileSchema(schema)
	if err != nil {
		return nil, err
	}
	if err := compiled.Validate(raw); err != nil { // json.Number keeps precision
		return nil, fmt.Errorf("%w: %s", ErrInvalidOutput, validationSummary(err))
	}
	return plain(raw), nil
}

// plain converts json.Number to int64 or float64.
func plain(v any) any {
	switch v := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return i
		}
		f, _ := v.Float64()
		return f
	case []any:
		for i := range v {
			v[i] = plain(v[i])
		}
		return v
	case map[string]any:
		for k := range v {
			v[k] = plain(v[k])
		}
		return v
	}
	return v
}

// validationSummary lists failing locations and keywords, without values.
func validationSummary(err error) string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return "invalid"
	}
	var parts []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			loc := "/" + strings.Join(e.InstanceLocation, "/")
			kw := "invalid"
			if e.ErrorKind != nil {
				kw = strings.Join(e.ErrorKind.KeywordPath(), "/")
			}
			parts = append(parts, loc+": "+kw)
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(parts) > 8 {
		parts = append(parts[:8], fmt.Sprintf("and %d more", len(parts)-8))
	}
	return strings.Join(parts, "; ")
}

// Metered charges every call to a ledger, like decide.Metered.
type Metered struct {
	Inner  Generator
	Ledger *ledger.Ledger
	// EstimateUSD is reserved before the first call; default $0.01.
	EstimateUSD float64

	mu      sync.Mutex
	maxSeen float64
}

func (m *Metered) Name() string { return m.Inner.Name() }

func (m *Metered) Generate(ctx context.Context, req Request) (*Response, error) {
	m.mu.Lock()
	est := m.EstimateUSD
	if est <= 0 {
		est = 0.01
	}
	est = max(est, m.maxSeen)
	m.mu.Unlock()
	res, err := m.Ledger.Reserve("generate", est)
	if err != nil {
		return nil, err
	}
	resp, callErr := m.Inner.Generate(ctx, req)
	e := ledger.Entry{Backend: m.Inner.Name(), Err: callErr != nil}
	if resp != nil {
		e.Model, e.Provider, e.Latency = resp.Model, resp.Provider, resp.Latency
		e.InputTokens, e.OutputTokens, e.CostUSD = resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CostUSD
	}
	e.CostUSD = charge(e.CostUSD, est, resp, callErr)
	settleErr := res.Settle(e)
	if settleErr == nil {
		m.mu.Lock()
		m.maxSeen = max(m.maxSeen, e.CostUSD)
		m.mu.Unlock()
	}
	if callErr != nil {
		return resp, callErr
	}
	return resp, settleErr
}

// charge is what a call costs the ledger: the reported cost, plus the
// estimate for every attempt that may have been billed without a report.
// A failure httpx can describe is charged by its attempts: nothing for a
// refusal or a request never sent, the estimate for each request sent and
// not answered (#7). Any other failure is charged at least the estimate.
func charge(reported, est float64, resp *Response, callErr error) float64 {
	cost := reported
	if resp != nil {
		cost += est * float64(resp.Usage.UnreportedCalls)
	}
	if callErr == nil {
		return cost
	}
	if n, known := httpx.MaybeBilled(callErr); known {
		return cost + est*float64(n)
	}
	return max(cost, est)
}
