package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/cron"
	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// schedule is a compiled schedule Trigger.
type schedule struct {
	name, function string
	cron           *cron.Schedule
	loc            *time.Location
	maxDelay       time.Duration
}

func schedules(docs []admission.Doc, namespace string) ([]schedule, error) {
	zone := ""
	for _, d := range docs {
		if d.Agent != nil && d.Agent.Metadata.Namespace == namespace {
			zone = d.Agent.Spec.Timezone
		}
	}
	var out []schedule
	for _, d := range docs {
		t := d.Trigger
		if t == nil || t.Metadata.Namespace != namespace || t.Spec.Schedule == nil {
			continue
		}
		s, err := compileSchedule(t, zone)
		if err != nil {
			return nil, fmt.Errorf("trigger %s: %w", t.Metadata.Name, err)
		}
		out = append(out, s)
	}
	return out, nil
}

func compileSchedule(t *agents.Trigger, agentZone string) (schedule, error) {
	c, err := cron.Parse(t.Spec.Schedule.Cron)
	if err != nil {
		return schedule{}, err
	}
	zone := t.Spec.Schedule.Timezone
	if zone == "" {
		zone = agentZone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return schedule{}, err
	}
	delay := time.Hour
	if t.Spec.Schedule.MaxDelay != "" {
		if delay, err = time.ParseDuration(t.Spec.Schedule.MaxDelay); err != nil {
			return schedule{}, err
		}
	}
	return schedule{name: t.Metadata.Name, function: t.Spec.Function, cron: c, loc: loc, maxDelay: delay}, nil
}

// Tick does the work that is due without a message: it wakes sleeping runs
// whose time came and starts the runs of schedule Triggers. Call it every
// few seconds; several processes may call it at once. The results carry
// messages for the channel layer to deliver.
func (h *Host) Tick(ctx context.Context) ([]runtime.Result, error) {
	// A run that fails to wake must not stop the schedules and funnels:
	// every part runs, and the errors are returned together.
	out, wakeErr := h.Runtime.WakeDue(ctx, 100)
	var errs []error
	if wakeErr != nil {
		errs = append(errs, wakeErr)
	}
	now := h.Runtime.Clock()
	errs = append(errs, h.tickLessons(ctx, now)...)
	if h.kb != nil && len(h.kb.sources) > 0 {
		errs = append(errs, h.kb.pending...)
		h.kb.pending = nil
		changed, kerrs := h.readSources(ctx, now, false)
		errs = append(errs, kerrs...)
		if changed {
			if err := h.rebuild(); err != nil {
				errs = append(errs, fmt.Errorf("knowledge: %w", err))
			}
		}
	}
	for _, s := range h.schedules {
		slot, ok := s.cron.Last(now, s.maxDelay, s.loc)
		if !ok {
			continue
		}
		// The run's ID is the trigger and the time: a second scheduler, a
		// retry or a restart finds it taken and starts nothing.
		id := fmt.Sprintf("%s/%s@%s", h.opts.Namespace, s.name, slot.UTC().Format("2006-01-02T15:04Z"))
		res, err := h.Runtime.Start(ctx, runtime.Start{
			Namespace: h.opts.Namespace, Function: s.function, Conversation: "trigger:" + id,
			Caller: map[string]any{"trigger": s.name}, Trigger: s.name, RunID: id, Time: slot,
		})
		if errors.Is(err, journal.ErrExists) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("trigger %s at %s: %w", s.name, slot.Format(time.RFC3339), err))
			continue
		}
		out = append(out, res)
	}
	stuck, err := h.checkFunnels(ctx, now)
	if err != nil {
		errs = append(errs, err)
	}
	out = append(out, stuck...)
	started, err := h.checkMilestones(ctx)
	if err != nil {
		errs = append(errs, err)
	}
	return append(out, started...), errors.Join(errs...)
}

// StuckEvery is how often Tick looks for stuck subjects: funnels read all
// of a subject kind's milestones, so not every second.
const StuckEvery = 5 * time.Minute

