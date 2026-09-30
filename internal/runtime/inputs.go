package runtime

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// resolveInput tries the input's sources in order (RFC-0003, "Resolving an
// input"). A fromAction source resolves the input only when the action's
// declared output type is the input's type; otherwise its output is an
// intermediate value for later sources. value, ask and fromFunction sources
// resolve it with any non-null value, which must conform to the type.
func (e *execution) resolveInput(f *frame, in *input, extracted map[string]any) (any, error) {
	sources := map[string]any{}
	path := "spec.inputs." + in.name
	var lastErr *callError
	failed := map[string]bool{}
sourceLoop:
	for i, s := range in.sources {
		sp := fmt.Sprintf("%s.resolve[%d]", path, i)
		site := f.prefix + "inputs." + in.name + "/" + s.id
		// A source that reads a failed source has nothing to work with:
		// skip it, and let the input go unresolved rather than fail on a
		// missing key.
		for _, id := range s.reads {
			if failed[id] {
				failed[s.id] = true
				continue sourceLoop
			}
		}
		act := e.activation(f, sources)
		if s.when != nil {
			ok, err := s.when.EvalBool(e.ctx, act)
			if err != nil {
				return nil, e.evalFailure(f, sp+".when", err)
			}
			if !ok {
				continue
			}
		}
		var (
			v        any
			resolves bool
		)
		switch {
		case s.fromAction != "":
			out, err := e.read(f, site, s.fromAction, s, sources)
			if err != nil {
				ce, ok := err.(*callError)
				if !ok {
					return nil, err
				}
				lastErr = ce
				failed[s.id] = true
				continue
			}
			v, resolves = out, s.outputType == in.typ
		case s.fromFunction != "":
			out, err := e.callFunction(f, s.fromFunction, s.with, site, sources)
			if err != nil {
				return nil, err
			}
			v, resolves = out, true
		case s.value != nil:
			out, err := s.value.Eval(e.ctx, act)
			if err != nil {
				return nil, e.evalFailure(f, sp+".value", err)
			}
			if v, err = journal.Normalize(out); err != nil {
				return nil, e.fail(f, fmt.Sprintf("%s.value: %v", sp, err))
			}
			resolves = true
		case s.extract != nil:
			v = fromModel(in.typ, extracted[in.name])
			if s.extract.future && in.typ == "date" && v != nil {
				v = e.nextOccurrence(v.(string))
			}
			resolves = v != nil
		case s.ask != nil && s.ask.choices == nil:
			out, ok, err := e.askFree(f, s, in, site, sources, extracted)
			if err != nil {
				return nil, err
			}
			v, resolves = out, ok
		case s.ask != nil:
			out, ok, err := e.ask(f, s, site, sources)
			if err != nil {
				return nil, err
			}
			v, resolves = out, ok
		}
		sources[s.id] = map[string]any{"output": v}
		if resolves && v != nil {
			if !conforms(in.typ, v) {
				return nil, e.fail(f, fmt.Sprintf("%s: produced %s, the input is %s", sp, describe(v), in.typ))
			}
			return v, nil
		}
	}
	return e.unresolved(f, in, sources, lastErr)
}

// read calls a read action once.
func (e *execution) read(f *frame, site, action string, s *source, sources map[string]any) (any, error) {
	m := map[string]any{}
	if s.with != nil {
		args, err := s.with.Resolve(e.ctx, e.activation(f, sources))
		if err != nil {
			return nil, e.evalFailure(f, site+".with", err)
		}
		m, _ = args.(map[string]any)
	}
	return e.once(site, func() (any, error) {
		return e.readChecked(site, action, m)
	})
}

// unresolved runs the input's onUnresolved strategies. Without any, the run
// fails.
func (e *execution) unresolved(f *frame, in *input, sources map[string]any, lastErr *callError) (any, error) {
	site := f.prefix + "inputs." + in.name + "/unresolved"
	for i, st := range in.onUnresolved {
		ss := fmt.Sprintf("%s[%d]", site, i)
		if st.when != nil {
			// The input's sources plus the last failure, as sources.error
			// (#30); "" when the sources found nothing rather than failed.
			withErr := map[string]any{}
			for k, v := range sources {
				withErr[k] = v
			}
			failure := map[string]any{"code": "", "message": "", "definite": true}
			if lastErr != nil {
				failure = map[string]any{"code": lastErr.Code, "message": lastErr.Message, "definite": lastErr.Definite}
			}
			withErr["error"] = failure
			if ok, err := st.when.Eval(e.ctx, e.activation(f, withErr)); err != nil || ok != true {
				continue
			}
		}
		switch st.kind {
		case stFallback:
			fb := st.fallback
			var out any
			var err error
			switch {
			case fb.useNull:
				return nil, nil
			case fb.function != "":
				out, err = e.callFunction(f, fb.function, fb.with, ss, sources)
			case fb.act != "":
				out, err = e.read(f, ss, fb.act, &source{with: fb.with}, sources)
				if ce, ok := err.(*callError); ok {
					lastErr = ce
					continue
				}
			}
			if err == nil && out != nil && !conforms(in.typ, out) {
				return nil, e.fail(f, fmt.Sprintf("%s: the fallback produced %s, the input is %s", ss, describe(out), in.typ))
			}
			return out, err
		case stEscalate:
			return e.escalateValue(f, ss, st.escalate, in, sources)
		case stRespond:
			return nil, e.respond(f, ss, st.respond, sources)
		case stFail:
			return nil, e.fail(f, st.message)
		}
	}
	reason := "input " + in.name + " could not be resolved"
	if lastErr != nil {
		reason += ": " + lastErr.Error()
	}
	return nil, e.fail(f, reason)
}

