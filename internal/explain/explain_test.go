package explain

import (
	"strings"
	"testing"
)

func TestExplain(t *testing.T) {
	cases := []struct {
		query, typ, syntax, desc string
		fields                   []string
	}{
		{"Function", "object", "", "capability of an agent", []string{"apiVersion", "kind", "metadata", "spec"}},
		{"Function.spec.action.idempotencyKey", "expression", "CEL", "identifying the effect", nil},
		{"Function.spec.output.template", "template", "{{ expr }}", "is the message", nil},
		{"Function.spec.inputs", "map of Input", "", "Inputs the function needs", []string{"onUnresolved", "resolve", "type"}},
		{"Function.spec.inputs.resolve", "list of Source", "", "tried in order", []string{"ask", "fromAction", "id", "value", "when", "with"}},
		{"Function.spec.inputs.newSlot.resolve.ask", "Ask", "", "asks the user", []string{"choices", "minConfidence", "onLowConfidence", "text"}},
		{"Function.spec.onError.retry.attempts", "integer", "", "extra attempts", nil},
		{"PluginManifest.spec.actions.effect", "string", "", "The manifest is the\nauthority", nil},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			f, err := Explain(c.query)
			if err != nil {
				t.Fatal(err)
			}
			if f.Type != c.typ {
				t.Errorf("type = %q, want %q", f.Type, c.typ)
			}
			if !strings.Contains(f.Syntax, c.syntax) {
				t.Errorf("syntax = %q, want it to mention %q", f.Syntax, c.syntax)
			}
			if !strings.Contains(f.Description, c.desc) {
				t.Errorf("description = %q, want it to contain %q", f.Description, c.desc)
			}
			for _, want := range c.fields {
				found := false
				for _, got := range f.Fields {
					found = found || got.Name == want
				}
				if !found {
					t.Errorf("missing sub-field %q in %+v", want, f.Fields)
				}
			}
			if !strings.Contains(f.Text(), "KIND:  "+f.Kind) {
				t.Error("text output malformed")
			}
		})
	}
}

func TestExplainEnumAndRequired(t *testing.T) {
	f, err := Explain("Function.spec.inputs.resolve.ask.onLowConfidence")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Enum) != 3 || !strings.Contains(f.Text(), "VALUES: ask-user, escalate-model, human") {
		t.Errorf("enum = %v\n%s", f.Enum, f.Text())
	}
	spec, _ := Explain("Function.spec")
	for _, c := range spec.Fields {
		if c.Name == "description" && !c.Required {
			t.Error("description must be marked required")
		}
		if c.Name == "action" && c.Required {
			t.Error("action must be optional")
		}
	}
}

func TestExplainErrors(t *testing.T) {
	if _, err := Explain("Gadget"); err == nil || !strings.Contains(err.Error(), "Function") {
		t.Errorf("unknown kind should list the known ones, got %v", err)
	}
	if _, err := Explain("Function.spec.nope"); err == nil || !strings.Contains(err.Error(), "fields: action") {
		t.Errorf("unknown field should list the fields, got %v", err)
	}
}
