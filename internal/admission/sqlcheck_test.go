package admission

import (
	"strings"
	"testing"
)

const sqlPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: memo, version: 1.0.0 }
spec:
  sql:
    migrations:
      - CREATE TABLE searches (conversation TEXT, key TEXT UNIQUE, results TEXT)
  actions:
    save:
      input: { conversation: string, results: any }
      effect: idempotent-write
      sql:
        query: "INSERT INTO searches (conversation, key, results) VALUES (:c, :idempotencyKey, :r) ON CONFLICT (key) DO NOTHING"
        params: { c: "${ inputs.conversation }", r: "${ inputs.results }" }
    last:
      input: { conversation: string }
      output: map
      effect: read
      sql:
        query: "SELECT results FROM searches WHERE conversation = :c ORDER BY rowid DESC LIMIT 1"
        params: { c: "${ inputs.conversation }" }
        result: row
        json: [results]
`

func TestSQLBaselineIsClean(t *testing.T) {
	if is := errorsOnly(validate(t, map[string]string{"p.yaml": sqlPlugin})); len(is) > 0 {
		t.Fatalf("baseline must be clean: %v", is)
	}
}

func TestSQLRules(t *testing.T) {
	cases := []struct{ name, old, new, want string }{
		{"select must be a read", "      effect: read\n", "      effect: idempotent-write\n", "EFFECT_NOT_ALLOWED@spec.actions.last.sql.query"},
		{"a write cannot be a read", "      effect: idempotent-write\n", "      effect: read\n", "EFFECT_NOT_ALLOWED@spec.actions.save.sql.query"},
		{"two statements", "LIMIT 1\"", "LIMIT 1; DELETE FROM searches\"", "INVALID_FIELD@spec.actions.last.sql.query"},
		{"schema changes are migrations", "query: \"SELECT results", "query: \"DROP TABLE searches -- SELECT results", "INVALID_FIELD@spec.actions.last.sql.query"},
		{"a parameter the query uses is missing", "params: { c: \"${ inputs.conversation }\" }\n        result", "params: {}\n        result", "MISSING_PARAM@spec.actions.last.sql.params"},
		{"a parameter the query never uses", "params: { c: \"${ inputs.conversation }\" }\n        result", "params: { c: \"${ inputs.conversation }\", x: 1 }\n        result", "UNKNOWN_PARAM@spec.actions.last.sql.params.x"},
		{"params read only inputs and caller", "r: \"${ inputs.results }\"", "r: \"${ env.X }\"", "SCOPE_NOT_ALLOWED@spec.actions.save.sql.params"},
		{"no database", "  sql:\n    migrations:\n      - CREATE TABLE searches (conversation TEXT, key TEXT UNIQUE, results TEXT)\n", "", "MISSING_FIELD@spec.sql"},
		{"platform tables", "FROM searches WHERE", "FROM _tessera_migrations WHERE", "INVALID_FIELD@spec.actions.last.sql.query"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(sqlPlugin, c.old) {
				t.Fatalf("fixture does not contain %q", c.old)
			}
			got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": strings.Replace(sqlPlugin, c.old, c.new, 1)})))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %s, got %s", c.want, got)
			}
		})
	}
}
