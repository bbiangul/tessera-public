package expr

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Escaper makes an interpolated value safe for one channel. It is applied
// only to values, never to the text the author wrote around them.
type Escaper func(string) string

// PlainText removes characters that are invisible or change how the text
// around them is shown: control characters other than newline and tab,
// Unicode format characters (zero-width spaces and joiners, bidi overrides
// such as U+202E, tag characters, the byte-order mark), line and paragraph
// separators, and noncharacters. A value cannot reorder or hide the text of
// the message it is placed in.
func PlainText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		case unicode.Is(unicode.Cf, r), r == 0x2028, r == 0x2029:
			return -1
		case r >= 0xfdd0 && r <= 0xfdef, r&0xfffe == 0xfffe:
			return -1
		}
		return r
	}, s)
}

// zwsp is a zero-width space.
const zwsp = "\u200b"

// WhatsApp neutralizes WhatsApp formatting inside a value, so a customer's
// name cannot restyle the message: inline markers (*bold*, _italic_,
// ~strike~, `mono`, ```block```) get a zero-width space after them, and a
// line that starts with a quote (>) or a list marker (-, *, 1.) gets one in
// front. The zero-width space is added after PlainText has removed any the
// value brought itself.
func WhatsApp(s string) string {
	var b strings.Builder
	for i, line := range strings.Split(PlainText(s), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		if t := strings.TrimLeft(line, " "); strings.HasPrefix(t, ">") || strings.HasPrefix(t, "- ") ||
			strings.HasPrefix(t, "* ") || orderedItem.MatchString(t) {
			b.WriteString(zwsp)
		}
		for _, r := range line {
			b.WriteRune(r)
			switch r {
			case '*', '_', '~', '`':
				b.WriteString(zwsp)
			}
		}
	}
	return b.String()
}

var orderedItem = regexp.MustCompile(`^[0-9]+\. `)

// HTML escapes a value for HTML text and for quoted attribute values. It is
// not safe for unquoted attributes or for URLs (javascript: passes).
func HTML(s string) string { return html.EscapeString(PlainText(s)) }

// Template is text with "{{ expr }}" interpolations (RFC-0001: rule 3).
type Template struct {
	src   string
	parts []part
}

type part struct {
	lit string
	x   *Expr
}

// MaxInterpolations bounds the {{ }} in one template. Each interpolation has
// its own cost limit, so without this bound a template could multiply it.
const MaxInterpolations = 64

// CompileTemplate parses a template and compiles every interpolation. The
// parser understands CEL string literals and nested braces, so
// {{ "}}" }} and {{ {'a': {'b': 1}}.a.b }} work.
func (e *Engine) CompileTemplate(src string) (*Template, error) {
	if i := strings.Index(src, "${"); i >= 0 && !strings.HasPrefix(src[i:], "${{") {
		return nil, fmt.Errorf("template %q: use {{ }} in text, not ${ }", src)
	}
	t := &Template{src: src}
	rest := src
	interpolations := 0
	for {
		i := strings.Index(rest, "{{")
		lit := rest
		if i >= 0 {
			lit = rest[:i]
		}
		if strings.Contains(lit, "}}") {
			return nil, fmt.Errorf("template %q: \"}}\" without \"{{\"", src)
		}
		if lit != "" {
			t.parts = append(t.parts, part{lit: lit})
		}
		if i < 0 {
			return t, nil
		}
		rest = rest[i+2:]
		j, err := interpolationEnd(rest)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", src, err)
		}
		if interpolations++; interpolations > MaxInterpolations {
			return nil, fmt.Errorf("template %q: more than %d interpolations", src[:min(len(src), 40)], MaxInterpolations)
		}
		x, err := e.Compile(rest[:j], Any)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", src, err)
		}
		t.parts = append(t.parts, part{x: x})
		rest = rest[j+2:]
	}
}

// interpolationEnd finds the "}}" that closes an interpolation, skipping
// braces and "}}" inside string literals and nested maps.
func interpolationEnd(s string) (int, error) {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{':
			depth++
		case c == '}' && depth > 0:
			depth--
		case c == '}' && i+1 < len(s) && s[i+1] == '}':
			return i, nil
		}
	}
	return 0, errors.New("unterminated {{")
}

// Render evaluates the interpolations and escapes each value with esc.
func (t *Template) Render(ctx context.Context, act Activation, esc Escaper) (string, error) {
	s, _, err := t.RenderFacts(ctx, act, esc)
	return s, err
}

// RenderFacts is Render, and also the interpolated values as they appear
// in the text: the facts a rewrite of the text must keep word for word.
func (t *Template) RenderFacts(ctx context.Context, act Activation, esc Escaper) (string, []string, error) {
	if esc == nil {
		return "", nil, errors.New("render: an escaper is required; pick the channel's")
	}
	var b strings.Builder
	var facts []string
	for _, p := range t.parts {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		if p.x == nil {
			b.WriteString(p.lit)
			continue
		}
		v, err := p.x.Eval(ctx, act)
		if err != nil {
			return "", nil, err
		}
		s, err := format(v)
		if err != nil {
			return "", nil, fmt.Errorf("template %q: {{ %s }}: %w", t.src, p.x.Source(), err)
		}
		s = esc(s)
		b.WriteString(s)
		if f := strings.TrimSpace(s); f != "" {
			facts = append(facts, f)
		}
	}
	return b.String(), facts, nil
}

// References lists the scope paths read by the template.
func (t *Template) References() []string {
	set := map[string]bool{}
	for _, p := range t.parts {
		if p.x != nil {
			for _, r := range p.x.References() {
				set[r] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// format turns an evaluated value into text. Maps are refused: a template
// that prints a whole object is almost always a mistake that leaks fields.
func format(v any) (string, error) {
	switch v := v.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case time.Time:
		return v.Format(time.RFC3339), nil
	case time.Duration:
		return v.String(), nil
	case []byte:
		return "", errors.New("cannot print bytes; convert them with string()")
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			s, err := format(item)
			if err != nil {
				return "", err
			}
			items[i] = s
		}
		return strings.Join(items, ", "), nil
	case map[string]any:
		return "", errors.New("cannot print an object; pick its fields")
	}
	return fmt.Sprint(v), nil
}
