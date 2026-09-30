// Package learn proposes lessons from what the team answered (RFC-0008
// part B, learn-001). It reads the journal for escalate tasks of kind
// answer that a person answered, keeps the answers that hold for any
// customer (a fact about the business) and drops the ones about a
// person's own case, writes each as a general question and fact with no
// one's data, groups the same questions, flags groups whose answers
// contradict, and proposes a lesson only with enough answered cases. It
// never changes the agent: a person accepts lessons into a
// KnowledgeBase file (Accept).
package learn

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Store is what Propose reads.
type Store interface {
	Runs(ctx context.Context, f journal.RunFilter) ([]journal.Run, error)
	Records(ctx context.Context, runID string) ([]journal.Record, error)
}

// Case is one answered question.
type Case struct {
	Run      string    `json:"run"`
	Question string    `json:"question"`           // masked: what the customer asked
	Task     string    `json:"task"`               // masked: what the person was shown
	Answer   string    `json:"answer"`             // masked: what the person answered
	Nameless string    `json:"nameless,omitempty"` // the answer without greetings or the customer's name
	By       string    `json:"by,omitempty"`
	At       time.Time `json:"at"`
	// Set when the case is general.
	General  string `json:"generalQuestion,omitempty"`
	Fact     string `json:"fact,omitempty"`
	Kind     string `json:"kind,omitempty"` // fact, price, policy
	Rejected string `json:"rejected,omitempty"`
}

// Proposal is a lesson for a person to accept.
type Proposal struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Fact     string   `json:"fact"`
	Kind     string   `json:"kind"`
	Evidence []Case   `json:"evidence"`
	Members  []string `json:"members,omitempty"`
}

// Conflict is a group whose answers contradict: a person settles it.
type Conflict struct {
	Question string `json:"question"`
	Cases    []Case `json:"cases"`
}

