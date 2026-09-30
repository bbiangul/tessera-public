package cassette

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type resp struct{ Answer string }

func TestRecordSaveReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	// Another engine's line in the same file must survive a save untouched.
	other := `{"key":"k0","target":"other:model","request":{},"response":{"totally":"different"}}`
	if err := os.WriteFile(path, []byte(other+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := Load[resp](path, "gen:m", Record)
	if err != nil {
		t.Fatal(err)
	}
	key, canon, _ := rec.Key(map[string]any{"b": 1, "a": 2})
	key2, _, _ := rec.Key(map[string]any{"a": 2, "b": 1})
	if key != key2 {
		t.Fatal("equal requests must have equal keys")
	}
	rec.Put(key, canon, resp{"first"})
	rec.Put(key, canon, resp{"second"})
	if err := rec.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), other) {
		t.Errorf("the other target's line changed:\n%s", data)
	}

	rep, err := Load[resp](path, "gen:m", Replay)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"first", "second"} {
		if r, ok := rep.Replay(key); !ok || r.Answer != want {
			t.Errorf("replay %v %v, want %s", r, ok, want)
		}
	}
	if _, ok := rep.Replay(key); ok {
		t.Error("a third call was never recorded: it must miss, not repeat an old answer")
	}
	rep.ResetReplay()
	if r, ok := rep.Replay(key); !ok || r.Answer != "first" {
		t.Errorf("after a reset replay starts over: %v %v", r, ok)
	}
	if _, ok := rep.Replay("nope"); ok {
		t.Error("an unknown key must miss")
	}

	// Re-recording a key replaces its old answers.
	rec2, _ := Load[resp](path, "gen:m", Record)
	rec2.Put(key, canon, resp{"new"})
	_ = rec2.Save(path)
	rep2, _ := Load[resp](path, "gen:m", Replay)
	if r, _ := rep2.Replay(key); r.Answer != "new" {
		t.Errorf("after re-recording: %v", r)
	}
	if _, ok := rep2.Replay(key); ok {
		t.Error("old answers must be gone")
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load[resp]("x", "t", "sometimes"); err == nil {
		t.Error("unknown mode")
	}
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	_ = os.WriteFile(path, []byte("{not json\n"), 0o600)
	if _, err := Load[resp](path, "t", Replay); err == nil {
		t.Error("a corrupt line must be an error")
	}
	if c, err := Load[resp](filepath.Join(t.TempDir(), "missing.jsonl"), "t", Replay); err != nil || c == nil {
		t.Errorf("a missing file is an empty cassette: %v", err)
	}
}
