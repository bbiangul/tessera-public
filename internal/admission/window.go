package admission

import (
	"fmt"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// windowCheck refuses free text that may reach a user outside the
// channel's window (RFC-0001: "a admissão recusa respond de texto livre em
// passos que podem cair fora da janela"). It follows every path a run can
// take in a namespace whose Agent uses a channel with a window:
//
//   - the window may be closed at the start of a Function a Trigger or a
//     Funnel starts (the user may never have written), after a wait that
//     alone or added to earlier ones reaches the window, after an escalate
//     (a person may take days), after a call that waits, and on the
//     timeout path of a wait with orReply;
//   - it is open again after an ask the user answered, and on the reply
//     path of a wait with orReply: a branch case whose when is exactly
//     "steps.<wait>.output.reply != null", or "!steps.<wait>.output.timedOut"
//     when the wait has no events (an event is no message from the user);
//   - free text is a respond or ask to the user, a freeText action, an
//     output template, or a call to a Function that sends free text in
//     that state. A respond, ask or output is allowed with an onError
//     fallback act that sends a template (an external-message action that
//     is not freeText): the runtime routes WINDOW_CLOSED there. A freeText
//     action is not: a fallback after it is refused (UNSAFE_FALLBACK).
type windowCheck struct {
	window    time.Duration
	ns        string
	plugins   map[string]*plugins.PluginManifest
	functions map[string]*agents.Function // namespace/name
	starts    map[string]string           // Function -> what starts it without a user message
	memo      map[string]string           // Function|state -> its first free text in that state, "" none
	waits     map[string]bool             // Function -> it can wait long (a wait, escalate or a call that does)
	visiting  map[string]bool
}

// state is where a path stands: risk says why the window may be closed
// ("" when it is open), elapsed is the time waited since it was last known
// open.
type state struct {
	risk    string
	elapsed time.Duration
}

func (c *Checker) checkWindows(docs []Doc, pluginsByName map[string]*plugins.PluginManifest, functions map[string]*agents.Function) []Issue {
	var issues []Issue
	checks := map[string]*windowCheck{}
	for _, d := range docs {
		if d.Function == nil {
			continue
		}
		ns := d.Function.Metadata.Namespace
		w, ok := checks[ns]
		if !ok {
			w = &windowCheck{window: channelWindow(docs, pluginsByName, ns), ns: ns, plugins: pluginsByName, functions: functions,
				starts: map[string]string{}, memo: map[string]string{}, waits: map[string]bool{}, visiting: map[string]bool{}}
			for _, t := range docs {
				switch {
				case t.Trigger != nil && t.Trigger.Metadata.Namespace == ns:
					w.starts[t.Trigger.Spec.Function] = "Trigger " + t.Trigger.Metadata.Name
				case t.Funnel != nil && t.Funnel.Metadata.Namespace == ns && t.Funnel.Spec.OnStuck != nil:
					w.starts[t.Funnel.Spec.OnStuck.Function] = "Funnel " + t.Funnel.Metadata.Name
				}
			}
			checks[ns] = w
		}
		if w.window <= 0 {
			continue
		}
		entry := state{}
		if by := w.starts[d.Function.Metadata.Name]; by != "" {
			entry.risk = "the start by " + by + " (the user may never have written)"
		}
		w.scan(d.Function, entry, func(path, msg, fix string) {
			issues = append(issues, Issue{Severity: SeverityError, Code: "WINDOW_RISK", File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
		})
	}
	return issues
}

const windowFix = "add onError with a fallback act that sends a template (an external-message action that is not freeText)"

// scan walks a Function's steps from st, calling emit for free text where
// the window may be closed.
func (w *windowCheck) scan(fn *agents.Function, st state, emit func(path, msg, fix string)) {
	orReply, orReplyPath, orEvents := "", "", false // the last wait with orReply
	sink := func(path, what string, s state, onError []agents.ErrorStrategy) {
		if s.risk == "" || w.guarded(onError) {
			return
		}
		emit(path, fmt.Sprintf("%s after %s: it can reach the user outside the channel's %s window", what, s.risk, w.window), windowFix)
	}
	call := func(path, name string, s state) {
		if s.risk == "" && s.elapsed == 0 {
			return
		}
		if first := w.sinks(name, s); first != "" {
			emit(path, fmt.Sprintf("calls %s, which sends free text (%s) after %s: it can reach the user outside the channel's %s window",
				name, first, s.risk+w.elapsedNote(s), w.window), "send a template on this path, or call a Function whose free text has a template fallback")
		}
	}
	for i, step := range fn.Spec.Steps {
		path := fmt.Sprintf("spec.steps[%d]", i)
		switch {
		case step.Respond != nil && toUser(step.Respond.To):
			sink(path+".respond", "free text to the user", st, step.OnError)
		case step.Ask != nil:
			sink(path+".ask", "a question to the user", st, step.OnError)
		case step.Act != "" && w.freeText(step.Act) && st.risk != "":
			// No onError makes this safe: a fallback after a send whose
			// outcome may be unknown is refused (UNSAFE_FALLBACK).
			emit(path+".act", fmt.Sprintf("%s sends free text after %s: it can reach the user outside the channel's %s window", step.Act, st.risk, w.window),
				"send a template on this path (an external-message action that is not freeText), or respond with a template fallback")
		case step.Call != "":
			call(path+".call", step.Call, st)
		}
		for j, bc := range step.Branch {
			cs := st
			if orReply != "" {
				if isReplyGuard(string(bc.When), orReply, orEvents) {
					cs = state{} // the user just replied
				} else {
					cs = state{risk: "the timeout path of " + orReplyPath}
				}
			}
			bp := fmt.Sprintf("%s.branch[%d]", path, j)
			if bc.Respond != nil && toUser(bc.Respond.To) {
				sink(bp+".respond", "free text to the user", cs, step.OnError)
			}
			if bc.Call != "" {
				call(bp+".call", bc.Call, cs)
			}
		}
		// What the step leaves behind.
		switch {
		case step.Wait != nil && step.Wait.OrReply:
			orReply, orReplyPath, orEvents = step.ID, path, len(step.Wait.Events) > 0
			st = state{risk: path + " (when it times out)"}
		case step.Wait != nil:
			d, err := time.ParseDuration(step.Wait.For)
			if err != nil || step.Wait.Until != "" {
				d = w.window // a computed time may be far
			}
			st.elapsed += d
			if st.risk == "" && st.elapsed >= w.window {
				st.risk = path
				if st.elapsed > d {
					st.risk += fmt.Sprintf(" (%s waited in all)", st.elapsed)
				}
			}
		case step.Escalate != nil:
			st.risk = path + " (a person may take longer)"
		case step.Ask != nil:
			st = state{} // the user answered
		case step.Call != "" && w.canWait(step.Call):
			st.risk = path + " (" + step.Call + " can wait)"
		}
		for _, bc := range step.Branch {
			if bc.Escalate != nil || bc.Call != "" && w.canWait(bc.Call) {
				st.risk = path + " (a branch can wait)"
			}
		}
	}
	if o := fn.Spec.Output; o != nil && o.Template != "" && st.risk != "" && !w.guarded(fn.Spec.OnError) {
		emit("spec.output.template", fmt.Sprintf("the output reaches the user after %s, which can outlast the channel's %s window", st.risk, w.window),
			"return a value instead (output.value) and send messages with a template fallback")
	}
}

func (w *windowCheck) elapsedNote(s state) string {
	if s.risk == "" && s.elapsed > 0 {
		return fmt.Sprintf("%s already waited", s.elapsed)
	}
	return ""
}

// sinks is the first free text of a called Function entered in state s,
// "" for none.
func (w *windowCheck) sinks(name string, s state) string {
	fn := w.functions[w.ns+"/"+name]
	if fn == nil {
		return "" // reported elsewhere
	}
	key := fmt.Sprintf("%s|%t|%d", name, s.risk != "", s.elapsed)
	if v, ok := w.memo[key]; ok {
		return v
	}
	if w.visiting[key] {
		return ""
	}
	w.visiting[key] = true
	defer delete(w.visiting, key)
	first := ""
	w.scan(fn, s, func(path, _, _ string) {
		if first == "" {
			first = name + " " + path
		}
	})
	w.memo[key] = first
	return first
}

// canWait: a Function that can wait long (a wait, an escalate, an ask, or
// a call to one that can).
func (w *windowCheck) canWait(name string) bool {
	if v, ok := w.waits[name]; ok {
		return v
	}
	w.waits[name] = false // cycles
	fn := w.functions[w.ns+"/"+name]
	if fn == nil {
		return false
	}
	for _, s := range fn.Spec.Steps {
		if s.Wait != nil || s.Escalate != nil || s.Call != "" && w.canWait(s.Call) {
			w.waits[name] = true
			return true
		}
		for _, bc := range s.Branch {
			if bc.Escalate != nil || bc.Call != "" && w.canWait(bc.Call) {
				w.waits[name] = true
				return true
			}
		}
	}
	return false
}

func (w *windowCheck) action(ref string) *plugins.ActionSpec {
	pl, name, _ := strings.Cut(ref, "/")
	p := w.plugins[pl]
	if p == nil {
		return nil
	}
	a, ok := p.Spec.Actions[name]
	if !ok {
		return nil
	}
	return &a
}

func (w *windowCheck) freeText(ref string) bool {
	a := w.action(ref)
	return a != nil && a.FreeText
}

// guarded: an onError that sends a template when the window is closed.
func (w *windowCheck) guarded(onError []agents.ErrorStrategy) bool {
	for _, s := range onError {
		if s.Fallback == nil || s.Fallback.Act == "" {
			continue
		}
		if a := w.action(s.Fallback.Act); a != nil && a.Effect == plugins.EffectExternalMessage && !a.FreeText {
			return true
		}
	}
	return false
}

func toUser(to string) bool { return to == "" || to == "user" }

// isReplyGuard recognizes the reply path of a wait with orReply: the
// user wrote. With events, "not timed out" may also be an event, which
// opens no window: only "reply != null" is the reply path then.
func isReplyGuard(when, wait string, events bool) bool {
	e := strings.Join(strings.Fields(when), "")
	out := "steps." + wait + ".output."
	if e == out+"reply!=null" {
		return true
	}
	return !events && (e == "!"+out+"timedOut" || e == "!("+out+"timedOut)" || e == out+"timedOut==false")
}
