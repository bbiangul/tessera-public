package act

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register("crm/get", func(_ context.Context, c Call) (any, error) {
		return map[string]any{"id": c.Args["id"]}, nil
	})
	out, err := r.Call(context.Background(), Call{Action: "crm/get", Args: map[string]any{"id": "1"}})
	if err != nil || out.(map[string]any)["id"] != "1" {
		t.Fatalf("%v %v", out, err)
	}
	if _, err := r.Call(context.Background(), Call{Action: "crm/nope"}); !IsDefinite(err) {
		t.Errorf("unknown action must be definite: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Call(ctx, Call{Action: "crm/get"}); !IsDefinite(err) {
		t.Errorf("a cancelled call that never started is definite: %v", err)
	}
	if got := r.Actions(); len(got) != 1 || got[0] != "crm/get" {
		t.Errorf("actions = %v", got)
	}
}

func TestRegistryPanicsOnWiringBugs(t *testing.T) {
	for name, f := range map[string]func(r *Registry){
		"no plugin": func(r *Registry) { r.Register("get", nil) },
		"twice": func(r *Registry) {
			r.Register("a/b", nil)
			r.Register("a/b", nil)
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want a panic", name)
				}
			}()
			f(NewRegistry())
		}()
	}
}

func TestUnclassifiedErrorsAreAmbiguous(t *testing.T) {
	if IsDefinite(errors.New("connection reset")) {
		t.Error("an unclassified error must count as ambiguous")
	}
	wrapped := fmt.Errorf("calling: %w", &DefiniteError{Code: "INVALID", Message: "bad slot"})
	if !IsDefinite(wrapped) {
		t.Error("a wrapped definite error is still definite")
	}
	amb := &AmbiguousError{Message: "timeout", Err: context.DeadlineExceeded}
	if IsDefinite(amb) || !errors.Is(amb, context.DeadlineExceeded) || amb.Error() == "" {
		t.Errorf("ambiguous: %v", amb)
	}
}
