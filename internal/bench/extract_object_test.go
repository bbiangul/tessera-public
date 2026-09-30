package bench

import (
	"reflect"
	"testing"
)

func TestCompareFieldsNestedObjects(t *testing.T) {
	gold := map[string]any{"request": map[string]any{
		"time": "17:00", "period": "any",
		"date":     map[string]any{"day": "2026-10-08", "year": 2026.0},
		"optional": nil,
	}}
	valid := map[string]any{"time": "17:00", "period": "any",
		"date": map[string]any{"day": "2026-10-08", "year": int64(2026)}, "optional": nil}
	if wrong := CompareFields(gold, map[string]any{"request": valid}); len(wrong) != 0 {
		t.Fatalf("identical nested extracted objects must match: %v", wrong)
	}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"wrong-period", map[string]any{"time": "17:00", "period": "evening", "date": valid["date"], "optional": nil}},
		{"wrong-nested-date", map[string]any{"time": "17:00", "period": "any", "date": map[string]any{"day": "2026-10-09", "year": 2026.0}, "optional": nil}},
		{"missing-null-member", map[string]any{"time": "17:00", "period": "any", "date": valid["date"]}},
		{"extra-member", map[string]any{"time": "17:00", "period": "any", "date": valid["date"], "optional": nil, "invented": true}},
		{"wrong-type", "17:00"},
		{"null", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if wrong := CompareFields(gold, map[string]any{"request": tc.value}); !reflect.DeepEqual(wrong, []string{"request"}) {
				t.Fatalf("wrong=%v", wrong)
			}
		})
	}
}

func TestCompareFieldsObjectAlternatives(t *testing.T) {
	gold := map[string]any{"request": map[string]any{"$any": []any{nil, map[string]any{"period": map[string]any{"$any": []any{"any", "afternoon"}}}}}}
	for _, value := range []any{nil, map[string]any{"period": "any"}, map[string]any{"period": "afternoon"}} {
		if wrong := CompareFields(gold, map[string]any{"request": value}); len(wrong) != 0 {
			t.Fatalf("alternative %#v: %v", value, wrong)
		}
	}
	if wrong := CompareFields(gold, map[string]any{"request": map[string]any{"period": "evening"}}); len(wrong) != 1 {
		t.Fatalf("unexpected match: %v", wrong)
	}
}
