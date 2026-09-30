package admission

import (
	"fmt"
	"slices"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/metrics"
)

// checkSubject checks a Subject: an id expression and closed, typed
// attributes (free text could carry personal data into every metric).
func (c *Checker) checkSubject(d Doc) []Issue {
	var issues []Issue
	s := d.Subject
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	if _, err := c.expr.Compile(string(s.Spec.ID), expr.Any); err != nil {
		add("INVALID_EXPR", "spec.id", err.Error(), "")
	}
	for _, name := range sortedKeys(s.Spec.Attributes) {
		a := s.Spec.Attributes[name]
		if a.Type == "string" && len(a.Enum) == 0 {
			add("MISSING_FIELD", "spec.attributes."+name+".enum", "a string attribute lists its values: free text could carry personal data", "list the values in enum")
		}
		if a.Type != "string" && len(a.Enum) > 0 {
			add("UNUSED_FIELD", "spec.attributes."+name+".enum", "enum only applies to string attributes", "remove it")
		}
	}
	return issues
}

// checkMilestone checks a milestone on a step or an action: a Subject of
// the namespace, an id, and only the Subject's attributes.
func (f *fnCheck) checkMilestone(m *agents.MilestoneRef, path string, sc scope) {
	if m == nil {
		return
	}
	s, ok := f.subjects[f.fn.Metadata.Namespace+"/"+m.Subject]
	if !ok {
		f.err("UNKNOWN_SUBJECT", path+".subject", fmt.Sprintf("no Subject %q in namespace %s", m.Subject, f.fn.Metadata.Namespace), "declare it: kind Subject")
		return
	}
	if m.ID != "" {
		f.expression(string(m.ID), expr.Any, path+".id", sc)
	} else {
		f.expression(string(s.Spec.ID), expr.Any, path+" (the Subject's spec.id)", sc)
	}
	for _, k := range sortedKeys(m.Attributes) {
		if _, ok := s.Spec.Attributes[k]; !ok {
			f.err("UNKNOWN_PARAM", path+".attributes."+k, fmt.Sprintf("Subject %s has no attribute %q", m.Subject, k), "declare it in the Subject")
			continue
		}
		f.expression(string(m.Attributes[k]), expr.Any, path+".attributes."+k, sc)
	}
}

// checkFunnel checks a Funnel: its Subject, its stages' windows, and that
// it is cut only by the Subject's attributes.
func checkFunnel(d Doc, subjects map[string]*agents.Subject, functions map[string]*agents.Function) []Issue {
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	f := d.Funnel
	s, ok := subjects[f.Metadata.Namespace+"/"+f.Spec.Subject]
	if !ok {
		add("UNKNOWN_SUBJECT", "spec.subject", fmt.Sprintf("no Subject %q in namespace %s", f.Spec.Subject, f.Metadata.Namespace), "declare it: kind Subject")
	}
	var names []string
	for i, st := range f.Spec.Stages {
		p := fmt.Sprintf("spec.stages[%d]", i)
		if slices.Contains(names, st.Name) {
			add("DUPLICATE", p+".name", fmt.Sprintf("stage %q is declared twice", st.Name), "")
		}
		names = append(names, st.Name)
		for field, v := range map[string]string{"within": st.Within, "expected": st.Expected} {
			if v == "" {
				continue
			}
			if i == 0 {
				add("UNUSED_FIELD", p+"."+field, "the first stage is the entry: nothing comes before it", "remove it")
				continue
			}
			if _, err := metrics.ParseDuration(v); err != nil {
				add("INVALID_FIELD", p+"."+field, err.Error(), "")
			}
		}
	}
	if o := f.Spec.OnStuck; o != nil {
		if _, ok := functions[f.Metadata.Namespace+"/"+o.Function]; !ok {
			add("UNKNOWN_FUNCTION", "spec.onStuck.function", fmt.Sprintf("no Function %q in namespace %s", o.Function, f.Metadata.Namespace), "")
		}
		expected := false
		for _, st := range f.Spec.Stages {
			expected = expected || st.Expected != ""
		}
		if !expected {
			add("UNUSED_FIELD", "spec.onStuck", "no stage has expected: nobody can be stuck", "set expected on the stages that have a deadline")
		}
	}
	if ok {
		for i, dim := range f.Spec.Dimensions {
			if _, ok := s.Spec.Attributes[dim]; !ok {
				add("UNKNOWN_PARAM", fmt.Sprintf("spec.dimensions[%d]", i), fmt.Sprintf("Subject %s has no attribute %q", s.Metadata.Name, dim), "")
			}
		}
	}
	return issues
}

