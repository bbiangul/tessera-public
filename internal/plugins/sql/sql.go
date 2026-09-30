// Package sql is the built-in sql plugin (RFC-0001, "Plugins que vêm com a
// plataforma"): a plugin's own database for the agent's state between
// turns, with manifest-declared, parameterized statements.
//
// The SQL text of an action is literal; values only enter as named
// parameters, so nothing a user says can change a statement. A statement
// runs atomically in SQLite: when it fails nothing changed, so every
// failure is definite.
package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
)

// KeyParam is bound to the call's idempotency key, so a write can refuse
// to happen twice (INSERT ... ON CONFLICT DO NOTHING on a key column), as :idempotencyKey.
const KeyParam = "idempotencyKey"

type action struct {
	name   string
	spec   plugins.ActionSpec
	st     *plugins.SQLStatement
	params *expr.Value
	names  []string // parameters used by the query
	result string
	json   map[string]bool
}

// Plugin serves one manifest's sql actions on its database.
type Plugin struct {
	manifest *plugins.PluginManifest
	db       *sql.DB
	actions  map[string]*action
}

// New compiles a manifest's sql actions and applies its migrations to db.
// The manifest must have passed admission.
func New(ctx context.Context, m *plugins.PluginManifest, db *sql.DB) (*Plugin, error) {
	if m.Spec.SQL == nil {
		return nil, fmt.Errorf("plugin %s: no sql database", m.Metadata.Name)
	}
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	p := &Plugin{manifest: m, db: db, actions: map[string]*action{}}
	for name, spec := range m.Spec.Actions {
		if spec.SQL == nil {
			continue
		}
		info, err := Scan(spec.SQL.Query)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		a := &action{name: name, spec: spec, st: spec.SQL, names: info.Params, result: spec.SQL.Result, json: map[string]bool{}}
		if a.result == "" {
			a.result = "affected"
			if info.Kind == "SELECT" {
				a.result = "rows"
			}
		}
		for _, c := range spec.SQL.JSON {
			a.json[c] = true
		}
		params := spec.SQL.Params
		if params == nil {
			params = map[string]any{}
		}
		if a.params, err = e.CompileValue(params); err != nil {
			return nil, fmt.Errorf("%s.params: %w", name, err)
		}
		p.actions[name] = a
	}
	if err := p.migrate(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// ErrEditedMigration means a migration that already ran was changed.
var ErrEditedMigration = errors.New("a migration that already ran was edited")

func (p *Plugin) migrate(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _tessera_migrations (n INTEGER PRIMARY KEY, statement TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for i, stmt := range p.manifest.Spec.SQL.Migrations {
		var done string
		err := p.db.QueryRowContext(ctx, `SELECT statement FROM _tessera_migrations WHERE n = ?`, i+1).Scan(&done)
		switch {
		case err == nil:
			if done != stmt {
				return fmt.Errorf("plugin %s migration %d: %w; add a new migration instead", p.manifest.Metadata.Name, i+1, ErrEditedMigration)
			}
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		tx, err := p.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("plugin %s migration %d: %w", p.manifest.Metadata.Name, i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO _tessera_migrations (n, statement, applied_at) VALUES (?, ?, ?)`, i+1, stmt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Register adds the plugin's sql actions to a registry, except those in
// skip (a test sandbox).
func (p *Plugin) Register(r *act.Registry, skip map[string]act.Handler) {
	names := make([]string, 0, len(p.actions))
	for n := range p.actions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		full := p.manifest.Metadata.Name + "/" + n
		if _, skipped := skip[full]; skipped {
			continue
		}
		a := p.actions[n]
		r.Register(full, func(ctx context.Context, c act.Call) (any, error) { return p.call(ctx, a, c) })
	}
}

func (p *Plugin) call(ctx context.Context, a *action, c act.Call) (any, error) {
	v, err := a.params.Resolve(ctx, expr.Activation{"inputs": c.Args, "caller": c.Caller})
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": params: " + err.Error()}
	}
	values, _ := v.(map[string]any)
	args := make([]any, 0, len(a.names))
	for _, name := range a.names {
		if name == KeyParam {
			args = append(args, sql.Named(name, c.IdempotencyKey))
			continue
		}
		val, err := bindable(values[name])
		if err != nil {
			return nil, &act.DefiniteError{Code: "INVALID", Message: fmt.Sprintf("%s: parameter %s: %v", a.name, name, err)}
		}
		args = append(args, sql.Named(name, val))
	}
	switch a.result {
	case "affected":
		res, err := p.db.ExecContext(ctx, a.st.Query, args...)
		if err != nil {
			return nil, failure(a, err)
		}
		n, _ := res.RowsAffected()
		return map[string]any{"affected": n}, nil
	}
	rows, err := p.db.QueryContext(ctx, a.st.Query, args...)
	if err != nil {
		return nil, failure(a, err)
	}
	defer rows.Close()
	out, err := p.read(a, rows)
	if err != nil {
		return nil, failure(a, err)
	}
	if a.result == "row" {
		if len(out) == 0 {
			return nil, nil
		}
		return out[0], nil
	}
	return out, nil
}

// failure: a statement failed atomically, so nothing changed. The driver's
// message may quote values; only its first clause (the kind) is kept.
func failure(a *action, err error) error {
	msg := err.Error()
	if i := strings.IndexAny(msg, ":("); i > 0 {
		msg = msg[:i]
	}
	code := "SQL_ERROR"
	if strings.Contains(err.Error(), "constraint") {
		code = "CONFLICT"
	}
	return &act.DefiniteError{Code: code, Message: a.name + ": " + strings.TrimSpace(msg)}
}

func (p *Plugin) read(a *action, rows *sql.Rows) ([]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			if a.json[c] {
				if s, ok := v.(string); ok && s != "" {
					var decoded any
					d := json.NewDecoder(strings.NewReader(s))
					d.UseNumber()
					if err := d.Decode(&decoded); err != nil {
						return nil, fmt.Errorf("column %s is not JSON", c)
					}
					v = numbers(decoded)
				}
			}
			row[c] = v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// bindable converts a value to one SQLite stores: lists and maps as JSON
// text, timestamps as RFC 3339 text.
func bindable(v any) (any, error) {
	switch x := v.(type) {
	case nil, string, int64, float64, bool:
		return x, nil
	case time.Time:
		return x.Format(time.RFC3339Nano), nil
	case []any, map[string]any:
		data, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	}
	return nil, fmt.Errorf("type %T cannot be stored", v)
}

func numbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		return expr.Number(x)
	case []any:
		for i := range x {
			x[i] = numbers(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = numbers(x[k])
		}
	}
	return v
}

// Statement describes a query for admission and binding.
type Statement struct {
	Kind   string   // SELECT, INSERT, UPDATE, DELETE, REPLACE, or the first keyword
	Params []string // :name parameters, in order of first use
}

// Scan reads a statement's kind and parameters, skipping string literals,
// quoted identifiers and comments. It refuses more than one statement.
func Scan(q string) (Statement, error) {
	var st Statement
	seen := map[string]bool{}
	i, n := 0, len(q)
	first := true
	for i < n {
		c := q[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for j < n {
				if q[j] == c {
					if j+1 < n && q[j+1] == c { // doubled quote
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= n {
				return st, errors.New("unterminated quote")
			}
			i = j + 1
		case c == '-' && i+1 < n && q[i+1] == '-':
			for i < n && q[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && q[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				return st, errors.New("unterminated comment")
			}
			i += end + 4
		case c == ';':
			if strings.TrimSpace(stripComments(q[i+1:])) != "" {
				return st, errors.New("one statement per action")
			}
			i = n
		case c == ':' && i+1 < n && (unicode.IsLetter(rune(q[i+1])) || q[i+1] == '_'):
			j := i + 1
			for j < n && (unicode.IsLetter(rune(q[j])) || unicode.IsDigit(rune(q[j])) || q[j] == '_') {
				j++
			}
			name := q[i+1 : j]
			if !seen[name] {
				seen[name] = true
				st.Params = append(st.Params, name)
			}
			i = j
		case unicode.IsLetter(rune(c)) && first:
			j := i
			for j < n && unicode.IsLetter(rune(q[j])) {
				j++
			}
			st.Kind = strings.ToUpper(q[i:j])
			if st.Kind == "WITH" {
				st.Kind = "SELECT"
			}
			first = false
			i = j
		default:
			i++
		}
	}
	if st.Kind == "" {
		return st, errors.New("empty statement")
	}
	return st, nil
}

func stripComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
