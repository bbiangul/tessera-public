package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/expr"
)

// Program is a set of admitted resources compiled for execution. Build it
// only from documents that passed admission: Build compiles, it does not
// re-check the rules admission enforces.
type Program struct {
	functions     map[string]*function // namespace/name
	actions       map[string]plugins.ActionSpec
	types         map[string]plugins.TypeSpec // plugin.Type
	guards        map[string]*guard           // per action, when it has authorize or verify
	scripts       map[string]*compute.Script
	subjects      map[string]*agents.Subject    // namespace/name
	experiments   map[string]*agents.Experiment // namespace/name
	channels      map[string]*channelRules      // plugin name
	agentChannel  map[string]string             // namespace -> channel plugin
	queues        map[string]*queueSLA          // namespace/name
	queueRoles    map[string][]string           // namespace/name: roles that answer
	queueChannels map[string][]string           // namespace/name: whatsapp
	teams         map[string]*team              // namespace
	// hash covers every resource and script; see Generation.
	hash hash.Hash
	gen  string
}

// Generation identifies exactly what this Program runs: a hash of every
// admitted resource and every script it loaded. A run records the
// generation it started with and is only ever executed by a Program with
// the same generation, so a changed file can never make a replay take a
// different path (RFC-0001, "Run ... preso à versão em que começou").
func (p *Program) Generation() string { return p.gen }

// guard holds an action's compiled manifest expressions.
type guard struct {
	authorize *expr.Expr
	probeArgs *expr.Value
	happened  *expr.Expr
	output    *expr.Expr
}

type function struct {
	def           *agents.Function
	authorize     *expr.Expr
	inputs        []*input // in dependency order
	preconditions []precondition
	action        *action
	steps         []*step
	output        *expr.Template
	outputValue   *expr.Expr
	onError       []strategy

	actionMilestone *milestone
}

type input struct {
	name         string
	typ          string
	format       string // email, phone, cpf: read by pattern, never by a model
	description  string
	sources      []*source
	onUnresolved []strategy
}

type source struct {
	id    string
	reads []string // earlier sources this one reads

	when         *expr.Expr
	fromAction   string
	outputType   string // the action's declared output type
	fromFunction string
	with         *expr.Value
	value        *expr.Expr
	extract      *extractSrc
	ask          *ask
}

type extractSrc struct {
	instructions string
	conversation bool // may come from the user's earlier messages
	future       bool // a date more than a week past moves a year on
}

type ask struct {
	text          *expr.Template
	choices       *expr.Expr
	minConfidence float64
	onLow         string
}

type precondition struct {
	cel     *expr.Expr
	message *expr.Template // may name what was understood ({{ inputs.departDate }})
}

type action struct {
	act     string
	spec    plugins.ActionSpec
	with    *expr.Value
	confirm *expr.Template
	key     *expr.Expr
}

type strategyKind string

const (
	stProbe    strategyKind = "probe"
	stRetry    strategyKind = "retry"
	stFallback strategyKind = "fallback"
	stEscalate strategyKind = "escalate"
	stRespond  strategyKind = "respond"
	stFail     strategyKind = "fail"
)

type strategy struct {
	// when, when set, must hold for the strategy to run (#30).
	when     *expr.Expr
	kind     strategyKind
	attempts int
	backoff  string
	fallback *fallback
	escalate *escalate
	respond  *expr.Template
	message  string // fail
}

type fallback struct {
	function string
	act      string
	spec     plugins.ActionSpec
	with     *expr.Value
	key      *expr.Expr
	useNull  bool
}

type escalate struct {
	queue        string
	task         string
	message      *expr.Template
	whileWaiting *expr.Template
	onDeny       *expr.Template
	route        []escalateRoute
}

// escalateRoute sends the task to queue when the rule holds (RFC-0006).
type escalateRoute struct {
	when  *expr.Expr
	queue string
}