// checkFunnels starts each funnel's onStuck Function for the subjects
// stuck in a stage, once per funnel, stage and subject: the run's ID says
// which, so a second scheduler or the next check starts nothing new.
func (h *Host) checkFunnels(ctx context.Context, now time.Time) ([]runtime.Result, error) {
	h.mu.Lock()
	due := (len(h.funnels) > 0 || len(h.guarded) > 0) && (h.lastStuck.IsZero() || now.Sub(h.lastStuck) >= StuckEvery)
	if due {
		h.lastStuck = now
	}
	h.mu.Unlock()
	if !due {
		return nil, nil
	}
	var out []runtime.Result
	for _, f := range h.funnels {
		ms, err := h.Runtime.Store.Milestones(ctx, h.opts.Namespace, f.Spec.Subject)
		if err != nil {
			return out, err
		}
		rep, err := metrics.Compute(f, ms, now, h.loc, "")
		if err != nil {
			return out, fmt.Errorf("funnel %s: %w", f.Metadata.Name, err)
		}
		for _, c := range rep.Cohorts {
			for _, st := range c.Stages {
				for _, s := range st.Stuck {
					id := fmt.Sprintf("%s/%s/%s/%s@%s", h.opts.Namespace, f.Metadata.Name, st.Name, s.ID, s.Since.UTC().Format("2006-01-02T15:04Z"))
					res, err := h.Runtime.Start(ctx, runtime.Start{
						Namespace: h.opts.Namespace, Function: f.Spec.OnStuck.Function, Conversation: "funnel:" + id,
						Caller: map[string]any{"funnel": f.Metadata.Name}, Trigger: f.Metadata.Name, RunID: id,
						Payload: map[string]any{"funnel": f.Metadata.Name, "stage": st.Name, "subject": s.ID,
							"since": s.Since, "days": int64(now.Sub(s.Since).Hours() / 24)},
					})
					if errors.Is(err, journal.ErrExists) {
						continue
					}
					if err != nil {
						return out, fmt.Errorf("funnel %s onStuck: %w", f.Metadata.Name, err)
					}
					out = append(out, res)
				}
			}
		}
	}
	for _, x := range h.guarded {
		res, err := h.checkGuardrails(ctx, x, now)
		if err != nil {
			return out, fmt.Errorf("experiment %s: %w", x.Metadata.Name, err)
		}
		if res != nil {
			out = append(out, *res)
		}
	}
	return out, nil
}

// checkGuardrails stops an experiment whose variant breached a guardrail:
// it starts onBreach with the stop run's ID, once (the run's existence is
// the stop; the experiment step gives the control from then on).
func (h *Host) checkGuardrails(ctx context.Context, x *agents.Experiment, now time.Time) (*runtime.Result, error) {
	id := runtime.ExperimentStopRun(h.opts.Namespace, x.Metadata.Name)
	if stopped, err := h.experimentStopped(ctx, x.Metadata.Name); err != nil || stopped {
		return nil, err
	}
	ms, err := h.Runtime.Store.Milestones(ctx, h.opts.Namespace, x.Spec.Subject)
	if err != nil {
		return nil, err
	}
	exposed := metrics.Exposures(x.Metadata.Name, ms, now)
	names := make([]string, len(x.Spec.Variants))
	for i, v := range x.Spec.Variants {
		names[i] = v.Name
	}
	alpha := x.Spec.Alpha
	if alpha == 0 {
		alpha = 0.05
	}
	for _, g := range x.Spec.Guardrails {
		within, err := metrics.ParseDuration(g.Within)
		if err != nil {
			return nil, err
		}
		r := metrics.CheckGuardrail(metrics.Guardrail{Name: g.Name, Milestone: g.Milestone, Within: within, MaxIncrease: g.MaxIncrease}, names, exposed, ms, now, alpha)
		if r.Breach == "" {
			continue
		}
		rate := func(v string) float64 {
			for _, a := range r.Arms {
				if a.Variant == v {
					return a.Rate
				}
			}
			return 0
		}
		res, err := h.Runtime.Start(ctx, runtime.Start{
			Namespace: h.opts.Namespace, Function: x.Spec.OnBreach.Function, Conversation: "experiment:" + id,
			Caller: map[string]any{"experiment": x.Metadata.Name}, Trigger: x.Metadata.Name, RunID: id,
			Payload: map[string]any{"experiment": x.Metadata.Name, "guardrail": g.Name, "variant": r.Breach,
				"controlRate": rate(names[0]), "variantRate": rate(r.Breach), "difference": r.Difference},
		})
		if errors.Is(err, journal.ErrExists) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &res, nil
	}
	return nil, nil
}

func guarded(docs []admission.Doc, namespace string) []*agents.Experiment {
	var out []*agents.Experiment
	for _, d := range docs {
		if x := d.Experiment; x != nil && x.Metadata.Namespace == namespace && len(x.Spec.Guardrails) > 0 && x.Spec.OnBreach != nil {
			out = append(out, x)
		}
	}
	return out
}

func funnels(docs []admission.Doc, namespace string) []*agents.Funnel {
	var out []*agents.Funnel
	for _, d := range docs {
		if f := d.Funnel; f != nil && f.Metadata.Namespace == namespace && f.Spec.OnStuck != nil {
			out = append(out, f)
		}
	}
	return out
}
