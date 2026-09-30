package admission

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/expr"
)

// checkSteps checks a Function's steps in order and returns the ids every
// later part of the function may read.
func (f *fnCheck) checkSteps(steps []agents.Step) map[string]bool {
	visible := map[string]bool{}
	for i, st := range steps {
		path := fmt.Sprintf("spec.steps[%d]", i)
		if visible[st.ID] {
			f.err("DUPLICATE_ID", path+".id", fmt.Sprintf("step id %q is used twice", st.ID), "give each step a unique id")
		}
		sc := scope{steps: copySet(visible)}
		f.checkStep(st, path, sc)
		visible[st.ID] = true
	}
	return visible
}

func copySet(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func (f *fnCheck) checkStep(st agents.Step, path string, sc scope) {
	kinds := map[string]bool{
		"compute": st.Compute != nil, "act": st.Act != "", "call": st.Call != "", "decide": st.Decide != nil, "decideBatch": st.DecideBatch != nil,
		"extract": st.Extract != nil, "generate": st.Generate != nil,
		"ask": st.Ask != nil, "respond": st.Respond != nil, "escalate": st.Escalate != nil, "branch": st.Branch != nil,
		"wait": st.Wait != nil, "experiment": st.Experiment != "", "retrieve": st.Retrieve != nil,
	}
	var set []string
	for k, v := range kinds {
		if v {
			set = append(set, k)
		}
	}
	sort.Strings(set)
	if len(set) != 1 {
		f.err("STEP_KIND", path, fmt.Sprintf("a step needs exactly one of compute, act, call, decide, decideBatch, extract, generate, ask, respond, escalate, branch, wait, experiment, retrieve; found %d (%s)", len(set), strings.Join(set, ", ")), "split it into separate steps")
		return
	}
	if st.When != "" {
		f.expression(string(st.When), expr.Bool, path+".when", sc)
	}
	f.checkMilestone(st.Milestone, path+".milestone", sc)
	if st.With != nil && st.Act == "" && st.Call == "" {
		f.err("UNUSED_FIELD", path+".with", "with only applies to act or call", "remove it")
	}
	if st.Act == "" {
		if st.IdempotencyKey != "" {
			f.err("UNUSED_FIELD", path+".idempotencyKey", "idempotencyKey only applies to act", "remove it")
		}
		if st.Confirm != "" {
			f.err("UNUSED_FIELD", path+".confirm", "confirm only applies to act", "remove it")
		}
	}

	var action *plugins.ActionSpec
	unresolved := false
	switch {
	case st.Compute != nil:
		f.checkCompute(st.Compute, path+".compute", sc)
	case st.Act != "":
		action = f.checkEffect(st.Act, st.With, st.IdempotencyKey, st.Confirm, path, sc)
		unresolved = action == nil
	case st.Call != "":
		f.checkFunctionRef(st.Call, st.With, path+".call", path+".with", sc)
	case st.DecideBatch != nil:
		b := st.DecideBatch
		f.value(b.State, path+".decideBatch.state", sc)
		for _, id := range sortedKeys(b.Questions) {
			q := b.Questions[id]
			qp := path + ".decideBatch.questions." + id
			if strings.TrimSpace(id) == "" {
				f.err("INVALID_FIELD", qp, "question needs a nonempty name", "")
			}
			f.checkDecide(&agents.DecideStep{Instructions: q.Instructions, Choice: q.Choice, YesNo: q.YesNo, Score: q.Score}, qp, sc)
		}
	case st.Decide != nil:
		f.checkDecide(st.Decide, path+".decide", sc)
	case st.Extract != nil:
		if st.Extract.Schema == nil {
			f.err("MISSING_FIELD", path+".extract.schema", "extract needs the JSON Schema of what it fills", "")
		}
		f.checkGenerate(st.Extract, path+".extract", sc)
	case st.Generate != nil:
		f.checkGenerate(st.Generate, path+".generate", sc)
	case st.Ask != nil:
		f.template(string(st.Ask.Text), path+".ask.text", sc)
		if st.Ask.Choices == "" {
			f.err("MISSING_FIELD", path+".ask.choices", "an ask step needs choices", "set choices to a list expression")
		} else {
			f.expression(string(st.Ask.Choices), expr.Any, path+".ask.choices", sc)
		}
	case st.Respond != nil:
		f.template(string(st.Respond.Template), path+".respond.template", sc)
	case st.Escalate != nil:
		f.checkEscalate(st.Escalate, path+".escalate", sc)
	case st.Branch != nil:
		f.checkBranch(st.Branch, path+".branch", sc)
	case st.Experiment != "":
		x, ok := f.experiments[f.fn.Metadata.Namespace+"/"+st.Experiment]
		if !ok {
			f.err("UNKNOWN_EXPERIMENT", path+".experiment", fmt.Sprintf("no Experiment %q in namespace %s", st.Experiment, f.fn.Metadata.Namespace), "")
			break
		}
		if s, ok := f.subjects[f.fn.Metadata.Namespace+"/"+x.Spec.Subject]; ok {
			f.expression(string(s.Spec.ID), expr.Any, path+" (the Subject's spec.id)", sc)
		}
	case st.Retrieve != nil:
		for i, b := range st.Retrieve.Bases {
			if !f.bases[f.fn.Metadata.Namespace+"/"+b] {
				f.err("UNKNOWN_KNOWLEDGE", fmt.Sprintf("%s.retrieve.bases[%d]", path, i), fmt.Sprintf("no KnowledgeBase %q in namespace %s", b, f.fn.Metadata.Namespace), "")
			}
		}
		if st.Retrieve.K < 0 || st.Retrieve.K > 10 {
			f.err("INVALID_FIELD", path+".retrieve.k", "k is between 1 and 10", "")
		}
		f.expression(string(st.Retrieve.Query), expr.String, path+".retrieve.query", sc)
	case st.Wait != nil:
		if st.Wait.Relay && !st.Wait.OrReply {
			f.err("INVALID_FIELD", path+".wait.relay", "relay requires orReply", "set orReply to true")
		}
		if st.Wait.IncludeSideMessages && !st.Wait.OrReply {
			f.err("INVALID_FIELD", path+".wait.includeSideMessages", "includeSideMessages requires orReply", "set orReply to true")
		}
		switch {
		case (st.Wait.For == "") == (st.Wait.Until == ""):
			f.err("STEP_KIND", path+".wait", "a wait needs exactly one of for and until", "")
		case st.Wait.For != "":
			if d, err := time.ParseDuration(st.Wait.For); err != nil || d <= 0 {
				f.err("INVALID_FIELD", path+".wait.for", fmt.Sprintf("%q is not a positive duration", st.Wait.For), "write it as 90m or 36h; for days use until with addBusinessDays or event.time + duration('72h')")
			}
		default:
			f.expression(string(st.Wait.Until), expr.Any, path+".wait.until", sc)
		}
		seen := map[string]bool{}
		for i, ev := range st.Wait.Events {
			p := fmt.Sprintf("%s.wait.events[%d]", path, i)
			if seen[ev.Name] {
				f.err("DUPLICATE", p+".name", fmt.Sprintf("event %q is listed twice", ev.Name), "")
			}
			seen[ev.Name] = true
			f.expression(string(ev.Key), expr.Any, p+".key", sc)
		}
	}
	// A step's onError may read the steps before it, not the step itself:
	// it failed.
	f.checkErrorChain(st.OnError, path+".onError", action, unresolved, false, sc)
}

func (f *fnCheck) checkCompute(c *agents.Compute, path string, sc scope) {
	if (c.CEL != "") == (c.Starlark != nil) {
		f.err("STEP_KIND", path, "compute needs exactly one of cel and starlark", "")
		return
	}
	if c.CEL != "" {
		f.expression(string(c.CEL), expr.Any, path+".cel", sc)
		return
	}
	sl := c.Starlark
	f.value(sl.With, path+".starlark.with", sc)
	script, err := f.c.script(filepath.Join(filepath.Dir(f.doc.File), sl.Script))
	if err != nil {
		f.err("INVALID_SCRIPT", path+".starlark.script", err.Error(), "the path is relative to this file")
		return
	}
	sig, ok := script.Signature(sl.Function)
	if !ok {
		f.err("UNKNOWN_FUNCTION", path+".starlark.function", fmt.Sprintf("%s has no public function %q", sl.Script, sl.Function), "functions: "+strings.Join(script.Functions(), ", "))
		return
	}
	params := map[string]bool{}
	for _, p := range sig.Params {
		params[p] = true
	}
	for _, k := range sortedKeys(sl.With) {
		if !params[k] && !sig.Kwargs {
			f.err("UNKNOWN_PARAM", path+".starlark.with."+k, fmt.Sprintf("%s has no parameter %q", sl.Function, k), "parameters: "+strings.Join(sig.Params, ", "))
		}
	}
	for _, p := range sig.Required {
		if _, ok := sl.With[p]; !ok {
			f.err("MISSING_PARAM", path+".starlark.with", fmt.Sprintf("%s needs parameter %q", sl.Function, p), "")
		}
	}
}

func (f *fnCheck) checkDecide(d *agents.DecideStep, path string, sc scope) {
	n := 0
	for _, b := range []bool{d.Choice != nil, d.YesNo != nil, d.Score != nil} {
		if b {
			n++
		}
	}
	if n != 1 {
		f.err("STEP_KIND", path, fmt.Sprintf("decide needs exactly one of choice, yesNo, score; found %d", n), "")
	}
	for _, k := range sortedKeys(d.Choice) {
		if k == "" || strings.TrimSpace(d.Choice[k]) == "" {
			f.err("INVALID_FIELD", path+".choice", fmt.Sprintf("option %q needs a key and a description", k), "")
		}
	}
	f.value(d.State, path+".state", sc)
}

func (f *fnCheck) checkGenerate(g *agents.GenerateStep, path string, sc scope) {
	f.value(g.State, path+".state", sc)
	if g.Schema != nil {
		req := generate.Request{Instructions: g.Instructions, Schema: g.Schema}
		if err := req.Validate(); err != nil {
			f.err("INVALID_SCHEMA", path+".schema", err.Error(), "write a JSON Schema (draft 2020-12)")
		}
	}
}

func (f *fnCheck) checkBranch(cases []agents.BranchCase, path string, sc scope) {
	if len(cases) == 0 {
		f.err("MISSING_FIELD", path, "a branch needs at least one case and an else", "")
		return
	}
	for i, c := range cases {
		p := fmt.Sprintf("%s[%d]", path, i)
		last := i == len(cases)-1
		switch {
		case c.Else && c.When != "":
			f.err("STEP_KIND", p, "a case sets when or else, not both", "")
		case !c.Else && c.When == "":
			f.err("STEP_KIND", p, "a case needs when, or else for the last case", "")
		case c.Else && !last:
			f.err("STEP_KIND", p, "else must be the last case", "move it to the end")
		case !c.Else && last:
			f.err("MISSING_FIELD", p, "a branch must end with an else case, so every situation is covered", "add a last case with else: true")
		}
		if c.When != "" {
			f.expression(string(c.When), expr.Bool, p+".when", sc)
		}
		targets := 0
		for _, b := range []bool{c.Call != "", c.Respond != nil, c.Escalate != nil} {
			if b {
				targets++
			}
		}
		if targets != 1 {
			f.err("STEP_KIND", p, fmt.Sprintf("a case needs exactly one of call, respond, escalate; found %d", targets), "")
		}
		if c.With != nil && c.Call == "" {
			f.err("UNUSED_FIELD", p+".with", "with only applies to call", "remove it")
		}
		switch {
		case c.Call != "":
			f.checkFunctionRef(c.Call, c.With, p+".call", p+".with", sc)
		case c.Respond != nil:
			f.template(string(c.Respond.Template), p+".respond.template", sc)
		case c.Escalate != nil:
			f.checkEscalate(c.Escalate, p+".escalate", sc)
		}
	}
}

// scriptCache compiles each script once per Checker.
type scriptCache struct {
	mu      sync.Mutex
	scripts map[string]scriptEntry
}

type scriptEntry struct {
	script *compute.Script
	err    error
}

// MaxScriptBytes bounds a Starlark file.
const MaxScriptBytes = 1 << 20

func (c *Checker) script(path string) (*compute.Script, error) {
	c.scripts.mu.Lock()
	defer c.scripts.mu.Unlock()
	if c.scripts.scripts == nil {
		c.scripts.scripts = map[string]scriptEntry{}
	}
	if e, ok := c.scripts.scripts[path]; ok {
		return e.script, e.err
	}
	s, err := LoadScript(path)
	c.scripts.scripts[path] = scriptEntry{s, err}
	return s, err
}

// LoadScript reads and compiles a Starlark file, with the size limit.
func LoadScript(path string) (*compute.Script, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("script %s does not exist", path)
		}
		return nil, err
	}
	if info.Size() > MaxScriptBytes {
		return nil, fmt.Errorf("script %s is larger than %d bytes", path, MaxScriptBytes)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return compute.Compile(filepath.Base(path), src, compute.Limits{})
}
