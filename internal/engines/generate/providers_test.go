package generate

import (
	"reflect"
	"testing"
)

func TestParseModel(t *testing.T) {
	for _, c := range []struct {
		spec, model string
		providers   []string
		defaulted   bool
	}{
		{"deepseek/deepseek-v4.1-flash", "deepseek/deepseek-v4.1-flash", []string{"DeepSeek"}, true},
		{"deepseek/deepseek-v4.1-flash@any", "deepseek/deepseek-v4.1-flash", nil, false},
		{"deepseek/deepseek-v4.1-flash@Any", "deepseek/deepseek-v4.1-flash", nil, false},
		{"deepseek/deepseek-v4.1-flash@DeepInfra,Together", "deepseek/deepseek-v4.1-flash", []string{"DeepInfra", "Together"}, false},
		{"other/model", "other/model", nil, false},
		{"other/model@X", "other/model", []string{"X"}, false},
	} {
		m, p, d := ParseModel(c.spec)
		if m != c.model || !reflect.DeepEqual(p, c.providers) || d != c.defaulted {
			t.Errorf("%s: got %s %v %v", c.spec, m, p, d)
		}
	}
	// The table is not shared with callers.
	_, p, _ := ParseModel("deepseek/deepseek-v4.1-flash")
	p[0] = "X"
	if DefaultProviders["deepseek/deepseek-v4.1-flash"][0] != "DeepSeek" {
		t.Error("ParseModel returned the table's own slice")
	}
}
