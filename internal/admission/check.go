package admission

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
)

// freeScopes can be read by any expression in a Function.
var freeScopes = map[string]bool{"event": true, "caller": true, "conversation": true, "agent": true, "env": true}

// Checker runs the semantic checks.
type Checker struct {
	expr    *expr.Engine
	scripts scriptCache
}

// NewChecker builds a Checker.
func NewChecker() (*Checker, error) {
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	return &Checker{expr: e}, nil
}

// Check validates documents together: functions against plugin manifests
// and against each other.
func (c *Checker) Check(docs []Doc) []Issue {
	var issues []Issue
	pluginsByName := map[string]*plugins.PluginManifest{}
	functions := map[string]*agents.Function{} // namespace/name
	for _, d := range docs {
		switch {
		case d.Plugin != nil:
			if _, dup := pluginsByName[d.Plugin.Metadata.Name]; dup {
				issues = append(issues, Issue{Severity: SeverityError, Code: "DUPLICATE", File: d.File, Resource: d.ID(), Message: "plugin declared twice"})
			}
			pluginsByName[d.Plugin.Metadata.Name] = d.Plugin
		case d.Function != nil:
			key := d.Function.Metadata.Namespace + "/" + d.Function.Metadata.Name
			if _, dup := functions[key]; dup {
				issues = append(issues, Issue{Severity: SeverityError, Code: "DUPLICATE", File: d.File, Resource: d.ID(), Message: "function declared twice"})
			}
			functions[key] = d.Function
		}
	}
	r := c.checkResources(docs, pluginsByName, functions)
	issues = append(issues, r.issues...)
	subjects, experiments := r.subjects, r.experiments
	agentsByNS := map[string]bool{}
	for _, d := range docs {
		if d.Plugin != nil {
			issues = append(issues, c.checkPlugin(d)...)
		}
		if d.Agent != nil {
			if agentsByNS[d.Agent.Metadata.Namespace] {
				issues = append(issues, Issue{Severity: SeverityError, Code: "DUPLICATE", File: d.File, Resource: d.ID(), Path: "metadata.namespace",
					Message: "a namespace has at most one Agent", Fix: "merge them"})
			}
			agentsByNS[d.Agent.Metadata.Namespace] = true
		}
	}
	issues = append(issues, c.checkKnowledge(docs)...)
	issues = append(issues, c.checkWindows(docs, pluginsByName, functions)...)
	kbases := map[string]bool{}
	for _, d := range docs {
		if d.Knowledge != nil {
			kbases[d.Knowledge.Metadata.Namespace+"/"+d.Knowledge.Metadata.Name] = true
		}
	}
	for _, d := range docs {
		if d.Function != nil {
			fc := &fnCheck{c: c, doc: d, fn: d.Function, plugins: pluginsByName, functions: functions, subjects: subjects, experiments: experiments, bases: kbases}
			fc.run()
			issues = append(issues, fc.issues...)
		}
		if d.EvalSuite != nil {
			issues = append(issues, c.checkEvalSuite(d, pluginsByName, functions, triggerNames(docs))...)
		}
	}
	return issues
}

