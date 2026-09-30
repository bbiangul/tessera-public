package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/mask"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Extraction from the conversation (RFC-0001: "extração da conversa").
// All extract sources of a Function are answered by one Generate call at the
// start of the run, so a request with seven details costs one model call,
// not seven. A free-text ask interprets each reply with one call.

// typeSchema is the JSON Schema a model must fill for an input type. Every
// value may be null: "the message does not say" must be expressible, or the
// model is pushed to invent.
func typeSchema(typ string) map[string]any {
	if inner, ok := strings.CutPrefix(typ, "list<"); ok && strings.HasSuffix(inner, ">") {
		items := typeSchema(strings.TrimSuffix(inner, ">"))
		if t, ok := items["type"]; ok {
			items["type"] = nonNull(t)
		} else {
			// list<map> and list<any>: items are any JSON value; an object
			// for map, so a model fills fields instead of a null type.
			if strings.TrimSuffix(inner, ">") == "map" {
				items["type"] = "object"
			}
		}
		return map[string]any{"type": []any{"array", "null"}, "items": items}
	}
	switch typ {
	case "string":
		return map[string]any{"type": []any{"string", "null"}}
	case "int":
		return map[string]any{"type": []any{"integer", "null"}}
	case "float":
		return map[string]any{"type": []any{"number", "null"}}
	case "bool":
		return map[string]any{"type": []any{"boolean", "null"}}
	case "date":
		return map[string]any{"type": []any{"string", "null"}, "pattern": `^\d{4}-\d{2}-\d{2}$`}
	case "timestamp":
		return map[string]any{"type": []any{"string", "null"}, "description": "RFC 3339 with offset"}
	case "duration":
		return map[string]any{"type": []any{"string", "null"}, "description": "Go duration, e.g. 1h30m"}
	}
	return map[string]any{} // map, any, plugin types: any JSON value
}

func nonNull(t any) any {
	if l, ok := t.([]any); ok && len(l) == 2 && l[1] == "null" {
		return l[0]
	}
	return t
}

// fromModel converts an extracted JSON value to the input's type. A value
// the model got wrong (an impossible date, a malformed timestamp) is null:
// the next source, usually a question, takes over.
func fromModel(typ string, v any) any {
	if v == nil {
		return nil
	}
	switch typ {
	case "date":
		s, _ := v.(string)
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil
		}
		return s
	case "timestamp":
		s, _ := v.(string)
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil
		}
		return t
	case "duration":
		s, _ := v.(string)
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil
		}
		return d
	}
	if !conforms(typ, v) {
		return nil
	}
	return v
}

