package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/learn"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Lessons approved in the team's channel (learn-003). A KnowledgeBase with
// lessons learns: every so often Tick proposes lessons from the journal,
// each one becomes a run of a generated Function, learn-<base>, that asks
// the queue to approve the exact lesson and then calls lessons/accept,
// which appends it to the base's file and rebuilds the index. Every
// approval surface (channels, hash binding, roles, SLA) applies.

type lessonBase struct {
	name, file, queue string
	every             time.Duration
	min               int
	next              time.Time
}

// learnFunction is the name of a base's approval Function.
func learnFunction(base string) string { return "learn-" + base }

// learnDocs are the generated approval Functions of the namespace's
// learning bases.
func learnDocs(docs []admission.Doc, ns string) []admission.Doc {
	defaultQueue := ""
	for _, d := range docs {
		if d.Agent != nil && d.Agent.Metadata.Namespace == ns && d.Agent.Spec.HumanInTheLoop != nil {
			defaultQueue = d.Agent.Spec.HumanInTheLoop.Queue
		}
	}
	var out []admission.Doc
	for _, d := range docs {
		kb := d.Knowledge
		if kb == nil || kb.Metadata.Namespace != ns || kb.Spec.Lessons == nil {
			continue
		}
		queue := kb.Spec.Lessons.Queue
		if queue == "" {
			queue = defaultQueue
		}
		out = append(out, admission.Doc{File: d.File, Kind: "Function", Function: learnFunctionDoc(ns, kb.Metadata.Name, queue)},
			admission.Doc{File: d.File, Kind: "Function", Function: renewFunctionDoc(ns, kb.Metadata.Name, queue)})
	}
	return out
}

func learnFunctionDoc(ns, base, queue string) *agents.Function {
	in := func(name, typ, desc string) agents.Input {
		return agents.Input{Type: typ, Description: desc, Resolve: []agents.Source{{ID: "p", Value: agents.Expr("event.payload." + name)}}}
	}
	order := []string{"id", "question", "fact", "kind", "cases", "members", "runs", "supersede", "replaces"}
	return &agents.Function{
		APIVersion: "agents.tessera.dev/v1alpha1", Kind: "Function",
		Metadata: agents.ObjectMeta{Name: learnFunction(base), Namespace: ns},
		Spec: agents.FunctionSpec{
			Description: "Asks the team to approve a lesson learned from its answers, for the knowledge base " + base + " (learn-003).",
			Authorize:   "has(caller.learn)",
			Inputs: map[string]agents.Input{
				"id":        in("id", "string", "the lesson's id"),
				"question":  in("question", "string", "the question the lesson answers"),
				"fact":      in("fact", "string", "the lesson"),
				"kind":      in("kind", "string", "fact, price or policy"),
				"cases":     in("cases", "int", "how many answers it rests on"),
				"members":   in("members", "string", "who answered"),
				"runs":      in("runs", "string", "the runs it rests on"),
				"supersede": in("supersede", "string", "the lesson it replaces, or empty"),
				"replaces":  in("replaces", "string", "the replaced lesson's text, or empty"),
			},
			Steps: []agents.Step{{ID: "ok", Escalate: &agents.Escalate{Queue: queue, Task: "approve",
				Message: "Nova lição para as respostas do agente:\nPergunta: {{ inputs.question }}\nResposta: {{ inputs.fact }}\n{{ inputs.replaces != '' ? 'Substitui: ' + inputs.replaces + '\\n' : '' }}Baseada em {{ inputs.cases }} respostas da equipe. Aprovar?"}}},
			Action: &agents.Action{Act: admission.LessonsPlugin + "/accept", IdempotencyKey: "conversation.id", With: map[string]any{
				"base": base, "id": "${ inputs.id }", "question": "${ inputs.question }", "fact": "${ inputs.fact }", "kind": "${ inputs.kind }",
				"cases": "${ inputs.cases }", "members": "${ inputs.members }", "runs": "${ inputs.runs }", "supersede": "${ inputs.supersede }",
			}},
			Output: &agents.Output{Value: "'accepted'"},
		},
		InputOrder: order,
	}
}

// renewFunction is the name of a base's renewal Function.
func renewFunction(base string) string { return "renew-" + base }