// Build compiles admitted documents.
func Build(docs []admission.Doc) (*Program, error) {
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	p := &Program{functions: map[string]*function{}, actions: map[string]plugins.ActionSpec{}, types: map[string]plugins.TypeSpec{}, guards: map[string]*guard{},
		scripts: map[string]*compute.Script{}, subjects: map[string]*agents.Subject{}, experiments: map[string]*agents.Experiment{}, hash: sha256.New()}
	for _, d := range docs {
		if d.Subject != nil {
			p.subjects[d.Subject.Metadata.Namespace+"/"+d.Subject.Metadata.Name] = d.Subject
		}
		if d.Experiment != nil {
			p.experiments[d.Experiment.Metadata.Namespace+"/"+d.Experiment.Metadata.Name] = d.Experiment
		}
	}
	// Hash resources in a fixed order, independent of file names and of
	// the order they were loaded in.
	sorted := append([]admission.Doc(nil), docs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID() < sorted[j].ID() })
	for _, d := range sorted {
		// What a run executes: Functions, plugins, Subjects (milestone ids)
		// and the Agent (its messages). Not EvalSuites or Triggers: editing
		// a test or a schedule must not strand runs in flight.
		var body any
		switch {
		case d.Function != nil:
			body = d.Function
		case d.Plugin != nil:
			body = d.Plugin
		case d.Subject != nil:
			body = d.Subject
		case d.Agent != nil:
			body = d.Agent
		case d.Experiment != nil: // variants and weights decide assignments
			body = d.Experiment
		case d.Queue != nil: // deadlines decide when a run moves on
			body = d.Queue
		default:
			continue
		}
		data, err := json.Marshal(body) // map keys are sorted: canonical
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(p.hash, "%s\n%d\n", d.ID(), len(data))
		p.hash.Write(data)
		if d.Function != nil {
			// The written order of inputs decides which is asked first; it
			// is not in the JSON, so it is hashed on its own.
			fmt.Fprintf(p.hash, "order %q\n", d.Function.InputOrder)
		}
	}
	for _, d := range docs {
		if d.Plugin == nil {
			continue
		}
		for name, t := range d.Plugin.Spec.Types {
			p.types[d.Plugin.Metadata.Name+"."+name] = t
		}
		for _, name := range sortedKeys(d.Plugin.Spec.Actions) {
			a := d.Plugin.Spec.Actions[name]
			ref := d.Plugin.Metadata.Name + "/" + name
			p.actions[ref] = a
			c := &compiler{e: e}
			g := &guard{authorize: c.expr(a.Authorize, expr.Bool, ref+".authorize")}
			if v := a.Verify; v != nil && v.Mode == plugins.VerifyProbe {
				g.probeArgs = c.value(v.Args, ref+".verify.args")
				g.happened = c.expr(v.Happened, expr.Bool, ref+".verify.happened")
				g.output = c.expr(v.Output, expr.Any, ref+".verify.output")
			}
			if c.err != nil {
				return nil, fmt.Errorf("%s: %w", d.ID(), c.err)
			}
			p.guards[ref] = g
		}
	}
	for _, d := range docs {
		if d.Function == nil {
			continue
		}
		fn, err := p.compile(e, d.Function, d.File)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.ID(), err)
		}
		p.functions[d.Function.Metadata.Namespace+"/"+d.Function.Metadata.Name] = fn
	}
	p.gen = hex.EncodeToString(p.hash.Sum(nil))[:16]
	manifests := map[string]*plugins.PluginManifest{}
	var agentDocs []*agents.Agent
	for _, d := range docs {
		if d.Plugin != nil {
			manifests[d.Plugin.Metadata.Name] = d.Plugin
		}
		if d.Agent != nil {
			agentDocs = append(agentDocs, d.Agent)
		}
	}
	if err := p.compileChannels(e, manifests, agentDocs); err != nil {
		return nil, err
	}
	zones := map[string]string{}
	for _, a := range agentDocs {
		zones[a.Metadata.Namespace] = a.Spec.Timezone
	}
	var queueDocs []*agents.HumanQueue
	for _, d := range docs {
		if d.Queue != nil {
			queueDocs = append(queueDocs, d.Queue)
		}
	}
	if err := p.compileQueues(queueDocs, zones); err != nil {
		return nil, err
	}
	var teamDocs []*agents.Team
	for _, d := range docs {
		if d.Team != nil {
			teamDocs = append(teamDocs, d.Team)
		}
	}
	p.compileTeams(teamDocs, queueDocs)
	return p, nil
}

// Routable is a Function that events can start: it has a selector.
type Routable struct {
	Namespace   string
	Name        string
	Intent      string
	Description string
	After       []string // routable only when the last run was one of these
	Who         string   // offered only to callers this bool expression holds for (RFC-0006)
}

