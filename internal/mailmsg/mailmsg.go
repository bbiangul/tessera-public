// Package mailmsg builds RFC 822 messages for the APIs that take one
// whole (Gmail's raw): the headers from one-line values, the subject as a
// UTF-8 encoded word and the body in base64, so no text can add a header.
package mailmsg

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/mail"
	"strings"
)

// OneLine keeps a header-bound text on one line.
func OneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Address formats one address for a header, encoding a display name that
// is not ASCII ("Clínica <agente@clinica.example>"). Anything that is not
// exactly one address ("a@x, b@y") is refused, never written as it came.
func Address(s string) (string, error) {
	a, err := mail.ParseAddress(OneLine(s))
	if err != nil {
		return "", fmt.Errorf("not one e-mail address: %q", OneLine(s))
	}
	return a.String(), nil
}

// Raw is the RFC 822 message, base64url-encoded (Gmail's raw). The
// headers are built here from one-line values, the subject as a UTF-8
// encoded word and the body in base64, so nothing in the task can add a
// header.
func Raw(from, to, subject, text string) (string, error) {
	var b strings.Builder
	if from != "" {
		f, err := Address(from)
		if err != nil {
			return "", err
		}
		b.WriteString("From: " + f + "\r\n")
	}
	t, err := Address(to)
	if err != nil {
		return "", err
	}
	b.WriteString("To: " + t + "\r\n")
	// Long subjects are several encoded words, folded one per line (RFC
	// 5322 keeps lines under 998 bytes).
	b.WriteString("Subject: " + strings.ReplaceAll(mime.BEncoding.Encode("UTF-8", OneLine(subject)), "?= =?", "?=\r\n =?") + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	body := base64.StdEncoding.EncodeToString([]byte(text))
	for len(body) > 76 {
		b.WriteString(body[:76] + "\r\n")
		body = body[76:]
	}
	b.WriteString(body + "\r\n")
	return base64.URLEncoding.EncodeToString([]byte(b.String())), nil
}
