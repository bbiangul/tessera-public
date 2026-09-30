// Package act implements the Act engine's contract: calling plugin actions
// (RFC-0001, "Os quatro motores"; RFC-0003, "Effects").
//
// A call ends in one of three ways, and the difference is what makes effects
// safe to recover:
//
//   - success, with the action's output;
//   - a definite failure (*DefiniteError): nothing happened, so trying again
//     or trying something else is safe;
//   - an ambiguous failure (*AmbiguousError): the effect may have happened,
//     for example a timeout after the request was sent. The runtime then
//     verifies before doing anything else, or asks a human.
//
// Any other error returned by an Actor is treated as ambiguous: when a plugin
// does not say, the runtime assumes the worst.
package act

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Call is one action call.
type Call struct {
	// Action is plugin/action.
	Action string
	// Args are the resolved arguments.
	Args map[string]any
	// IdempotencyKey is set for effects; plugins with native verification
	// pass it to the external system.
	IdempotencyKey string
	// Caller is the caller scope, for the plugin's authorize rule.
	Caller map[string]any
	// Extra are top-level body fields a repair adds to the binding's
	// request (RFC-0005 step 3, drift-007). A plugin that cannot add them,
	// or a field the binding already sends, is a definite failure: a
	// repair only adds, never changes.
	Extra map[string]any
}

// Actor calls plugin actions.
type Actor interface {
	Call(ctx context.Context, c Call) (any, error)
}

// DefiniteError means the action did not happen.
type DefiniteError struct {
	Code    string // stable, e.g. "NOT_FOUND", "INVALID", "FORBIDDEN"
	Message string // must not contain personal data
	// Detail is what the system said, in its own words ("online
	// rescheduling is suspended: call ..."), for the user (drift-002). It
	// may hold personal data: the runtime masks it unless Public.
	Detail string
	// Public says Detail is the business's own words (rules, phones,
	// fees), not personal data: it reaches the user unmasked
	// (connection.publicErrors).
	Public bool
}

func (e *DefiniteError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// AmbiguousError means the action may or may not have happened.
type AmbiguousError struct {
	Message string
	Err     error
}

func (e *AmbiguousError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("outcome unknown: %s: %v", e.Message, e.Err)
	}
	return "outcome unknown: " + e.Message
}

func (e *AmbiguousError) Unwrap() error { return e.Err }

// IsDefinite reports whether err is a definite failure. Everything else,
// including errors a plugin did not classify, is ambiguous.
func IsDefinite(err error) bool {
	var d *DefiniteError
	return errors.As(err, &d)
}

// Handler implements one action in process.
type Handler func(ctx context.Context, c Call) (any, error)

// Registry is an Actor backed by in-process handlers. Built-in plugins
// (http, sql) and test fakes register here.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{handlers: map[string]Handler{}} }

// Register adds a handler for plugin/action. Registering twice panics:
// that is a wiring bug.
func (r *Registry) Register(action string, h Handler) {
	if !strings.Contains(action, "/") {
		panic("act: action must be plugin/action, got " + action)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.handlers[action]; dup {
		panic("act: " + action + " registered twice")
	}
	r.handlers[action] = h
}

// Actions lists the registered actions, sorted.
func (r *Registry) Actions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for a := range r.handlers {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Call runs the handler. An unknown action is a definite failure: nothing
// was called.
func (r *Registry) Call(ctx context.Context, c Call) (any, error) {
	r.mu.RLock()
	h, ok := r.handlers[c.Action]
	r.mu.RUnlock()
	if !ok {
		return nil, &DefiniteError{Code: "UNKNOWN_ACTION", Message: "no handler for " + c.Action}
	}
	if err := ctx.Err(); err != nil {
		return nil, &DefiniteError{Code: "CANCELLED", Message: "not called: " + err.Error()}
	}
	return h(ctx, c)
}