// Report is Propose's result.
type Report struct {
	Namespace string     `json:"namespace"`
	Since     time.Time  `json:"since"`
	Answered  int        `json:"answered"`
	Proposals []Proposal `json:"proposals"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
	Rejected  []Case     `json:"rejected,omitempty"`
	Pending   []Case     `json:"belowThreshold,omitempty"` // general, but too few cases yet
	// Against the active lessons (--lessons, RFC-0008 part C).
	Contradicts []Match `json:"contradicts,omitempty"` // held: a person decides, and may supersede
	Reconfirms  []Match `json:"reconfirms,omitempty"`  // the same answer again: renew the lesson
}

// Match is a proposal about the same question as an active lesson.
type Match struct {
	Lesson   string   `json:"lesson"`
	Fact     string   `json:"lessonFact"`
	Proposal Proposal `json:"proposal"`
}

// Options configure Propose.
type Options struct {
	Namespace string
	Since     time.Time
	Min       int // answered cases a lesson needs; default 2
	Decider   decide.Decider
	Generator generate.Generator
	Mask      func(string) string
	// Lessons are the lessons already in the agent's file; proposals on
	// the same question as an active one are reported against it.
	Lessons []Lesson
}

// Propose reads the stores and proposes lessons.
func Propose(ctx context.Context, stores []Store, o Options) (*Report, error) {
	if o.Min <= 0 {
		o.Min = 2
	}
	if o.Mask == nil {
		o.Mask = func(s string) string { return s }
	}
	cases, err := answered(ctx, stores, o)
	if err != nil {
		return nil, err
	}
	rep := &Report{Namespace: o.Namespace, Since: o.Since, Answered: len(cases)}
	var general []Case
	for _, c := range cases {
		c, err := o.lesson(ctx, c)
		if err != nil {
			return nil, err
		}
		if c.Rejected != "" {
			rep.Rejected = append(rep.Rejected, c)
			continue
		}
		general = append(general, c)
	}
	groups, err := o.group(ctx, general)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if len(g) < o.Min {
			rep.Pending = append(rep.Pending, g...)
			continue
		}
		if len(g) > 1 {
			conflict, err := o.contradict(ctx, g)
			if err != nil {
				return nil, err
			}
			if conflict {
				rep.Conflicts = append(rep.Conflicts, Conflict{Question: g[0].General, Cases: g})
				continue
			}
		}
		p := proposal(g)
		if m, kind := o.against(ctx, p); kind != "" {
			if kind == "contradicts" {
				rep.Contradicts = append(rep.Contradicts, m)
			} else {
				rep.Reconfirms = append(rep.Reconfirms, m)
			}
			continue
		}
		rep.Proposals = append(rep.Proposals, p)
	}
	return rep, nil
}

// against finds an active lesson on the same question as p: "contradicts"
// when their answers disagree, else "reconfirms"; "" for none.
func (o Options) against(ctx context.Context, p Proposal) (Match, string) {
	for _, l := range o.Lessons {
		if l.Status() != "active" {
			continue
		}
		resp, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"a": l.Question + " " + l.Fact, "b": p.Question + " " + p.Fact}, Questions: map[string]decide.Question{"same": {
			Kind:         decide.KindNoul,
			Instructions: "Do A and B ask about the same thing of a business, so that one answer serves both?",
		}}})
		if err != nil || resp.Answers["same"].Noul < 0.5 {
			continue
		}
		m := Match{Lesson: l.ID, Fact: l.Fact, Proposal: p}
		c, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"question": l.Question, "answers": []any{l.Fact, p.Fact}}, Questions: map[string]decide.Question{"contradict": {
			Kind:         decide.KindNoul,
			Instructions: "ANSWERS were given by the business's people to the same QUESTION. Do any two of them contradict each other (one says yes, another no; different prices or rules)? Different wording or one giving more detail is not a contradiction.",
		}}})
		if err != nil || c.Answers["contradict"].Noul >= 0.5 {
			return m, "contradicts" // unsure counts as a contradiction: a person decides
		}
		return m, "reconfirms"
	}
	return Match{}, ""
}

// answered finds the answer tasks a person answered.
func answered(ctx context.Context, stores []Store, o Options) ([]Case, error) {
	var out []Case
	for _, st := range stores {
		f := journal.RunFilter{Namespace: o.Namespace, Limit: 500}
		for {
			runs, err := st.Runs(ctx, f)
			if err != nil {
				return nil, err
			}
			for _, run := range runs {
				if run.CreatedAt.Before(o.Since) {
					continue
				}
				recs, err := st.Records(ctx, run.ID)
				if err != nil {
					return nil, err
				}
				out = append(out, casesOf(run, recs, o.Mask)...)
			}
			if len(runs) < 500 {
				break
			}
			f.Before = runs[len(runs)-1].CreatedAt
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func casesOf(run journal.Run, recs []journal.Record, mask func(string) string) []Case {
	var scopes struct {
		Event map[string]any `json:"event"`
	}
	_ = json.Unmarshal(run.Scopes, &scopes)
	question, _ := scopes.Event["message"].(string)
	tasks := map[string]string{}
	var out []Case
	for _, rec := range recs {
		var v map[string]any
		if json.Unmarshal(rec.Value, &v) != nil {
			continue
		}
		switch rec.Kind {
		case journal.KindSuspend:
			to, _ := v["to"].(string)
			if task, _ := v["task"].(string); task == "answer" && strings.HasPrefix(to, "queue:") {
				text, _ := v["text"].(string)
				tasks[rec.Site] = text
			}
		case journal.KindResume:
			task, ok := tasks[rec.Site]
			if from, _ := v["from"].(string); !ok || from != "human" {
				continue
			}
			text, _ := v["text"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			by, _ := v["by"].(string)
			q := question
			if q == "" {
				q = task
			}
			out = append(out, Case{Run: run.ID, Question: mask(q), Task: mask(task), Answer: mask(text), By: by, At: rec.At})
		}
	}
	return out
}

const lessonRules = `A customer asked a business QUESTION, and a person from the business answered ANSWER. Write the lesson another customer could be told.
Reply with JSON only: {"question": "...", "fact": "...", "kind": "fact|price|policy"}.
- question: the question as any customer would ask it, general, in the language of QUESTION.
- fact: what ANSWER says that holds for any customer, in one or two sentences, in the language of ANSWER. Only what ANSWER states: add nothing, and leave out greetings and anything about this customer (their name, their appointment, their payment).
- kind: price when it states a price, policy when it states a rule of the business, else fact.`

const namelessRules = `Rewrite ANSWER without greetings and without the customer's name or any word addressing them (vocative). Change nothing else: keep every fact, number, date and condition exactly. Reply with the rewritten answer only.`

// personalQuestion asks whether the answer was decided for this customer
// (learn-002, wording tuned on learn-001's journal): a personal
// arrangement repeated for two customers is still personal.
const personalQuestion = "A customer asked QUESTION and a person from the business answered ANSWER (greetings and the customer's name removed). Does ANSWER give something decided or looked up for THIS customer: the final value of their own quote or treatment, a payment date or deadline for them, a discount or exception granted to them, a time opened for them, their exam results, their appointment? Examples of that: \"pode pagar até o dia 15\", \"fechamos 10% pra você\", \"sua consulta está confirmada\", \"o seu ficou R$ 1.400\". Not that: what the business offers or accepts and who does it (\"fazemos implante com o Dr. Bruno, antes precisa de avaliação\"), a list or starting price (\"a partir de R$ 9.800\"), how long things take, its rules, place and equipment."

// lesson decides whether c is general and writes its lesson.
func (o Options) lesson(ctx context.Context, c Case) (Case, error) {
	c.Nameless = c.Answer
	if g, err := o.Generator.Generate(ctx, generate.Request{Instructions: namelessRules, State: map[string]any{"answer": c.Answer}, MaxTokens: 300}); err == nil {
		if t := strings.TrimSpace(g.Text); t != "" && len(t) <= 2*len(c.Answer)+40 {
			c.Nameless = t
		}
	}
	state := map[string]any{"question": c.Question, "answer": c.Nameless}
	resp, err := o.Decider.Decide(ctx, decide.Request{State: state, Questions: map[string]decide.Question{"personal": {
		Kind:         decide.KindNoul,
		Instructions: personalQuestion,
	}}})
	if err != nil {
		c.Rejected = "error: " + err.Error() // not proposed; a later run retries it
		return c, nil
	}
	if resp.Answers["personal"].Noul >= 0.5 {
		c.Rejected = "personal: the answer is about this customer's own case"
		return c, nil
	}
	g, err := o.Generator.Generate(ctx, generate.Request{Instructions: lessonRules, State: map[string]any{"question": c.Question, "answer": c.Nameless}, MaxTokens: 300})
	if err != nil {
		c.Rejected = "error: " + err.Error()
		return c, nil
	}
	var l struct{ Question, Fact, Kind string }
	text := strings.TrimSpace(g.Text)
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		text = text[i : j+1]
	}
	if json.Unmarshal([]byte(text), &l) != nil || l.Question == "" || l.Fact == "" {
		c.Rejected = "the lesson could not be written"
		return c, nil
	}
	switch l.Kind {
	case "price", "policy":
	default:
		l.Kind = "fact"
	}
	// Kept on a rejection too, so a person can see what was written.
	c.General, c.Fact, c.Kind = l.Question, l.Fact, l.Kind
	adds, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"question": c.Question, "answer": c.Nameless, "fact": l.Fact}, Questions: map[string]decide.Question{"adds": {
		Kind:         decide.KindNoul,
		Instructions: "ANSWER answered QUESTION. Does FACT state something that ANSWER does not state (a new fact, number, condition, name or promise)? Naming what QUESTION asked about (\"fazemos sim\" to \"fazemos implante com carga imediata\"), rewording, a more general phrasing, and leaving details out are not that.",
	}}})
	if err != nil {
		c.Rejected = "error: " + err.Error()
		return c, nil
	}
	if adds.Answers["adds"].Noul >= 0.5 {
		c.Rejected = "unfaithful: the written lesson adds to the answer"
		return c, nil
	}
	return c, nil
}

