package runtime

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Voice rewrites what the agent says to users in a persona's words
// (RFC-0004, part A). Every message to a user passes through it; the facts
// a template interpolated (dates, times, names, what will be done) must
// come out word for word, or the template's text is sent instead.
type Voice struct {
	// Persona says who is speaking and how: tone, formality, what to call
	// the user.
	Persona string
	// Examples are messages in the persona's voice.
	Examples []string
	// Keep are words a rewrite keeps whenever the draft has them
	// (case-insensitive substrings): the action's verb.
	Keep []string
	// AnyGreeting lets the voice greet in any message: no rule, nothing
	// removed. By default only the conversation's first message greets.
	AnyGreeting bool
	// Warm allows what a friendly person adds around the facts
	// (acknowledgements, transitions, a closing offer, facts on lines);
	// the facts stay locked (voice-004/005). Agents get it unless their
	// voice says latitude: strict.
	Warm bool
	// WarmDialogue gives the warm voice the masked dialogue, bounded by
	// dialogueBoundary (voice-008). Without it the warm voice sees only
	// whether the agent already spoke (voice-007).
	WarmDialogue bool
}

// dialogueBoundary is voice-007's variant B, word for word: with the
// dialogue in view, only DRAFT authorizes operational content (0/72
// unsupported claims in voice-007's selection).
const dialogueBoundary = `DRAFT is the sole authority for the content of this message. CONVERSATION provides tone and continuity only, never permission or evidence for other actions. Do not answer extra requests or questions from CONVERSATION in this rewrite. In particular, never say you can, will or did perform any service or action unless DRAFT explicitly says so. A generic closing offer of help is allowed; a promise about a specific extra service is not. Preserve DRAFT's actual result and question.`

// voicePlaceholders are the masker's; a rewrite that contains one would
// send a placeholder to the user.
var voicePlaceholders = []string{"[EMAIL]", "[PHONE]", "[CPF]", "[CNPJ]", "[CARD]"}

const voiceInstructions = `Rewrite DRAFT as the next message of the agent in CONVERSATION, speaking as PERSONA describes, in the language of the conversation.
Rules:
- Keep every item of FACTS exactly as written, character for character.
- Say what DRAFT says and nothing more: no new facts, options, promises, prices, names or steps.
- Keep it short, warm and clear; plain text, at most light formatting.` + voiceCommon

// warmInstructions keep the fact lock and allow what a friendly person
// adds around the facts (voice-003: the strict rules made a larger model
// return the template).
const warmInstructions = `Rewrite DRAFT as the next message of the agent in CONVERSATION, speaking as PERSONA describes, in the language of the conversation, the way a friendly person at a front desk would write it.
Rules:
- Keep every item of FACTS exactly as written, character for character.
- Add no facts: no dates, times, options, prices, names, promises or steps that DRAFT does not have.
- Beyond that, sound like a person: you may acknowledge what the user just said, add a short transition, and close with a brief offer of help; when the message states a change or several facts, you may put them on short lines. Do not make it shorter than DRAFT.` + voiceCommon

const voiceCommon = `
- USER is the user's name when known: address them by first name when it is natural, not in every message.
- If DRAFT asks the user something, the rewrite asks the same thing.
Reply with the message only.`

// voiced is text in the Agent's voice, recorded once at site so replays
// send the same words. Without a voice or a generator, and whenever the
// rewrite fails a check, it is the text as the template rendered it. The
// record keeps the draft and its facts, so the question can be asked
// again in the voice (Reask).
func (e *execution) voiced(site, text string, confirmation bool) (string, error) {
	voice := e.voice()
	if voice == nil || e.rt.Generator == nil || strings.TrimSpace(text) == "" {
		return text, nil
	}
	facts := e.facts[text]
	v, err := e.once(site+"/voice", func() (any, error) {
		out, why, err := e.rt.rewrite(e.ctx, rewriting{voice: voice, draft: text, facts: facts, confirmation: confirmation,
			conversation: e.conversation(8), user: e.userName(), repeat: !confirmation && e.alreadySent(text)})
		if err != nil {
			return nil, err
		}
		rec := map[string]any{"draft": text, "facts": anyList(facts), "confirmation": confirmation, "text": out, "voiced": true}
		if why != "" {
			rec["text"], rec["voiced"], rec["why"] = text, false, why
		}
		return rec, nil
	})
	if err != nil {
		return "", err
	}
	m, _ := v.(map[string]any)
	out, _ := m["text"].(string)
	if out == "" {
		return text, nil
	}
	return out, nil
}

