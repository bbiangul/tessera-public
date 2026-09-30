package sql

import (
	"context"
	gosql "database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

func manifest() *plugins.PluginManifest {
	return &plugins.PluginManifest{
		Metadata: plugins.PluginMeta{Name: "memo", Version: "1.0.0"},
		Spec: plugins.PluginSpec{
			SQL: &plugins.SQLDatabase{Migrations: []string{
				`CREATE TABLE searches (conversation TEXT NOT NULL, key TEXT UNIQUE, results TEXT NOT NULL, at TEXT NOT NULL)`,
				`CREATE INDEX searches_conversation ON searches (conversation, at)`,
			}},
			Actions: map[string]plugins.ActionSpec{
				"save": {Effect: plugins.EffectIrreversible, Input: map[string]string{"conversation": "string", "results": "any", "at": "timestamp"},
					SQL: &plugins.SQLStatement{
						Query:  `INSERT INTO searches (conversation, key, results, at) VALUES (:c, :idempotencyKey, :r, :at) ON CONFLICT (key) DO NOTHING`,
						Params: map[string]any{"c": "${ inputs.conversation }", "r": "${ inputs.results }", "at": "${ inputs.at }"},
					}},
				"last": {Effect: plugins.EffectRead, Input: map[string]string{"conversation": "string"},
					SQL: &plugins.SQLStatement{
						Query:  `SELECT results, at FROM searches WHERE conversation = :c ORDER BY at DESC LIMIT 1 -- the newest`,
						Params: map[string]any{"c": "${ inputs.conversation }"},
						Result: "row", JSON: []string{"results"},
					}},
				"all": {Effect: plugins.EffectRead, Input: map[string]string{"conversation": "string"},
					SQL: &plugins.SQLStatement{Query: `SELECT key FROM searches WHERE conversation = :c ORDER BY at`, Params: map[string]any{"c": "${ inputs.conversation }"}}},
			},
		},
	}
}

func open(t *testing.T) *gosql.DB {
	t.Helper()
	db, err := gosql.Open("sqlite", filepath.Join(t.TempDir(), "memo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func registry(t *testing.T, db *gosql.DB, m *plugins.PluginManifest) *act.Registry {
	t.Helper()
	p, err := New(context.Background(), m, db)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r, nil)
	return r
}

func TestSaveAndReadBack(t *testing.T) {
	db := open(t)
	r := registry(t, db, manifest())
	ctx := context.Background()
	results := []any{map[string]any{"token": "t1", "price": int64(640)}, map[string]any{"token": "t2", "price": 720.5}}
	for _, key := range []string{"k1", "k1", "k2"} { // k1 twice: the key makes it happen once
		if _, err := r.Call(ctx, act.Call{Action: "memo/save", IdempotencyKey: key,
			Args: map[string]any{"conversation": "c1", "results": results, "at": "2026-09-23T09:00:0" + key[1:] + "Z"}}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := r.Call(ctx, act.Call{Action: "memo/all", Args: map[string]any{"conversation": "c1"}})
	if err != nil || len(all.([]any)) != 2 {
		t.Fatalf("rows %v %v: the repeated key must not insert twice", all, err)
	}
	last, err := r.Call(ctx, act.Call{Action: "memo/last", Args: map[string]any{"conversation": "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	got := last.(map[string]any)["results"].([]any)
	if !reflect.DeepEqual(got, results) {
		t.Errorf("json column: got %#v, want %#v (ints stay ints)", got, results)
	}
	none, err := r.Call(ctx, act.Call{Action: "memo/last", Args: map[string]any{"conversation": "nobody"}})
	if err != nil || none != nil {
		t.Errorf("no row is null: %v %v", none, err)
	}
}

func TestParametersCannotChangeTheStatement(t *testing.T) {
	db := open(t)
	r := registry(t, db, manifest())
	ctx := context.Background()
	_, _ = r.Call(ctx, act.Call{Action: "memo/save", IdempotencyKey: "k", Args: map[string]any{"conversation": "c1", "results": []any{}, "at": "x"}})
	evil := "c1' OR '1'='1"
	rows, err := r.Call(ctx, act.Call{Action: "memo/all", Args: map[string]any{"conversation": evil}})
	if err != nil || len(rows.([]any)) != 0 {
		t.Errorf("an injection attempt is a value, not SQL: %v %v", rows, err)
	}
}

func TestMigrationsRunOnceAndCannotBeEdited(t *testing.T) {
	db := open(t)
	registry(t, db, manifest())
	registry(t, db, manifest()) // again: nothing to do
	m := manifest()
	m.Spec.SQL.Migrations[0] = strings.Replace(m.Spec.SQL.Migrations[0], "at TEXT", "at TEXT, extra TEXT", 1)
	if _, err := New(context.Background(), m, db); !errors.Is(err, ErrEditedMigration) {
		t.Errorf("an edited migration must be refused: %v", err)
	}
	m = manifest()
	m.Spec.SQL.Migrations = append(m.Spec.SQL.Migrations, `ALTER TABLE searches ADD COLUMN extra TEXT`)
	if _, err := New(context.Background(), m, db); err != nil {
		t.Errorf("a new migration is applied: %v", err)
	}
}

func TestFailuresAreDefiniteAndCarryNoValues(t *testing.T) {
	db := open(t)
	m := manifest()
	m.Spec.Actions["dup"] = plugins.ActionSpec{Effect: plugins.EffectIrreversible, Input: map[string]string{"k": "string"},
		SQL: &plugins.SQLStatement{Query: `INSERT INTO searches (conversation, key, results, at) VALUES ('c', :k, '[]', 'x')`, Params: map[string]any{"k": "${ inputs.k }"}}}
	r := registry(t, db, m)
	ctx := context.Background()
	if _, err := r.Call(ctx, act.Call{Action: "memo/dup", Args: map[string]any{"k": "secret-123"}}); err != nil {
		t.Fatal(err)
	}
	_, err := r.Call(ctx, act.Call{Action: "memo/dup", Args: map[string]any{"k": "secret-123"}})
	var d *act.DefiniteError
	if !errors.As(err, &d) || d.Code != "CONFLICT" || strings.Contains(err.Error(), "secret-123") {
		t.Errorf("a constraint failure is definite, with no values: %v", err)
	}
}

func TestScan(t *testing.T) {
	for q, want := range map[string]Statement{
		`SELECT a FROM t WHERE x = :x AND y = ':not_a_param' AND z = :x`: {Kind: "SELECT", Params: []string{"x"}},
		`with r as (select 1) select * from r where a = :a`:              {Kind: "SELECT", Params: []string{"a"}},
		`/* :c */ UPDATE t SET a = :a -- :b`:                             {Kind: "UPDATE", Params: []string{"a"}},
		`INSERT INTO t VALUES (:idempotencyKey);`:                        {Kind: "INSERT", Params: []string{"idempotencyKey"}},
	} {
		got, err := Scan(q)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v %v, want %+v", q, got, err, want)
		}
	}
	for _, bad := range []string{`SELECT 1; DROP TABLE t`, `SELECT 'unterminated`, `/* never closed`, ``} {
		if _, err := Scan(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
