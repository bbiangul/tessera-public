// Package explain documents any field of any kind from the embedded JSON
// Schemas, like kubectl explain. Because the schemas are generated from the
// Go types and their doc comments, this output can never drift from the code.
package explain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bbiangul/tessera/api/schemas"
)

// Field is the documentation of one field.
type Field struct {
	Kind        string  `json:"kind"`
	Path        string  `json:"path"`
	Type        string  `json:"type"`
	Syntax      string  `json:"syntax,omitempty"` // expression or template
	Description string  `json:"description"`
	Enum        []any   `json:"enum,omitempty"`
	Fields      []Child `json:"fields,omitempty"`
}

// Child is a sub-field summary.
type Child struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Summary  string `json:"summary"`
}

type node = map[string]any

var listIndex = regexp.MustCompile(`\[[0-9]+\]`)

// Explain resolves "Kind" or "Kind.path.to.field". Map keys and list indexes
// are skipped, so Function.spec.inputs.resolve works without an input name;
// a concrete key like Function.spec.inputs.newSlot.resolve works too.
func Explain(query string) (*Field, error) {
	query = listIndex.ReplaceAllString(query, "") // accept paths as validate prints them
	kind, path, _ := strings.Cut(query, ".")
	raw, err := schemas.Get(kind)
	if err != nil {
		return nil, fmt.Errorf("%w (known kinds: %s)", err, strings.Join(kindNames(), ", "))
	}
	var root node
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	defs, _ := root["$defs"].(node)
	cur := deref(root, defs)
	curProp := cur
	desc := description(cur)
	walked := []string{}
	if path != "" {
		for _, seg := range strings.Split(path, ".") {
			next, ok := child(cur, seg, defs)
			if !ok {
				return nil, fmt.Errorf("%s has no field %q; fields: %s", joinPath(kind, walked), seg, strings.Join(propertyNames(cur, defs), ", "))
			}
			walked = append(walked, seg)
			desc = description(next.prop)
			cur, curProp = next.schema, next.prop
		}
	}
	if d := description(cur); desc == "" {
		desc = d
	}
	f := &Field{Kind: kind, Path: strings.Join(walked, "."), Type: typeName(curProp, defs), Description: desc}
	if s := syntax(cur, defs); s != "" {
		f.Syntax = s
	}
	if e, ok := cur["enum"].([]any); ok {
		f.Enum = e
	}
	obj := containerTarget(cur, defs)
	if props, ok := obj["properties"].(node); ok {
		req := map[string]bool{}
		if r, ok := obj["required"].([]any); ok {
			for _, x := range r {
				req[fmt.Sprint(x)] = true
			}
		}
		names := make([]string, 0, len(props))
		for n := range props {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			p := props[n].(node)
			f.Fields = append(f.Fields, Child{Name: n, Type: typeName(p, defs), Required: req[n], Summary: firstSentence(description(p))})
		}
	}
	return f, nil
}

type step struct {
	prop   node // the property as written (holds the field description)
	schema node // the resolved schema
}

// child descends by one segment, passing through maps and arrays.
func child(cur node, seg string, defs node) (step, bool) {
	obj := deref(cur, defs)
	if props, ok := obj["properties"].(node); ok {
		if p, ok := props[seg].(node); ok {
			return step{prop: p, schema: deref(p, defs)}, true
		}
	}
	// Inside a map or list: a field name of the element wins (the key or
	// index was skipped); otherwise the segment is itself a key or index.
	if elem := element(obj, defs); elem != nil {
		if next, ok := child(elem, seg, defs); ok {
			return next, true
		}
		if isKeyOrIndex(obj, seg) {
			return step{prop: elem, schema: elem}, true
		}
	}
	return step{}, false
}

func isKeyOrIndex(obj node, seg string) bool {
	if _, ok := obj["additionalProperties"].(node); ok {
		return true // any string is a map key
	}
	if _, ok := obj["items"]; ok {
		return strings.Trim(seg, "0123456789") == ""
	}
	return false
}