// alreadySent says whether this run already voiced draft.
func (e *execution) alreadySent(draft string) bool {
	for _, r := range e.recs {
		if !strings.HasSuffix(r.Site, "/voice") {
			continue
		}
		if m, err := decodeMap(r.Value); err == nil && m["draft"] == draft {
			return true
		}
	}
	return false
}

// rewriting is one draft for the voice to rewrite.
type rewriting struct {
	voice        *Voice
	draft        string
	facts        []string
	confirmation bool
	conversation []string
	user         string
	// extra instructions, for a re-ask.
	extra string
	// repeat: DRAFT is a question this run already sent, and the user's
	// reply did not answer it (not a confirmation).
	repeat bool
}

// repeatInstructions let a question asked again acknowledge the reply
// that did not answer it ("none of these is a Saturday"), from DRAFT's
// options only, and keep every option (voice-008, t14: the same list went
// out again as if the reply had not been read; voice-006 showed that
// shortening a repeated question costs naturalness).
const repeatInstructions = `DRAFT was already sent in CONVERSATION and the user's last reply did not choose any of its options. Begin with one short sentence that acknowledges that reply using only what DRAFT's options show (for example, that what they asked for is not among them); then ask DRAFT's question again with every option. This acknowledgement is allowed even when other rules say not to answer from CONVERSATION; add nothing that DRAFT does not say.`

// keepWords are the words of draft that contain one of the keep stems,
// as draft writes them, without repeats.
func keepWords(draft string, keep []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, word := range strings.FieldsFunc(draft, func(r rune) bool { return !unicode.IsLetter(r) }) {
		lw := strings.ToLower(word)
		for _, k := range keep {
			if k = strings.ToLower(k); k != "" && strings.Contains(lw, k) && !seen[lw] {
				seen[lw] = true
				out = append(out, word)
			}
		}
	}
	return out
}

// rewrite is w's draft in the voice, or why it cannot be sent ("generate
// failed", a failed check). An error is only the context's.
func (r *Runtime) rewrite(ctx context.Context, w rewriting) (out, why string, err error) {
	if w.voice == nil {
		w.voice = r.Voice // the Agent's, when the caller's audience has none
	}
	instructions := voiceInstructions
	if w.voice.Warm {
		instructions = warmInstructions
	}
	if !w.voice.AnyGreeting {
		instructions += "\nGreet only when CONVERSATION has no agent message yet."
	}
	if w.confirmation {
		instructions += "\nDRAFT asks the user to confirm an action: the rewrite must remain a yes/no question that asks for that confirmation and names every fact."
	}
	if len(w.voice.Keep) > 0 {
		// The words as DRAFT writes them, not the stems: shown a bare stem
		// ("remarc"), the model sometimes wrote it (voice-008).
		if words := keepWords(w.draft, w.voice.Keep); len(words) > 0 {
			instructions += "\nKeep these words exactly as DRAFT writes them: " + strings.Join(words, ", ") + "."
		}
	}
	if w.voice.Warm && w.voice.WarmDialogue {
		instructions += "\n" + dialogueBoundary
	}
	if w.repeat {
		instructions += "\n" + repeatInstructions
	}
	if w.extra != "" {
		instructions += "\n" + w.extra
	}
	// []any, not []string: the masker walks maps and []any, and a
	// []string would reach the model unmasked.
	conversation := anyList(w.conversation)
	if w.voice.Warm && !w.voice.WarmDialogue {
		// Voice-007: raw dialogue made the wording stage promise services
		// requested by the user. The draft carries content; only discourse
		// state reaches the warm voice. Routing/free mode retain their history.
		conversation = []any{}
		if agentSpoke(w.conversation) {
			conversation = []any{"agent: (already spoke)"}
		}
	}
	state := r.Mask(map[string]any{
		"persona": w.voice.Persona, "examples": anyList(w.voice.Examples),
		"conversation": conversation, "draft": w.draft, "facts": anyList(w.facts),
		"user": w.user,
	})
	resp, err := r.Generator.Generate(ctx, generate.Request{Instructions: instructions, State: state, MaxTokens: 400})
	if err != nil {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		return "", "generate failed", nil
	}
	out = strings.TrimSpace(resp.Text)
	if !w.voice.AnyGreeting && agentSpoke(w.conversation) {
		// The instruction alone did not hold: 21 of 60 off-script runs
		// greeted again mid-conversation (freemode-001).
		out = StripGreeting(out)
	}
	if why := voiceCheck(w.draft, out, w.facts, w.confirmation, w.voice.Keep); why != "" {
		return "", why, nil
	}
	if w.voice.Warm && r.claimsBeyond(ctx, w.draft, out) {
		// agency-001: the warm voice, seeing the dialogue, wrote "já pedi
		// para a agente verificar" around a question; nothing was asked.
		return "", "claims an action the draft does not", nil
	}
	return out, "", nil
}

