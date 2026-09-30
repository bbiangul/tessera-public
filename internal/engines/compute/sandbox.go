package compute

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"
)

// Sandboxes are Starlark fakes of plugins for tests (RFC-0002, level 4).
// A sandbox function receives the plugin's state, a dict it may change,
// and the action's arguments; it returns the action's output, or fails the
// way a real system would with definite(code, message) or
// ambiguous(message).

// SandboxFailure is a failure a sandbox function reported.
type SandboxFailure struct {
	Definite bool
	Code     string
	Message  string
}

func (f *SandboxFailure) Error() string {
	if f.Definite {
		return f.Code + ": " + f.Message
	}
	return "ambiguous: " + f.Message
}

const failurePrefix = "\x00sandbox-failure:"

func sandboxPredeclared() starlark.StringDict {
	p := predeclared()
	p["definite"] = starlark.NewBuiltin("definite", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var code, msg string
		if err := starlark.UnpackPositionalArgs("definite", args, kwargs, 2, &code, &msg); err != nil {
			return nil, err
		}
		return nil, errors.New(failurePrefix + "D:" + code + ":" + msg)
	})
	p["ambiguous"] = starlark.NewBuiltin("ambiguous", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var msg string
		if err := starlark.UnpackPositionalArgs("ambiguous", args, kwargs, 1, &msg); err != nil {
			return nil, err
		}
		return nil, errors.New(failurePrefix + "A::" + msg)
	})
	return p
}

// CompileSandbox loads a sandbox script.
func CompileSandbox(name string, src []byte) (*Script, error) {
	s, err := compileWith(name, src, Limits{}, sandboxPredeclared())
	if err != nil {
		return nil, err
	}
	s.sandbox = true
	return s, nil
}

// ActionFunction is the sandbox function name for an action.
func ActionFunction(action string) string { return strings.ReplaceAll(action, "-", "_") }

// Sandbox holds a script and the state it changes. It is not safe for
// concurrent use; a scenario runs its turns in order.
type Sandbox struct {
	script *Script
	state  *starlark.Dict
}

// NewSandbox starts a sandbox from an initial state.
func NewSandbox(s *Script, state map[string]any) (*Sandbox, error) {
	if state == nil {
		state = map[string]any{}
	}
	v, err := ToStarlark(state)
	if err != nil {
		return nil, fmt.Errorf("sandbox state: %w", err)
	}
	return &Sandbox{script: s, state: v.(*starlark.Dict)}, nil
}

// Has reports whether the sandbox implements an action.
func (sb *Sandbox) Has(action string) bool {
	_, ok := sb.script.globals[ActionFunction(action)].(*starlark.Function)
	return ok
}

// Call runs the action's function. A reported failure comes back as
// *SandboxFailure; a bug in the script as an ErrCompute error.
func (sb *Sandbox) Call(ctx context.Context, action string, args map[string]any) (any, error) {
	fn := ActionFunction(action)
	f, ok := sb.script.globals[fn].(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%w: the sandbox has no function %s", ErrCompute, fn)
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kwargs := []starlark.Tuple{{starlark.String("state"), sb.state}}
	for _, k := range keys {
		v, err := ToStarlark(args[k])
		if err != nil {
			return nil, fmt.Errorf("%w: argument %s: %v", ErrCompute, k, err)
		}
		kwargs = append(kwargs, starlark.Tuple{starlark.String(k), v})
	}
	limits := Limits{}.withDefaults()
	thread := &starlark.Thread{Name: "sandbox:" + fn, Print: func(*starlark.Thread, string) {}}
	thread.SetMaxExecutionSteps(limits.Steps)
	stop := watch(ctx, thread, limits.Memory)
	out, err := starlark.Call(thread, f, nil, kwargs)
	stop()
	if err != nil {
		if fail := failureOf(err); fail != nil {
			return nil, fail
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrCompute, err) // test code: keep the details
	}
	n := 0
	return toGo(out, &n, Limits{}.withDefaults().Values)
}

// State returns a copy of the current state.
func (sb *Sandbox) State() (map[string]any, error) {
	v, err := ToGo(sb.state)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	return m, nil
}

func failureOf(err error) *SandboxFailure {
	msg := err.Error()
	i := strings.Index(msg, failurePrefix)
	if i < 0 {
		return nil
	}
	rest := msg[i+len(failurePrefix):]
	kind, rest, _ := strings.Cut(rest, ":")
	code, text, _ := strings.Cut(rest, ":")
	// Drop Starlark's traceback after the message.
	text, _, _ = strings.Cut(text, "\n")
	return &SandboxFailure{Definite: kind == "D", Code: code, Message: text}
}