func (c *Checker) checkPlugin(d Doc) []Issue {
	issues := checkPluginTypes(d)
	p := d.Plugin
	if ch := p.Spec.Channel; ch != nil {
		if ch.Window != "" {
			if w, err := time.ParseDuration(ch.Window); err != nil || w <= 0 {
				issues = append(issues, Issue{Severity: SeverityError, Code: "INVALID_FIELD", File: d.File, Resource: d.ID(), Path: "spec.channel.window", Message: fmt.Sprintf("%q is not a positive duration", ch.Window), Fix: "e.g. 24h"})
			}
		}
		if ch.Consent != "" {
			if _, err := c.expr.Compile(ch.Consent, expr.Bool); err != nil {
				issues = append(issues, Issue{Severity: SeverityError, Code: "INVALID_EXPR", File: d.File, Resource: d.ID(), Path: "spec.channel.consent", Message: err.Error()})
			}
		}
	}
	for _, name := range sortedKeys(p.Spec.Actions) {
		if a := p.Spec.Actions[name]; a.FreeText && a.Effect != plugins.EffectExternalMessage {
			issues = append(issues, Issue{Severity: SeverityError, Code: "INVALID_FIELD", File: d.File, Resource: d.ID(), Path: "spec.actions." + name + ".freeText", Message: "freeText only applies to external-message actions", Fix: "remove it or declare effect external-message"})
		}
	}
	names := sortedKeys(p.Spec.Actions)
	for _, name := range names {
		a := p.Spec.Actions[name]
		path := "spec.actions." + name
		add := func(code, sub, msg, fix string) {
			issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path + sub, Message: msg, Fix: fix})
		}
		if a.Verify != nil && a.Verify.Mode == plugins.VerifyProbe {
			probe, ok := p.Spec.Actions[a.Verify.With]
			switch {
			case a.Verify.With == "":
				add("MISSING_FIELD", ".verify.with", "verify mode probe needs the read action that checks the effect", "set verify.with to a read action of this plugin")
			case !ok:
				add("UNKNOWN_ACTION", ".verify.with", fmt.Sprintf("action %q does not exist in this plugin", a.Verify.With), "use one of "+strings.Join(names, ", "))
			case probe.Effect != plugins.EffectRead:
				add("EFFECT_NOT_ALLOWED", ".verify.with", fmt.Sprintf("probe action %q must be read, is %s", a.Verify.With, probe.Effect), "")
			default:
				for _, k := range sortedKeys(a.Verify.Args) {
					if _, ok := probe.Input[k]; !ok {
						add("UNKNOWN_PARAM", ".verify.args."+k, fmt.Sprintf("probe action %q has no parameter %q", a.Verify.With, k), "parameters: "+strings.Join(sortedKeys(probe.Input), ", "))
					}
				}
				for _, k := range sortedKeys(probe.Input) {
					if _, ok := a.Verify.Args[k]; !ok {
						add("MISSING_PARAM", ".verify.args", fmt.Sprintf("probe action %q needs parameter %q", a.Verify.With, k), "")
					}
				}
			}
			if v, err := c.expr.CompileValue(a.Verify.Args); err != nil {
				add("INVALID_EXPR", ".verify.args", err.Error(), "")
			} else {
				pluginRefs(v.References(), a, callScopes, "", ".verify.args", add)
			}
			if a.Verify.Happened == "" {
				add("MISSING_FIELD", ".verify.happened", "verify mode probe needs happened: an expression on sources.probe.output that says whether the effect happened", "")
			}
			for _, f := range []struct {
				src, sub string
				want     expr.Result
			}{{a.Verify.Happened, ".verify.happened", expr.Bool}, {a.Verify.Output, ".verify.output", expr.Any}} {
				if f.src == "" {
					continue
				}
				x, err := c.expr.Compile(f.src, f.want)
				if err != nil {
					add("INVALID_EXPR", f.sub, err.Error(), "")
					continue
				}
				pluginRefs(x.References(), a, callScopes, "probe", f.sub, add)
			}
		}
		if a.Compensate != "" {
			if _, ok := p.Spec.Actions[a.Compensate]; !ok {
				add("UNKNOWN_ACTION", ".compensate", fmt.Sprintf("action %q does not exist in this plugin", a.Compensate), "use one of "+strings.Join(names, ", "))
			}
		}
		if a.Authorize != "" {
			x, err := c.expr.Compile(a.Authorize, expr.Bool)
			if err != nil {
				add("INVALID_EXPR", ".authorize", err.Error(), "")
			}
			pluginRefs(refsOf(x), a, callScopes, "", ".authorize", add)
		}
	}
	issues = append(issues, c.checkHTTP(d)...)
	issues = append(issues, c.checkSQL(d)...)
	return issues
}

// Scopes the runtime provides to manifest expressions, by where they run.
var (
	callScopes     = map[string]bool{"inputs": true, "caller": true, "event": true, "conversation": true} // authorize, verify
	requestScopes  = map[string]bool{"inputs": true, "caller": true}                                      // http path, query, body
	responseScopes = map[string]bool{}                                                                    // http output: sources.response only
)

