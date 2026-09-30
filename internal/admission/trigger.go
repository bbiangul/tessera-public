package admission

import (
	"fmt"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/cron"
	"github.com/bbiangul/tessera/internal/expr"
)

// checkTrigger checks a Trigger: its Function, its schedule and the time
// zone the schedule is read in.
func checkTrigger(d Doc, functions map[string]*agents.Function, agent *agents.Agent, subjects map[string]*agents.Subject) []Issue {
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	t := d.Trigger
	switch {
	case t.Spec.Function == "" && t.Spec.Event == nil:
		add("MISSING_FIELD", "spec.function", "a trigger starts a Function, delivers an event, or both", "set function or event")
	case t.Spec.Function != "":
		if _, ok := functions[t.Metadata.Namespace+"/"+t.Spec.Function]; !ok {
			add("UNKNOWN_FUNCTION", "spec.function", fmt.Sprintf("no Function %q in namespace %s", t.Spec.Function, t.Metadata.Namespace), "")
		}
	}
	kinds := 0
	for _, set := range []bool{t.Spec.Schedule != nil, t.Spec.Webhook != nil, t.Spec.Milestone != nil} {
		if set {
			kinds++
		}
	}
	if kinds != 1 {
		add("STEP_KIND", "spec", "a trigger needs exactly one of schedule, webhook and milestone", "")
		return issues
	}
	if m := t.Spec.Milestone; m != nil {
		if t.Spec.Function == "" {
			add("MISSING_FIELD", "spec.function", "a milestone trigger starts a Function", "")
		}
		if subjects != nil && subjects[t.Metadata.Namespace+"/"+m.Subject] == nil {
			add("UNKNOWN_SUBJECT", "spec.milestone.subject", fmt.Sprintf("no Subject %q in namespace %s", m.Subject, t.Metadata.Namespace), "")
		}
	}
	if t.Spec.Event != nil {
		if t.Spec.Webhook == nil {
			add("INVALID_FIELD", "spec.event", "only a webhook delivers events", "use a webhook, or start a Function")
		} else {
			issues = append(issues, triggerExpr(d, string(t.Spec.Event.Key), "spec.event.key")...)
			if !listened(functions, t.Metadata.Namespace, t.Spec.Event.Name) {
				issues = append(issues, Issue{Severity: SeverityWarning, Code: "UNUSED_FIELD", File: d.File, Resource: d.ID(), Path: "spec.event.name",
					Message: fmt.Sprintf("no wait in namespace %s listens for event %q: every delivery %s", t.Metadata.Namespace, t.Spec.Event.Name,
						map[bool]string{true: "starts the Function", false: "is dropped"}[t.Spec.Function != ""]), Fix: "add it to a wait's events"})
			}
		}
	}
	if w := t.Spec.Webhook; w != nil {
		if w.Key != "" {
			issues = append(issues, triggerExpr(d, string(w.Key), "spec.webhook.key")...)
		}
		if w.Conversation != "" {
			issues = append(issues, triggerExpr(d, string(w.Conversation), "spec.webhook.conversation")...)
		}
	}
	s := t.Spec.Schedule
	if s == nil {
		return issues // a webhook: the schema checks the secret's name
	}
	if _, err := cron.Parse(s.Cron); err != nil {
		add("INVALID_FIELD", "spec.schedule.cron", err.Error(), `minute hour day month weekday, e.g. "0 9 * * 1-5"`)
	}
	zone := s.Timezone
	if zone == "" && agent != nil {
		zone = agent.Spec.Timezone
	}
	switch {
	case zone == "":
		add("MISSING_FIELD", "spec.schedule.timezone", "a schedule needs a time zone: its own or the Agent's spec.timezone", "a server's zone is never assumed")
	case zone == "Local":
		add("INVALID_FIELD", "spec.schedule.timezone", "Local is the server's zone", "an IANA name such as America/Sao_Paulo")
	default:
		if _, err := time.LoadLocation(zone); err != nil {
			add("INVALID_FIELD", "spec.schedule.timezone", fmt.Sprintf("unknown time zone %q", zone), "")
		}
	}
	if s.MaxDelay != "" {
		if d, err := time.ParseDuration(s.MaxDelay); err != nil || d <= 0 || d > 7*24*time.Hour {
			add("INVALID_FIELD", "spec.schedule.maxDelay", fmt.Sprintf("%q is not a duration between 0 and 168h", s.MaxDelay), "")
		}
	}
	return issues
}

// triggerExpr checks an expression of a webhook: it reads event.payload.
func triggerExpr(d Doc, src, path string) []Issue {
	e, err := expr.New()
	if err != nil {
		return nil
	}
	x, err := e.Compile(src, expr.Any)
	if err != nil {
		return []Issue{{Severity: SeverityError, Code: "INVALID_EXPR", File: d.File, Resource: d.ID(), Path: path, Message: err.Error()}}
	}
	for _, r := range x.References() {
		if r != "event" && !strings.HasPrefix(r, "event.payload") {
			return []Issue{{Severity: SeverityError, Code: "UNKNOWN_REF", File: d.File, Resource: d.ID(), Path: path,
				Message: fmt.Sprintf("%s: a webhook's expressions read only event.payload", r), Fix: "e.g. event.payload.leadId"}}
		}
	}
	return nil
}

// listened: some wait of the namespace listens for the event.
func listened(functions map[string]*agents.Function, ns, event string) bool {
	for key, fn := range functions {
		if !strings.HasPrefix(key, ns+"/") {
			continue
		}
		for _, st := range fn.Spec.Steps {
			if st.Wait == nil {
				continue
			}
			for _, ev := range st.Wait.Events {
				if ev.Name == event {
					return true
				}
			}
		}
	}
	return false
}
