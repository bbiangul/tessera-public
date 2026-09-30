// Package tasklink signs links to a person's task (RFC-0001: approving is
// an authenticated act, bound to the task's content). A link names a run
// and the task's hash, expires, and is signed; once the task is answered
// its hash changes, so the link stops working: one use.
package tasklink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Claims are what a link carries.
type Claims struct {
	RunID string `json:"r"`
	Hash  string `json:"h"`
	Exp   int64  `json:"e"` // Unix seconds
	// Member, when set, binds the link to one Team member: its page
	// offers only them (the e-mail surface, backoffice-005).
	Member string `json:"m,omitempty"`
}

// ErrInvalid is a link that is malformed, forged or expired.
var ErrInvalid = errors.New("invalid or expired link")

// Sign makes the token of a link.
func Sign(secret []byte, c Claims) string {
	body, _ := json.Marshal(c)
	b := base64.RawURLEncoding.EncodeToString(body)
	return b + "." + mac(secret, b)
}

// Verify checks a token and returns its claims.
func Verify(secret []byte, token string, now time.Time) (Claims, error) {
	b, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(mac(secret, b))) {
		return Claims{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(b)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil || c.RunID == "" || c.Hash == "" || now.Unix() > c.Exp {
		return Claims{}, ErrInvalid
	}
	return c, nil
}

func mac(secret []byte, s string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// LoadSecret is the link-signing key: env when set (at least 32
// characters), otherwise a random key kept in file, made on first use.
// It is its own secret, never derived from the API token: every link
// carries a MAC, and a key derived from the token would let anyone
// holding a link guess the token offline.
func LoadSecret(env, file string) ([]byte, error) {
	if env != "" {
		if len(env) < 32 {
			return nil, errors.New("the task link secret must have 32 characters or more")
		}
		return []byte(env), nil
	}
	if b, err := os.ReadFile(file); err == nil {
		if len(b) < 32 {
			return nil, fmt.Errorf("%s: too short for a key", file)
		}
		return b, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	enc := []byte(hex.EncodeToString(key))
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) { // made by another process meanwhile
		return LoadSecret("", file)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(enc); err != nil {
		_ = f.Close()
		return nil, err
	}
	return enc, f.Close()
}
