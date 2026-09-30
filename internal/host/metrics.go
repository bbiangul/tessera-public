package host

import (
	"context"
	"errors"
	"fmt"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// registerMetrics serves the built-in metrics plugin for this namespace:
// Functions read funnels and experiments as of the run's clock.
func (h *Host) registerMetrics(reg *act.Registry) {
	prefix := admission.MetricsPlugin + "/"
	if _, ok := h.opts.Handlers[prefix+"funnel"]; !ok {
		reg.Register(prefix+"funnel", func(ctx context.Context, c act.Call) (any, error) {
			name, _ := c.Args["funnel"].(string)
			return h.FunnelReport(ctx, name)
		})
	}
	if _, ok := h.opts.Handlers[prefix+"experiment"]; !ok {
		reg.Register(prefix+"experiment", func(ctx context.Context, c act.Call) (any, error) {
			name, _ := c.Args["experiment"].(string)
			return h.ExperimentReport(ctx, name)
		})
	}
}

func days(ns int64) float64 { return float64(ns) / 86400e9 }

// funnelMap is a report in plain values for expressions and scripts:
// durations in days, and totals over all cohorts.
func funnelMap(rep metrics.Report, h *Host) map[string]any {
	cohorts := []any{}
	type tot struct{ reached, pending, lost int }
	var names []string
	totals := map[string]*tot{}
	entered := 0
	for _, c := range rep.Cohorts {
		entered += c.Entered
		stages := []any{}
		for _, s := range c.Stages {
			if totals[s.Name] == nil {
				totals[s.Name] = &tot{}
				names = append(names, s.Name)
			}
			t := totals[s.Name]
			t.reached, t.pending, t.lost = t.reached+s.Reached, t.pending+s.Pending, t.lost+s.Lost
			stuck := []any{}
			for _, x := range s.Stuck {
				stuck = append(stuck, map[string]any{"id": x.ID, "days": days(int64(h.Runtime.Clock().Sub(x.Since)))})
			}
			stages = append(stages, map[string]any{"name": s.Name, "reached": int64(s.Reached), "pending": int64(s.Pending), "lost": int64(s.Lost),
				"conversion": s.Conversion, "medianDays": days(int64(s.Median)), "p90Days": days(int64(s.P90)), "stuck": stuck})
		}
		cohorts = append(cohorts, map[string]any{"start": c.Start.In(h.loc).Format("2006-01-02"), "group": c.Group, "entered": int64(c.Entered), "stages": stages,
			"total": map[string]any{"reached": int64(c.Total.Reached), "medianDays": days(int64(c.Total.Median)), "p90Days": days(int64(c.Total.P90))}})
	}
	all := []any{}
	for _, n := range names {
		t := totals[n]
		conv := 0.0
		if t.reached+t.lost > 0 {
			conv = float64(t.reached) / float64(t.reached+t.lost)
		}
		all = append(all, map[string]any{"name": n, "reached": int64(t.reached), "pending": int64(t.pending), "lost": int64(t.lost), "conversion": conv})
	}
	return map[string]any{"funnel": rep.Funnel, "cohorts": cohorts, "totals": map[string]any{"entered": int64(entered), "stages": all}}
}

func (h *Host) funnel(name string) *agents.Funnel {
	for _, d := range h.docs {
		if d.Funnel != nil && d.Funnel.Metadata.Namespace == h.opts.Namespace && d.Funnel.Metadata.Name == name {
			return d.Funnel
		}
	}
	return nil
}

// FunnelReport is a Funnel of the namespace as of the runtime's clock, in
// plain values (days, totals over cohorts).
func (h *Host) FunnelReport(ctx context.Context, name string) (map[string]any, error) {
	f := h.funnel(name)
	if f == nil {
		return nil, &act.DefiniteError{Code: "NOT_FOUND", Message: fmt.Sprintf("no Funnel %q in namespace %s", name, h.opts.Namespace)}
	}
	ms, err := h.Runtime.Store.Milestones(ctx, h.opts.Namespace, f.Spec.Subject)
	if err != nil {
		return nil, err
	}
	rep, err := metrics.Compute(f, ms, h.Runtime.Clock(), h.loc, "")
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: err.Error()}
	}
	return funnelMap(rep, h), nil
}

// ExperimentReport is an Experiment of the namespace as of the runtime's
// clock.
func (h *Host) ExperimentReport(ctx context.Context, name string) (map[string]any, error) {
	var x *agents.Experiment
	for _, d := range h.docs {
		if d.Experiment != nil && d.Experiment.Metadata.Namespace == h.opts.Namespace && d.Experiment.Metadata.Name == name {
			x = d.Experiment
		}
	}
	if x == nil {
		return nil, &act.DefiniteError{Code: "NOT_FOUND", Message: fmt.Sprintf("no Experiment %q in namespace %s", name, h.opts.Namespace)}
	}
	ms, err := h.Runtime.Store.Milestones(ctx, h.opts.Namespace, x.Spec.Subject)
	if err != nil {
		return nil, err
	}
	rep, err := metrics.AnalyzeExperiment(x, h.funnel(x.Spec.Primary.Funnel), ms, h.Runtime.Clock(), h.loc)
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: err.Error()}
	}
	arms := []any{}
	for _, a := range rep.Verdict.Arms {
		arms = append(arms, map[string]any{"name": a.Name, "exposed": int64(a.Exposed), "decided": int64(a.Decided),
			"converted": int64(a.Converted), "rate": a.Rate, "low": a.Low, "high": a.High, "difference": a.Difference})
	}
	out := map[string]any{"experiment": name, "arms": arms, "decision": rep.Verdict.Decision, "srm": rep.SRM}
	if len(rep.Guardrails) > 0 {
		gs := []any{}
		for _, g := range rep.Guardrails {
			ga := []any{}
			for _, a := range g.Arms {
				ga = append(ga, map[string]any{"variant": a.Variant, "decided": int64(a.Decided), "hit": int64(a.Hit), "rate": a.Rate})
			}
			gs = append(gs, map[string]any{"name": g.Name, "arms": ga, "breach": g.Breach, "difference": g.Difference})
		}
		out["guardrails"] = gs
		stopped, err := h.experimentStopped(ctx, name)
		if err != nil {
			return nil, err
		}
		out["stopped"] = stopped
	}
	return out, nil
}

func (h *Host) experimentStopped(ctx context.Context, name string) (bool, error) {
	_, err := h.Runtime.Store.GetRun(ctx, runtime.ExperimentStopRun(h.opts.Namespace, name))
	if errors.Is(err, journal.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