// pluginRefs checks what a manifest expression reads: only the scopes the
// runtime provides where it runs (a reference to anything else passes
// admission and fails every call), and one named source when the
// expression reads a result (sources.probe in verify, sources.response in
// http.output).
func pluginRefs(refs []string, a plugins.ActionSpec, allowed map[string]bool, source string, sub string, add func(code, sub, msg, fix string)) {
	for _, r := range refs {
		root, field, _ := strings.Cut(r, ".")
		name, _, _ := strings.Cut(field, ".")
		switch {
		case root == "sources" && source != "":
			if name != source && name != "" {
				add("UNRESOLVED_REF", sub, r+": only sources."+source+" exists here", "")
			}
		case !allowed[root]:
			var names []string
			for k := range allowed {
				names = append(names, k)
			}
			sort.Strings(names)
			if source != "" {
				names = append(names, "sources."+source)
			}
			add("SCOPE_NOT_ALLOWED", sub, fmt.Sprintf("%s: this expression can read only %s", r, strings.Join(names, ", ")), "")
		case root == "inputs":
			if _, ok := a.Input[name]; !ok && name != "" {
				add("UNRESOLVED_REF", sub, fmt.Sprintf("%s: the action has no parameter %q", r, name), "parameters: "+strings.Join(sortedKeys(a.Input), ", "))
			}
		}
	}
}

func refsOf(x *expr.Expr) []string {
	if x == nil {
		return nil
	}
	return x.References()
}

// fnCheck holds the state of checking one Function.
type fnCheck struct {
	c           *Checker
	doc         Doc
	fn          *agents.Function
	plugins     map[string]*plugins.PluginManifest
	functions   map[string]*agents.Function
	subjects    map[string]*agents.Subject    // namespace/name
	experiments map[string]*agents.Experiment // namespace/name
	bases       map[string]bool               // KnowledgeBases, namespace/name
	issues      []Issue
	deps        map[string][]string // input -> inputs it reads
}

func (f *fnCheck) add(sev Severity, code, path, msg, fix string) {
	f.issues = append(f.issues, Issue{Severity: sev, Code: code, File: f.doc.File, Resource: f.doc.ID(), Path: path, Message: msg, Fix: fix})
}

func (f *fnCheck) err(code, path, msg, fix string) { f.add(SeverityError, code, path, msg, fix) }

// scope says what an expression at a given place may read.
type scope struct {
	input   string          // the input being resolved, "" outside inputs
	sources map[string]bool // earlier source ids of that input
	steps   map[string]bool // steps readable here; nil where steps are not
}

func (f *fnCheck) run() {
	spec := f.fn.Spec
	f.deps = map[string][]string{}
	if spec.Authorize != "" {
		f.expression(string(spec.Authorize), expr.Bool, "spec.authorize", scope{})
	}
	if spec.Selector != nil {
		if spec.Selector.Who != "" {
			f.expression(string(spec.Selector.Who), expr.Bool, "spec.selector.who", scope{})
		}
		for i, name := range spec.Selector.After {
			if other, ok := f.functions[f.fn.Metadata.Namespace+"/"+name]; !ok || other.Spec.Selector == nil {
				f.err("UNKNOWN_FUNCTION", fmt.Sprintf("spec.selector.after[%d]", i),
					fmt.Sprintf("no routable Function %q in namespace %s", name, f.fn.Metadata.Namespace), "after names Functions whose runs this one follows")
			}
		}
	}
	for _, name := range sortedKeys(spec.Inputs) {
		f.checkInput(name, spec.Inputs[name])
	}
	f.checkCycles()
	for i, p := range spec.Preconditions {
		f.expression(string(p.CEL), expr.Bool, fmt.Sprintf("spec.preconditions[%d].cel", i), scope{})
		f.template(p.Message, fmt.Sprintf("spec.preconditions[%d].message", i), scope{})
	}
	// The action, the output and the function's onError run after every
	// step, so they may read all of them.
	after := scope{steps: f.checkSteps(spec.Steps)}
	var action *plugins.ActionSpec
	if spec.Action != nil {
		action = f.checkAction(*spec.Action, after)
	}
	if o := spec.Output; o != nil {
		if o.Template == "" && o.Value == "" {
			f.err("MISSING_FIELD", "spec.output", "output needs a template, a value, or both", "")
		}
		if o.Template != "" {
			f.template(string(o.Template), "spec.output.template", after)
		}
		if o.Value != "" {
			f.expression(string(o.Value), expr.Any, "spec.output.value", after)
		}
	}
	// When the action itself failed to resolve, its effect rules cannot be
	// judged; reporting them too would only repeat the first error.
	unresolvedAction := spec.Action != nil && action == nil
	f.checkErrorChain(spec.OnError, "spec.onError", action, unresolvedAction, false, after)
}

