// Package inspect explains runs from their journal (the console, tessera
// runs, the /v1 API): each record with what it was in the definitions
// (which input source, step or action) and a one-line summary; a
// conversation's transcript across its runs.
package inspect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Entry is one journal record, explained.
type Entry struct {
	Seq     int          `json:"seq"`
	At      time.Time    `json:"at"`
	Kind    journal.Kind `json:"kind"`
	Site    string       `json:"site"`
	What    string       `json:"what"`    // the definition behind the site
	Summary string       `json:"summary"` // what happened, in one line
	Value   any          `json:"value"`
}

// Run is a run with its timeline.
type Run struct {
	journal.Run
	Scopes   any     `json:"scopes"`
	Timeline []Entry `json:"timeline"`
}

// Line is one line of a conversation's transcript.
type Line struct {
	At    time.Time `json:"at"`
	From  string    `json:"from"` // user, agent, queue:<name>
	Text  string    `json:"text"`
	RunID string    `json:"runId"`
}

// Explainer annotates sites with the admitted definitions.
type Explainer struct {
	functions map[string]*agents.Function // namespace/name
}

// New indexes the definitions.
func New(docs []admission.Doc) *Explainer {
	x := &Explainer{functions: map[string]*agents.Function{}}
	for _, d := range docs {
		if d.Function != nil {
			x.functions[d.Function.Metadata.Namespace+"/"+d.Function.Metadata.Name] = d.Function
		}
	}
	return x
}

// Explain builds a run's timeline.
func (x *Explainer) Explain(run journal.Run, recs []journal.Record) (Run, error) {
	out := Run{Run: run}
	if scopes, err := journal.Decode(run.Scopes); err == nil {
		out.Scopes = scopes
	}
	fn := x.functions[run.Namespace+"/"+run.Function]
	for _, r := range recs {
		v, err := journal.Decode(r.Value)
		if err != nil {
			return Run{}, fmt.Errorf("record %d: %w", r.Seq, err)
		}
		out.Timeline = append(out.Timeline, Entry{Seq: r.Seq, At: r.At, Kind: r.Kind, Site: r.Site, What: what(fn, r.Site), Summary: summary(r.Kind, r.Site, v), Value: v})
	}
	return out, nil
}