// prefetchExtracts answers every extract source of the frame's inputs with
// one Generate call, recorded once. Inputs given by the caller are skipped.
func (e *execution) prefetchExtracts(f *frame) (map[string]any, error) {
	var want []*input
	message, _ := e.scopes["event"].(map[string]any)["message"].(string)
	byCode := map[string]any{}
	for _, in := range f.fn.inputs {
		if v, preset := f.inputs[in.name]; preset && v != nil {
			continue
		}
		if in.format != "" {
			// Personal data is read by pattern, from the raw message; with
			// conversation, then from the user's earlier messages, the
			// latest first.
			if v := mask.Find(in.format, message); v != "" {
				byCode[in.name] = v
				continue
			}
			for _, s := range in.sources {
				if s.extract == nil || !s.extract.conversation {
					continue
				}
				earlier := e.earlierUserMessages()
				for i := len(earlier) - 1; i >= 0; i-- {
					if v := mask.Find(in.format, earlier[i]); v != "" {
						byCode[in.name] = v
						break
					}
				}
				break
			}
			continue
		}
		for _, s := range in.sources {
			if s.extract != nil {
				want = append(want, in)
				break
			}
		}
	}
	if len(want) == 0 {
		if len(byCode) > 0 {
			return byCode, nil
		}
		return nil, nil
	}
	if message == "" || e.rt.Generator == nil {
		return byCode, nil
	}
	props := map[string]any{}
	required := []any{}
	var lines []string
	useEarlier := false
	for _, in := range want {
		props[in.name] = typeSchema(in.typ)
		required = append(required, in.name)
		line := "- " + in.name + " (" + in.typ + ")"
		if in.description != "" {
			line += ": " + in.description
		}
		fromEarlier := false
		for _, s := range in.sources {
			if s.extract != nil && s.extract.instructions != "" {
				line += ". " + s.extract.instructions
			}
			fromEarlier = fromEarlier || (s.extract != nil && s.extract.conversation)
		}
		earlier := e.earlierUserMessages()
		if fromEarlier && len(earlier) > 0 {
			line += ". May come from EARLIER when MESSAGE does not say it"
			useEarlier = true
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
	instructions := "Extract these values from the user's MESSAGE. Use null for anything the message does not say; " +
		"never guess. STATE.now is the current time, for relative dates.\n" + strings.Join(lines, "\n")
	state := map[string]any{"message": message, "now": e.nowText()}
	if useEarlier {
		// agency-002 (#70): what the user said before this request, for
		// the values that may come from it. The message always wins.
		instructions += "\nEARLIER are the user's earlier messages in this conversation, oldest first. Only a value marked " +
			"'May come from EARLIER' may be taken from them, when MESSAGE does not say it and EARLIER clearly says it still " +
			"applies; when MESSAGE says it, MESSAGE wins."
		state["earlier"] = anyList(e.earlierUserMessages())
	}
	v, err := e.once(f.prefix+"inputs/extract", func() (any, error) {
		resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{
			Instructions: instructions,
			State:        e.rt.Masker.Mask(state),
			Schema:       schema,
		})
		if err != nil {
			if e.ctx.Err() != nil {
				return nil, e.ctx.Err()
			}
			return map[string]any{"failed": true, "reason": generateFailure(err)}, nil
		}
		return map[string]any{"values": resp.Value, "model": resp.Model}, nil
	})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	values, _ := m["values"].(map[string]any)
	if values == nil {
		values = map[string]any{}
	}
	for k, x := range byCode {
		values[k] = x
	}
	return values, nil
}

func (e *execution) nowText() string {
	t, _ := e.scopes["event"].(map[string]any)["time"].(time.Time)
	return t.Format(time.RFC3339) + " (" + t.Weekday().String() + ")"
}

// askFree asks a free-text question and interprets each reply as the
// input's type, up to MaxAskRounds times. The same call reads the later
// inputs the reply may also give ("dia 15, volto dia 22" answers the
// return too): each one found fills extracted, where that input's extract
// source reads it, unless the first message already gave it.
func (e *execution) askFree(f *frame, s *source, in *input, site string, sources map[string]any, extracted map[string]any) (any, bool, error) {
	text, err := e.render(f, s.ask.text, sources)
	if err != nil {
		return nil, false, e.evalFailure(f, site+".text", err)
	}
	if e.rt.Generator == nil {
		return nil, false, e.fail(f, site+": a free-text question needs a Generate backend")
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"value"},
		"properties": map[string]any{"value": typeSchema(in.typ)}}
	what := describeInput(in)
	later := e.laterExtracts(f, in, extracted)
	also := ""
	if modelLater(later) > 0 {
		props, required := map[string]any{}, []any{}
		var lines []string
		for _, l := range later {
			if l.format != "" {
				continue // read by pattern, never shown to a model
			}
			props[l.name] = typeSchema(l.typ)
			required = append(required, l.name)
			lines = append(lines, "- "+describeInput(l))
		}
		sort.Strings(lines)
		schema["required"] = []any{"value", "also"}
		schema["properties"].(map[string]any)["also"] = map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
		also = " In also, extract each of these only when the reply says it, null otherwise:\n" + strings.Join(lines, "\n")
	}
	// What the user already said, so a relative answer ("fico 2 noites"
	// after the check-in) can be resolved.
	known := map[string]any{}
	for k, v := range f.inputs {
		if v != nil && k != in.name {
			known[k] = v
		}
	}
	for round := 1; round <= e.limits.MaxAskRounds; round++ {
		reply, _, err := e.waitFor(fmt.Sprintf("%s/ask%d", site, round), journal.StatusWaitingForUser, Message{To: "user", Text: text})
		if err != nil {
			return nil, false, err
		}
		if in.format != "" {
			// Personal data: by pattern from the raw reply, no model.
			if v := mask.Find(in.format, reply); v != "" {
				return v, true, nil
			}
			continue
		}
		for _, l := range later {
			if l.format != "" && extracted[l.name] == nil {
				if v := mask.Find(l.format, reply); v != "" {
					extracted[l.name] = v
				}
			}
		}
		v, err := e.once(fmt.Sprintf("%s/extract%d", site, round), func() (any, error) {
			resp, err := e.rt.Generator.Generate(e.ctx, generate.Request{
				Instructions: "The user was asked the QUESTION and answered with the REPLY. Extract " + what +
					" from the reply. Use null when the reply does not answer it; never guess. STATE.now is the current time; " +
					"STATE.known has what the user already told, for answers relative to it." + also,
				State:  e.rt.Masker.Mask(map[string]any{"question": text, "reply": reply, "now": e.nowText(), "known": known}),
				Schema: schema,
			})
			if err != nil {
				if e.ctx.Err() != nil {
					return nil, e.ctx.Err()
				}
				return map[string]any{"value": nil, "failed": generateFailure(err)}, nil
			}
			return resp.Value, nil
		})
		if err != nil {
			return nil, false, err
		}
		m, _ := v.(map[string]any)
		if a, ok := m["also"].(map[string]any); ok {
			for _, l := range later {
				if l.format != "" {
					continue
				}
				if val := fromModel(l.typ, a[l.name]); val != nil && fromModel(l.typ, extracted[l.name]) == nil {
					extracted[l.name] = a[l.name]
				}
			}
		}
		if val := fromModel(in.typ, m["value"]); val != nil {
			for _, src := range in.sources {
				if src.extract != nil && src.extract.future && in.typ == "date" {
					val = e.nextOccurrence(val.(string))
				}
			}
			return val, true, nil
		}
	}
	if s.ask.onLow == "human" || s.ask.onLow == "escalate-model" {
		_, hv, err := e.waitFor(site+"/human", journal.StatusWaitingForHuman, Message{
			To: "queue:" + e.limits.DefaultQueue, Task: "answer", Text: text + " (" + what + ")",
		})
		if err != nil {
			return nil, false, err
		}
		val := fromModel(in.typ, hv)
		return val, val != nil, nil
	}
	return nil, false, nil
}