func (f *fnCheck) checkInput(name string, in agents.Input) {
	base := "spec.inputs." + name
	if msg := f.checkType(in.Type); msg != "" {
		f.err("UNKNOWN_TYPE", base+".type", msg, "use a basic type (string, int, float, bool, date, timestamp, duration, map, any), plugin.Type, or list<T>")
	}
	if in.Format != "" && in.Type != "string" {
		f.err("INVALID_FIELD", base+".format", fmt.Sprintf("format %s reads text; the input's type is %s", in.Format, in.Type), "use type: string")
	}
	sc := scope{input: name, sources: map[string]bool{}}
	for i, s := range in.Resolve {
		path := fmt.Sprintf("%s.resolve[%d]", base, i)
		if in.Format != "" && s.Ask != nil && s.Ask.Choices != "" {
			f.err("INVALID_FIELD", path+".ask.choices", "an input with a format is read from the user's words, not picked from choices", "remove choices or format")
		}
		if sc.sources[s.ID] {
			f.err("DUPLICATE_ID", path+".id", fmt.Sprintf("source id %q is used twice in this input", s.ID), "give each source a unique id")
		}
		kinds := 0
		for _, set := range []bool{s.FromAction != "", s.FromFunction != "", s.Value != "", s.Extract != nil, s.Ask != nil} {
			if set {
				kinds++
			}
		}
		if kinds != 1 {
			f.err("SOURCE_KIND", path, fmt.Sprintf("a source needs exactly one of fromAction, fromFunction, value, extract, ask; found %d", kinds), "split it into separate sources")
		}
		if s.When != "" {
			f.expression(string(s.When), expr.Bool, path+".when", sc)
		}
		if s.Value != "" {
			f.expression(string(s.Value), expr.Any, path+".value", sc)
		}
		if s.With != nil && s.FromAction == "" && s.FromFunction == "" {
			f.err("UNUSED_FIELD", path+".with", "with only applies to fromAction or fromFunction", "remove it")
		}
		if s.FromAction != "" {
			if a := f.lookupAction(s.FromAction, path+".fromAction"); a != nil {
				if a.Effect != plugins.EffectRead {
					f.err("EFFECT_NOT_ALLOWED", path+".fromAction", fmt.Sprintf("sources may only call read actions; %s is %s", s.FromAction, a.Effect), "move it to spec.action or a Workflow step")
				}
				f.checkArgs(s.With, a.Input, path+".with", sc)
			}
		}
		if s.FromFunction != "" {
			f.checkFunctionRef(s.FromFunction, s.With, path+".fromFunction", path+".with", sc)
			if e := f.effectOf(s.FromFunction, map[string]bool{}); e != "" {
				f.err("EFFECT_NOT_ALLOWED", path+".fromFunction", fmt.Sprintf("resolving an input must not change the world, and function %s can (%s)", s.FromFunction, e), "call it from a step or the action instead")
			}
		}
		if s.Ask != nil {
			f.template(string(s.Ask.Text), path+".ask.text", sc)
			if s.Ask.Choices != "" {
				f.expression(string(s.Ask.Choices), expr.Any, path+".ask.choices", sc)
			}
		}
		sc.sources[s.ID] = true
	}
	// Resolving an input must not change the world, so its fallbacks are
	// held to the same read-only rule as its sources. They may read what
	// the sources found, to say why the input could not be resolved.
	f.checkErrorChain(in.OnUnresolved, base+".onUnresolved", nil, false, true, sc)
}

// basicTypes are the types every input may use without a plugin.
var basicTypes = map[string]bool{"string": true, "int": true, "float": true, "bool": true,
	"date": true, "timestamp": true, "duration": true, "map": true, "any": true}

// checkType returns a message when t names nothing known.
func (f *fnCheck) checkType(t string) string {
	if inner, ok := strings.CutPrefix(t, "list<"); ok && strings.HasSuffix(inner, ">") {
		return f.checkType(strings.TrimSuffix(inner, ">"))
	}
	if basicTypes[t] {
		return ""
	}
	if plugin, name, ok := strings.Cut(t, "."); ok && name != "" {
		if _, known := f.plugins[plugin]; known {
			return ""
		}
		return fmt.Sprintf("type %q: no PluginManifest named %q", t, plugin)
	}
	return fmt.Sprintf("unknown type %q", t)
}

