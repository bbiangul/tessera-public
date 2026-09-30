package expr

import (
	"reflect"
	"strings"
	"testing"
)

func TestValueResolvesWholeExpressionsOnly(t *testing.T) {
	e := engine(t)
	v, err := e.CompileValue(map[string]any{
		"appointmentId": "${ inputs.appointment.id }",
		"days":          "${inputs.days}",
		"template":      "apresentacao-v3", // literal, not a subtraction
		"literalNum":    int64(3),
		"nested":        []any{"${ caller.patientId }", "plain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Resolve(ctx, act())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"appointmentId": "ap-1",
		"days":          int64(7), // keeps its type
		"template":      "apresentacao-v3",
		"literalNum":    int64(3),
		"nested":        []any{"p-1", "plain"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
	if refs := v.References(); !reflect.DeepEqual(refs, []string{"caller.patientId", "inputs.appointment", "inputs.days"}) {
		t.Errorf("references = %v", refs)
	}
}

func TestValueRejectsMixedText(t *testing.T) {
	e := engine(t)
	for _, bad := range []any{
		"Olá ${ inputs.name }",
		map[string]any{"x": []any{"id-${inputs.id}"}},
		"${ a } ${ b }",
	} {
		if _, err := e.CompileValue(bad); err == nil || !strings.Contains(err.Error(), "whole value") {
			t.Errorf("%v: want mixed-text error, got %v", bad, err)
		}
	}
	if _, err := e.CompileValue(map[string]any{"k": "${ undeclared }"}); err == nil || !strings.Contains(err.Error(), "$.k") {
		t.Errorf("errors must carry the field path, got %v", err)
	}
}

func TestTemplateRenderAndEscape(t *testing.T) {
	e := engine(t)
	tpl, err := e.CompileTemplate("Tenho estes horários: {{ sources.slots.output.map(s, s.label).join(', ') }}. Qual prefere, {{ inputs.name }}?")
	if err != nil {
		t.Fatal(err)
	}
	a := act()
	a["inputs"].(map[string]any)["name"] = "*Ana*"

	plain, err := tpl.Render(ctx, a, PlainText)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "Tenho estes horários: quinta 10h, sexta 9h. Qual prefere, *Ana*?" {
		t.Errorf("plain = %q", plain)
	}
	wa, err := tpl.Render(ctx, a, WhatsApp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wa, "*"+zwsp+"Ana*"+zwsp+"?") {
		t.Errorf("WhatsApp markers in values must be neutralized: %q", wa)
	}
	if !strings.HasPrefix(wa, "Tenho estes horários:") {
		t.Errorf("author text must not be escaped: %q", wa)
	}
	if got := tpl.References(); !reflect.DeepEqual(got, []string{"inputs.name", "sources.slots"}) {
		t.Errorf("references = %v", got)
	}
}

func TestEscapers(t *testing.T) {
	if got := PlainText("a\x1b[31mb\u0085c\nd\te"); got != "a[31mbc\nd\te" {
		t.Errorf("PlainText = %q", got)
	}
	if got := HTML(`<b onclick="x">&</b>`); got != "&lt;b onclick=&#34;x&#34;&gt;&amp;&lt;/b&gt;" {
		t.Errorf("HTML = %q", got)
	}
}

func TestTemplateErrors(t *testing.T) {
	e := engine(t)
	for src, msg := range map[string]string{
		"Olá {{ inputs.name":   "unterminated",
		"Olá }} x":             "without",
		"Olá ${ inputs.name }": "not ${ }",
		"Olá {{ nope.x }}":     "undeclared",
	} {
		if _, err := e.CompileTemplate(src); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%q: want %q, got %v", src, msg, err)
		}
	}
	tpl, err := e.CompileTemplate("{{ inputs.appointment }}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Render(ctx, act(), PlainText); err == nil || !strings.Contains(err.Error(), "cannot print an object") {
		t.Errorf("printing a map must fail, got %v", err)
	}
	if _, err := tpl.Render(ctx, act(), nil); err == nil {
		t.Error("a nil escaper must fail")
	}
	literal, err := e.CompileTemplate("Sem variáveis.")
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := literal.Render(ctx, nil, PlainText); s != "Sem variáveis." {
		t.Errorf("literal template = %q", s)
	}
}
