// Package mask replaces personal data with placeholders before content
// leaves the process for a model (RFC-0001, "Privacidade e LGPD").
//
// This is the default, pattern-based masker: email addresses, phone numbers,
// Brazilian CPF and CNPJ, and payment card numbers (checked with Luhn, so
// order numbers are not masked by accident). Names and free-text health
// information need a model and are out of its reach; the Masker interface
// lets a namespace plug in a stronger one.
package mask

import (
	"regexp"
	"strings"
)

// Masker masks a value: strings are masked, maps and lists recursively,
// everything else is returned as is.
type Masker interface {
	Mask(v any) any
}

// Placeholders put in place of each kind of data.
const (
	Email = "[EMAIL]"
	Phone = "[PHONE]"
	CPF   = "[CPF]"
	CNPJ  = "[CNPJ]"
	Card  = "[CARD]"
)

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	cnpjRe  = regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)
	cpfRe   = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	// Card numbers: 13 to 19 digits, optionally grouped by spaces or dashes.
	cardRe = regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`)
	// Phones: optional +country, optional (area), 8 or 9 digit subscriber
	// number with an optional separator.
	// A Brazilian number in E.164 without the +, as WhatsApp IDs are
	// written: 55, the area code, then a mobile (9 and 8 digits) or a
	// landline (8 digits). The general phone pattern misses it when the
	// digits run together (#55); other countries' bare numbers are left
	// alone, since as bare digits they look like order numbers.
	brBareRe = regexp.MustCompile(`55[1-9][1-9](?:9\d{8}|[2-8]\d{7})`)
	phoneRe  = regexp.MustCompile(`(?:\+\d{1,3}[ .\-]?)?(?:\(?\d{2,3}\)?[ .\-]?)?9?\d{4}[ .\-]?\d{4}\b`)
)

// Patterns is the default Masker.
type Patterns struct{}

// Default is the pattern-based masker.
var Default Masker = Patterns{}

// Mask implements Masker.
func (Patterns) Mask(v any) any {
	switch v := v.(type) {
	case string:
		return Text(v)
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = Patterns{}.Mask(x)
		}
		return out
	case []string: // typed lists must not slip through unmasked
		out := make([]string, len(v))
		for i, x := range v {
			out[i] = Text(x)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, x := range v {
			out[k] = Text(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = Patterns{}.Mask(x)
		}
		return out
	}
	return v
}

// Text masks one string. Order matters: emails first (they contain digits
// that look like nothing else), then document numbers, cards, and phones
// last, since a phone pattern would also match parts of the others.
func Text(s string) string {
	s = emailRe.ReplaceAllString(s, Email)
	s = replaceIsolated(s, brBareRe, func(string) bool { return true }, Phone)
	s = cnpjRe.ReplaceAllString(s, CNPJ)
	s = cpfRe.ReplaceAllString(s, CPF)
	s = cardRe.ReplaceAllStringFunc(s, func(m string) string {
		if luhn(digits(m)) {
			return Card
		}
		return m
	})
	return replaceIsolated(s, phoneRe, func(m string) bool {
		n := len(digits(m))
		return n >= 8 && n <= 13
	}, Phone)
}

// replaceIsolated replaces matches of re that are not part of a longer run
// of digits (Go's regexp has no lookbehind) and that pass ok.
func replaceIsolated(s string, re *regexp.Regexp, ok func(string) bool, with string) string {
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		start, end := loc[0], loc[1]
		if start > 0 && isDigit(s[start-1]) || end < len(s) && isDigit(s[end]) || !ok(s[start:end]) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(with)
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func luhn(d string) bool {
	if len(d) < 13 || len(d) > 19 {
		return false
	}
	sum := 0
	double := false
	for i := len(d) - 1; i >= 0; i-- {
		n := int(d[i] - '0')
		if double {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum%10 == 0
}

// Find returns the first email, phone or cpf (kind) written in text, as
// written, or "". It is how a value the user gives on purpose (the e-mail
// for the booking) is read without a model.
func Find(kind, text string) string {
	var re *regexp.Regexp
	switch kind {
	case "email":
		re = emailRe
	case "cpf":
		re = cpfRe
	case "phone":
		if m := brBareRe.FindString(text); m != "" {
			return m
		}
		re = phoneRe
	default:
		return ""
	}
	return strings.TrimSpace(re.FindString(text))
}
