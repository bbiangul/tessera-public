// Package compute implements the Compute engine's Starlark half (RFC-0001,
// "Os quatro motores"; CEL lives in internal/expr).
//
// A Script is a Starlark file whose top-level functions a Function calls as
// compute steps. Scripts are deterministic by construction: no time.now, no
// randomness, no load(), no I/O. Every call runs with a step limit and stops
// when its context ends.
//
// Memory: a watchdog cancels a call whose heap grows past Limits.Memory,
// at the next instruction. One instruction can still allocate gigabytes
// ([0] * (200 << 20)), which kills the process (starmem-001). With
// Isolate, calls run in a child process the OS bounds, and only the child
// dies (isolate.go).
package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	starjson "go.starlark.net/lib/json"
	starmath "go.starlark.net/lib/math"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// Limits bound one call.
type Limits struct {
	// Steps is the most Starlark execution steps; default 10 million
	// (roughly 100 ms of work).
	Steps uint64
	// Values is the most values in the result; default 100 000.
	Values int
	// Memory is how much the process heap may grow during a call before
	// the call is cancelled; default 256 MiB. It is a watchdog, not a hard
	// limit: it stops a script that allocates in a loop, not one single
	// huge allocation. Hard limits come with out-of-process execution
	// (RFC-0001, plugin "code").
	Memory uint64
}

func (l Limits) withDefaults() Limits {
	if l.Steps == 0 {
		l.Steps = 10_000_000
	}
	if l.Values == 0 {
		l.Values = 100_000
	}
	if l.Memory == 0 {
		l.Memory = 256 << 20
	}
	return l
}

// ErrCompute wraps every script failure. Messages keep the position and the
// kind of error, never the values involved: scripts process user data and
// errors end up in logs.
var ErrCompute = errors.New("compute failed")

// Script is a compiled Starlark file.
type Script struct {
	name    string
	globals starlark.StringDict
	src     []byte // for isolated calls, which compile again in the child
	sandbox bool   // sandboxes run in-process: they are test fakes
}

// predeclared is what every script can use.
func predeclared() starlark.StringDict {
	tm := starlark.StringDict{}
	for k, v := range startime.Module.Members {
		if k != "now" { // the clock is an input, never read by a script
			tm[k] = v
		}
	}
	return starlark.StringDict{
		"time":   &starlarkstruct.Module{Name: "time", Members: tm},
		"math":   starmath.Module,
		"json":   starjson.Module,
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}
}

var fileOptions = &syntax.FileOptions{Set: true, While: true}

// Compile loads a script. Top-level code runs once, with the same step
// limit as a call, and the resulting globals are frozen, so one call cannot
// leave state for the next.
func Compile(name string, src []byte, limits Limits) (*Script, error) {
	return compileWith(name, src, limits, predeclared())
}

func compileWith(name string, src []byte, limits Limits, predecl starlark.StringDict) (*Script, error) {
	limits = limits.withDefaults()
	thread := &starlark.Thread{
		Name: name,
		Load: func(*starlark.Thread, string) (starlark.StringDict, error) {
			return nil, errors.New("load is not available: a script is one file")
		},
		Print: func(*starlark.Thread, string) {}, // print goes nowhere
	}
	thread.SetMaxExecutionSteps(limits.Steps)
	globals, err := starlark.ExecFileOptions(fileOptions, thread, name, src, predecl)
	if err != nil {
		return nil, scrub(err)
	}
	globals.Freeze()
	return &Script{name: name, globals: globals, src: src}, nil
}

