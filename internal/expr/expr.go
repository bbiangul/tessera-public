// Package expr is the expression layer of RFC-0001 ("Expressões, literais e
// escopos"). It compiles and evaluates CEL with three rules:
//
//  1. A field declared as an expression (when, cel, value, choices,
//     authorize, idempotencyKey) is always CEL, without delimiters: Compile.
//  2. Any other field is a literal, unless its whole value is "${ expr }":
//     CompileValue. A "${" anywhere else is an error, never half-evaluated.
//  3. Text for people interpolates "{{ expr }}" and escapes each value for the
//     channel it goes to: CompileTemplate.
//
// Expressions can only see the scopes listed in Scopes. There is no now():
// time comes from event.time, so re-running a step with the same event gives
// the same result. Every evaluation has a cost limit.
package expr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"
	"cel.dev/cel-go/ext"
)

// Scopes are the only variables an expression can read.
var Scopes = []string{"inputs", "sources", "steps", "event", "caller", "conversation", "agent", "env"}

// Limits that keep a hostile or careless expression from hurting the engine.
const (
	MaxSourceLength  = 4096
	MaxDepth         = 64
	DefaultCostLimit = 100_000
)

// Result is the type an expression must produce.
type Result int

const (
	// Any accepts every result type.
	Any Result = iota
	// Bool is required for conditions (when, preconditions, authorize).
	Bool
	// String is required for keys (idempotencyKey).
	String
)

// Engine compiles expressions against the fixed set of scopes.
type Engine struct {
	env       *cel.Env
	costLimit uint64
}

// Option configures an Engine.
type Option func(*Engine)

// WithCostLimit overrides DefaultCostLimit.
func WithCostLimit(n uint64) Option { return func(e *Engine) { e.costLimit = n } }

// New builds an Engine.
func New(opts ...Option) (*Engine, error) {
	envOpts := []cel.EnvOption{ext.Strings(), ext.Lists()}
	for _, s := range Scopes {
		envOpts = append(envOpts, cel.Variable(s, cel.MapType(cel.StringType, cel.DynType)))
	}
	envOpts = append(envOpts, timeFunctions()...)
	env, err := cel.NewEnv(envOpts...)
	if err != nil {
		return nil, err
	}
	e := &Engine{env: env, costLimit: DefaultCostLimit}
	for _, o := range opts {
		o(e)
	}
	return e, nil
}

// Expr is a compiled expression.
type Expr struct {
	src  string
	want Result
	prog cel.Program
	refs []string
}

// Error is a compile or evaluation error that keeps the expression text.
type Error struct {
	Source string
	Err    error
}

