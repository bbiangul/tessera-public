package host

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Docs are the admitted definitions (with the built-in plugins).
func (h *Host) Docs() []admission.Doc { return h.docs }

// Namespace served.
func (h *Host) Namespace() string { return h.opts.Namespace }

// Describe summarizes what the agent can do, for people and coding
// agents: its routable Functions, the other Functions, triggers, plugin
// actions with their effect classes, funnels and experiments.
func (h *Host) Describe() map[string]any {
	routes := []any{}
	for _, r := range h.routes {
		routes = append(routes, map[string]any{"name": r.Name, "intent": r.Intent, "description": r.Description, "after": r.After})
	}
	functions, triggers, funnels, experiments, subjects := []any{}, []any{}, []any{}, []any{}, []any{}
	actions := map[string]any{}
	for _, d := range h.docs {
		switch {
		case d.Function != nil && d.Function.Metadata.Namespace == h.opts.Namespace:
			functions = append(functions, map[string]any{"name": d.Function.Metadata.Name, "description": d.Function.Spec.Description, "routable": d.Function.Spec.Selector != nil})
		case d.Trigger != nil && d.Trigger.Metadata.Namespace == h.opts.Namespace:
			t := map[string]any{"name": d.Trigger.Metadata.Name, "function": d.Trigger.Spec.Function}
			if s := d.Trigger.Spec.Schedule; s != nil {
				t["schedule"] = s.Cron
			}
			if d.Trigger.Spec.Webhook != nil {
				t["webhook"] = "/v1/hooks/" + d.Trigger.Metadata.Name
			}
			triggers = append(triggers, t)
		case d.Funnel != nil && d.Funnel.Metadata.Namespace == h.opts.Namespace:
			funnels = append(funnels, d.Funnel.Metadata.Name)
		case d.Experiment != nil && d.Experiment.Metadata.Namespace == h.opts.Namespace:
			experiments = append(experiments, d.Experiment.Metadata.Name)
		case d.Subject != nil && d.Subject.Metadata.Namespace == h.opts.Namespace:
			subjects = append(subjects, d.Subject.Metadata.Name)
		case d.Plugin != nil:
			for name, a := range d.Plugin.Spec.Actions {
				actions[d.Plugin.Metadata.Name+"/"+name] = map[string]any{"effect": a.Effect, "input": a.Input, "output": a.Output}
			}
		}
	}
	for _, l := range [][]any{functions, triggers} {
		sort.Slice(l, func(i, j int) bool { return fmt.Sprint(l[i]) < fmt.Sprint(l[j]) })
	}
	return map[string]any{"namespace": h.opts.Namespace, "generation": h.Runtime.Program.Generation(), "routes": routes,
		"functions": functions, "triggers": triggers, "actions": actions, "subjects": subjects, "funnels": funnels, "experiments": experiments,
		"routing": map[string]any{"minConfidence": h.opts.MinRouteConfidence}}
}

// ErrNotSchedule means the trigger is not a schedule (webhooks are called).
var ErrNotSchedule = errors.New("not a schedule trigger: call its webhook")

// Fire starts a schedule trigger's Function now, by hand (testing,
// catching up). Its run ID says it was manual.
func (h *Host) Fire(ctx context.Context, name string) (runtime.Result, error) {
	for _, s := range h.schedules {
		if s.name != name {
			continue
		}
		now := h.Runtime.Clock()
		id := fmt.Sprintf("%s/%s@manual-%d", h.opts.Namespace, name, now.UnixNano())
		return h.Runtime.Start(ctx, runtime.Start{Namespace: h.opts.Namespace, Function: s.function, Conversation: "trigger:" + id,
			Caller: map[string]any{"trigger": name}, Trigger: name, RunID: id, Time: now})
	}
	if _, ok := h.hooks[name]; ok {
		return runtime.Result{}, ErrNotSchedule
	}
	return runtime.Result{}, fmt.Errorf("%w: no trigger %q", journal.ErrNotFound, name)
}

// Engines are what the host was loaded with, for running EvalSuites
// against the same definitions and models (they re-read the paths, so an
// edited definition is tested without a restart).
func (h *Host) Engines() (paths []string, d decide.Decider, g generate.Generator) {
	return h.opts.Paths, h.opts.Decider, h.opts.Generator
}

// Channel is the plugin carrying the Agent's messages to users (its
// spec.channel), or "".
func (h *Host) Channel() string {
	for _, d := range h.docs {
		if d.Agent != nil && d.Agent.Metadata.Namespace == h.opts.Namespace {
			return d.Agent.Spec.Channel
		}
	}
	return ""
}