// describeInput is an input as a model reads it: name, type, description
// and its extract instructions.
func describeInput(in *input) string {
	what := in.name + " (" + in.typ + ")"
	if in.description != "" {
		what += ": " + in.description
	}
	for _, src := range in.sources {
		if src.extract != nil && src.extract.instructions != "" {
			what += ". " + src.extract.instructions
		}
	}
	return what
}

// laterExtracts are the inputs after in, not given and not yet resolved,
// that have an extract source the first message left empty: what a reply
// to in's question may also answer.
func (e *execution) laterExtracts(f *frame, in *input, extracted map[string]any) []*input {
	if extracted == nil {
		return nil
	}
	var out []*input
	after := false
	for _, l := range f.fn.inputs {
		if l == in {
			after = true
			continue
		}
		if !after {
			continue
		}
		if v, ok := f.inputs[l.name]; ok && v != nil {
			continue
		}
		if fromModel(l.typ, extracted[l.name]) != nil {
			continue
		}
		for _, src := range l.sources {
			if src.extract != nil {
				out = append(out, l)
				break
			}
		}
	}
	return out
}

// modelLater counts the later inputs a model may read (not personal data).
func modelLater(later []*input) int {
	n := 0
	for _, l := range later {
		if l.format == "" {
			n++
		}
	}
	return n
}

// earlierUserMessages are what the user wrote in the conversation before
// this run, as kept when it started (conversation.earlier), oldest first.
func (e *execution) earlierUserMessages() []string {
	c, _ := e.scopes["conversation"].(map[string]any)
	earlier, _ := c["earlier"].([]any)
	var out []string
	for _, l := range earlier {
		if s, ok := l.(string); ok {
			if t, ok := strings.CutPrefix(s, "user: "); ok {
				out = append(out, t)
			}
		}
	}
	return out
}