// Functions lists the script's top-level functions, sorted.
func (s *Script) Functions() []string {
	var out []string
	for name, v := range s.globals {
		if _, ok := v.(*starlark.Function); ok && !strings.HasPrefix(name, "_") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Signature describes a function's keyword parameters: all of them, the
// ones without a default, and whether it accepts any other (**kwargs).
type Signature struct {
	Params   []string
	Required []string
	Kwargs   bool
}

// Signature returns a function's parameters; ok is false when the script
// has no such public function.
func (s *Script) Signature(fn string) (Signature, bool) {
	f, ok := s.globals[fn].(*starlark.Function)
	if !ok || strings.HasPrefix(fn, "_") {
		return Signature{}, false
	}
	sig := Signature{Kwargs: f.HasKwargs()}
	n := f.NumParams()
	if f.HasVarargs() {
		n--
	}
	if f.HasKwargs() {
		n--
	}
	for i := range n {
		name, _ := f.Param(i)
		sig.Params = append(sig.Params, name)
		if f.ParamDefault(i) == nil {
			sig.Required = append(sig.Required, name)
		}
	}
	return sig, true
}

// Call runs a top-level function with keyword arguments and returns its
// result as plain Go values (see ToGo).
func (s *Script) Call(ctx context.Context, fn string, args map[string]any, limits Limits) (any, error) {
	limits = limits.withDefaults()
	if _, ok := s.Signature(fn); !ok {
		return nil, fmt.Errorf("%w: %s has no function %q", ErrCompute, s.name, fn)
	}
	if iso := isolation.Load(); iso != nil && !s.sandbox {
		return s.callIsolated(ctx, iso, fn, args, limits)
	}
	return s.call(ctx, fn, args, limits)
}

func (s *Script) call(ctx context.Context, fn string, args map[string]any, limits Limits) (any, error) {
	f := s.globals[fn].(*starlark.Function)
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kwargs := make([]starlark.Tuple, 0, len(args))
	for _, k := range keys {
		v, err := ToStarlark(args[k])
		if err != nil {
			return nil, fmt.Errorf("%w: argument %s: %v", ErrCompute, k, err)
		}
		kwargs = append(kwargs, starlark.Tuple{starlark.String(k), v})
	}
	thread := &starlark.Thread{Name: s.name + ":" + fn, Print: func(*starlark.Thread, string) {}}
	thread.SetMaxExecutionSteps(limits.Steps)
	stop := watch(ctx, thread, limits.Memory)
	out, err := starlark.Call(thread, f, nil, kwargs)
	stop()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, scrub(err)
	}
	n := 0
	return toGo(out, &n, limits.Values)
}

// quoted matches string literals and numbers inside error messages.
var quoted = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\b\d[\d.eE+-]*\b`)

// scrub keeps where and what kind of error, and drops the values.
func scrub(err error) error {
	var ee *starlark.EvalError
	msg := err.Error()
	pos := ""
	if errors.As(err, &ee) {
		msg = ee.Msg
		if len(ee.CallStack) > 0 {
			pos = ee.CallStack.At(0).Pos.String() + ": "
		}
	}
	var se syntax.Error
	if errors.As(err, &se) {
		pos, msg = se.Pos.String()+": ", se.Msg
	}
	if strings.Contains(msg, "too many steps") {
		return fmt.Errorf("%w: %sthe step limit was reached", ErrCompute, pos)
	}
	if strings.Contains(msg, "memory limit") {
		return fmt.Errorf("%w: %sthe memory limit was reached", ErrCompute, pos)
	}
	if text, ok := strings.CutPrefix(msg, "fail: "); ok {
		// fail() may carry anything a script had, personal data included;
		// keep it only when it is a code such as NO_PLACE.
		if !isCode(text) {
			text = "(message withheld)"
		}
		return fmt.Errorf("%w: %sfail: %s", ErrCompute, pos, text)
	}
	return fmt.Errorf("%w: %s%s", ErrCompute, pos, quoted.ReplaceAllString(msg, "…"))
}

func isCode(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if !(r >= 'A' && r <= 'Z' || r == '_' || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ToStarlark converts plain Go values (as expressions and the journal use
// them) to Starlark values.
func ToStarlark(v any) (starlark.Value, error) {
	switch v := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(v), nil
	case string:
		return starlark.String(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case uint64:
		return starlark.MakeUint64(v), nil
	case float64:
		return starlark.Float(v), nil
	case json.Number: // a literal from a definition
		if i, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return starlark.MakeInt64(i), nil
		}
		f, err := v.Float64()
		if err != nil {
			return nil, err
		}
		return starlark.Float(f), nil
	case time.Time:
		return startime.Time(v), nil
	case time.Duration:
		return startime.Duration(v), nil
	case []any:
		out := make([]starlark.Value, len(v))
		for i, x := range v {
			sv, err := ToStarlark(x)
			if err != nil {
				return nil, err
			}
			out[i] = sv
		}
		return starlark.NewList(out), nil
	case map[string]any:
		// Keys go in sorted order: a Starlark dict iterates in insertion
		// order, and Go's map order is random, so any other order would
		// make scripts (and tie-breaks) differ between runs.
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		d := starlark.NewDict(len(v))
		for _, k := range keys {
			x := v[k]
			sv, err := ToStarlark(x)
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(starlark.String(k), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	}
	return nil, fmt.Errorf("type %T cannot be passed to a script", v)
}

// ToGo converts a script's result back. Dict keys must be strings; tuples
// become lists; structs become maps.
func ToGo(v starlark.Value) (any, error) {
	n := 0
	return toGo(v, &n, Limits{}.withDefaults().Values)
}

func toGo(v starlark.Value, n *int, max int) (any, error) {
	if *n++; *n > max {
		return nil, fmt.Errorf("%w: the result has more than %d values", ErrCompute, max)
	}
	switch v := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.String:
		return string(v), nil
	case starlark.Int:
		if i, ok := v.Int64(); ok {
			return i, nil
		}
		if u, ok := v.Uint64(); ok {
			return u, nil
		}
		return nil, fmt.Errorf("%w: an integer in the result is too large", ErrCompute)
	case starlark.Float:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%w: the result has NaN or infinity", ErrCompute)
		}
		return f, nil
	case startime.Time:
		return time.Time(v), nil
	case startime.Duration:
		return time.Duration(v), nil
	case *starlark.List:
		return seqToGo(v, v.Len(), n, max)
	case starlark.Tuple:
		return seqToGo(v, v.Len(), n, max)
	case *starlark.Dict:
		out := make(map[string]any, v.Len())
		for _, item := range v.Items() {
			k, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("%w: a dict key in the result is %s, not a string", ErrCompute, item[0].Type())
			}
			x, err := toGo(item[1], n, max)
			if err != nil {
				return nil, err
			}
			out[string(k)] = x
		}
		return out, nil
	case *starlarkstruct.Struct:
		d := starlark.StringDict{}
		v.ToStringDict(d)
		out := make(map[string]any, len(d))
		for k, x := range d {
			gx, err := toGo(x, n, max)
			if err != nil {
				return nil, err
			}
			out[k] = gx
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: a %s cannot be returned", ErrCompute, v.Type())
}

func seqToGo(seq starlark.Indexable, length int, n *int, max int) (any, error) {
	out := make([]any, 0, length)
	for i := range length {
		x, err := toGo(seq.Index(i), n, max)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}
