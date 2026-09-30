package bench

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/generate"
)

func TestCompareFields(t *testing.T) {
	gold := map[string]any{
		"destination":  "São Paulo",
		"origin":       nil,
		"departDate":   map[string]any{"$any": []any{"2026-09-25", nil}},
		"adults":       2.0,
		"childrenAges": []any{5.0, 0.0},
	}
	right := map[string]any{"destination": " sao paulo ", "origin": nil, "departDate": nil, "adults": int64(2), "childrenAges": []any{int64(0), int64(5)}}
	if w := CompareFields(gold, right); len(w) != 0 {
		t.Errorf("accents, case, $any, int vs float and order must not matter: %v", w)
	}
	wrong := map[string]any{"destination": "Paulo", "origin": "", "departDate": "2026-09-26", "adults": 2.5, "childrenAges": []any{int64(5)}}
	if w := CompareFields(gold, wrong); !reflect.DeepEqual(w, []string{"adults", "childrenAges", "departDate", "destination", "origin"}) {
		t.Errorf("wrong = %v", w)
	}
	if w := CompareFields(gold, map[string]any{}); len(w) != 3 {
		t.Errorf("missing fields count as wrong unless gold is null: %v", w)
	}
}

type fakeGen struct{ values map[string]any }

func (f *fakeGen) Name() string { return "fake" }
func (f *fakeGen) Generate(_ context.Context, r generate.Request) (*generate.Response, error) {
	msg := r.State.(map[string]any)["message"].(string)
	v, ok := f.values[msg]
	if !ok {
		return &generate.Response{Attempts: 2, Usage: generate.Usage{CostUSD: 0.001}}, errors.New("output does not match the schema")
	}
	return &generate.Response{Value: v, Attempts: 1, Model: "m", Provider: "p", Usage: generate.Usage{CostUSD: 0.001}}, nil
}

func TestRunAndSummarizeExtract(t *testing.T) {
	cases := []ExtractCase{
		{ID: "a", State: map[string]any{"message": "a"}, Gold: map[string]any{"x": "1", "y": 2.0}},
		{ID: "b", State: map[string]any{"message": "b"}, Gold: map[string]any{"x": "2", "y": 3.0}},
		{ID: "c", State: map[string]any{"message": "c"}, Gold: map[string]any{"x": "3", "y": 4.0}},
	}
	g := &fakeGen{values: map[string]any{
		"a": map[string]any{"x": "1", "y": int64(2)},
		"b": map[string]any{"x": "2", "y": int64(9)},
	}}
	trials := RunExtract(context.Background(), ExtractTask{Instructions: "x"}, cases, []GenTarget{{Label: "t", Generator: g}}, 2, 3)
	gold := map[string]map[string]any{"a": cases[0].Gold, "b": cases[1].Gold, "c": cases[2].Gold}
	s := SummarizeExtract("t", trials, gold)
	if s.Trials != 6 || s.Errors != 2 || s.Repaired != 2 {
		t.Errorf("summary %+v", s)
	}
	if s.RecordAcc != 2.0/6 || s.FieldAcc["x"] != 4.0/6 || s.FieldAcc["y"] != 2.0/6 {
		t.Errorf("accuracy %v, fields %v", s.RecordAcc, s.FieldAcc)
	}
	if !reflect.DeepEqual(s.FailingCases, []string{"b", "c"}) || s.CostPerCase != 0.001 {
		t.Errorf("failing %v, cost %v", s.FailingCases, s.CostPerCase)
	}
}
