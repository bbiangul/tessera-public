package tasklink

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	s := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_800_000_000, 0)
	tok := Sign(s, Claims{RunID: "run-1", Hash: "abc", Exp: now.Add(time.Hour).Unix()})
	c, err := Verify(s, tok, now)
	if err != nil || c.RunID != "run-1" || c.Hash != "abc" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := Verify(s, tok, now.Add(2*time.Hour)); err == nil {
		t.Error("an expired link must be refused")
	}
	if _, err := Verify([]byte("another key, 32 characters long!"), tok, now); err == nil {
		t.Error("a link signed with another key must be refused")
	}
	body, sig, _ := strings.Cut(tok, ".")
	forged := Sign([]byte("x"), Claims{RunID: "run-2", Hash: "abc", Exp: now.Add(time.Hour).Unix()})
	fb, _, _ := strings.Cut(forged, ".")
	if _, err := Verify(s, fb+"."+sig, now); err == nil {
		t.Error("a changed body with the old signature must be refused")
	}
	for _, bad := range []string{"", "nodot", body + ".", "." + sig} {
		if _, err := Verify(s, bad, now); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestLoadSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "link-secret")
	a, err := LoadSecret("", file)
	if err != nil || len(a) < 32 {
		t.Fatalf("%q %v", a, err)
	}
	if b, err := LoadSecret("", file); err != nil || string(b) != string(a) {
		t.Fatalf("the same key after a restart: %v", err)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	if _, err := LoadSecret("short", file); err == nil {
		t.Error("a short secret is refused")
	}
	if b, _ := LoadSecret("an explicit secret of 32+ characters", file); string(b) != "an explicit secret of 32+ characters" {
		t.Error("the environment wins")
	}
}
