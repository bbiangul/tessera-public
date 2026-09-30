package mailmsg

import (
	"encoding/base64"
	"mime"
	"net/mail"
	"strings"
	"testing"
)

func parse(t *testing.T, raw string, err error) (*mail.Message, string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	return m, string(b)
}

// Adversarial review: line breaks of every kind in the subject add no
// header; in an address they make it not one address, and nothing is
// built.
func TestNoHeaderInjection(t *testing.T) {
	for _, inj := range []string{"\r\nBcc: e@evil.example", "\nBcc: e@evil.example", "\u0085Bcc: e@evil.example", "\u2028Bcc: e@evil.example", "\rBcc: e@evil.example"} {
		raw, err := Raw("agente@x.example", "rita@x.example", "Aprovação"+inj, "body"+inj)
		m, text := parse(t, raw, err)
		if m.Header.Get("Bcc") != "" || len(m.Header) != 6 {
			t.Errorf("injected %q:\n%s", inj, text)
		}
		if _, err := Raw("agente@x.example", "rita@x.example"+inj, "s", "b"); err == nil {
			t.Errorf("an address with %q was accepted", inj)
		}
	}
}

// ADV-9: a list, or anything that is not one address, is refused, never
// written as it came.
func TestOnlyOneAddress(t *testing.T) {
	for _, bad := range []string{"rita,attacker@x.example", "rita@x.example, attacker@x.example", "rita", ""} {
		if _, err := Raw("", bad, "s", "b"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	raw, err := Raw("Clínica <agente@x.example>", "rita@x.example", "s", "b")
	m, _ := parse(t, raw, err)
	if from, err := m.Header.AddressList("From"); err != nil || from[0].Name != "Clínica" {
		t.Errorf("From %v %v", from, err)
	}
}

// ADV-10: a long subject is folded into lines under RFC 5322's 998 bytes
// and decodes back exactly.
func TestLongSubjectIsFolded(t *testing.T) {
	subject := strings.TrimSpace(strings.Repeat("ção ", 400))
	enc, err := Raw("", "rita@x.example", subject, "b")
	m, raw := parse(t, enc, err)
	for _, l := range strings.Split(raw, "\r\n") {
		if len(l) > 998 {
			t.Fatalf("a %d-byte line", len(l))
		}
	}
	if dec, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); err != nil || dec != subject {
		t.Errorf("subject decoded %q %v", dec[:min(40, len(dec))], err)
	}
}