// group puts cases asking the same thing together (Jev, pairwise against
// each group's first case).
func (o Options) group(ctx context.Context, cases []Case) ([][]Case, error) {
	var groups [][]Case
	for _, c := range cases {
		placed := false
		for i, g := range groups {
			resp, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"a": g[0].General + " " + g[0].Question, "b": c.General + " " + c.Question}, Questions: map[string]decide.Question{"same": {
				Kind:         decide.KindNoul,
				Instructions: "Do A and B ask about the same thing of a business, so that one answer serves both?",
			}}})
			if err != nil {
				continue // unsure: kept apart (a smaller group proposes nothing)
			}
			if resp.Answers["same"].Noul >= 0.5 {
				groups[i] = append(g, c)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []Case{c})
		}
	}
	return groups, nil
}

// contradict says whether a group's answers disagree.
func (o Options) contradict(ctx context.Context, g []Case) (bool, error) {
	var facts []any
	for _, c := range g {
		facts = append(facts, c.Nameless)
	}
	resp, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"question": g[0].General, "answers": facts}, Questions: map[string]decide.Question{"contradict": {
		Kind:         decide.KindNoul,
		Instructions: "ANSWERS were given by the business's people to the same QUESTION. Do any two of them contradict each other (one says yes, another no; different prices or rules)? Different wording or one giving more detail is not a contradiction.",
	}}})
	if err != nil {
		return true, nil // unsure: a person settles it
	}
	return resp.Answers["contradict"].Noul >= 0.5, nil
}

// proposal is a group's lesson: the most complete fact of the group.
func proposal(g []Case) Proposal {
	best := g[0]
	for _, c := range g {
		if len(c.Fact) > len(best.Fact) {
			best = c
		}
	}
	p := Proposal{ID: id(best.General), Question: best.General, Fact: best.Fact, Kind: best.Kind, Evidence: g}
	seen := map[string]bool{}
	for _, c := range g {
		if c.By != "" && !seen[c.By] {
			seen[c.By] = true
			p.Members = append(p.Members, c.By)
		}
	}
	return p
}

// id is a short stable slug of the question, accents folded.
func id(q string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(q)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
