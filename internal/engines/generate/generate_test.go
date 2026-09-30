package generate

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bbiangul/tessera/internal/cassette"
	"github.com/bbiangul/tessera/internal/httpx"
	"github.com/bbiangul/tessera/internal/ledger"
)

var tripSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"destination", "adults"},
	"properties": map[string]any{
		"destination": map[string]any{"type": "string", "minLength": 1},
		"adults":      map[string]any{"type": "integer", "minimum": 1},
		"departDate":  map[string]any{"type": []any{"string", "null"}, "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
	},
}

func TestParseObject(t *testing.T) {
	v, err := ParseObject("```json\n{\"destination\": \"Lisboa\", \"adults\": 2, \"departDate\": null}\n```", tripSchema)
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["adults"] != int64(2) || m["destination"] != "Lisboa" || m["departDate"] != nil {
		t.Errorf("value %#v", m)
	}
	for reply, want := range map[string]string{
		"Claro! Aqui está.":                                  "not JSON",
		`{"destination": "Lisboa", "adults": 0}`:             "/adults: minimum",
		`{"destination": "Lisboa", "adults": 1, "x": 1}`:     "additionalProperties",
		`{"destination": "Lisboa", "adults": 1.5}`:           "/adults: type",
		`{"destination": "Lisboa", "adults": 1} {"more": 1}`: "text after",
		`{"adults": 1, "departDate": "amanhã"}`:              "required",
	} {
		_, err := ParseObject(reply, tripSchema)
		if !errors.Is(err, ErrInvalidOutput) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", reply, want, err)
		}
		if err != nil && strings.Contains(err.Error(), "Lisboa") {
			t.Errorf("errors must not quote the reply: %v", err)
		}
	}
}

// fakeRouter replies with the given contents in order.
func fakeRouter(t *testing.T, replies ...string) (*httptest.Server, *int32, *[]map[string]any) {
	t.Helper()
	var n int32
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		i := atomic.AddInt32(&n, 1) - 1
		reply := replies[min(int(i), len(replies)-1)]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "deepseek/deepseek-v4.1-flash", "provider": "DeepSeek",
			"choices": []any{map[string]any{"message": map[string]any{"content": reply}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20, "cost": 0.0001},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &n, &bodies
}

func TestOpenRouterRepairsOnce(t *testing.T) {
	srv, calls, bodies := fakeRouter(t, `{"destination": "Lisboa"}`, `{"destination": "Lisboa", "adults": 1}`)
	o := &OpenRouter{APIKey: "k", Model: "m", BaseURL: srv.URL, StructuredOutputs: true, Client: &httpx.Client{}}
	resp, err := o.Generate(context.Background(), Request{Instructions: "extract", State: map[string]any{"m": "quero ir a Lisboa"}, Schema: tripSchema})
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 2 || resp.Attempts != 2 || resp.Value.(map[string]any)["adults"] != int64(1) {
		t.Errorf("calls %d, resp %+v", *calls, resp)
	}
	if resp.Usage.CostUSD < 0.00019 || resp.Usage.InputTokens != 200 {
		t.Errorf("usage must sum both attempts: %+v", resp.Usage)
	}
	second := (*bodies)[1]["messages"].([]any)
	if len(second) != 4 || !strings.Contains(second[3].(map[string]any)["content"].(string), "required") {
		t.Errorf("the repair must show the validation error: %v", second)
	}
	if (*bodies)[0]["response_format"] == nil || (*bodies)[0]["reasoning"] == nil {
		t.Errorf("structured outputs and reasoning off must be requested: %v", (*bodies)[0])
	}
}

func TestOpenRouterRefusesAfterTheRepair(t *testing.T) {
	srv, calls, _ := fakeRouter(t, `nope`)
	o := &OpenRouter{APIKey: "k", Model: "m", BaseURL: srv.URL, Client: &httpx.Client{}}
	resp, err := o.Generate(context.Background(), Request{Instructions: "extract", Schema: tripSchema})
	if !errors.Is(err, ErrInvalidOutput) || *calls != 2 {
		t.Fatalf("err %v after %d calls", err, *calls)
	}
	if resp == nil || resp.Usage.CostUSD == 0 {
		t.Error("a refused reply was still billed: usage must come back with the error")
	}
}

func TestOpenRouterText(t *testing.T) {
	srv, _, _ := fakeRouter(t, "Olá!")
	o := &OpenRouter{APIKey: "k", Model: "m", BaseURL: srv.URL, Client: &httpx.Client{}}
	resp, err := o.Generate(context.Background(), Request{Instructions: "greet"})
	if err != nil || resp.Text != "Olá!" || resp.Value != nil {
		t.Errorf("%+v %v", resp, err)
	}
	if _, err := o.Generate(context.Background(), Request{}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("no instructions: %v", err)
	}
	if _, err := o.Generate(context.Background(), Request{Instructions: "x", Schema: map[string]any{"type": "nonsense"}}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("invalid schema: %v", err)
	}
}

func TestMeteredChargesAndStops(t *testing.T) {
	srv, calls, _ := fakeRouter(t, "ok")
	l := ledger.New(ledger.Budget{MaxCostUSD: 0.00025})
	m := &Metered{Inner: &OpenRouter{APIKey: "k", Model: "m", BaseURL: srv.URL, Client: &httpx.Client{}}, Ledger: l, EstimateUSD: 0.0001}
	var err error
	for range 10 {
		if _, err = m.Generate(context.Background(), Request{Instructions: "x"}); err != nil {
			break
		}
	}
	if !errors.Is(err, ledger.ErrBudgetExceeded) || *calls != 2 {
		t.Errorf("err %v after %d calls; the budget allows 2", err, *calls)
	}
	if tot := l.Totals(); tot.CostUSD > 0.00025 {
		t.Errorf("overspent: %+v", tot)
	}
}

// Review 2026-09-23: a $ref could read local files.
func TestSchemaRefsCannotReachFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	_ = os.WriteFile(path, []byte(`{"type": "string"}`), 0o600)
	req := Request{Instructions: "x", Schema: map[string]any{"$ref": "file://" + path}}
	if err := req.Validate(); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("want a refusal, got %v", err)
	}
	inner := Request{Instructions: "x", Schema: map[string]any{
		"$defs": map[string]any{"name": map[string]any{"type": "string"}},
		"type":  "object", "properties": map[string]any{"n": map[string]any{"$ref": "#/$defs/name"}},
	}}
	if err := inner.Validate(); err != nil {
		t.Errorf("references inside the document still work: %v", err)
	}
}