func (f *fnCheck) lookupAction(ref, path string) *plugins.ActionSpec {
	pluginName, actionName, ok := strings.Cut(ref, "/")
	if !ok {
		f.err("UNKNOWN_ACTION", path, fmt.Sprintf("%q is not plugin/action", ref), "")
		return nil
	}
	p, ok := f.plugins[pluginName]
	if !ok {
		f.err("UNKNOWN_PLUGIN", path, fmt.Sprintf("no PluginManifest named %q", pluginName), "add the plugin's manifest to the files being validated")
		return nil
	}
	a, ok := p.Spec.Actions[actionName]
	if !ok {
		f.err("UNKNOWN_ACTION", path, fmt.Sprintf("plugin %s has no action %q", pluginName, actionName), "use one of "+strings.Join(sortedKeys(p.Spec.Actions), ", "))
		return nil
	}
	return &a
}

// checkArgs compares with-arguments to an action's parameters and compiles them.
func (f *fnCheck) checkArgs(with map[string]any, params map[string]string, path string, sc scope) {
	for _, k := range sortedKeys(with) {
		if _, ok := params[k]; !ok {
			f.err("UNKNOWN_PARAM", path+"."+k, fmt.Sprintf("the action has no parameter %q", k), "parameters: "+strings.Join(sortedKeys(params), ", "))
		}
	}
	for _, k := range sortedKeys(params) {
		if _, ok := with[k]; !ok {
			f.err("MISSING_PARAM", path, fmt.Sprintf("missing parameter %q (%s)", k, params[k]), "")
		}
	}
	f.value(with, path, sc)
}

// checkFunctionRef checks a call to another Function: it exists, it is not
// this Function (which would recurse forever), and the arguments match.
func (f *fnCheck) checkFunctionRef(name string, with map[string]any, path, withPath string, sc scope) {
	if name == f.fn.Metadata.Name {
		f.err("RECURSION", path, fmt.Sprintf("function %s calls itself", name), "call a different function")
		return
	}
	target, ok := f.functions[f.fn.Metadata.Namespace+"/"+name]
	if !ok {
		f.err("UNKNOWN_FUNCTION", path, fmt.Sprintf("no Function %q in namespace %s", name, f.fn.Metadata.Namespace), "")
		return
	}
	f.checkFunctionArgs(with, target, withPath, sc)
}

// effectOf returns the first effect a Function can have ("p/a is
// irreversible"), following call, fromFunction, branches and fallbacks, or
// "" when it only reads. Resolving an input must not change the world, so
// such a Function cannot be a source or an input's fallback.
func (f *fnCheck) effectOf(name string, seen map[string]bool) string {
	key := f.fn.Metadata.Namespace + "/" + name
	if seen[key] {
		return ""
	}
	seen[key] = true
	fn, ok := f.functions[key]
	if !ok {
		return ""
	}
	act := func(ref string) string {
		plugin, action, _ := strings.Cut(ref, "/")
		if p, ok := f.plugins[plugin]; ok {
			if a, ok := p.Spec.Actions[action]; ok && a.Effect != plugins.EffectRead {
				return fmt.Sprintf("%s is %s", ref, a.Effect)
			}
		}
		return ""
	}
	chain := func(list []agents.ErrorStrategy) string {
		for _, st := range list {
			if fb := st.Fallback; fb != nil {
				if e := act(fb.Act); fb.Act != "" && e != "" {
					return e
				}
				if fb.Function != "" {
					if e := f.effectOf(fb.Function, seen); e != "" {
						return e
					}
				}
			}
		}
		return ""
	}
	if a := fn.Spec.Action; a != nil {
		if e := act(a.Act); e != "" {
			return e
		}
	}
	if e := chain(fn.Spec.OnError); e != "" {
		return e
	}
	for _, in := range fn.Spec.Inputs {
		for _, src := range in.Resolve {
			if src.FromFunction != "" {
				if e := f.effectOf(src.FromFunction, seen); e != "" {
					return e
				}
			}
		}
		if e := chain(in.OnUnresolved); e != "" {
			return e
		}
	}
	for _, st := range fn.Spec.Steps {
		if e := act(st.Act); st.Act != "" && e != "" {
			return e
		}
		if st.Call != "" {
			if e := f.effectOf(st.Call, seen); e != "" {
				return e
			}
		}
		for _, bc := range st.Branch {
			if bc.Call != "" {
				if e := f.effectOf(bc.Call, seen); e != "" {
					return e
				}
			}
		}
		if e := chain(st.OnError); e != "" {
			return e
		}
	}
	return ""
}