// checkExperiment checks an Experiment: its Subject, distinct variants,
// and a primary stage of a Funnel that follows the same Subject.
func checkExperiment(d Doc, subjects map[string]*agents.Subject, funnels map[string]*agents.Funnel, functions map[string]*agents.Function) []Issue {
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	x := d.Experiment
	ns := x.Metadata.Namespace
	if _, ok := subjects[ns+"/"+x.Spec.Subject]; !ok {
		add("UNKNOWN_SUBJECT", "spec.subject", fmt.Sprintf("no Subject %q in namespace %s", x.Spec.Subject, ns), "")
	}
	seen := map[string]bool{}
	for i, v := range x.Spec.Variants {
		if seen[v.Name] {
			add("DUPLICATE", fmt.Sprintf("spec.variants[%d].name", i), fmt.Sprintf("variant %q is declared twice", v.Name), "")
		}
		seen[v.Name] = true
	}
	guards := map[string]bool{}
	for i, g := range x.Spec.Guardrails {
		p := fmt.Sprintf("spec.guardrails[%d]", i)
		if guards[g.Name] {
			add("DUPLICATE", p+".name", fmt.Sprintf("guardrail %q is declared twice", g.Name), "")
		}
		guards[g.Name] = true
		if _, err := metrics.ParseDuration(g.Within); err != nil {
			add("INVALID_FIELD", p+".within", err.Error(), "e.g. 14d or 72h")
		}
	}
	switch o := x.Spec.OnBreach; {
	case len(x.Spec.Guardrails) > 0 && o == nil:
		add("MISSING_FIELD", "spec.onBreach", "guardrails stop the experiment and someone must hear of it", "set onBreach.function, e.g. a Function that escalates to the team")
	case o != nil && len(x.Spec.Guardrails) == 0:
		add("UNUSED_FIELD", "spec.onBreach", "no guardrails: nothing can breach", "add guardrails or remove it")
	case o != nil:
		if _, ok := functions[ns+"/"+o.Function]; !ok {
			add("UNKNOWN_FUNCTION", "spec.onBreach.function", fmt.Sprintf("no Function %q in namespace %s", o.Function, ns), "")
		}
	}
	f, ok := funnels[ns+"/"+x.Spec.Primary.Funnel]
	if !ok {
		add("UNKNOWN_FUNNEL", "spec.primary.funnel", fmt.Sprintf("no Funnel %q in namespace %s", x.Spec.Primary.Funnel, ns), "")
		return issues
	}
	if f.Spec.Subject != x.Spec.Subject {
		add("INVALID_FIELD", "spec.primary.funnel", fmt.Sprintf("funnel %s follows %s, the experiment assigns %s", f.Metadata.Name, f.Spec.Subject, x.Spec.Subject), "")
	}
	found := false
	for i, st := range f.Spec.Stages {
		if st.Name == x.Spec.Primary.Stage {
			found = true
			if i == 0 {
				add("INVALID_FIELD", "spec.primary.stage", "the entry stage cannot be the outcome", "pick a later stage")
			} else if st.Within == "" {
				add("MISSING_FIELD", "spec.primary.stage", fmt.Sprintf("stage %s has no within: nobody can fail to reach it, so every variant would convert 100%%", st.Name), "set within on that stage of the funnel")
			}
		}
	}
	if !found {
		add("UNKNOWN_PARAM", "spec.primary.stage", fmt.Sprintf("funnel %s has no stage %q", f.Metadata.Name, x.Spec.Primary.Stage), "")
	}
	return issues
}