func renewFunctionDoc(ns, base, queue string) *agents.Function {
	in := func(name, typ, desc string) agents.Input {
		return agents.Input{Type: typ, Description: desc, Resolve: []agents.Source{{ID: "p", Value: agents.Expr("event.payload." + name)}}}
	}
	return &agents.Function{
		APIVersion: "agents.tessera.dev/v1alpha1", Kind: "Function",
		Metadata: agents.ObjectMeta{Name: renewFunction(base), Namespace: ns},
		Spec: agents.FunctionSpec{
			Description: "Asks the team whether a lesson of the knowledge base " + base + " still holds, and renews it (renew-001).",
			Authorize:   "has(caller.learn)",
			Inputs: map[string]agents.Input{
				"id":          in("id", "string", "the lesson's id"),
				"question":    in("question", "string", "the question the lesson answers"),
				"fact":        in("fact", "string", "the lesson"),
				"review":      in("review", "string", "its review date"),
				"reconfirmed": in("reconfirmed", "int", "answers since, giving the same"),
			},
			Steps: []agents.Step{{ID: "ok", Escalate: &agents.Escalate{Queue: queue, Task: "approve",
				Message: "Esta lição continua valendo?\nPergunta: {{ inputs.question }}\nResposta: {{ inputs.fact }}\nRevisão: {{ inputs.review }}.{{ inputs.reconfirmed > 0 ? '\\nA equipe respondeu o mesmo ' + string(inputs.reconfirmed) + ' vez(es).' : '' }}\nRenovar?"}}},
			Action: &agents.Action{Act: admission.LessonsPlugin + "/renew", IdempotencyKey: "conversation.id", With: map[string]any{
				"base": base, "id": "${ inputs.id }",
			}},
			Output: &agents.Output{Value: "'renewed'"},
		},
		InputOrder: []string{"id", "question", "fact", "review", "reconfirmed"},
	}
}

// lessonsState is what the host keeps for learning bases.
type lessonsState struct {
	mu     sync.Mutex // one file write at a time
	bases  []*lessonBase
	tmpDir string // without a data dir; removed on Close
}

// lessonFiles prepares each learning base's file, and returns them by
// base, to be indexed with the base's other sources.
func (h *Host) prepareLessons(docs []admission.Doc) (map[string]string, error) {
	st := &lessonsState{}
	files := map[string]string{}
	for _, d := range docs {
		kb := d.Knowledge
		if kb == nil || kb.Metadata.Namespace != h.opts.Namespace || kb.Spec.Lessons == nil {
			continue
		}
		dir := h.opts.DataDir
		if dir == "" {
			if st.tmpDir == "" {
				t, err := os.MkdirTemp("", "tessera-lessons-")
				if err != nil {
					return nil, err
				}
				st.tmpDir = t
			}
			dir = st.tmpDir
		}
		file := filepath.Join(dir, "lessons", h.opts.Namespace, kb.Metadata.Name+".md")
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return nil, err
		}
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(file, []byte("# Lições aprendidas\n\nAprovadas pela equipe no canal (learn-003).\n"), 0o600); err != nil {
				return nil, err
			}
		}
		l := kb.Spec.Lessons
		b := &lessonBase{name: kb.Metadata.Name, file: file, queue: l.Queue, every: 168 * time.Hour, min: l.Min}
		if l.Every != "" {
			d, err := time.ParseDuration(l.Every)
			if err != nil {
				return nil, err
			}
			b.every = d
		}
		st.bases = append(st.bases, b)
		files[kb.Metadata.Name] = file
	}
	if len(st.bases) > 0 {
		h.lessons = st
	}
	return files, nil
}

