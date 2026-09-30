package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func TestServeRefusesAWeakToken(t *testing.T) {
	t.Setenv("TESSERA_API_TOKEN", "sixteen-chars-ok")
	if _, err := run(t, "serve", "../../testdata/lojas", "--namespace", "rede"); err == nil || !strings.Contains(err.Error(), "32 characters") {
		t.Fatalf("want a token error, got %v", err)
	}
}

// serve with --public-url makes the link key once in the data directory,
// and a second process on the same database is refused.
func TestServeWiring(t *testing.T) {
	t.Setenv("TESSERA_API_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("TYPESAFE_API_KEY", "unused: no model is called")
	dir := t.TempDir()
	args := []string{"serve", "../../testdata/lojas", "--namespace", "rede", "--addr", "127.0.0.1:0",
		"--db", filepath.Join(dir, "runs.db"), "--data-dir", filepath.Join(dir, "data"), "--generate", "none",
		"--public-url", "https://tessera.example"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		root := newRoot()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)
		done <- root.ExecuteContext(ctx)
	}()
	key := filepath.Join(dir, "data", "link-secret")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(key); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("no link key after 10s: %v", <-done)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info, _ := os.Stat(key); info.Mode().Perm() != 0o600 {
		t.Errorf("link key mode %v", info.Mode())
	}
	if _, err := run(t, args...); !errors.Is(err, journal.ErrLocked) {
		t.Errorf("a second process on the same database: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("shutdown: %v", err)
	}
}
