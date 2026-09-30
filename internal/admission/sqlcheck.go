package admission

import (
	"fmt"
	"strings"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	sqlplugin "github.com/bbiangul/tessera/internal/plugins/sql"
)

// checkSQL checks the built-in sql plugin's parts of a manifest: one
// statement per action, its kind against the effect class, and every
// :name parameter given (and no parameter given that the query never uses).
func (c *Checker) checkSQL(d Doc) []Issue {
	var issues []Issue
	p := d.Plugin
	add := func(path, code, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	uses := false
	for _, name := range sortedKeys(p.Spec.Actions) {
		a := p.Spec.Actions[name]
		if a.SQL == nil {
			continue
		}
		uses = true
		path := "spec.actions." + name + ".sql"
		if a.HTTP != nil {
			add("spec.actions."+name, "ONE_STRATEGY", "an action is implemented by http or sql, not both", "")
		}
		st, err := sqlplugin.Scan(a.SQL.Query)
		if err != nil {
			add(path+".query", "INVALID_FIELD", err.Error(), "")
			continue
		}
		switch st.Kind {
		case "SELECT":
			if a.Effect != plugins.EffectRead {
				add(path+".query", "EFFECT_NOT_ALLOWED", fmt.Sprintf("a SELECT reads; declare the action effect: read, not %s", a.Effect), "")
			}
		case "INSERT", "UPDATE", "DELETE", "REPLACE":
			if a.Effect == plugins.EffectRead {
				add(path+".query", "EFFECT_NOT_ALLOWED", fmt.Sprintf("%s changes data; a read action cannot run it", st.Kind), "declare idempotent-write or irreversible")
			}
		default:
			add(path+".query", "INVALID_FIELD", fmt.Sprintf("%s statements belong in spec.sql.migrations, not in actions", st.Kind), "")
		}
		used := map[string]bool{}
		for _, prm := range st.Params {
			used[prm] = true
			if prm == sqlplugin.KeyParam {
				continue
			}
			if _, ok := a.SQL.Params[prm]; !ok {
				add(path+".params", "MISSING_PARAM", fmt.Sprintf("the query uses :%s but params does not give it", prm), "")
			}
		}
		for _, k := range sortedKeys(a.SQL.Params) {
			if !used[k] {
				add(path+".params."+k, "UNKNOWN_PARAM", fmt.Sprintf("the query never uses :%s", k), "remove it")
			}
		}
		if cv, err := c.expr.CompileValue(a.SQL.Params); err != nil {
			add(path+".params", "INVALID_EXPR", err.Error(), "")
		} else {
			pluginRefs(cv.References(), a, requestScopes, "", "", func(code, _, msg, fix string) { add(path+".params", code, msg, fix) })
		}
		if a.SQL.Result == "affected" && st.Kind == "SELECT" {
			add(path+".result", "INVALID_FIELD", "a SELECT returns rows or a row", "")
		}
		if strings.Contains(strings.ToLower(a.SQL.Query), "_tessera_") {
			add(path+".query", "INVALID_FIELD", "tables named _tessera_* belong to the platform", "")
		}
	}
	if uses && p.Spec.SQL == nil {
		add("spec.sql", "MISSING_FIELD", "actions use sql but the plugin declares no database", "add spec.sql.migrations")
	}
	return issues
}