// registerLessons serves lessons/accept: the lesson is appended to its
// base's file (superseding the one it replaces), marked with who approved
// it, and the index is rebuilt. The idempotency key is the approval run's
// conversation, so a retry writes nothing twice.
func (h *Host) registerLessons(reg *act.Registry) {
	reg.Register(admission.LessonsPlugin+"/accept", func(ctx context.Context, c act.Call) (any, error) {
		if h.lessons == nil {
			return nil, &act.DefiniteError{Code: "NO_LESSONS", Message: "no knowledge base learns in this namespace"}
		}
		base, _ := c.Args["base"].(string)
		var lb *lessonBase
		for _, b := range h.lessons.bases {
			if b.name == base {
				lb = b
			}
		}
		if lb == nil {
			return nil, &act.DefiniteError{Code: "UNKNOWN_BASE", Message: "no learning base " + base}
		}
		task := strings.ReplaceAll(c.IdempotencyKey, " ", "_")
		h.lessons.mu.Lock()
		defer h.lessons.mu.Unlock()
		raw, err := os.ReadFile(lb.file)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(raw), " task="+task+" ") {
			return map[string]any{"written": false}, nil // this approval was applied
		}
		str := func(k string) string { s, _ := c.Args[k].(string); return s }
		cases, _ := c.Args["cases"].(int64)
		var evidence []learn.Case
		for _, r := range strings.Split(str("runs"), ",") {
			if r != "" {
				evidence = append(evidence, learn.Case{Run: r})
			}
		}
		for len(evidence) < int(cases) {
			evidence = append(evidence, learn.Case{})
		}
		var members []string
		if m := str("members"); m != "" {
			members = strings.Split(m, ",")
		}
		p := learn.Proposal{ID: str("id"), Question: str("question"), Fact: str("fact"), Kind: str("kind"), Evidence: evidence, Members: members}
		var supersede []string
		if s := str("supersede"); s != "" {
			supersede = []string{s}
		}
		now := h.Runtime.Clock()
		wrote, err := learn.AcceptSuperseding(lb.file, []learn.Proposal{p}, []string{p.ID}, supersede, now)
		if err != nil {
			return nil, &act.DefiniteError{Code: "NOT_WRITTEN", Message: err.Error()}
		}
		if len(wrote) > 0 {
			meta := map[string]string{"task": task}
			if by := h.approver(ctx, c.IdempotencyKey); by != "" {
				meta["approvedBy"] = by
			}
			if _, err := learn.SetMeta(lb.file, wrote, meta); err != nil {
				return nil, err
			}
			if err := h.rebuild(); err != nil {
				return nil, err
			}
		}
		return map[string]any{"written": len(wrote) > 0, "ids": anyStrings(wrote)}, nil
	})
}

// registerRenew serves lessons/renew (renew-001).
func (h *Host) registerRenew(reg *act.Registry) {
	reg.Register(admission.LessonsPlugin+"/renew", func(ctx context.Context, c act.Call) (any, error) {
		if h.lessons == nil {
			return nil, &act.DefiniteError{Code: "NO_LESSONS", Message: "no knowledge base learns in this namespace"}
		}
		base, _ := c.Args["base"].(string)
		id, _ := c.Args["id"].(string)
		var lb *lessonBase
		for _, b := range h.lessons.bases {
			if b.name == base {
				lb = b
			}
		}
		if lb == nil {
			return nil, &act.DefiniteError{Code: "UNKNOWN_BASE", Message: "no learning base " + base}
		}
		h.lessons.mu.Lock()
		defer h.lessons.mu.Unlock()
		if _, err := learn.Renew(lb.file, []string{id}, h.Runtime.Clock()); err != nil {
			return nil, &act.DefiniteError{Code: "NOT_RENEWED", Message: err.Error()}
		}
		if by := h.approver(ctx, c.IdempotencyKey); by != "" {
			if _, err := learn.SetMeta(lb.file, []string{id}, map[string]string{"renewedBy": by}); err != nil {
				return nil, err
			}
		}
		return map[string]any{"renewed": id}, h.rebuild()
	})
}

// approver is the member who answered the approval of the run in this
// conversation.
func (h *Host) approver(ctx context.Context, conversation string) string {
	run, ok, err := h.opts.Store.LastRun(ctx, conversation)
	if err != nil || !ok {
		return ""
	}
	recs, err := h.opts.Store.Records(ctx, run.ID)
	if err != nil {
		return ""
	}
	by := ""
	for _, r := range recs {
		var v map[string]any
		if r.Kind == journal.KindResume && json.Unmarshal(r.Value, &v) == nil && v["from"] == "human" {
			by, _ = v["by"].(string)
		}
	}
	return by
}

// tickLessons proposes the lessons of each learning base that is due:
// one approval run per proposal and per contradiction of an active
// lesson, unless every run it rests on was already in an earlier approval
// (approved, denied or pending).
func (h *Host) tickLessons(ctx context.Context, now time.Time) []error {
	if h.lessons == nil || h.Runtime.Generator == nil {
		return nil
	}
	var errs []error
	for _, b := range h.lessons.bases {
		if now.Before(b.next) {
			continue
		}
		b.next = now.Add(b.every)
		if err := h.proposeLessons(ctx, b); err != nil {
			errs = append(errs, fmt.Errorf("lessons %s: %w", b.name, err))
		}
	}
	return errs
}