// Review 2026-09-23: a failed call that reported no usage was charged 0.
func TestMeteredChargesFailedCallsAtLeastTheEstimate(t *testing.T) {
	srv, _, _ := fakeRouter(t, "nope")
	l := ledger.New(ledger.Budget{MaxCostUSD: 1})
	m := &Metered{Inner: &OpenRouter{APIKey: "k", Model: "m", BaseURL: srv.URL, Client: &httpx.Client{}}, Ledger: l, EstimateUSD: 0.01}
	_, err := m.Generate(context.Background(), Request{Instructions: "x", Schema: tripSchema})
	if err == nil {
		t.Fatal("want the refused reply")
	}
	if got := l.Totals().CostUSD; got < 0.01 {
		t.Errorf("charged %v, want at least the 0.01 estimate", got)
	}
}

// A replayed value has the types of the live one: an int stays an int.
// Found when a replayed "2 adultos" arrived as 2.0, failed the int input
// and fell to its default of 1.
func TestCassetteKeepsInts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	live := stub{value: map[string]any{"adults": int64(2), "ages": []any{int64(5), int64(8)}, "price": 1.5}}
	rec, err := LoadCassette(path, "t", &live, cassette.Record)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Instructions: "x", Schema: map[string]any{"type": "object"}}
	if _, err := rec.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := rec.Save(path); err != nil {
		t.Fatal(err)
	}
	rep, err := LoadCassette(path, "t", nil, cassette.Replay)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rep.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Value, live.value) {
		t.Errorf("replayed %#v, recorded %#v", got.Value, live.value)
	}
}

type stub struct{ value any }

func (s *stub) Name() string { return "stub" }
func (s *stub) Generate(context.Context, Request) (*Response, error) {
	return &Response{Value: s.value}, nil
}

// errGen fails with err, or succeeds with usage.
type errGen struct {
	err   error
	usage Usage
}

func (g errGen) Name() string { return "err" }
func (g errGen) Generate(context.Context, Request) (*Response, error) {
	return &Response{Usage: g.usage}, g.err
}

// #7: a refused request costs nothing, a request sent and not answered
// costs the estimate per attempt, and a success after a gateway failure
// adds the estimate for it. Unknown failures keep costing the estimate.
func TestMeteredChargesByWhatMayHaveBeenBilled(t *testing.T) {
	for _, c := range []struct {
		name string
		gen  errGen
		want float64
	}{
		{"refused", errGen{err: &httpx.Error{Err: errors.New("http 404"), Stats: httpx.Stats{Attempts: 1}}}, 0},
		{"sent, no answer", errGen{err: &httpx.Error{Err: errors.New("timeout"), Stats: httpx.Stats{Attempts: 1, MaybeBilled: 1}}}, 0.01},
		{"two gateway failures", errGen{err: &httpx.Error{Err: errors.New("502"), Stats: httpx.Stats{Attempts: 3, MaybeBilled: 2}}}, 0.02},
		{"success after a 502", errGen{usage: Usage{CostUSD: 0.003, UnreportedCalls: 1}}, 0.013},
		{"unknown failure", errGen{err: errors.New("empty reply")}, 0.01},
	} {
		l := ledger.New(ledger.Budget{MaxCostUSD: 1})
		m := &Metered{Inner: c.gen, Ledger: l, EstimateUSD: 0.01}
		_, _ = m.Generate(context.Background(), Request{Instructions: "x"})
		if got := l.Totals().CostUSD; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: charged %v, want %v", c.name, got, c.want)
		}
	}
}