// escalateValue asks a person for an input's value; an answer that does
// not fit the input's type is asked again, never used.
func (e *execution) escalateValue(f *frame, site string, es *escalate, in *input, sources map[string]any) (any, error) {
	for round := 1; round <= maxHumanRounds; round++ {
		s := site
		if round > 1 {
			s = fmt.Sprintf("%s/%d", site, round)
		}
		v, err := e.escalate(f, s, es, sources)
		if err != nil {
			return nil, err
		}
		if val := fromModel(in.typ, v); val != nil {
			return val, nil
		}
	}
	return nil, e.fail(f, "input "+in.name+": the person's answers did not fit the type "+in.typ)
}

// respond sends a template and ends the run as handled-but-failed.
func (e *execution) respond(f *frame, site string, t *expr.Template, sources map[string]any) error {
	text, err := e.render(f, t, sources)
	if err != nil {
		return e.evalFailure(f, site, err)
	}
	if err := e.send(site, Message{To: "user", Text: text}); err != nil {
		return err
	}
	return &finished{status: journal.StatusFailed, reason: "responded: " + site}
}

// ask resolves a choice among a list (RFC-0003, "ask").
func (e *execution) ask(f *frame, s *source, site string, sources map[string]any) (any, bool, error) {
	actv := e.activation(f, sources)
	raw, err := s.ask.choices.Eval(e.ctx, actv)
	if err != nil {
		return nil, false, e.evalFailure(f, site+".choices", err)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, false, e.fail(f, fmt.Sprintf("%s: choices produced %s, want a list", site, describe(raw)))
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	if len(items) > decide.MaxChoiceOptions-1 {
		return nil, false, e.fail(f, fmt.Sprintf("%s: %d choices, at most %d", site, len(items), decide.MaxChoiceOptions-1))
	}
	items, err = normalizeList(items)
	if err != nil {
		return nil, false, e.fail(f, fmt.Sprintf("%s: %v", site, err))
	}
	text, err := e.render(f, s.ask.text, sources)
	if err != nil {
		return nil, false, e.evalFailure(f, site+".text", err)
	}
	opts := make([]decide.Option, 0, len(items)+1)
	labels := map[string]string{}
	for i, it := range items {
		key := "o" + strconv.Itoa(i+1)
		opts = append(opts, decide.Option{Key: key, Description: label(it)})
		labels[key] = label(it)
	}
	opts = append(opts, decide.Option{Key: noneKey, Description: "none of the options, or not clear"})
	pick := func(key string) (any, bool) {
		n, err := strconv.Atoi(strings.TrimPrefix(key, "o"))
		if err != nil || !strings.HasPrefix(key, "o") || n < 1 || n > len(items) {
			return nil, false
		}
		return items[n-1], true
	}

	if msg, _ := e.scopes["event"].(map[string]any)["message"].(string); msg != "" {
		key, err := e.decideChoice(site+"/decide0", text, opts, map[string]any{"message": msg}, s.ask.minConfidence)
		if err != nil {
			return nil, false, err
		}
		if v, ok := pick(key); ok {
			return v, true, nil
		}
	}
	switch s.ask.onLow {
	case "ask-user":
		for round := 1; round <= e.limits.MaxAskRounds; round++ {
			reply, _, err := e.waitFor(fmt.Sprintf("%s/ask%d", site, round), journal.StatusWaitingForUser, Message{To: "user", Text: text})
			if err != nil {
				return nil, false, err
			}
			key, err := e.decideChoice(fmt.Sprintf("%s/decide%d", site, round), text, opts,
				map[string]any{"question": text, "reply": reply}, s.ask.minConfidence)
			if err != nil {
				return nil, false, err
			}
			if v, ok := pick(key); ok {
				return v, true, nil
			}
		}
		return nil, false, nil
	default: // human, escalate-model (no escalation backend yet: RFC-0003)
		_, value, err := e.waitFor(site+"/human", journal.StatusWaitingForHuman, Message{
			To: "queue:" + e.limits.DefaultQueue, Task: "answer", Text: text, Options: labels,
		})
		if err != nil {
			return nil, false, err
		}
		key, _ := value.(string)
		v, ok := pick(key)
		return v, ok, nil
	}
}

const noneKey = "none"

// decideChoice asks Decide once per site and returns the chosen key, or ""
// when the answer is below the threshold, "none", or Decide failed. A
// failure degrades to asking a person; it is recorded so replay agrees.
func (e *execution) decideChoice(site, question string, opts []decide.Option, state map[string]any, min float64) (string, error) {
	v, err := e.once(site, func() (any, error) {
		req := decide.Request{
			State: e.rt.Masker.Mask(state),
			Questions: map[string]decide.Question{"pick": {
				Kind:         decide.KindChoice,
				Instructions: "Which option does the user's message choose? The question the user was asked: " + question,
				Options:      opts,
			}},
		}
		resp, err := e.rt.Decider.Decide(e.ctx, req)
		if err != nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return map[string]any{"choice": "", "p": 0.0, "error": "decide failed"}, nil
		}
		a := resp.Answers["pick"]
		probs := map[string]any{}
		for k, p := range a.Probabilities {
			probs[k] = p
		}
		// The question and the options are kept for inspection (why this
		// choice); replay reads only choice and p.
		options := map[string]any{}
		for _, o := range opts {
			options[o.Key] = o.Description
		}
		return map[string]any{"choice": a.Choice, "p": a.Probabilities[a.Choice], "probabilities": probs, "model": resp.Model,
			"question": question, "options": options}, nil
	})
	if err != nil {
		return "", err
	}
	m, _ := v.(map[string]any)
	choice, _ := m["choice"].(string)
	p, _ := m["p"].(float64)
	if choice == noneKey || p < min {
		return "", nil
	}
	return choice, nil
}

