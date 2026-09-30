package cassette

import (
	"path/filepath"
	"testing"
)

func TestScenarioScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scoped.jsonl")
	record, err := Load[resp](path, "model", Record)
	if err != nil {
		t.Fatal(err)
	}
	put := func(scope, text string) {
		t.Helper()
		record.SetScope(scope)
		key, canon, err := record.Key("identical")
		if err != nil {
			t.Fatal(err)
		}
		record.Put(key, canon, resp{text})
	}
	put("ns/suite/a", "A first")
	put("ns/suite/a", "A second")
	put("ns/suite/b", "B")
	if err := record.Save(path); err != nil {
		t.Fatal(err)
	}
	replay, err := Load[resp](path, "model", Replay)
	if err != nil {
		t.Fatal(err)
	}
	check := func(scope string, wants ...string) {
		t.Helper()
		replay.SetScope(scope)
		key, _, err := replay.Key("identical")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			got, ok := replay.Replay(key)
			if !ok || got.Answer != want {
				t.Fatalf("scope %s got %+v %v want %s", scope, got, ok, want)
			}
		}
		if _, ok := replay.Replay(key); ok {
			t.Fatal("extra occurrence must miss")
		}
	}
	check("ns/suite/b", "B")
	check("ns/suite/a", "A first", "A second")
	check("ns/suite/b", "B")
	check("other/suite/b")
	check("ns/other/b")
	check("ns/suite/missing")
	check("")
	// Recording just B again must preserve A and replace B's sequence.
	record, err = Load[resp](path, "model", Record)
	if err != nil {
		t.Fatal(err)
	}
	put("ns/suite/b", "B new")
	if err := record.Save(path); err != nil {
		t.Fatal(err)
	}
	replay, err = Load[resp](path, "model", Replay)
	if err != nil {
		t.Fatal(err)
	}
	check("ns/suite/a", "A first", "A second")
	check("ns/suite/b", "B new")
	replay.SetScope("ns/suite/a")
	key, _, err := replay.Key("changed prompt")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replay.Replay(key); ok {
		t.Fatal("changed request must miss")
	}
}

func TestLegacyAndMixedScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	record, err := Load[resp](path, "model", Record)
	if err != nil {
		t.Fatal(err)
	}
	key, canon, err := record.Key("same")
	if err != nil {
		t.Fatal(err)
	}
	record.Put(key, canon, resp{"legacy"})
	if err := record.Save(path); err != nil {
		t.Fatal(err)
	}
	replay, err := Load[resp](path, "model", Replay)
	if err != nil {
		t.Fatal(err)
	}
	replay.SetScope("ns/suite/a")
	legacy, _, err := replay.Key("same")
	if err != nil {
		t.Fatal(err)
	}
	if legacy != key {
		t.Fatal("legacy request key changed")
	}
	if got, ok := replay.Replay(legacy); !ok || got.Answer != "legacy" {
		t.Fatal("legacy replay failed")
	}
	record.SetScope("ns/suite/a")
	scoped, canon, err := record.Key("same")
	if err != nil {
		t.Fatal(err)
	}
	record.Put(scoped, canon, resp{"scoped"})
	if err := record.Save(path); err != nil {
		t.Fatal(err)
	}
	replay, err = Load[resp](path, "model", Replay)
	if err != nil {
		t.Fatal(err)
	}
	replay.SetScope("ns/suite/missing")
	missing, _, err := replay.Key("same")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replay.Replay(missing); ok {
		t.Fatal("scoped target fell back to legacy")
	}
}
