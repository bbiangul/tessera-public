package admission

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/expr"
)

// checkEvalSuite checks a suite against the plugins and Functions it tests.
func (c *Checker) checkEvalSuite(d Doc, pluginsByName map[string]*plugins.PluginManifest, functions map[string]*agents.Function, triggers map[string]bool) []Issue {
	var issues []Issue
	s := d.EvalSuite
	add := func(sev Severity, path, code, msg, fix string) {
		issues = append(issues, Issue{Severity: sev, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	ns := s.Metadata.Namespace
	checkNow := func(v, path string) {
		if v == "" {
			return
		}
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			add(SeverityError, path, "INVALID_FIELD", fmt.Sprintf("%q is not RFC 3339 with an offset", v), "e.g. 2026-09-23T09:00:00-03:00")
		}
	}
	checkNow(s.Spec.Now, "spec.now")
	action := func(ref, path string) {
		pl, name, ok := strings.Cut(ref, "/")
		m, known := pluginsByName[pl]
		if !ok || !known {
			add(SeverityError, path, "UNKNOWN_PLUGIN", fmt.Sprintf("no PluginManifest for %s", ref), "")
			return
		}
		if _, ok := m.Spec.Actions[name]; !ok {
			add(SeverityError, path, "UNKNOWN_ACTION", fmt.Sprintf("plugin %s has no action %q", pl, name), "")
		}
	}
	seenSandbox := map[string]bool{}
	for i, sb := range s.Spec.Sandboxes {
		path := fmt.Sprintf("spec.sandboxes[%d]", i)
		m, ok := pluginsByName[sb.Plugin]
		if !ok {
			add(SeverityError, path+".plugin", "UNKNOWN_PLUGIN", fmt.Sprintf("no PluginManifest named %q", sb.Plugin), "")
			continue
		}
		if seenSandbox[sb.Plugin] {
			add(SeverityError, path+".plugin", "DUPLICATE", fmt.Sprintf("plugin %s has two sandboxes", sb.Plugin), "")
		}
		seenSandbox[sb.Plugin] = true
		file := filepath.Join(filepath.Dir(d.File), sb.Script)
		src, err := os.ReadFile(file)
		if err != nil {
			add(SeverityError, path+".script", "INVALID_SCRIPT", fmt.Sprintf("script %s: %v", sb.Script, err), "the path is relative to this file")
			continue
		}
		script, err := compute.CompileSandbox(filepath.Base(file), src)
		if err != nil {
			add(SeverityError, path+".script", "INVALID_SCRIPT", err.Error(), "")
			continue
		}
		byFunc := map[string]string{}
		for a := range m.Spec.Actions {
			byFunc[compute.ActionFunction(a)] = a
		}
		for _, fn := range script.Functions() {
			if _, ok := byFunc[fn]; !ok {
				add(SeverityWarning, path+".script", "UNUSED_FUNCTION", fmt.Sprintf("%s: %s matches no action of %s", sb.Script, fn, sb.Plugin), "name functions after actions, with - as _")
			}
		}
	}
	ids := map[string]bool{}
	for i, sc := range s.Spec.Scenarios {
		path := fmt.Sprintf("spec.scenarios[%d]", i)
		if ids[sc.ID] {
			add(SeverityError, path+".id", "DUPLICATE_ID", fmt.Sprintf("scenario %q is declared twice", sc.ID), "")
		}
		ids[sc.ID] = true
		for _, pl := range sortedKeys(sc.State) {
			if !seenSandbox[pl] {
				add(SeverityError, path+".state."+pl, "UNRESOLVED_REF", fmt.Sprintf("plugin %s has no sandbox in this suite", pl), "add it to spec.sandboxes")
			}
		}
		checkNow(sc.Now, path+".now")
		if s.Spec.Now == "" && sc.Now == "" {
			add(SeverityError, path+".now", "MISSING_FIELD", "a scenario needs a clock: set spec.now or the scenario's now", "")
		}
		switch {
		case sc.Simulator == "llm" && sc.Persona == nil:
			add(SeverityError, path+".persona", "MISSING_FIELD", "an llm scenario needs a persona: the goal the simulated user pursues", "")
		case sc.Simulator != "llm" && len(sc.Turns) == 0:
			add(SeverityError, path+".turns", "MISSING_FIELD", "a scripted scenario needs at least one turn", "")
		case sc.Simulator != "llm" && (sc.Persona != nil || sc.MaxTurns != 0 || sc.Humans != nil):
			add(SeverityError, path+".persona", "INVALID_FIELD", "persona, maxTurns and humans are for the llm simulator", "set simulator: llm")
		}
		for j, t := range sc.Turns {
			tp := fmt.Sprintf("%s.turns[%d]", path, j)
			set := 0
			for _, b := range []bool{t.User != "", t.Human != nil, t.Advance != "", t.Trigger != nil} {
				if b {
					set++
				}
			}
			if set != 1 {
				add(SeverityError, tp, "STEP_KIND", "a turn sets exactly one of user, human, advance and trigger", "")
			}
			if t.Advance != "" {
				if d, err := time.ParseDuration(t.Advance); err != nil || d <= 0 {
					add(SeverityError, tp+".advance", "INVALID_FIELD", fmt.Sprintf("%q is not a positive duration", t.Advance), "e.g. 48h")
				}
			}
			if t.Trigger != nil && !triggers[ns+"/"+t.Trigger.Name] {
				add(SeverityError, tp+".trigger.name", "UNKNOWN_TRIGGER", fmt.Sprintf("no Trigger %q in namespace %s", t.Trigger.Name, ns), "")
			}
			if t.Expect != nil && t.Expect.Handoff != "" && t.Expect.Handoff != "answer" {
				if _, ok := functions[ns+"/"+t.Expect.Handoff]; !ok {
					add(SeverityError, tp+".expect.handoff", "UNKNOWN_FUNCTION", fmt.Sprintf("no Function %q in namespace %s", t.Expect.Handoff, ns), "\"answer\" or a Function of the Agent's freeMode.functions")
				}
			}
			if t.Expect != nil && t.Expect.Routed != "" {
				if fn, ok := functions[ns+"/"+t.Expect.Routed]; !ok || fn.Spec.Selector == nil {
					add(SeverityError, tp+".expect.routed", "UNKNOWN_FUNCTION", fmt.Sprintf("no routable Function %q in namespace %s", t.Expect.Routed, ns), "a Function the free mode starts is checked with expect.handoff")
				}
			}
		}
		if e := sc.Expect; e != nil {
			for j, ef := range e.Effects {
				action(ef.Action, fmt.Sprintf("%s.expect.effects[%d].action", path, j))
				if ef.Where == "" {
					continue
				}
				wp := fmt.Sprintf("%s.expect.effects[%d].where", path, j)
				x, err := c.expr.Compile(ef.Where, expr.Bool)
				if err != nil {
					add(SeverityError, wp, "INVALID_EXPR", err.Error(), "")
					continue
				}
				for _, ref := range x.References() {
					if scope, _, _ := strings.Cut(ref, "."); scope != "inputs" {
						add(SeverityError, wp, "SCOPE_NOT_ALLOWED", fmt.Sprintf("where reads %s; it can only read the call's arguments, as inputs", ref), "")
					}
				}
			}
			for j, a := range e.Never {
				action(a, fmt.Sprintf("%s.expect.never[%d]", path, j))
			}
		}
	}
	return issues
}
