package admission

import (
	"fmt"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// resources are the kinds around Functions, checked before them: what
// the Function checks need (subjects, experiments) and their issues.
type resources struct {
	subjects    map[string]*agents.Subject
	experiments map[string]*agents.Experiment
	issues      []Issue
}

// checkResources checks Subjects, Funnels, Experiments, HumanQueues,
// Triggers and the Agents' channel and time zone.
func (c *Checker) checkResources(docs []Doc, pluginsByName map[string]*plugins.PluginManifest, functions map[string]*agents.Function) resources {
	var out resources
	subjects := map[string]*agents.Subject{}
	for _, d := range docs {
		if d.Subject != nil {
			subjects[d.Subject.Metadata.Namespace+"/"+d.Subject.Metadata.Name] = d.Subject
			out.issues = append(out.issues, c.checkSubject(d)...)
		}
	}
	funnelsByName := map[string]*agents.Funnel{}
	experiments := map[string]*agents.Experiment{}
	for _, d := range docs {
		if d.Funnel != nil {
			funnelsByName[d.Funnel.Metadata.Namespace+"/"+d.Funnel.Metadata.Name] = d.Funnel
			out.issues = append(out.issues, checkFunnel(d, subjects, functions)...)
		}
		if d.Experiment != nil {
			experiments[d.Experiment.Metadata.Namespace+"/"+d.Experiment.Metadata.Name] = d.Experiment
		}
	}
	for _, d := range docs {
		if d.Experiment != nil {
			out.issues = append(out.issues, checkExperiment(d, subjects, funnelsByName, functions)...)
		}
	}
	agentByNS := map[string]*agents.Agent{}
	for _, d := range docs {
		if d.Agent != nil {
			agentByNS[d.Agent.Metadata.Namespace] = d.Agent
		}
	}
	out.issues = append(out.issues, c.checkTeams(docs, pluginsByName)...)
	for _, d := range docs {
		if d.Queue != nil {
			zone := ""
			if a := agentByNS[d.Queue.Metadata.Namespace]; a != nil {
				zone = a.Spec.Timezone
			}
			out.issues = append(out.issues, checkQueue(d, zone)...)
			out.issues = append(out.issues, c.checkNotify(d, pluginsByName)...)
		}
		if d.Trigger != nil {
			out.issues = append(out.issues, checkTrigger(d, functions, agentByNS[d.Trigger.Metadata.Namespace], subjects)...)
		}
		if d.Agent != nil {
			out.issues = append(out.issues, checkAgentChannel(d, pluginsByName)...)
			out.issues = append(out.issues, c.checkFreeMode(d, pluginsByName, functions)...)
			out.issues = append(out.issues, c.checkAudiences(d, functions)...)
			out.issues = append(out.issues, c.checkAutonomy(d)...)
			out.issues = append(out.issues, checkImprovise(d, pluginsByName)...)
		}
		if d.Agent != nil && d.Agent.Spec.Timezone != "" {
			if _, err := time.LoadLocation(d.Agent.Spec.Timezone); err != nil || d.Agent.Spec.Timezone == "Local" {
				out.issues = append(out.issues, Issue{Severity: SeverityError, Code: "INVALID_FIELD", File: d.File, Resource: d.ID(), Path: "spec.timezone",
					Message: fmt.Sprintf("unknown time zone %q", d.Agent.Spec.Timezone), Fix: "an IANA name such as America/Sao_Paulo"})
			}
		}
	}
	out.subjects, out.experiments = subjects, experiments
	return out
}
