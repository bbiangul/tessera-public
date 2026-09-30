package admission

import (
	"fmt"
	"slices"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// notifyInputs are what a queue's notify actions read, as inputs.<name>.
var notifyInputs = []string{"kind", "link", "queue", "runId", "since", "stage", "text"}

// checkNotify checks a HumanQueue's notify actions: each names an
// external-message action of a plugin being validated that is not a users'
// channel, its with gives exactly the action's parameters, and its
// expressions read only the task (inputs.*).
func (c *Checker) checkNotify(d Doc, pluginsByName map[string]*plugins.PluginManifest) []Issue {
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	for i, n := range d.Queue.Spec.Notify {
		path := fmt.Sprintf("spec.notify[%d]", i)
		pl, name, ok := strings.Cut(n.Act, "/")
		p := pluginsByName[pl]
		if !ok || name == "" {
			add("UNKNOWN_ACTION", path+".act", fmt.Sprintf("%q is not plugin/action", n.Act), "e.g. slack/post")
			continue
		}
		if p == nil {
			add("UNKNOWN_PLUGIN", path+".act", fmt.Sprintf("no PluginManifest named %q", pl), "add the plugin's manifest to the files being validated")
			continue
		}
		a, ok := p.Spec.Actions[name]
		if !ok {
			add("UNKNOWN_ACTION", path+".act", fmt.Sprintf("plugin %s has no action %q", pl, name), "use one of "+strings.Join(sortedKeys(p.Spec.Actions), ", "))
			continue
		}
		// Notifications reach the agent's team, outside the rules that
		// protect its users: never through the users' channel plugin
		// (consent, window), and only messages, never other effects.
		if p.Spec.Channel != nil {
			add("NOTIFY_USER_CHANNEL", path+".act", fmt.Sprintf("plugin %s is the users' channel (consent, window); notifications would skip its rules", pl),
				"declare the team's channel as its own plugin, without spec.channel (e.g. whatsapp-equipe)")
		}
		if a.Effect != plugins.EffectExternalMessage {
			add("EFFECT_NOT_ALLOWED", path+".act", fmt.Sprintf("%s has effect %s; a notification only sends a message", n.Act, a.Effect), "use an external-message action")
		}
		for _, k := range sortedKeys(n.With) {
			if _, ok := a.Input[k]; !ok {
				add("UNKNOWN_PARAM", path+".with."+k, fmt.Sprintf("the action has no parameter %q", k), "parameters: "+strings.Join(sortedKeys(a.Input), ", "))
			}
		}
		for _, k := range sortedKeys(a.Input) {
			if _, ok := n.With[k]; !ok {
				add("MISSING_PARAM", path+".with", fmt.Sprintf("missing parameter %q (%s)", k, a.Input[k]), "")
			}
		}
		if n.With == nil {
			continue
		}
		cv, err := c.expr.CompileValue(n.With)
		if err != nil {
			add("INVALID_EXPR", path+".with", err.Error(), "")
			continue
		}
		for _, r := range cv.References() {
			root, field, _ := strings.Cut(r, ".")
			field, _, _ = strings.Cut(field, ".")
			if root != "inputs" || !slices.Contains(notifyInputs, field) {
				add("UNKNOWN_REF", path+".with", fmt.Sprintf("%s: a notification reads only the task", r), "inputs."+strings.Join(notifyInputs, ", inputs."))
			}
		}
	}
	return issues
}

// checkFreeMode checks an Agent's free mode: every read names a
// read-only action of a plugin being validated, with the action's
// parameters, and its expressions read only the caller. The free mode
// never acts: effects happen only in Functions (RFC-0004).
func (c *Checker) checkFreeMode(d Doc, pluginsByName map[string]*plugins.PluginManifest, functions map[string]*agents.Function) []Issue {
	issues := c.checkFreeModeAt(d, d.Agent.Spec.FreeMode, "spec.freeMode", pluginsByName, functions)
	for i, a := range d.Agent.Spec.Audiences {
		issues = append(issues, c.checkFreeModeAt(d, a.FreeMode, fmt.Sprintf("spec.audiences[%d].freeMode", i), pluginsByName, functions)...)
	}
	return issues
}

// checkFreeModeAt checks one free mode (the Agent's or an audience's) at
// base.
func (c *Checker) checkFreeModeAt(d Doc, fm *agents.FreeMode, base string, pluginsByName map[string]*plugins.PluginManifest, functions map[string]*agents.Function) []Issue {
	if fm == nil {
		return nil
	}
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	for i, r := range fm.Reads {
		path := fmt.Sprintf("%s.reads[%d]", base, i)
		pl, name, _ := strings.Cut(r.Act, "/")
		p := pluginsByName[pl]
		if p == nil {
			add("UNKNOWN_PLUGIN", path+".act", fmt.Sprintf("no PluginManifest named %q", pl), "")
			continue
		}
		a, ok := p.Spec.Actions[name]
		if !ok {
			add("UNKNOWN_ACTION", path+".act", fmt.Sprintf("plugin %s has no action %q", pl, name), "use one of "+strings.Join(sortedKeys(p.Spec.Actions), ", "))
			continue
		}
		if a.Effect != plugins.EffectRead {
			add("EFFECT_NOT_ALLOWED", path+".act", fmt.Sprintf("%s has effect %s: the free mode only reads; actions happen in Functions", r.Act, a.Effect), "start a Function for it instead")
		}
		for _, k := range sortedKeys(a.Input) {
			if _, ok := r.With[k]; !ok {
				add("MISSING_PARAM", path+".with", fmt.Sprintf("missing parameter %q (%s)", k, a.Input[k]), "")
			}
		}
		if len(r.With) == 0 {
			continue
		}
		cv, err := c.expr.CompileValue(r.With)
		if err != nil {
			add("INVALID_EXPR", path+".with", err.Error(), "")
			continue
		}
		for _, ref := range cv.References() {
			if ref != "caller" && !strings.HasPrefix(ref, "caller.") {
				add("UNKNOWN_REF", path+".with", ref+": the free mode's reads know only the caller", "e.g. caller.patientId")
			}
		}
	}
	for i, f := range fm.Functions {
		path := fmt.Sprintf("%s.functions[%d]", base, i)
		if _, ok := functions[d.Agent.Metadata.Namespace+"/"+f.Function]; !ok {
			add("UNKNOWN_FUNCTION", path+".function", fmt.Sprintf("no Function %q in namespace %s", f.Function, d.Agent.Metadata.Namespace), "")
		}
		if f.MinConfidence < 0 || f.MinConfidence > 1 {
			add("INVALID_FIELD", path+".minConfidence", "must be between 0 and 1", "")
		}
	}
	if fm.WhenUnsure != "" {
		if _, ok := functions[d.Agent.Metadata.Namespace+"/"+fm.WhenUnsure]; !ok {
			add("UNKNOWN_FUNCTION", base+".whenUnsure", fmt.Sprintf("no Function %q in namespace %s", fm.WhenUnsure, d.Agent.Metadata.Namespace), "")
		}
	}
	if s := fm.Search; s != nil {
		pl, name, _ := strings.Cut(s.Act, "/")
		switch p := pluginsByName[pl]; {
		case p == nil:
			add("UNKNOWN_PLUGIN", base+".search.act", fmt.Sprintf("no PluginManifest named %q", pl), "tessera add web writes one")
		case p.Spec.Actions[name].Effect == "":
			add("UNKNOWN_ACTION", base+".search.act", fmt.Sprintf("plugin %s has no action %q", pl, name), "use one of "+strings.Join(sortedKeys(p.Spec.Actions), ", "))
		default:
			a := p.Spec.Actions[name]
			if a.Effect != plugins.EffectRead {
				add("EFFECT_NOT_ALLOWED", base+".search.act", fmt.Sprintf("%s has effect %s: a search only reads", s.Act, a.Effect), "")
			}
			if _, ok := a.Input["query"]; !ok || len(a.Input) != 1 {
				add("INVALID_FIELD", base+".search.act", s.Act+" must take exactly one input, query", "the contract of tessera add web")
			}
		}
		if s.MaxResults < 0 || s.MaxResults > 10 {
			add("INVALID_FIELD", base+".search.maxResults", "must be between 0 and 10", "")
		}
	}
	for i, ref := range fm.RunReads {
		path := fmt.Sprintf("%s.runReads[%d]", base, i)
		pl, name, _ := strings.Cut(ref, "/")
		p := pluginsByName[pl]
		if p == nil {
			add("UNKNOWN_PLUGIN", path, fmt.Sprintf("no PluginManifest named %q", pl), "")
			continue
		}
		a, ok := p.Spec.Actions[name]
		if !ok {
			add("UNKNOWN_ACTION", path, fmt.Sprintf("plugin %s has no action %q", pl, name), "")
		} else if a.Effect != plugins.EffectRead {
			add("EFFECT_NOT_ALLOWED", path, "only read-only input source results may be shared with the free mode", "choose a read action")
		}
	}
	return issues
}

// checkImprovise checks an Agent's improvise.repair: each action exists and
// is bound over http with a body (a repair only adds body fields), and a
// person can approve repairs (humanInTheLoop).
func checkImprovise(d Doc, pluginsByName map[string]*plugins.PluginManifest) []Issue {
	im := d.Agent.Spec.Improvise
	if im == nil {
		return nil
	}
	var issues []Issue
	add := func(code, path, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	if len(im.Repair) > 0 && d.Agent.Spec.HumanInTheLoop == nil {
		add("MISSING_FIELD", "spec.humanInTheLoop", "a person approves every repair: the Agent needs a humanInTheLoop queue", "")
	}
	for i, ref := range im.Repair {
		path := fmt.Sprintf("spec.improvise.repair[%d]", i)
		pl, name, _ := strings.Cut(ref, "/")
		p := pluginsByName[pl]
		if p == nil {
			add("UNKNOWN_PLUGIN", path, fmt.Sprintf("no PluginManifest named %q", pl), "")
			continue
		}
		a, ok := p.Spec.Actions[name]
		switch {
		case !ok:
			add("UNKNOWN_ACTION", path, fmt.Sprintf("plugin %s has no action %q", pl, name), "")
		case a.HTTP == nil || a.HTTP.Body == nil:
			add("INVALID_FIELD", path, ref+" must be an http action that sends a body: a repair only adds body fields", "")
		}
	}
	return issues
}
