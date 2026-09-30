package admission

import (
	"fmt"
	"sort"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
)

// checkAudiences checks an Agent's audiences (RFC-0006, audiences-001):
// unique names; who reads only the caller; at most one catch-all, last;
// each menu names routable Functions of the namespace.
func (c *Checker) checkAudiences(d Doc, functions map[string]*agents.Function) []Issue {
	auds := d.Agent.Spec.Audiences
	if len(auds) == 0 {
		return nil
	}
	ns := d.Agent.Metadata.Namespace
	var issues []Issue
	add := func(sev Severity, code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: sev, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	names := map[string]bool{}
	onMenu := map[string]bool{}
	catchAll := -1
	for i, a := range auds {
		p := fmt.Sprintf("spec.audiences[%d]", i)
		if names[a.Name] {
			add(SeverityError, "DUPLICATE", p+".name", fmt.Sprintf("audience %q is declared twice", a.Name), "")
		}
		names[a.Name] = true
		if a.Who == "" {
			if catchAll >= 0 {
				add(SeverityError, "INVALID_FIELD", p+".who", "only one audience may omit who (the catch-all)", "give it a who")
			}
			catchAll = i
		} else if x, err := c.expr.Compile(string(a.Who), expr.Bool); err != nil {
			add(SeverityError, "INVALID_EXPR", p+".who", err.Error(), "")
		} else {
			for _, ref := range x.References() {
				if ref != "caller" && !strings.HasPrefix(ref, "caller.") {
					add(SeverityError, "UNKNOWN_REF", p+".who", ref+": who reads only the caller", "e.g. 'dentista' in caller.roles")
				}
			}
		}
		for j, fn := range a.Functions {
			f, ok := functions[ns+"/"+fn]
			switch {
			case !ok:
				add(SeverityError, "UNKNOWN_FUNCTION", fmt.Sprintf("%s.functions[%d]", p, j), fmt.Sprintf("no Function %q in namespace %s", fn, ns), "")
			case f.Spec.Selector == nil:
				add(SeverityError, "NOT_ROUTABLE", fmt.Sprintf("%s.functions[%d]", p, j), fmt.Sprintf("Function %s has no selector: the router never offers it", fn), "add a selector, or start it from a free mode or a Trigger")
			}
			onMenu[fn] = true
		}
	}
	switch {
	case catchAll >= 0 && catchAll != len(auds)-1:
		add(SeverityError, "INVALID_FIELD", fmt.Sprintf("spec.audiences[%d].who", catchAll), "the catch-all audience (no who) must be the last: the audiences after it would never match", "move it to the end")
	case catchAll < 0:
		add(SeverityWarning, "NO_CATCH_ALL", "spec.audiences", "a caller matching no audience is offered nothing and hears messages.notUnderstood", "add a last audience without who, if that is not intended")
	}
	var unreachable []string
	for key, f := range functions {
		if strings.HasPrefix(key, ns+"/") && f.Spec.Selector != nil && !onMenu[f.Metadata.Name] {
			unreachable = append(unreachable, f.Metadata.Name)
		}
	}
	sort.Strings(unreachable)
	for _, fn := range unreachable {
		add(SeverityWarning, "NO_AUDIENCE", "spec.audiences", fmt.Sprintf("Function %s has a selector but is on no audience's menu: the router never offers it", fn), "add it to an audience's functions")
	}
	return issues
}

// checkAutonomy checks the autonomy levels (audiences-002): approve-actions
// and draft need someone to approve (humanInTheLoop); a draft level has no
// free mode, whose answers no person could review first; sees names
// audiences that exist.
func (c *Checker) checkAutonomy(d Doc) []Issue {
	spec := d.Agent.Spec
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	check := func(level, path string, free bool) {
		if level == "" || level == "autonomous" {
			return
		}
		if spec.HumanInTheLoop == nil || spec.HumanInTheLoop.Queue == "" {
			add("MISSING_FIELD", path, "autonomy "+level+" needs humanInTheLoop: the queue that approves", "add spec.humanInTheLoop.queue")
		}
		if level == "draft" && free {
			add("INVALID_FIELD", path, "a draft audience has no free mode: its answers go out without a person reviewing them", "remove the free mode, or use approve-actions")
		}
	}
	check(spec.Autonomy, "spec.autonomy", spec.FreeMode != nil && len(spec.Audiences) == 0)
	names := map[string]bool{}
	for _, a := range spec.Audiences {
		names[a.Name] = true
	}
	for i, a := range spec.Audiences {
		level := a.Autonomy
		if level == "" {
			level = spec.Autonomy
		}
		check(level, fmt.Sprintf("spec.audiences[%d].autonomy", i), a.FreeMode != nil || spec.FreeMode != nil)
		for j, v := range a.Sees {
			if !names[v.Audience] {
				add("UNKNOWN_AUDIENCE", fmt.Sprintf("spec.audiences[%d].sees[%d].audience", i, j), fmt.Sprintf("no audience %q", v.Audience), "")
			}
		}
	}
	return issues
}