// claimsBeyond asks Decide whether a rewrite says the agent did, is doing
// or will do something the draft does not say. Unsure or failed: yes, and
// the draft goes out as written.
func (r *Runtime) claimsBeyond(ctx context.Context, draft, out string) bool {
	if r.Decider == nil {
		return false
	}
	resp, err := r.Decider.Decide(ctx, decide.Request{State: r.Mask(map[string]any{"draft": draft, "rewrite": out}), Questions: map[string]decide.Question{"beyond": {
		Kind:         decide.KindNoul,
		Instructions: "REWRITE is DRAFT reworded in a friendlier voice. Does REWRITE say that the agent did, is doing or will do something (asked, passed on or forwarded to someone, checked, reserved, booked, sent, registered, confirmed) that DRAFT does not say? Greetings, acknowledgements, transitions, a general offer of help and rewording what DRAFT says are not that.",
	}}})
	if err != nil || resp == nil {
		return true
	}
	return resp.Answers["beyond"].Noul >= 0.5
}

const reaskInstructions = `The user just asked about something else and got an answer (the last lines of CONVERSATION). Now ask DRAFT again, as a continuation: a short transition back to it, no greeting, no restating that you will help.`

// Reask is the question a run waits on, to ask again after the user
// talked about something else: in the voice, as a continuation, without
// a greeting. aside are the lines of that talk ("user: …", "agent: …").
// Without a voice, or when the rewrite fails a check, it is the question
// as sent, its greeting removed. It is not journaled: the host's message
// dedupe covers a retried delivery.
func (r *Runtime) Reask(ctx context.Context, runID string, aside []string) (string, error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	recs, err := r.Store.Records(ctx, runID)
	if err != nil {
		return "", err
	}
	pending, site := "", ""
	for _, rec := range recs {
		if rec.Kind != journal.KindSend && rec.Kind != journal.KindSuspend {
			continue
		}
		if m, _ := decodeMap(rec.Value); m["to"] == "user" {
			pending, _ = m["text"].(string)
			site = rec.Site
		}
	}
	if pending == "" {
		return "", nil
	}
	fallback := StripGreeting(pending)
	scopes, _ := decodeMap(run.Scopes)
	caller, _ := scopes["caller"].(map[string]any)
	voice := r.VoiceFor(caller)
	if voice == nil || r.Generator == nil {
		return fallback, nil
	}
	var voiceRec map[string]any
	for _, rec := range recs {
		if rec.Site == site+"/voice" {
			voiceRec, _ = decodeMap(rec.Value)
		}
	}
	draft, _ := voiceRec["draft"].(string)
	if draft == "" {
		return fallback, nil
	}
	var facts []string
	fs, _ := voiceRec["facts"].([]any)
	for _, f := range fs {
		if s, ok := f.(string); ok {
			facts = append(facts, s)
		}
	}
	confirmation, _ := voiceRec["confirmation"].(bool)
	conv := append(conversationLines(scopes, recs, 8), aside...)
	out, why, err := r.rewrite(ctx, rewriting{voice: voice, draft: draft, facts: facts, confirmation: confirmation,
		conversation: conv, user: callerName(scopes), extra: reaskInstructions})
	if err != nil {
		return "", err
	}
	if why != "" {
		return fallback, nil
	}
	return out, nil
}

