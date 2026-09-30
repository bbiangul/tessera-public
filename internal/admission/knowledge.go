package admission

import (
	"fmt"
	"slices"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"

	"github.com/bbiangul/tessera/internal/knowledge"
)

// checkKnowledge checks KnowledgeBases (knowledge-001): their globs match
// documents the index reads, their audiences are the namespace Agent's,
// and every free mode names existing bases.
func (c *Checker) checkKnowledge(docs []Doc) []Issue {
	var issues []Issue
	actions := map[string]plugins.ActionSpec{} // plugin/action
	for _, d := range docs {
		if d.Plugin != nil {
			for name, a := range d.Plugin.Spec.Actions {
				actions[d.Plugin.Metadata.Name+"/"+name] = a
			}
		}
	}
	bases := map[string]bool{}  // namespace/name
	queues := map[string]bool{} // namespace/name
	hitl := map[string]string{} // namespace -> the Agent's humanInTheLoop queue
	for _, d := range docs {
		switch {
		case d.Queue != nil:
			queues[d.Queue.Metadata.Namespace+"/"+d.Queue.Metadata.Name] = true
		case d.Agent != nil && d.Agent.Spec.HumanInTheLoop != nil:
			hitl[d.Agent.Metadata.Namespace] = d.Agent.Spec.HumanInTheLoop.Queue
		}
	}
	audiences := map[string][]string{} // namespace -> the Agent's audience names
	for _, d := range docs {
		if d.Agent != nil {
			for _, a := range d.Agent.Spec.Audiences {
				audiences[d.Agent.Metadata.Namespace] = append(audiences[d.Agent.Metadata.Namespace], a.Name)
			}
		}
	}
	for _, d := range docs {
		kb := d.Knowledge
		if kb == nil {
			continue
		}
		add := func(code, path, msg, fix string) {
			issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
		}
		key := kb.Metadata.Namespace + "/" + kb.Metadata.Name
		if bases[key] {
			add("DUPLICATE", "metadata.name", "knowledge base declared twice", "")
		}
		bases[key] = true
		if len(kb.Spec.Sources) == 0 && kb.Spec.Lessons == nil {
			add("MISSING_FIELD", "spec.sources", "a knowledge base needs sources, or lessons", "")
		}
		if l := kb.Spec.Lessons; l != nil {
			q := l.Queue
			if q == "" {
				q = hitl[kb.Metadata.Namespace]
			}
			switch {
			case q == "":
				add("MISSING_FIELD", "spec.lessons.queue", "no queue to approve lessons: set lessons.queue or the Agent's humanInTheLoop", "")
			case !queues[kb.Metadata.Namespace+"/"+q]:
				add("UNKNOWN_QUEUE", "spec.lessons.queue", fmt.Sprintf("no HumanQueue %q in namespace %s", q, kb.Metadata.Namespace), "")
			}
			if l.Every != "" {
				if dur, err := time.ParseDuration(l.Every); err != nil || dur < time.Hour {
					add("INVALID_FIELD", "spec.lessons.every", fmt.Sprintf("%q is not a duration of at least 1h", l.Every), "e.g. 168h")
				}
			}
		}
		for i, s := range kb.Spec.Sources {
			sp := fmt.Sprintf("spec.sources[%d]", i)
			if (len(s.Files) > 0) == (s.List != nil) {
				add("INVALID_FIELD", sp, "a source has exactly one of files and list", "")
				continue
			}
			if s.List != nil {
				for field, a := range map[string]*agents.KnowledgeAct{"list": s.List, "fetch": s.Fetch} {
					if a == nil {
						continue
					}
					spec, ok := actions[a.Act]
					switch {
					case !ok:
						add("UNKNOWN_ACTION", sp+"."+field+".act", fmt.Sprintf("no plugin action %q", a.Act), "")
					case spec.Effect != plugins.EffectRead:
						add("EFFECT_NOT_ALLOWED", sp+"."+field+".act", fmt.Sprintf("%s is %s: a knowledge source only reads", a.Act, spec.Effect), "")
					}
					with := a.With
					if with == nil {
						with = map[string]any{}
					}
					if _, err := c.expr.CompileValue(with); err != nil {
						add("INVALID_EXPR", sp+"."+field+".with", err.Error(), "")
					}
				}
				if s.Refresh != "" {
					if dur, err := time.ParseDuration(s.Refresh); err != nil || dur < time.Minute {
						add("INVALID_FIELD", sp+".refresh", fmt.Sprintf("%q is not a duration of at least 1m", s.Refresh), "e.g. 24h")
					}
				}
				continue
			}
			if s.Fetch != nil || s.Refresh != "" {
				add("INVALID_FIELD", sp, "fetch and refresh are for a list source", "")
			}
			files, err := knowledge.Resolve(d.File, s.Files)
			p := fmt.Sprintf("spec.sources[%d].files", i)
			switch {
			case err != nil:
				add("INVALID_FIELD", p, err.Error(), "")
			case len(files) == 0:
				add("NO_FILES", p, fmt.Sprintf("%v matches no file (relative to %s)", s.Files, d.File), "check the paths; they are relative to this file")
			}
			for _, f := range files {
				if !knowledge.Supported(f) {
					add("UNSUPPORTED_FILE", p, f+": Markdown, text, PDF and DOCX are read", "convert it, or leave it out of the glob")
					continue
				}
				text, err := knowledge.Text(f)
				switch {
				case err != nil:
					add("UNREADABLE_FILE", p, err.Error(), "check the file opens; save it again as PDF or DOCX")
				case strings.TrimSpace(text) == "":
					add("NO_TEXT", p, f+": no text could be read (a scanned PDF?); OCR is not done", "export it with its text, or provide the text as Markdown")
				}
			}
		}
		for i, a := range kb.Spec.Audiences {
			if !slices.Contains(audiences[kb.Metadata.Namespace], a) {
				add("UNKNOWN_AUDIENCE", fmt.Sprintf("spec.audiences[%d]", i), fmt.Sprintf("the Agent of namespace %s has no audience %q", kb.Metadata.Namespace, a), "")
			}
		}
	}
	for _, d := range docs {
		ag := d.Agent
		if ag == nil {
			continue
		}
		check := func(path string, names []string) {
			for i, b := range names {
				if !bases[ag.Metadata.Namespace+"/"+b] {
					issues = append(issues, Issue{Severity: SeverityError, Code: "UNKNOWN_KNOWLEDGE", File: d.File, Resource: d.ID(), Path: fmt.Sprintf("%s.knowledge.bases[%d]", path, i),
						Message: fmt.Sprintf("no KnowledgeBase %q in namespace %s", b, ag.Metadata.Namespace), Fix: "declare it, or remove the name"})
				}
			}
		}
		if fm := ag.Spec.FreeMode; fm != nil && fm.Knowledge != nil {
			check("spec.freeMode", fm.Knowledge.Bases)
		}
		for i, a := range ag.Spec.Audiences {
			if a.FreeMode != nil && a.FreeMode.Knowledge != nil {
				check(fmt.Sprintf("spec.audiences[%d].freeMode", i), a.FreeMode.Knowledge.Bases)
			}
		}
	}
	return issues
}