func (f *fnCheck) checkFunctionArgs(with map[string]any, target *agents.Function, path string, sc scope) {
	for _, k := range sortedKeys(with) {
		if _, ok := target.Spec.Inputs[k]; !ok {
			f.err("UNKNOWN_PARAM", path+"."+k, fmt.Sprintf("function %s has no input %q", target.Metadata.Name, k), "inputs: "+strings.Join(sortedKeys(target.Spec.Inputs), ", "))
		}
	}
	f.value(with, path, sc)
}

func (f *fnCheck) checkAction(a agents.Action, sc scope) *plugins.ActionSpec {
	f.checkMilestone(a.Milestone, "spec.action.milestone", sc)
	return f.checkEffect(a.Act, a.With, a.IdempotencyKey, a.Confirm, "spec.action", sc)
}

// checkEffect checks a call of a plugin action by the main action or an
// act step: arguments, and the rules of its effect class.
func (f *fnCheck) checkEffect(act string, with map[string]any, key agents.Expr, confirm agents.Template, path string, sc scope) *plugins.ActionSpec {
	spec := f.lookupAction(act, path+".act")
	if spec == nil {
		f.value(with, path+".with", sc)
		return nil
	}
	f.checkArgs(with, spec.Input, path+".with", sc)
	if confirm != "" {
		f.template(string(confirm), path+".confirm", sc)
		f.confirmNames(with, string(confirm), path)
	}
	if key != "" {
		f.expression(string(key), expr.String, path+".idempotencyKey", sc)
	}
	if irreversible(spec.Effect) {
		if key == "" {
			f.err("IDEMPOTENCY_REQUIRED", path+".idempotencyKey", fmt.Sprintf("%s is %s: it needs an idempotency key", act, spec.Effect), "set idempotencyKey to an expression identifying the effect, e.g. inputs.appointment.id + ':' + inputs.newSlot.id")
		}
		// An external message on a channel with a consent rule is
		// authorized by the recipient's consent, checked before every send.
		consented := false
		if p, _, ok := strings.Cut(act, "/"); ok && spec.Effect == plugins.EffectExternalMessage {
			if m := f.plugins[p]; m != nil && m.Spec.Channel != nil && m.Spec.Channel.Consent != "" {
				consented = true
			}
		}
		if confirm == "" && !consented {
			f.add(SeverityWarning, "NO_CONFIRMATION", path+".confirm", fmt.Sprintf("%s is %s and has no confirm question; only the agent's approval policy stands between the model and the effect", act, spec.Effect), "add confirm, unless a human approves every call")
		}
	}
	return spec
}

func (f *fnCheck) checkEscalate(e *agents.Escalate, path string, sc scope) {
	if e.Message != "" {
		f.template(string(e.Message), path+".message", sc)
	}
	if e.WhileWaiting != nil {
		f.template(string(e.WhileWaiting.Template), path+".whileWaiting.template", sc)
	}
	if e.OnDeny != nil {
		if e.Task != "approve" {
			f.err("INVALID_FIELD", path+".onDeny", "onDeny is for task: approve (only an approval is denied)", "remove it, or make the task an approval")
		}
		f.template(string(e.OnDeny.Template), path+".onDeny.template", sc)
	}
	for i, r := range e.Route {
		f.expression(string(r.When), expr.Bool, fmt.Sprintf("%s.route[%d].when", path, i), sc)
	}
}

func irreversible(effect string) bool {
	return effect == plugins.EffectIrreversible || effect == plugins.EffectExternalMessage
}

// expression compiles an expression and checks what it reads.
func (f *fnCheck) expression(src string, want expr.Result, path string, sc scope) {
	x, err := f.c.expr.Compile(src, want)
	if err != nil {
		f.err("INVALID_EXPR", path, err.Error(), "")
		return
	}
	f.refs(x.References(), path, sc)
}

func (f *fnCheck) template(src, path string, sc scope) {
	t, err := f.c.expr.CompileTemplate(src)
	if err != nil {
		f.err("INVALID_TEMPLATE", path, err.Error(), "")
		return
	}
	f.refs(t.References(), path, sc)
}

func (f *fnCheck) value(v map[string]any, path string, sc scope) {
	if v == nil {
		return
	}
	cv, err := f.c.expr.CompileValue(v)
	if err != nil {
		f.err("INVALID_EXPR", path, err.Error(), "")
		return
	}
	f.refs(cv.References(), path, sc)
}