func (h *Host) proposeLessons(ctx context.Context, b *lessonBase) error {
	lessons, err := learn.ReadLessons(b.file)
	if err != nil {
		return err
	}
	rep, err := learn.Propose(ctx, []learn.Store{h.opts.Store}, learn.Options{
		Namespace: h.opts.Namespace, Min: b.min, Decider: h.opts.Decider, Generator: h.Runtime.Generator, Lessons: lessons,
		Mask: func(s string) string { v, _ := h.Runtime.Mask(s).(string); return v },
	})
	if err != nil {
		return err
	}
	used, err := h.lessonEvidence(ctx, b.name)
	if err != nil {
		return err
	}
	type item struct {
		p                   learn.Proposal
		supersede, replaces string
	}
	var items []item
	for _, p := range rep.Proposals {
		items = append(items, item{p: p})
	}
	for _, m := range rep.Contradicts {
		items = append(items, item{p: m.Proposal, supersede: m.Lesson, replaces: m.Fact})
	}
	if err := h.renewals(ctx, b, lessons, rep.Reconfirms); err != nil {
		return err
	}
	for _, it := range items {
		var runs []string
		fresh := false
		for _, c := range it.p.Evidence {
			runs = append(runs, c.Run)
			fresh = fresh || !used[c.Run]
		}
		if !fresh {
			continue // already asked: approved, denied or waiting
		}
		conv := fmt.Sprintf("learn:%s:%s", b.name, h.Runtime.NewID())
		if _, err := h.Runtime.Start(ctx, runtime.Start{
			Namespace: h.opts.Namespace, Function: learnFunction(b.name), Conversation: conv,
			Caller: map[string]any{"learn": b.name},
			Payload: map[string]any{"id": it.p.ID, "question": it.p.Question, "fact": it.p.Fact, "kind": it.p.Kind,
				"cases": int64(len(it.p.Evidence)), "members": strings.Join(it.p.Members, ","), "runs": strings.Join(runs, ","),
				"supersede": it.supersede, "replaces": it.replaces},
		}); err != nil {
			return err
		}
		for _, r := range runs {
			used[r] = true
		}
	}
	return nil
}

// renewals asks, once per review date, whether each due lesson still
// holds (renew-001): due is a review date within 14 days or past.
func (h *Host) renewals(ctx context.Context, b *lessonBase, lessons []learn.Lesson, reconfirms []learn.Match) error {
	now := h.Runtime.Clock()
	again := map[string]int{}
	for _, m := range reconfirms {
		again[m.Lesson] += len(m.Proposal.Evidence)
	}
	for _, l := range lessons {
		if l.Status() != "active" {
			continue
		}
		review, err := time.Parse("2006-01-02", l.Meta["review"])
		if err != nil || review.After(now.Add(14*24*time.Hour)) {
			continue
		}
		id := fmt.Sprintf("renew/%s/%s/%s/%s", h.opts.Namespace, b.name, l.ID, l.Meta["review"])
		_, err = h.Runtime.Start(ctx, runtime.Start{
			Namespace: h.opts.Namespace, Function: renewFunction(b.name), Conversation: "learn:" + id, RunID: id,
			Caller:  map[string]any{"learn": b.name},
			Payload: map[string]any{"id": l.ID, "question": l.Question, "fact": l.Fact, "review": l.Meta["review"], "reconfirmed": int64(again[l.ID])},
		})
		if err != nil && !errors.Is(err, journal.ErrExists) {
			return err
		}
	}
	return nil
}

// lessonEvidence is every run an earlier approval of this base rested on.
func (h *Host) lessonEvidence(ctx context.Context, base string) (map[string]bool, error) {
	used := map[string]bool{}
	f := journal.RunFilter{Namespace: h.opts.Namespace, Function: learnFunction(base), Limit: 500}
	for {
		runs, err := h.opts.Store.Runs(ctx, f)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			var s struct {
				Event struct {
					Payload map[string]any `json:"payload"`
				} `json:"event"`
			}
			_ = json.Unmarshal(r.Scopes, &s)
			if rs, _ := s.Event.Payload["runs"].(string); rs != "" {
				for _, id := range strings.Split(rs, ",") {
					used[id] = true
				}
			}
		}
		if len(runs) < 500 {
			break
		}
		f.Before = runs[len(runs)-1].CreatedAt
	}
	return used, nil
}