// element returns the element schema of a map or array, if any.
func element(n node, defs node) node {
	if ap, ok := n["additionalProperties"].(node); ok {
		return deref(ap, defs)
	}
	if it, ok := n["items"].(node); ok {
		return deref(it, defs)
	}
	return nil
}

// containerTarget returns the object whose fields should be listed: the
// value itself, or the element of a map or list of objects.
func containerTarget(n, defs node) node {
	if _, ok := n["properties"]; ok {
		return n
	}
	if e := element(n, defs); e != nil {
		return containerTarget(e, defs)
	}
	return n
}

func deref(n, defs node) node {
	for i := 0; i < 10; i++ {
		ref, ok := n["$ref"].(string)
		if !ok {
			return n
		}
		name := strings.TrimPrefix(ref, "#/$defs/")
		target, ok := defs[name].(node)
		if !ok {
			return n
		}
		merged := node{}
		for k, v := range target {
			merged[k] = v
		}
		if d, ok := n["description"]; ok {
			merged["description"] = d
		}
		n = merged
	}
	return n
}

func typeName(n, defs node) string {
	if ref, ok := n["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		t := deref(n, defs)
		if k, ok := t["x-tessera-kind"].(string); ok {
			return k
		}
		return name
	}
	switch t, _ := n["type"].(string); t {
	case "array":
		if it, ok := n["items"].(node); ok {
			return "list of " + typeName(it, defs)
		}
		return "list"
	case "object":
		if ap, ok := n["additionalProperties"].(node); ok {
			return "map of " + typeName(ap, defs)
		}
		if _, ok := n["properties"]; ok {
			return "object"
		}
		return "map of any"
	case "":
		return "any"
	default:
		return t
	}
}

func syntax(n, defs node) string {
	if k, ok := deref(n, defs)["x-tessera-kind"].(string); ok {
		switch k {
		case "expression":
			return "CEL expression without delimiters; reads inputs, sources, steps, event, caller, conversation, agent, env"
		case "template":
			return "text with {{ expr }} interpolations; values are escaped for the channel"
		}
	}
	return ""
}

func description(n node) string {
	d, _ := n["description"].(string)
	return strings.TrimSpace(d)
}

func firstSentence(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

func propertyNames(n, defs node) []string {
	obj := containerTarget(deref(n, defs), defs)
	props, _ := obj["properties"].(node)
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func joinPath(kind string, segs []string) string {
	if len(segs) == 0 {
		return kind
	}
	return kind + "." + strings.Join(segs, ".")
}

// Kinds lists the kinds explain knows, sorted.
func Kinds() []string { return kindNames() }

func kindNames() []string {
	out := make([]string, 0, len(schemas.Kinds))
	for k := range schemas.Kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Text renders a Field for a terminal.
func (f *Field) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "KIND:  %s\n", f.Kind)
	if f.Path != "" {
		fmt.Fprintf(&b, "FIELD: %s\n", f.Path)
	}
	fmt.Fprintf(&b, "TYPE:  %s\n", f.Type)
	if f.Syntax != "" {
		fmt.Fprintf(&b, "SYNTAX: %s\n", f.Syntax)
	}
	if len(f.Enum) > 0 {
		vals := make([]string, len(f.Enum))
		for i, v := range f.Enum {
			vals[i] = fmt.Sprint(v)
		}
		fmt.Fprintf(&b, "VALUES: %s\n", strings.Join(vals, ", "))
	}
	if f.Description != "" {
		b.WriteString("\nDESCRIPTION:\n")
		for _, line := range strings.Split(f.Description, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(f.Fields) > 0 {
		b.WriteString("\nFIELDS:\n")
		for _, c := range f.Fields {
			req := ""
			if c.Required {
				req = " (required)"
			}
			fmt.Fprintf(&b, "  %s <%s>%s\n      %s\n", c.Name, c.Type, req, c.Summary)
		}
	}
	return b.String()
}
