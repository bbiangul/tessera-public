package admission

import (
	"fmt"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// channelWindow is the free-text window of the channel the namespace's
// Agent uses, or 0.
func channelWindow(docs []Doc, pluginsByName map[string]*plugins.PluginManifest, ns string) time.Duration {
	for _, d := range docs {
		if d.Agent == nil || d.Agent.Metadata.Namespace != ns || d.Agent.Spec.Channel == "" {
			continue
		}
		if p := pluginsByName[d.Agent.Spec.Channel]; p != nil && p.Spec.Channel != nil {
			w, _ := time.ParseDuration(p.Spec.Channel.Window)
			return w
		}
	}
	return 0
}

// checkAgentChannel: an Agent's channel is a plugin that declares one.
func checkAgentChannel(d Doc, pluginsByName map[string]*plugins.PluginManifest) []Issue {
	ch := d.Agent.Spec.Channel
	if ch == "" {
		return nil
	}
	if p := pluginsByName[ch]; p == nil || p.Spec.Channel == nil {
		return []Issue{{Severity: SeverityError, Code: "UNKNOWN_PLUGIN", File: d.File, Resource: d.ID(), Path: "spec.channel",
			Message: fmt.Sprintf("no plugin %q declaring spec.channel", ch), Fix: "name the channel plugin (e.g. whatsapp)"}}
	}
	return nil
}

func triggerNames(docs []Doc) map[string]bool {
	out := map[string]bool{}
	for _, d := range docs {
		if d.Trigger != nil {
			out[d.Trigger.Metadata.Namespace+"/"+d.Trigger.Metadata.Name] = true
		}
	}
	return out
}

// checkQueue checks a HumanQueue's SLA.
func checkQueue(d Doc, agentZone string) []Issue {
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	s := d.Queue.Spec.SLA
	if s == nil {
		return nil
	}
	for field, v := range map[string]string{"remind": s.Remind, "escalate": s.Escalate, "timeout": s.Timeout} {
		if v == "" {
			continue
		}
		if dur, err := time.ParseDuration(v); err != nil || dur <= 0 {
			add("INVALID_FIELD", "spec.sla."+field, fmt.Sprintf("%q is not a positive duration", v), "e.g. 4h")
		}
	}
	if (s.Escalate == "") != (s.EscalateTo == "") {
		add("MISSING_FIELD", "spec.sla", "escalate and escalateTo go together", "")
	}
	if s.OnTimeout != nil {
		if s.Timeout == "" {
			add("MISSING_FIELD", "spec.sla.timeout", "onTimeout needs a timeout", "")
		}
		if (s.OnTimeout.Answer == nil) == !s.OnTimeout.Fail {
			add("STEP_KIND", "spec.sla.onTimeout", "onTimeout sets exactly one of answer and fail", "")
		}
		// An approval that times out gets this answer (#54): true approves
		// every one nobody looked at; anything but a boolean is no yes.
		warn := func(code, msg, fix string) {
			issues = append(issues, Issue{Severity: SeverityWarning, Code: code, File: d.File, Resource: d.ID(), Path: "spec.sla.onTimeout.answer", Message: msg, Fix: fix})
		}
		switch a := s.OnTimeout.Answer.(type) {
		case nil:
		case bool:
			if a {
				warn("APPROVES_ON_TIMEOUT", "answer: true approves every approval task that nobody answered in time", "answer: false (deny), or fail: true, unless approving unseen is intended")
			}
		default:
			warn("TIMEOUT_ANSWER_NOT_BOOL", "an approval task that times out reads this answer as no yes, so it is denied; other tasks receive the value", "use true or false for approvals, or a separate queue for tasks that need a value")
		}
	} else if s.Timeout != "" {
		add("MISSING_FIELD", "spec.sla.onTimeout", "a timeout needs onTimeout: the answer to use, or fail", "")
	}
	if s.BusinessHours != nil && agentZone == "" {
		add("MISSING_FIELD", "spec.sla.businessHours", "business hours need the Agent's spec.timezone", "")
	}
	return issues
}
