package host

import (
	"context"
	"slices"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime"
)

// Audiences (RFC-0006, audiences-001): each kind of caller gets its own
// menu, free mode, voice and messages. The host decides the caller's
// audience from its who (roles come from the Team through an identity a
// channel attests) and names it in caller.audience, which the run
// journals: the runtime speaks in that audience's voice and messages.

type audience struct {
	name          string
	who           *expr.Expr // nil: everyone else
	functions     []string
	free          *freeMode
	notUnderstood string
	withAPerson   string
	unsure        string
}

// compileAudiences builds the Agent's audiences, and gives the runtime
// their voices and messages. agentFree is the Agent's free mode, an
// audience's when it declares none.
func compileAudiences(spec agents.AgentSpec, agentFree *freeMode, rt *runtime.Runtime) ([]*audience, error) {
	if len(spec.Audiences) == 0 {
		return nil, nil
	}
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	rt.Audiences = map[string]runtime.Audience{}
	out := make([]*audience, 0, len(spec.Audiences))
	for _, a := range spec.Audiences {
		au := &audience{name: a.Name, functions: a.Functions, free: agentFree}
		if a.Who != "" {
			if au.who, err = e.Compile(string(a.Who), expr.Bool); err != nil {
				return nil, err
			}
		}
		if a.FreeMode != nil {
			if au.free, err = compileFreeMode(a.FreeMode); err != nil {
				return nil, err
			}
		}
		ra := runtime.Audience{}
		if v := a.Voice; v != nil {
			ra.Voice = &runtime.Voice{Persona: v.Persona, Examples: v.Examples, Keep: v.Keep, AnyGreeting: v.Greeting == "any", Warm: v.Latitude != "strict", WarmDialogue: v.Dialogue != "marker"}
		}
		if m := a.Messages; m != nil {
			ra.Messages = runtime.Messages{NotAllowed: m.NotAllowed, Cancelled: m.Cancelled, Denied: m.Denied, Failed: m.Failed}
			au.notUnderstood, au.withAPerson, au.unsure = m.NotUnderstood, m.WithAPerson, m.Unsure
		}
		ra.Autonomy = a.Autonomy
		for _, v := range a.Sees {
			if rt.Sees == nil {
				rt.Sees = map[string]map[string]string{}
			}
			if rt.Sees[a.Name] == nil {
				rt.Sees[a.Name] = map[string]string{}
			}
			rt.Sees[a.Name][v.Audience] = v.Conversation
		}
		rt.Audiences[a.Name] = ra
		out = append(out, au)
	}
	return out, nil
}

// audienceName is the audience a caller belongs to, "" for none (the
// runtime asks it for context visibility).
func (h *Host) audienceName(caller map[string]any) string {
	if _, a := h.withAudience(context.Background(), caller); a != nil {
		return a.name
	}
	return ""
}

// withAudience returns the caller naming its audience (a copy; any
// audience the caller came with is replaced), and the audience, nil when
// none matches. Without audiences the caller is returned as it came.
func (h *Host) withAudience(ctx context.Context, caller map[string]any) (map[string]any, *audience) {
	if len(h.audiences) == 0 {
		return caller, nil
	}
	out := make(map[string]any, len(caller)+1)
	for k, v := range caller {
		out[k] = v
	}
	delete(out, "audience")
	for _, a := range h.audiences {
		if a.who != nil {
			ok, err := a.who.EvalBool(ctx, expr.Activation{"caller": out})
			if err != nil || !ok {
				continue
			}
		}
		out["audience"] = a.name
		return out, a
	}
	return out, nil
}

// audienceOf is the audience a caller already names.
func (h *Host) audienceOf(caller map[string]any) *audience {
	name, _ := caller["audience"].(string)
	for _, a := range h.audiences {
		if a.name == name {
			return a
		}
	}
	return nil
}

// freeFor is the free mode a caller gets: their audience's; with
// audiences and none matching, none.
func (h *Host) freeFor(caller map[string]any) *freeMode {
	if len(h.audiences) == 0 {
		return h.free
	}
	if a := h.audienceOf(caller); a != nil {
		return a.free
	}
	return nil
}

// offered says whether the router may offer Function name to caller.
func (h *Host) offered(caller map[string]any, name string) bool {
	if len(h.audiences) == 0 || caller == nil {
		return true
	}
	a := h.audienceOf(caller)
	return a != nil && slices.Contains(a.functions, name)
}

func (h *Host) notUnderstood(caller map[string]any) string {
	if a := h.audienceOf(caller); a != nil && a.notUnderstood != "" {
		return a.notUnderstood
	}
	return h.opts.NotUnderstood
}

// unsure is what the user hears when the free mode's checks dropped its
// answer: the audience's, the Agent's, else notUnderstood.
func (h *Host) unsure(caller map[string]any) string {
	if a := h.audienceOf(caller); a != nil && a.unsure != "" {
		return a.unsure
	}
	if h.opts.Unsure != "" {
		return h.opts.Unsure
	}
	return h.notUnderstood(caller)
}

func (h *Host) withAPerson(caller map[string]any) string {
	if a := h.audienceOf(caller); a != nil && a.withAPerson != "" {
		return a.withAPerson
	}
	return h.opts.WithAPerson
}