// Routable lists the routable Functions of a namespace, sorted by name.
func (p *Program) Routable(namespace string) []Routable {
	var out []Routable
	for _, fn := range p.functions {
		d := fn.def
		if d.Metadata.Namespace != namespace || d.Spec.Selector == nil {
			continue
		}
		desc := d.Spec.Selector.Description
		if desc == "" {
			desc = d.Spec.Description
		}
		out = append(out, Routable{Namespace: namespace, Name: d.Metadata.Name, Intent: d.Spec.Selector.Intent, Description: desc, After: d.Spec.Selector.After, Who: string(d.Spec.Selector.Who)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// loadScript loads a Starlark file once and adds it to the generation.
func (p *Program) loadScript(c *compiler, path, field string) *compute.Script {
	if c.err != nil {
		return nil
	}
	if s, ok := p.scripts[path]; ok {
		return s
	}
	src, err := os.ReadFile(path)
	if err != nil {
		c.err = fmt.Errorf("%s: %w", field, err)
		return nil
	}
	s, err := compute.Compile(filepath.Base(path), src, compute.Limits{})
	if err != nil {
		c.err = fmt.Errorf("%s: %w", field, err)
		return nil
	}
	fmt.Fprintf(p.hash, "script\n%s\n%d\n", filepath.Base(path), len(src))
	p.hash.Write(src)
	p.scripts[path] = s
	return s
}

// compiler collects the first error, so compile reads as a list of fields.
type compiler struct {
	e   *expr.Engine
	err error
}

func (c *compiler) fail(path, want string) {
	if c.err == nil {
		c.err = fmt.Errorf("%s: want %s", path, want)
	}
}

func (c *compiler) expr(src string, want expr.Result, path string) *expr.Expr {
	if src == "" || c.err != nil {
		return nil
	}
	x, err := c.e.Compile(src, want)
	if err != nil {
		c.err = fmt.Errorf("%s: %w", path, err)
	}
	return x
}

func (c *compiler) template(src, path string) *expr.Template {
	if src == "" || c.err != nil {
		return nil
	}
	t, err := c.e.CompileTemplate(src)
	if err != nil {
		c.err = fmt.Errorf("%s: %w", path, err)
	}
	return t
}

func (c *compiler) value(v map[string]any, path string) *expr.Value {
	if c.err != nil {
		return nil
	}
	if v == nil {
		v = map[string]any{}
	}
	out, err := c.e.CompileValue(v)
	if err != nil {
		c.err = fmt.Errorf("%s: %w", path, err)
	}
	return out
}

func (p *Program) spec(ref, path string, c *compiler) plugins.ActionSpec {
	a, ok := p.actions[ref]
	if !ok && c.err == nil {
		c.err = fmt.Errorf("%s: unknown action %s", path, ref)
	}
	return a
}

func (p *Program) compile(e *expr.Engine, f *agents.Function, file string) (*function, error) {
	c := &compiler{e: e}
	s := f.Spec
	fn := &function{def: f, authorize: c.expr(string(s.Authorize), expr.Bool, "spec.authorize")}
	fn.steps = p.compileSteps(c, file, f.Metadata.Namespace, s.Steps)

	deps := map[string][]string{}
	byName := map[string]*input{}
	for _, name := range sortedKeys(s.Inputs) {
		in := s.Inputs[name]
		path := "spec.inputs." + name
		ci := &input{name: name, typ: in.Type, format: in.Format, description: in.Description, onUnresolved: p.strategies(in.OnUnresolved, path+".onUnresolved", c)}
		var refs []string
		for i, src := range in.Resolve {
			sp := fmt.Sprintf("%s.resolve[%d]", path, i)
			cs := &source{
				id:           src.ID,
				when:         c.expr(string(src.When), expr.Bool, sp+".when"),
				fromAction:   src.FromAction,
				fromFunction: src.FromFunction,
				value:        c.expr(string(src.Value), expr.Any, sp+".value"),
			}
			if src.FromAction != "" || src.FromFunction != "" {
				cs.with = c.value(src.With, sp+".with")
			}
			if src.FromAction != "" {
				cs.outputType = p.spec(src.FromAction, sp+".fromAction", c).Output
			}
			if src.Extract != nil {
				cs.extract = &extractSrc{instructions: src.Extract.Instructions, conversation: src.Extract.Conversation, future: src.Extract.Future}
			}
			if a := src.Ask; a != nil {
				cs.ask = &ask{
					text:          c.template(string(a.Text), sp+".ask.text"),
					choices:       c.expr(string(a.Choices), expr.Any, sp+".ask.choices"),
					minConfidence: a.MinConfidence,
					onLow:         a.OnLowConfidence,
				}
			}
			for _, x := range []interface{ References() []string }{cs.when, cs.value, cs.with} {
				if !isNil(x) {
					refs = append(refs, x.References()...)
				}
			}
			var own []string
			for _, x := range []interface{ References() []string }{cs.when, cs.value, cs.with} {
				if !isNil(x) {
					own = append(own, x.References()...)
				}
			}
			if cs.ask != nil {
				refs = append(refs, cs.ask.text.References()...)
				own = append(own, cs.ask.text.References()...)
				if cs.ask.choices != nil {
					refs = append(refs, cs.ask.choices.References()...)
					own = append(own, cs.ask.choices.References()...)
				}
			}
			seen := map[string]bool{}
			for _, r := range own {
				if root, rest, _ := strings.Cut(r, "."); root == "sources" && rest != "" {
					id, _, _ := strings.Cut(rest, ".")
					if !seen[id] {
						seen[id] = true
						cs.reads = append(cs.reads, id)
					}
				}
			}
			ci.sources = append(ci.sources, cs)
		}
		for _, r := range refs {
			if root, field, _ := strings.Cut(r, "."); root == "inputs" && field != "" {
				dep, _, _ := strings.Cut(field, ".")
				deps[name] = append(deps[name], dep)
			}
		}
		byName[name] = ci
	}
	order, err := topo(declared(f.InputOrder, s.Inputs), deps)
	if err != nil {
		return nil, err
	}
	for _, n := range order {
		fn.inputs = append(fn.inputs, byName[n])
	}

	for i, pc := range s.Preconditions {
		fn.preconditions = append(fn.preconditions, precondition{
			cel: c.expr(string(pc.CEL), expr.Bool, fmt.Sprintf("spec.preconditions[%d].cel", i)), message: c.template(pc.Message, fmt.Sprintf("spec.preconditions[%d].message", i)),
		})
	}
	if a := s.Action; a != nil {
		fn.action = &action{
			act:     a.Act,
			spec:    p.spec(a.Act, "spec.action.act", c),
			with:    c.value(a.With, "spec.action.with"),
			confirm: c.template(string(a.Confirm), "spec.action.confirm"),
			key:     c.expr(string(a.IdempotencyKey), expr.String, "spec.action.idempotencyKey"),
		}
	}
	if s.Output != nil {
		fn.output = c.template(string(s.Output.Template), "spec.output.template")
		fn.outputValue = c.expr(string(s.Output.Value), expr.Any, "spec.output.value")
	}
	fn.onError = p.strategies(s.OnError, "spec.onError", c)
	if s.Action != nil {
		fn.actionMilestone = p.milestone(c, f.Metadata.Namespace, s.Action.Milestone, "spec.action.milestone")
	}
	return fn, c.err
}

func (p *Program) strategies(list []agents.ErrorStrategy, path string, c *compiler) []strategy {
	var out []strategy
	for i, s := range list {
		sp := fmt.Sprintf("%s[%d]", path, i)
		n := len(out)
		switch {
		case s.Probe != nil:
			out = append(out, strategy{kind: stProbe})
		case s.Retry != nil:
			out = append(out, strategy{kind: stRetry, attempts: s.Retry.Attempts, backoff: s.Retry.Backoff})
		case s.Fallback != nil:
			fb := &fallback{function: s.Fallback.Function, act: s.Fallback.Act, useNull: s.Fallback.UseNull}
			if fb.act != "" {
				fb.spec = p.spec(fb.act, sp+".fallback.act", c)
			}
			if fb.act != "" || fb.function != "" {
				fb.with = c.value(s.Fallback.With, sp+".fallback.with")
			}
			fb.key = c.expr(string(s.Fallback.IdempotencyKey), expr.String, sp+".fallback.idempotencyKey")
			out = append(out, strategy{kind: stFallback, fallback: fb})
		case s.Escalate != nil:
			out = append(out, strategy{kind: stEscalate, escalate: compileEscalate(c, s.Escalate, sp+".escalate")})
		case s.Respond != nil:
			out = append(out, strategy{kind: stRespond, respond: c.template(string(s.Respond.Template), sp+".respond")})
		case s.Fail != nil:
			out = append(out, strategy{kind: stFail, message: s.Fail.Message})
		case s.Compensate != nil:
			if c.err == nil {
				c.err = fmt.Errorf("%s: compensate is not supported by the runtime yet", sp)
			}
		}
		if s.When != "" && len(out) > n {
			out[len(out)-1].when = c.expr(string(s.When), expr.Bool, sp+".when")
		}
	}
	return out
}

// topo orders inputs so each comes after the inputs it reads; ties keep
// name order, so the order is deterministic.
func topo(names []string, deps map[string][]string) ([]string, error) {
	state := map[string]int{}
	var out []string
	var visit func(n string, stack []string) error
	visit = func(n string, stack []string) error {
		switch state[n] {
		case 1:
			return fmt.Errorf("inputs depend on each other: %s", strings.Join(append(stack, n), " → "))
		case 2:
			return nil
		}
		state[n] = 1
		ds := append([]string(nil), deps[n]...)
		sort.Strings(ds)
		for _, d := range ds {
			if err := visit(d, append(stack, n)); err != nil {
				return err
			}
		}
		state[n] = 2
		out = append(out, n)
		return nil
	}
	for _, n := range names {
		if err := visit(n, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// declared lists the inputs in written order; inputs the loader did not
// see (built in code) follow in name order.
func declared(order []string, inputs map[string]agents.Input) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range order {
		if _, ok := inputs[n]; ok && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range sortedKeys(inputs) {
		if !seen[n] {
			out = append(out, n)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// isNil reports whether an interface holds a nil pointer.
func isNil(x interface{ References() []string }) bool {
	switch v := x.(type) {
	case *expr.Expr:
		return v == nil
	case *expr.Value:
		return v == nil
	case *expr.Template:
		return v == nil
	}
	return x == nil
}