func (e *Error) Error() string { return fmt.Sprintf("expression %q: %v", e.Source, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// ErrCostLimit is returned when an evaluation exceeds its cost limit.
var ErrCostLimit = errors.New("expression cost limit exceeded")

// Compile parses and type-checks a CEL expression that must produce want.
func (e *Engine) Compile(src string, want Result) (*Expr, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, &Error{Source: src, Err: errors.New("empty expression")}
	}
	if len(src) > MaxSourceLength {
		return nil, &Error{Source: src[:40] + "…", Err: fmt.Errorf("longer than %d characters", MaxSourceLength)}
	}
	parsed, iss := e.env.Parse(src)
	if iss.Err() != nil {
		return nil, &Error{Source: src, Err: iss.Err()}
	}
	if celast.ExceedsDepth(parsed.NativeRep(), MaxDepth) {
		return nil, &Error{Source: src, Err: fmt.Errorf("nested deeper than %d", MaxDepth)}
	}
	if v := shadowedScope(parsed.NativeRep()); v != "" {
		return nil, &Error{Source: src, Err: fmt.Errorf("loop variable %q has the name of a scope; rename it", v)}
	}
	checked, iss := e.env.Check(parsed)
	if iss.Err() != nil {
		return nil, &Error{Source: src, Err: iss.Err()}
	}
	if err := checkResult(checked.OutputType(), want); err != nil {
		return nil, &Error{Source: src, Err: err}
	}
	prog, err := e.env.Program(checked, cel.CostLimit(e.costLimit), cel.InterruptCheckFrequency(100))
	if err != nil {
		return nil, &Error{Source: src, Err: err}
	}
	return &Expr{src: src, want: want, prog: prog, refs: references(checked.NativeRep())}, nil
}

func checkResult(got *cel.Type, want Result) error {
	if want == Any || got.IsExactType(cel.DynType) {
		return nil // dyn is checked again at runtime
	}
	var need *cel.Type
	switch want {
	case Bool:
		need = cel.BoolType
	case String:
		need = cel.StringType
	}
	if !got.IsExactType(need) {
		return fmt.Errorf("must produce %s, produces %s", need, got)
	}
	return nil
}

// Source returns the expression text.
func (x *Expr) Source() string { return x.src }

// References lists the scope paths the expression reads, e.g.
// "inputs.newSlot" or "steps.intent", sorted. Admission uses it to reject
// references to inputs or steps that do not exist.
func (x *Expr) References() []string { return x.refs }

// Activation holds the values of the scopes for one evaluation. Missing
// scopes are treated as empty.
type Activation map[string]any

// Eval evaluates the expression and returns a plain Go value: bool, int64,
// uint64, float64, string, []byte, time.Time, time.Duration, nil, []any or
// map[string]any.
func (x *Expr) Eval(ctx context.Context, act Activation) (any, error) {
	vars := make(map[string]any, len(Scopes))
	for _, s := range Scopes {
		v, ok := act[s]
		if !ok || v == nil {
			v = map[string]any{}
		}
		vars[s] = v
	}
	out, _, err := x.prog.ContextEval(ctx, vars)
	if err != nil {
		return nil, &Error{Source: x.src, Err: sanitize(ctx, err)}
	}
	native, err := toNative(out)
	if err != nil {
		return nil, &Error{Source: x.src, Err: err}
	}
	switch x.want {
	case Bool:
		if _, ok := native.(bool); !ok {
			return nil, &Error{Source: x.src, Err: fmt.Errorf("produced %T, must produce bool", native)}
		}
	case String:
		if _, ok := native.(string); !ok {
			return nil, &Error{Source: x.src, Err: fmt.Errorf("produced %T, must produce string", native)}
		}
	}
	return native, nil
}

// EvalBool evaluates an expression compiled with Bool.
func (x *Expr) EvalBool(ctx context.Context, act Activation) (bool, error) {
	v, err := x.Eval(ctx, act)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}

// ErrEval is the generic evaluation failure. CEL's own messages often quote
// the data involved ("no such key: 4111111111111111"), and these errors end
// up in logs, traces and CI artifacts, so the data is dropped: the category
// and the expression are enough to debug.
var ErrEval = errors.New("evaluation failed")

// runtimeCategories maps CEL error texts to data-free descriptions.
var runtimeCategories = []struct{ match, category string }{
	{"actual cost limit exceeded", ""}, // ErrCostLimit
	{"addBusinessDays: at most", "addBusinessDays accepts at most 3660 days"},
	{"unknown time zone", "unknown time zone"},
	{"no such key", "a key is missing"},
	{"no such attribute", "a field is missing"},
	{"no such overload", "no function matches the argument types"},
	{"division by zero", "division by zero"},
	{"modulus by zero", "division by zero"},
	{"index out of range", "a list index is out of range"},
	{"invalid", "a value has the wrong format"},
	{"error parsing regexp", "invalid regular expression"},
	{"return error overflow", "a number overflowed"},
	{"overflow", "a number overflowed"},
}

func sanitize(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	msg := err.Error()
	for _, c := range runtimeCategories {
		if strings.Contains(msg, c.match) {
			if c.category == "" {
				return ErrCostLimit
			}
			return fmt.Errorf("%w: %s", ErrEval, c.category)
		}
	}
	return ErrEval
}

// toNative converts a CEL value into plain Go values.
func toNative(v ref.Val) (any, error) {
	switch v := v.(type) {
	case types.Null:
		return nil, nil
	case traits.Mapper:
		out := map[string]any{}
		it := v.Iterator()
		for it.HasNext() == types.True {
			k := it.Next()
			ks, ok := k.Value().(string)
			if !ok {
				return nil, fmt.Errorf("map key %v is not a string", k.Value())
			}
			nv, err := toNative(v.Get(k))
			if err != nil {
				return nil, err
			}
			out[ks] = nv
		}
		return out, nil
	case traits.Lister:
		out := []any{} // an empty list stays a list, never null
		it := v.Iterator()
		for it.HasNext() == types.True {
			nv, err := toNative(it.Next())
			if err != nil {
				return nil, err
			}
			out = append(out, nv)
		}
		return out, nil
	}
	if types.IsError(v) {
		return nil, fmt.Errorf("%v", v)
	}
	val := v.Value()
	if rv := reflect.ValueOf(val); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, nil
	}
	return val, nil
}

// shadowedScope returns a comprehension variable named like a scope, which
// would make references() and admission misread what the expression reads.
func shadowedScope(a *celast.AST) string {
	isScope := map[string]bool{}
	for _, s := range Scopes {
		isScope[s] = true
	}
	found := ""
	celast.PreOrderVisit(celast.NavigateAST(a), celast.NewExprVisitor(func(e celast.Expr) {
		if found != "" || e.Kind() != celast.ComprehensionKind {
			return
		}
		c := e.AsComprehension()
		for _, v := range []string{c.IterVar(), c.IterVar2(), c.AccuVar()} {
			if isScope[v] {
				found = v
				return
			}
		}
	}))
	return found
}

// references walks the checked AST and records reads of scope variables.
func references(a *celast.AST) []string {
	set := map[string]bool{}
	isScope := map[string]bool{}
	for _, s := range Scopes {
		isScope[s] = true
	}
	celast.PreOrderVisit(celast.NavigateAST(a), celast.NewExprVisitor(func(e celast.Expr) {
		switch e.Kind() {
		case celast.SelectKind:
			sel := e.AsSelect()
			if op := sel.Operand(); op.Kind() == celast.IdentKind && isScope[op.AsIdent()] {
				set[op.AsIdent()+"."+sel.FieldName()] = true
			}
		case celast.CallKind:
			call := e.AsCall()
			args := call.Args()
			if call.FunctionName() == "_[_]" && len(args) == 2 && args[0].Kind() == celast.IdentKind &&
				isScope[args[0].AsIdent()] && args[1].Kind() == celast.LiteralKind {
				if s, ok := args[1].AsLiteral().Value().(string); ok {
					set[args[0].AsIdent()+"."+s] = true
				}
			}
		case celast.IdentKind:
			if isScope[e.AsIdent()] && !parentSelects(e) {
				set[e.AsIdent()] = true
			}
		}
	}))
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// parentSelects reports whether an identifier is the operand of a field
// selection or index, which is recorded with its field instead.
func parentSelects(e celast.Expr) bool {
	nav, ok := e.(celast.NavigableExpr)
	if !ok {
		return false
	}
	p, ok := nav.Parent()
	if !ok {
		return false
	}
	if p.Kind() == celast.SelectKind {
		return true
	}
	if p.Kind() == celast.CallKind && p.AsCall().FunctionName() == "_[_]" {
		args := p.AsCall().Args()
		return len(args) == 2 && args[0].ID() == e.ID() && args[1].Kind() == celast.LiteralKind
	}
	return false
}