// refs checks that every scope path an expression reads exists here.
func (f *fnCheck) refs(refs []string, path string, sc scope) {
	for _, r := range refs {
		root, field, _ := strings.Cut(r, ".")
		switch {
		case freeScopes[root]:
		case root == "inputs":
			if field == "" {
				continue
			}
			if _, ok := f.fn.Spec.Inputs[field]; !ok {
				f.err("UNRESOLVED_REF", path, fmt.Sprintf("%s: no input named %q", r, field), "declared inputs: "+strings.Join(sortedKeys(f.fn.Spec.Inputs), ", "))
				continue
			}
			if sc.input != "" {
				f.deps[sc.input] = append(f.deps[sc.input], field)
			}
		case root == "sources":
			switch {
			case sc.input == "" && field == "error" && sc.sources["error"]:
				// onError reads the failure (#30)
			case sc.input == "" && strings.HasPrefix(field, "error.") && sc.sources["error"]:
			case sc.input == "":
				f.err("SCOPE_NOT_ALLOWED", path, r+": sources can only be read inside an input's resolve list, or sources.error inside onError", "")
			case !sc.sources[field]:
				f.err("UNRESOLVED_REF", path, fmt.Sprintf("%s: no earlier source %q in input %s", r, field, sc.input), "a source can only read sources listed before it: "+strings.Join(sortedKeys(sc.sources), ", "))
			}
		case root == "steps":
			step, _, _ := strings.Cut(field, ".")
			switch {
			case sc.steps == nil:
				f.err("SCOPE_NOT_ALLOWED", path, r+": steps can be read only by later steps, the action, the output and onError", "")
			case step != "" && !sc.steps[step]:
				f.err("UNRESOLVED_REF", path, fmt.Sprintf("%s: no earlier step %q", r, step), "a step can only read steps listed before it: "+strings.Join(sortedKeys(sc.steps), ", "))
			}
		}
	}
}

// checkCycles reports inputs that depend on each other.
func (f *fnCheck) checkCycles() {
	state := map[string]int{} // 0 new, 1 visiting, 2 done
	var stack []string
	var visit func(string) bool
	visit = func(n string) bool {
		switch state[n] {
		case 1:
			i := indexOf(stack, n)
			cycle := append(append([]string{}, stack[i:]...), n)
			f.err("INPUT_CYCLE", "spec.inputs."+n, "inputs depend on each other: "+strings.Join(cycle, " → "), "break the cycle; an input cannot need itself to resolve")
			return true
		case 2:
			return false
		}
		state[n] = 1
		stack = append(stack, n)
		deps := append([]string(nil), f.deps[n]...)
		sort.Strings(deps)
		for _, d := range deps {
			if visit(d) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = 2
		return false
	}
	for _, n := range sortedKeys(f.fn.Spec.Inputs) {
		if state[n] == 0 && visit(n) {
			return
		}
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// confirmNames warns when a confirmation does not mention every input or
// step the action acts on: the user says yes to what they read, so a
// "Confirm Tuesday at 10?" that does not say which appointment moves can
// move the wrong one (market-bench-001, t15).
func (f *fnCheck) confirmNames(with map[string]any, confirm, path string) {
	cv, err := f.c.expr.CompileValue(with)
	if err != nil {
		return
	}
	t, err := f.c.expr.CompileTemplate(confirm)
	if err != nil {
		return
	}
	said := map[string]bool{}
	for _, r := range t.References() {
		said[refRoot(r)] = true
	}
	var missing []string
	for _, r := range cv.References() {
		root := refRoot(r)
		if (strings.HasPrefix(root, "inputs.") || strings.HasPrefix(root, "steps.")) && !said[root] && !slices.Contains(missing, root) {
			missing = append(missing, root)
		}
	}
	if len(missing) > 0 {
		f.add(SeverityWarning, "CONFIRM_INCOMPLETE", path+".confirm",
			fmt.Sprintf("the confirmation does not mention %s, which the action acts on: the user may confirm something else", strings.Join(missing, ", ")),
			"name each of them in the confirm text, e.g. which appointment and the new time")
	}
}

// refRoot is the input or step a reference reads: inputs.appointment.id
// -> inputs.appointment.
func refRoot(r string) string {
	parts := strings.SplitN(r, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return r
}
