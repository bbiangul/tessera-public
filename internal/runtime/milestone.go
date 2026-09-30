package runtime

import (
	"errors"
	"fmt"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/metrics"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// milestone is a compiled MilestoneRef.
type milestone struct {
	name, namespace, subject string
	id                       *expr.Expr
	attrs                    map[string]*expr.Expr
}

func (p *Program) milestone(c *compiler, ns string, m *agents.MilestoneRef, path string) *milestone {
	if m == nil {
		return nil
	}
	s := p.subjects[ns+"/"+m.Subject]
	if s == nil {
		c.fail(path+".subject", "a Subject of namespace "+ns)
		return nil
	}
	src := string(m.ID)
	if src == "" {
		src = string(s.Spec.ID)
	}
	out := &milestone{name: m.Name, namespace: ns, subject: m.Subject, id: c.expr(src, expr.Any, path+".id"), attrs: map[string]*expr.Expr{}}
	for k, v := range m.Attributes {
		out.attrs[k] = c.expr(string(v), expr.Any, path+".attributes."+k)
	}
	return out
}

// recordMilestone records a milestone after the site succeeded. It is
// written once: a resumed run finds it and moves on.
func (e *execution) recordMilestone(f *frame, m *milestone, site string) error {
	at := site + "/milestone"
	if _, ok, err := e.get(at, journal.KindMilestone); err != nil || ok {
		return err
	}
	actv := e.activation(f, nil)
	idv, err := m.id.Eval(e.ctx, actv)
	if err != nil {
		return e.evalFailure(f, at+".id", err)
	}
	id := fmt.Sprint(idv)
	if idv == nil || id == "" {
		return e.fail(f, at+": the subject's id is empty")
	}
	attrs := map[string]any{}
	for k, x := range m.attrs {
		v, err := x.Eval(e.ctx, actv)
		if err != nil {
			return e.evalFailure(f, at+".attributes."+k, err)
		}
		attrs[k] = fmt.Sprint(v)
	}
	_, err = e.put(at, journal.KindMilestone, map[string]any{
		"namespace": m.namespace, "subject": m.subject, "id": id, "name": m.name,
		"at": e.rt.Clock(), "attributes": attrs,
	})
	return err
}

// experiment is a compiled experiment step.
type experiment struct {
	name, namespace, subject string
	id                       *expr.Expr
	variants                 []metrics.Variant
}

func (p *Program) experiment(c *compiler, ns, name, path string) *experiment {
	x := p.experiments[ns+"/"+name]
	if x == nil {
		c.fail(path, "an Experiment of namespace "+ns)
		return nil
	}
	s := p.subjects[ns+"/"+x.Spec.Subject]
	if s == nil {
		c.fail(path, "the experiment's Subject")
		return nil
	}
	out := &experiment{name: name, namespace: ns, subject: x.Spec.Subject, id: c.expr(string(s.Spec.ID), expr.Any, path+" (subject id)")}
	for _, v := range x.Spec.Variants {
		out.variants = append(out.variants, metrics.Variant{Name: v.Name, Weight: v.Weight})
	}
	return out
}

// ExposureName is the milestone that records a subject saw a variant.
func ExposureName(experiment string) string { return "experiment/" + experiment }

// expose assigns the subject's variant and records the exposure once, as
// a milestone with the variant as an attribute: analysis counts only
// subjects that saw a variant.
func (e *execution) expose(f *frame, x *experiment, site string) (any, error) {
	at := site + "/exposure"
	if v, ok, err := e.get(at, journal.KindMilestone); err != nil || ok {
		m, _ := v.(map[string]any)
		a, _ := m["attributes"].(map[string]any)
		return a["variant"], err
	}
	idv, err := x.id.Eval(e.ctx, e.activation(f, nil))
	if err != nil {
		return nil, e.evalFailure(f, at+".id", err)
	}
	id := fmt.Sprint(idv)
	if idv == nil || id == "" {
		return nil, e.fail(f, at+": the subject's id is empty")
	}
	variant := metrics.Assign(x.name, id, x.variants)
	attrs := map[string]any{"variant": variant}
	// A guardrail stopped the experiment: the control for everyone from
	// now on, left out of the comparison. Recorded once, like the rest.
	switch _, err := e.rt.Store.GetRun(e.ctx, ExperimentStopRun(x.namespace, x.name)); {
	case err == nil:
		variant = x.variants[0].Name
		attrs = map[string]any{"variant": variant, "stopped": "true"}
	case !errors.Is(err, journal.ErrNotFound):
		return nil, err
	}
	_, err = e.put(at, journal.KindMilestone, map[string]any{
		"namespace": x.namespace, "subject": x.subject, "id": id, "name": ExposureName(x.name),
		"at": e.rt.Clock(), "attributes": attrs,
	})
	return variant, err
}

// ExperimentStopRun is the ID of the run a guardrail breach starts: while
// it exists, the experiment is stopped.
func ExperimentStopRun(namespace, experiment string) string {
	return namespace + "/experiment/" + experiment + "/stopped"
}