// Transcript is what was said in a conversation, across its runs, in
// order: each run's triggering message, the agent's questions and
// messages, and the replies.
func Transcript(runs []journal.Run, records map[string][]journal.Record) []Line {
	var out []Line
	for _, run := range runs {
		if scopes, err := journal.Decode(run.Scopes); err == nil {
			if ev, ok := scopes.(map[string]any)["event"].(map[string]any); ok {
				if msg, _ := ev["message"].(string); msg != "" {
					out = append(out, Line{At: run.CreatedAt, From: "user", Text: msg, RunID: run.ID})
				}
			}
		}
		for _, r := range records[run.ID] {
			v, err := journal.Decode(r.Value)
			if err != nil {
				continue
			}
			m, _ := v.(map[string]any)
			switch r.Kind {
			case journal.KindSend, journal.KindSuspend:
				to, _ := m["to"].(string)
				text, _ := m["text"].(string)
				from := "agent"
				if strings.HasPrefix(to, "queue:") {
					from = "agent → " + to
				}
				out = append(out, Line{At: r.At, From: from, Text: text, RunID: run.ID})
			case journal.KindResume:
				if timer, _ := m["timer"].(bool); timer {
					continue
				}
				text, _ := m["text"].(string)
				if text == "" && m["value"] != nil {
					text = compact(m["value"])
				}
				out = append(out, Line{At: r.At, From: "reply", Text: text, RunID: run.ID})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// what names the definition behind a site: "inputs.appointment/upcoming"
// is the source upcoming of input appointment, a read of
// agenda/list-upcoming-appointments.
func what(fn *agents.Function, site string) string {
	parts := strings.Split(site, "/")
	head := parts[0]
	tail := strings.Join(parts[1:], "/")
	suffix := func(s string) string {
		switch {
		case s == "":
			return ""
		case strings.HasPrefix(s, "decide"):
			return ", Jev decision"
		case strings.HasPrefix(s, "ask"):
			return ", question to the user"
		case strings.HasPrefix(s, "extract"):
			return ", extraction of the answer"
		case s == "milestone":
			return ", milestone"
		case s == "exposure":
			return ", experiment exposure"
		case strings.HasPrefix(s, "wake"):
			return ", wake time"
		}
		return ", " + s
	}
	switch {
	case site == "router":
		return "router"
	case head == "free":
		return "free-mode reply (outside any Function)"
	case site == "output":
		return "the Function's output"
	case site == "inputs/extract" || strings.HasSuffix(site, "/inputs/extract"):
		return "extraction of the inputs from the message (one Generate call)"
	case head == "action" || strings.HasPrefix(head, "action"):
		if fn != nil && fn.Spec.Action != nil {
			return "action " + fn.Spec.Action.Act + suffix(tail)
		}
		return "action" + suffix(tail)
	case strings.HasPrefix(head, "confirm"):
		return "confirmation" + suffix(tail)
	case strings.HasPrefix(head, "preconditions"):
		return head
	case strings.HasPrefix(head, "inputs."):
		name := strings.TrimPrefix(head, "inputs.")
		src := ""
		if len(parts) > 1 {
			src = parts[1]
		}
		desc := "input " + name
		if fn != nil {
			if in, ok := fn.Spec.Inputs[name]; ok {
				for _, s := range in.Resolve {
					if s.ID != src {
						continue
					}
					switch {
					case s.FromAction != "":
						desc += ", read " + s.FromAction
					case s.FromFunction != "":
						desc += ", Function " + s.FromFunction
					case s.Extract != nil:
						desc += ", extracted from the message"
					case s.Ask != nil:
						desc += ", asked"
					case s.Value != "":
						desc += ", computed"
					}
				}
			}
		}
		if src != "" {
			desc += " (source " + src + ")"
		}
		rest := ""
		if len(parts) > 2 {
			rest = strings.Join(parts[2:], "/")
		}
		return desc + suffix(rest)
	case strings.HasPrefix(head, "steps."):
		id := strings.TrimPrefix(head, "steps.")
		desc := "step " + id
		if fn != nil {
			for _, st := range fn.Spec.Steps {
				if st.ID != id {
					continue
				}
				switch {
				case st.Act != "":
					desc += ": act " + st.Act
				case st.Call != "":
					desc += ": call " + st.Call
				case st.DecideBatch != nil:
					desc += ": decide batch"
				case st.Decide != nil:
					desc += ": decide"
				case st.Ask != nil:
					desc += ": ask"
				case st.Respond != nil:
					desc += ": respond"
				case st.Escalate != nil:
					desc += ": escalate to " + st.Escalate.Queue
				case st.Wait != nil:
					desc += ": wait"
				case st.Experiment != "":
					desc += ": experiment " + st.Experiment
				case st.Compute != nil:
					desc += ": compute"
				case st.Branch != nil:
					desc += ": branch"
				case st.Generate != nil, st.Extract != nil:
					desc += ": generate"
				}
			}
		}
		return desc + suffix(tail)
	}
	return site
}

func summary(kind journal.Kind, site string, v any) string {
	m, _ := v.(map[string]any)
	str := func(k string) string { s, _ := m[k].(string); return s }
	switch kind {
	case journal.KindSend:
		return fmt.Sprintf("message to %s: %s", str("to"), str("text"))
	case journal.KindSuspend:
		if t := str("task"); t != "" {
			return fmt.Sprintf("waits for %s (task %s): %s", str("to"), t, str("text"))
		}
		return fmt.Sprintf("waits for %s: %s", str("to"), str("text"))
	case journal.KindResume:
		if timer, _ := m["timer"].(bool); timer {
			return "woke up (time came)"
		}
		if str("text") != "" {
			return "reply: " + str("text")
		}
		return "answer: " + compact(m["value"])
	case journal.KindIntent:
		return fmt.Sprintf("about to call %s %s (key %s)", str("action"), compact(m["args"]), str("key"))
	case journal.KindError:
		return "failed: " + compact(v)
	case journal.KindSleep:
		return "sleeps until " + compact(m["wake"])
	case journal.KindMilestone:
		if strings.HasPrefix(str("name"), "experiment/") {
			attrs, _ := m["attributes"].(map[string]any)
			return fmt.Sprintf("%s %s saw variant %v of %s", str("subject"), str("id"), attrs["variant"], strings.TrimPrefix(str("name"), "experiment/"))
		}
		return fmt.Sprintf("milestone %s for %s %s", str("name"), str("subject"), str("id"))
	case journal.KindResult:
		if site == "router" {
			return fmt.Sprintf("routed to %q; probabilities %s (threshold %v)", str("choice"), compact(m["probabilities"]), m["threshold"])
		}
		if c, ok := m["choice"]; ok {
			return fmt.Sprintf("Jev chose %v (p=%v) for %q among %s", c, m["p"], str("question"), compact(m["options"]))
		}
		if _, ok := m["p"]; ok && m["choice"] == nil {
			return fmt.Sprintf("Jev: yes with p=%v for %q", m["p"], str("question"))
		}
		if vals, ok := m["values"]; ok && strings.HasSuffix(site, "extract") {
			return fmt.Sprintf("%s extracted %s", str("model"), compact(vals))
		}
		return "result: " + compact(v)
	}
	return compact(v)
}

// compact is a short JSON of a value, cut at 300 bytes.
func compact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}