// decideYes asks a yes/no question once per site and returns P(yes); a
// failure counts as 0.5 (unclear), which asks again or escalates.
func (e *execution) decideYes(site, instructions string, state map[string]any) (float64, error) {
	v, err := e.once(site, func() (any, error) {
		resp, err := e.rt.Decider.Decide(e.ctx, decide.Request{
			State:     e.rt.Masker.Mask(state),
			Questions: map[string]decide.Question{"yes": {Kind: decide.KindNoul, Instructions: instructions}},
		})
		if err != nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return map[string]any{"p": 0.5, "error": "decide failed"}, nil
		}
		return map[string]any{"p": resp.Answers["yes"].Noul, "model": resp.Model, "question": instructions}, nil
	})
	if err != nil {
		return 0, err
	}
	p, _ := v.(map[string]any)["p"].(float64)
	return p, nil
}

// label is how an item is shown in a question.
func label(v any) string {
	if m, ok := v.(map[string]any); ok {
		if l, ok := m["label"].(string); ok && l != "" {
			return l
		}
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := journal.Encode(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func normalizeList(items []any) ([]any, error) {
	v, err := journal.Normalize(items)
	if err != nil {
		return nil, err
	}
	return v.([]any), nil
}

// conforms checks a value against a declared type. Plugin types (name.Type)
// are objects; their fields are the plugin's contract.
func conforms(typ string, v any) bool {
	if inner, ok := strings.CutPrefix(typ, "list<"); ok && strings.HasSuffix(inner, ">") {
		l, ok := v.([]any)
		if !ok {
			return false
		}
		for _, x := range l {
			if !conforms(strings.TrimSuffix(inner, ">"), x) {
				return false
			}
		}
		return true
	}
	switch typ {
	case "any":
		return true
	case "string":
		_, ok := v.(string)
		return ok
	case "int":
		switch v.(type) {
		case int64, uint64:
			return true
		}
		return false
	case "float":
		switch v.(type) {
		case float64, int64:
			return true
		}
		return false
	case "bool":
		_, ok := v.(bool)
		return ok
	case "timestamp":
		_, ok := v.(time.Time)
		return ok
	case "duration":
		_, ok := v.(time.Duration)
		return ok
	case "map":
		_, ok := v.(map[string]any)
		return ok
	case "date":
		s, ok := v.(string)
		if !ok {
			return false
		}
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	}
	_, ok := v.(map[string]any) // plugin type
	return ok
}

func describe(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	case string:
		return "a string"
	case int64, uint64:
		return "an int"
	case float64:
		return "a float"
	case bool:
		return "a bool"
	case time.Time:
		return "a timestamp"
	case time.Duration:
		return "a duration"
	}
	return fmt.Sprintf("%T", v)
}

// nextOccurrence moves a date more than a week before the run's time to
// the same day a year later (extract.future).
func (e *execution) nextOccurrence(day string) string {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	now, _ := e.scopes["event"].(map[string]any)["time"].(time.Time)
	if now.IsZero() || !d.Before(now.AddDate(0, 0, -7)) {
		return day
	}
	return d.AddDate(1, 0, 0).Format("2006-01-02")
}
