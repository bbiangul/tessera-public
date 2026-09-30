package admission

import (
	"fmt"
	"regexp"
	"strings"
)

var typeName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)

// checkPluginTypes checks a plugin's declared types (drift-008): names,
// field types, and that an action's output naming one of the plugin's
// types names a declared one when the plugin declares any.
func checkPluginTypes(d Doc) []Issue {
	p := d.Plugin
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	var fieldType func(t string) bool
	fieldType = func(t string) bool {
		if inner, ok := strings.CutPrefix(t, "list<"); ok && strings.HasSuffix(inner, ">") {
			return fieldType(strings.TrimSuffix(inner, ">"))
		}
		if basicTypes[t] {
			return true
		}
		plugin, name, ok := strings.Cut(t, ".")
		if !ok || plugin != p.Metadata.Name {
			return ok && name != "" // another plugin's type: checked where it is declared
		}
		_, declared := p.Spec.Types[name]
		return declared
	}
	for _, name := range sortedKeys(p.Spec.Types) {
		path := "spec.types." + name
		if !typeName.MatchString(name) {
			add("INVALID_FIELD", path, fmt.Sprintf("type name %q: start with a capital letter, letters and digits only", name), "")
		}
		t := p.Spec.Types[name]
		if len(t.Fields) == 0 {
			add("MISSING_FIELD", path+".fields", "a type needs at least one field", "")
		}
		for _, f := range sortedKeys(t.Fields) {
			if !fieldType(strings.TrimSuffix(t.Fields[f], "?")) {
				add("UNKNOWN_TYPE", path+".fields."+f, fmt.Sprintf("unknown type %q", t.Fields[f]), "a basic type, "+p.Metadata.Name+".Type or list<T>; a trailing ? makes it optional")
			}
		}
	}
	if len(p.Spec.Types) == 0 {
		return issues
	}
	for _, name := range sortedKeys(p.Spec.Actions) {
		out := p.Spec.Actions[name].Output
		inner, _ := strings.CutPrefix(out, "list<")
		inner = strings.TrimSuffix(inner, ">")
		if plugin, tn, ok := strings.Cut(inner, "."); ok && plugin == p.Metadata.Name {
			if _, declared := p.Spec.Types[tn]; !declared {
				add("UNKNOWN_TYPE", "spec.actions."+name+".output", fmt.Sprintf("type %q is not in spec.types", inner), "declare it, or remove spec.types")
			}
		}
	}
	return issues
}
