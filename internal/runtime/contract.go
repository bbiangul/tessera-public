package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// A read's contract (drift-008): an action whose output names a declared
// plugin type has every read checked against it. APIs add fields freely;
// a missing required field, or one of the wrong type, is a changed API.

// contract returns what is wrong with v as typ, or "" when it conforms.
// It names fields, never values: the text is journaled and shown to
// people.
func (p *Program) contract(typ string, v any) string {
	if inner, ok := strings.CutPrefix(typ, "list<"); ok && strings.HasSuffix(inner, ">") {
		l, ok := v.([]any)
		if !ok {
			return "not a list"
		}
		for i, x := range l {
			if msg := p.contract(strings.TrimSuffix(inner, ">"), x); msg != "" {
				return fmt.Sprintf("item %d: %s", i, msg)
			}
		}
		return ""
	}
	t, declared := p.types[typ]
	if !declared {
		if !conforms(typ, v) {
			return "not a " + typ
		}
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "not an object"
	}
	var missing, wrong []string
	for _, name := range sortedFields(t.Fields) {
		ft, optional := strings.CutSuffix(t.Fields[name], "?")
		x, present := m[name]
		switch {
		case !present || x == nil:
			if !optional {
				missing = append(missing, name)
			}
		case p.contract(ft, x) != "":
			wrong = append(wrong, name)
		}
	}
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing "+strings.Join(missing, ", "))
	}
	if len(wrong) > 0 {
		parts = append(parts, "wrong type: "+strings.Join(wrong, ", "))
	}
	if len(parts) > 0 {
		return strings.Join(parts, "; ") + " (has " + strings.Join(sortedKeys(m), ", ") + ")"
	}
	return ""
}

// declaredElement is the declared object type a read returns (T for T or
// list<T>), when there is one.
func (p *Program) declaredElement(typ string) (string, bool) {
	inner, _ := strings.CutPrefix(typ, "list<")
	inner = strings.TrimSuffix(inner, ">")
	_, ok := p.types[inner]
	return inner, ok
}

func sortedFields(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readChecked calls a read and checks its output against the declared
// type. A mismatch is a definite CONTRACT failure, or, with
// improvise.repairReads, a rename of the answer's fields to the declared
// ones, checked again before it is used.
func (e *execution) readChecked(site, action string, args map[string]any) (any, error) {
	out, err := e.callAction(action, args, "")
	if err != nil {
		return nil, err
	}
	typ := e.rt.Program.actions[action].Output
	if _, ok := e.rt.Program.declaredElement(typ); !ok {
		return out, nil
	}
	msg := e.rt.Program.contract(typ, out)
	if msg == "" {
		return out, nil
	}
	if e.limits.RepairReads {
		if fixed, ok := e.repairRead(site, action, typ, out); ok {
			return fixed, nil
		}
	}
	return nil, &callError{Definite: true, Code: "CONTRACT", Message: action + ": the answer does not match " + typ + ": " + msg, Action: action}
}

const renameRules = `A read of ACTION used to return objects of type TYPE, with DECLARED fields. The system changed: SAMPLE is one object it returns now.
Map each DECLARED field that is missing from SAMPLE to the SAMPLE field that now holds the same information. APIs rename fields to synonyms (label -> description, name -> title, start -> startsAt); use the VALUES in SAMPLE as evidence: the same appointment's time written as text is its label. Never map to a field about something else, such as when a record was created or updated. Leave out a field only when no SAMPLE field holds its information.
Reply with JSON: {"rename": {"<declared field>": "<sample field>"}}.`

var renameSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{"rename": map[string]any{"type": "object"}},
	"required":   []any{"rename"},
}

// repairRead renames the answer's fields to the declared ones (drift-008).
// Only renames: every value comes from the answer unchanged, and the
// result must meet the contract. The rename is journaled for the drift
// report.
func (e *execution) repairRead(site, action, typ string, out any) (any, bool) {
	elem, _ := e.rt.Program.declaredElement(typ)
	items, isList := out.([]any)
	if !isList {
		items = []any{out}
	}
	if len(items) == 0 || e.rt.Generator == nil {
		return nil, false
	}
	sample, ok := items[0].(map[string]any)
	if !ok {
		return nil, false
	}
	declared := e.rt.Program.types[elem].Fields
	resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{Instructions: renameRules, Schema: renameSchema, MaxTokens: 300,
		State: map[string]any{"action": action, "type": elem, "declared": declared, "sample": e.rt.Mask(sample)}})
	if err != nil {
		return nil, false
	}
	m, _ := resp.Value.(map[string]any)
	rename, _ := m["rename"].(map[string]any)
	used := map[string]bool{}
	pairs := map[string]string{}
	for to, from := range rename {
		src, ok := from.(string)
		if _, present := sample[to]; ok && (src == to || present) {
			continue // the field is still there: nothing to rename
		}
		if _, isDeclared := declared[to]; !ok || !isDeclared || used[src] {
			return nil, false // renames only, each source once, onto declared fields
		}
		if _, clash := declared[src]; clash {
			return nil, false // a declared field is never moved onto another
		}
		used[src] = true
		pairs[to] = src
	}
	if len(pairs) == 0 {
		return nil, false
	}
	fixed := make([]any, 0, len(items))
	for _, x := range items {
		obj, ok := x.(map[string]any)
		if !ok {
			return nil, false
		}
		n := make(map[string]any, len(obj)+len(pairs))
		for k, v := range obj {
			n[k] = v
		}
		for to, from := range pairs {
			if v, ok := obj[from]; ok {
				if _, already := obj[to]; !already {
					n[to] = v
				}
			}
		}
		fixed = append(fixed, n)
	}
	var result any = fixed
	if !isList {
		result = fixed[0]
	}
	if e.rt.Program.contract(typ, result) != "" {
		return nil, false
	}
	mapping := make(map[string]any, len(pairs))
	for to, from := range pairs {
		mapping[to] = from
	}
	if _, err := e.put(site+"/repaired", journal.KindResult, map[string]any{"action": action, "type": elem, "rename": mapping}); err != nil {
		return nil, false
	}
	return result, true
}
