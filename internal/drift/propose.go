package drift

import (
	"fmt"
	"sort"
	"strings"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// propose turns an action's findings into changes for a person to review
// (RFC-0005 step 4). A read repaired the same way more than once becomes a
// binding output that renames the new fields back, so the plugin works
// again without improvising. An action repair becomes a note for the
// author: what the system now wants and how the users answered, never a
// flag that skips the user (a fee is always asked).
func propose(a *Action, manifests map[string]*plugins.PluginManifest) []Proposal {
	var out []Proposal
	if len(a.Renames) > 0 && a.Renames[0].Count >= 2 {
		if p, ok := bindingOutput(a, manifests); ok {
			out = append(out, p)
		}
	}
	for _, rp := range a.Repairs {
		if rp.Succeeded == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s now requires %s: repaired %d time(s) (%d approved by a person).", a.Action, rp.Fields, rp.Succeeded, rp.Approved)
		if rp.Asked != "" {
			fmt.Fprintf(&b, " A value only the user knows was asked (%q): add an input asked in the Function and send it in the binding's body.", rp.Asked)
		}
		if rp.Agree != "" {
			fmt.Fprintf(&b, " The user had to agree (%q): name it in the action's confirm, and send the flag only after that confirmation.", rp.Agree)
		}
		out = append(out, Proposal{Kind: "author-note", Text: b.String()})
	}
	return out
}

// bindingOutput writes an http output expression that maps the renamed
// fields back to the declared type. Only for an action whose binding has
// no output expression yet and whose output is a declared type.
func bindingOutput(a *Action, manifests map[string]*plugins.PluginManifest) (Proposal, bool) {
	plugin, name, _ := strings.Cut(a.Action, "/")
	m := manifests[plugin]
	if m == nil {
		return Proposal{}, false
	}
	spec, ok := m.Spec.Actions[name]
	if !ok || spec.HTTP == nil || spec.HTTP.Output != "" {
		return Proposal{}, false
	}
	typ := spec.Output
	inner, isList := strings.CutPrefix(typ, "list<")
	inner = strings.TrimSuffix(inner, ">")
	_, tn, _ := strings.Cut(inner, ".")
	t, ok := m.Spec.Types[tn]
	if !ok {
		return Proposal{}, false
	}
	from := map[string]string{}
	for _, pair := range strings.Split(a.Renames[0].Key, ", ") {
		to, src, ok := strings.Cut(pair, " ← ")
		if ok {
			from[to] = src
		}
	}
	fields := make([]string, 0, len(t.Fields))
	for f := range t.Fields {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	var parts []string
	for _, f := range fields {
		src := f
		if s, ok := from[f]; ok {
			src = s
		}
		if strings.HasSuffix(t.Fields[f], "?") {
			parts = append(parts, fmt.Sprintf("'%s': has(x.%s) ? x.%s : null", f, src, src))
		} else {
			parts = append(parts, fmt.Sprintf("'%s': x.%s", f, src))
		}
	}
	obj := "{" + strings.Join(parts, ", ") + "}"
	expr := "sources.response.body.map(x, " + obj + ")"
	if !isList {
		expr = strings.ReplaceAll(obj, "x.", "sources.response.body.")
	}
	return Proposal{
		Kind: "binding-output",
		Text: fmt.Sprintf("The API renamed fields of %s (%s), repaired %d time(s). This output maps them back, so the plugin works again without improvising. Review it before applying: it fits the API as it answers now.", inner, a.Renames[0].Key, a.Renames[0].Count),
		YAML: fmt.Sprintf("# PluginManifest %s, spec.actions.%s.http\noutput: %q\n", plugin, name, expr),
	}, true
}

// Markdown renders the report for a person.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Drift report: %s\n\n%d runs since %s.\n\n", r.Namespace, r.Runs, r.Since.Format("2006-01-02 15:04"))
	if len(r.Actions) == 0 {
		b.WriteString("No drift signals: no refusals with the system's words, no broken contracts, no repairs.\n")
		return b.String()
	}
	for _, a := range r.Actions {
		fmt.Fprintf(&b, "## %s\n\n", a.Action)
		list := func(title string, ss []Signal) {
			if len(ss) == 0 {
				return
			}
			fmt.Fprintf(&b, "**%s**\n\n", title)
			for _, s := range ss {
				fmt.Fprintf(&b, "- %d× %s (last %s; e.g. run %s)\n", s.Count, s.Key, s.Last.Format("2006-01-02 15:04"), s.ExampleRun)
			}
			b.WriteString("\n")
		}
		list("The system refused it and said", a.Refusals)
		list("Its answer broke the declared type", a.Contracts)
		list("Renamed fields repaired", a.Renames)
		if len(a.Repairs) > 0 {
			b.WriteString("**Repairs of the request**\n\n")
			for _, rp := range a.Repairs {
				fmt.Fprintf(&b, "- %s: proposed %d, approved %d, succeeded %d", rp.Fields, rp.Proposed, rp.Approved, rp.Succeeded)
				if rp.Agree != "" {
					fmt.Fprintf(&b, "; the user was asked to agree: %q", rp.Agree)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
		for _, p := range a.Proposals {
			fmt.Fprintf(&b, "**Proposal (%s).** %s\n\n", p.Kind, p.Text)
			if p.YAML != "" {
				fmt.Fprintf(&b, "```yaml\n%s```\n\n", p.YAML)
			}
		}
	}
	return b.String()
}