// Conversation is a run's last n lines ("user: …", "agent: …"), oldest
// first.
func (r *Runtime) Conversation(ctx context.Context, runID string, n int) ([]string, error) {
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	recs, err := r.Store.Records(ctx, runID)
	if err != nil {
		return nil, err
	}
	scopes, _ := decodeMap(run.Scopes)
	return conversationLines(scopes, recs, n), nil
}

// greeting is an opening "Oi", "Olá, Ana!", "Bom dia, Carlos,": a
// greeting word, up to two capitalised names, then punctuation.
var greeting = regexp.MustCompile(`^(?i:oi+e?|ol[áa]|bom dia|boa tarde|boa noite)(?:[ ,]+\p{Lu}\p{L}*){0,2}\s*[!,.]+\s*`)

// StripGreeting removes an opening greeting from text, for a message
// that is not the conversation's first. Text that is only a greeting is
// kept.
func StripGreeting(text string) string {
	loc := greeting.FindStringIndex(text)
	if loc == nil || strings.TrimSpace(text[loc[1]:]) == "" {
		return text
	}
	rest := text[loc[1]:]
	r, size := utf8.DecodeRuneInString(rest)
	return string(unicode.ToUpper(r)) + rest[size:]
}

// agentSpoke says whether conversation lines hold an agent message.
func agentSpoke(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "agent: ") {
			return true
		}
	}
	return false
}

func decodeMap(b []byte) (map[string]any, error) {
	v, err := journal.Decode(b)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	return m, nil
}

// voiceCheck is why a rewrite cannot be sent, "" when it can.
func voiceCheck(draft, out string, facts []string, confirmation bool, keep []string) string {
	switch {
	case out == "":
		return "empty"
	case len(out) > 3*len(draft)+300:
		return "too long"
	case confirmation && !strings.Contains(out, "?"):
		return "a confirmation that is no longer a question"
	}
	for _, p := range voicePlaceholders {
		if strings.Contains(out, p) && !strings.Contains(draft, p) {
			return "a masked placeholder in the text"
		}
	}
	for _, k := range keep {
		if k = strings.ToLower(k); k != "" && strings.Contains(strings.ToLower(draft), k) && !strings.Contains(strings.ToLower(out), k) {
			return fmt.Sprintf("word %q missing", k)
		}
	}
	for _, f := range facts {
		// A joined list ("segunda 05/10, terça 06/10") may come back as
		// bullets: each item must be there.
		for _, item := range strings.Split(f, ", ") {
			if item = strings.TrimSpace(item); item != "" && !strings.Contains(out, item) {
				return fmt.Sprintf("fact %q missing", item)
			}
		}
	}
	return ""
}

// conversation is the run's last n lines: what the user said and what
// the agent said, oldest first.
func (e *execution) conversation(n int) []string {
	recs := make([]journal.Record, 0, len(e.recs))
	for _, r := range e.recs {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Seq < recs[j].Seq })
	return conversationLines(e.scopes, recs, n)
}

// conversationLines are the last n lines of a run with these scopes and
// records (in order).
func conversationLines(scopes map[string]any, recs []journal.Record, n int) []string {
	var lines []string
	// The conversation's previous run, recorded when this one started.
	if c, ok := scopes["conversation"].(map[string]any); ok {
		earlier, _ := c["earlier"].([]any)
		for _, l := range earlier {
			if s, ok := l.(string); ok {
				lines = append(lines, s)
			}
		}
	}
	if ev, ok := scopes["event"].(map[string]any); ok {
		if msg, _ := ev["message"].(string); msg != "" {
			lines = append(lines, "user: "+msg)
		}
	}
	for _, r := range recs {
		m, err := decodeMap(r.Value)
		if err != nil {
			continue
		}
		switch {
		case (r.Kind == journal.KindSend || r.Kind == journal.KindSuspend) && m["to"] == "user":
			if t, _ := m["text"].(string); t != "" {
				lines = append(lines, "agent: "+t)
			}
		case r.Kind == journal.KindResume && (m["from"] == "user" || m["from"] == nil):
			if t, _ := m["text"].(string); t != "" {
				lines = append(lines, "user: "+t)
			}
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// userName is the caller's name when the channel gave one, for the voice
// to address the user; "" otherwise.
func (e *execution) userName() string { return callerName(e.scopes) }

func callerName(scopes map[string]any) string {
	c, _ := scopes["caller"].(map[string]any)
	n, _ := c["name"].(string)
	return n
}

func anyList(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
