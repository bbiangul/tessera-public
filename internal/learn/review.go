package learn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/knowledge"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Lesson is one lesson of a lessons file (RFC-0008 part C).
type Lesson struct {
	ID       string            `json:"id"`
	Question string            `json:"question"`
	Fact     string            `json:"fact"`
	Meta     map[string]string `json:"meta"`
}

// Status is active unless the file says otherwise.
func (l Lesson) Status() string {
	if s := l.Meta["status"]; s != "" {
		return s
	}
	return "active"
}

// ReadLessons parses a lessons file.
func ReadLessons(file string) ([]Lesson, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var out []Lesson
	for _, p := range knowledge.Split(string(raw)) {
		if p.Meta == nil || p.Meta["id"] == "" {
			continue
		}
		q := p.Section
		if i := strings.LastIndex(q, " > "); i >= 0 {
			q = q[i+3:]
		}
		out = append(out, Lesson{ID: p.Meta["id"], Question: q, Fact: p.Text, Meta: p.Meta})
	}
	return out, nil
}

// Use is a free turn that was given a lesson, and what the user wrote
// next in the conversation.
type Use struct {
	Run     string    `json:"run"`
	At      time.Time `json:"at"`
	Message string    `json:"message"`
	Reply   string    `json:"reply"`
	Next    string    `json:"next,omitempty"`
	Dispute bool      `json:"dispute,omitempty"`
}

// LessonReview is what review found about one lesson.
type LessonReview struct {
	Lesson
	Due      bool   `json:"due,omitempty"`     // review date within 14 days or past
	Expired  bool   `json:"expired,omitempty"` // review date past: not used
	Dead     bool   `json:"dead,omitempty"`    // not used for 90 days
	Harmful  bool   `json:"harmful,omitempty"` // disputed >= 2 times and in >= 30% of uses
	Uses     []Use  `json:"uses,omitempty"`
	Disputes int    `json:"disputes"`
	Applied  string `json:"applied,omitempty"` // what --apply did
}

// ReviewOptions configure Review.
type ReviewOptions struct {
	Namespace string
	Now       time.Time
	Decider   decide.Decider
	Mask      func(string) string
}

// Review reports each lesson of the file: due, dead, harmful.
func Review(ctx context.Context, stores []Store, lessons []Lesson, o ReviewOptions) ([]LessonReview, error) {
	if o.Mask == nil {
		o.Mask = func(s string) string { return s }
	}
	uses, err := lessonUses(ctx, stores, o)
	if err != nil {
		return nil, err
	}
	var out []LessonReview
	for _, l := range lessons {
		r := LessonReview{Lesson: l, Uses: uses[l.ID]}
		if review, err := time.Parse("2006-01-02", l.Meta["review"]); err == nil {
			r.Due = o.Now.Add(14 * 24 * time.Hour).After(review)
			r.Expired = o.Now.After(review.Add(24 * time.Hour))
		}
		last := time.Time{}
		for _, u := range r.Uses {
			if u.At.After(last) {
				last = u.At
			}
		}
		if last.IsZero() {
			if learned, err := time.Parse("2006-01-02", l.Meta["learned"]); err == nil && o.Now.Sub(learned) > 90*24*time.Hour {
				r.Dead = true
			}
		} else if o.Now.Sub(last) > 90*24*time.Hour {
			r.Dead = true
		}
		for i, u := range r.Uses {
			if u.Next == "" {
				continue
			}
			disputed, err := o.disputes(ctx, u)
			if err != nil {
				continue // unsure: not counted
			}
			r.Uses[i].Dispute = disputed
			if disputed {
				r.Disputes++
			}
		}
		r.Harmful = r.Disputes >= 2 && float64(r.Disputes) >= 0.3*float64(len(r.Uses))
		out = append(out, r)
	}
	return out, nil
}

func (o ReviewOptions) disputes(ctx context.Context, u Use) (bool, error) {
	resp, err := o.Decider.Decide(ctx, decide.Request{State: map[string]any{"message": u.Message, "reply": u.Reply, "next": u.Next}, Questions: map[string]decide.Question{"dispute": {
		Kind:         decide.KindNoul,
		Instructions: DisputeQuestion,
	}}})
	if err != nil {
		return false, err
	}
	return resp.Answers["dispute"].Noul >= 0.5, nil
}

// lessonUses reads the free turns that were given lessons, with the
// user's next message in the same conversation.
func lessonUses(ctx context.Context, stores []Store, o ReviewOptions) (map[string][]Use, error) {
	type turn struct {
		run     journal.Run
		message string
		earlier []string
		lessons []string
		reply   string
	}
	byConv := map[string][]turn{}
	for _, st := range stores {
		f := journal.RunFilter{Namespace: o.Namespace, Limit: 500}
		for {
			runs, err := st.Runs(ctx, f)
			if err != nil {
				return nil, err
			}
			for _, run := range runs {
				var scopes struct {
					Event        map[string]any `json:"event"`
					Conversation struct {
						Earlier []string `json:"earlier"`
					} `json:"conversation"`
				}
				_ = json.Unmarshal(run.Scopes, &scopes)
				msg, _ := scopes.Event["message"].(string)
				t := turn{run: run, message: msg, earlier: scopes.Conversation.Earlier}
				if run.Function == "(free)" {
					recs, err := st.Records(ctx, run.ID)
					if err != nil {
						return nil, err
					}
					for _, rec := range recs {
						var v map[string]any
						if json.Unmarshal(rec.Value, &v) != nil {
							continue
						}
						if rec.Site == "router" {
							docs, _ := v["documents"].([]any)
							for _, d := range docs {
								m, _ := d.(map[string]any)
								if id, _ := m["lesson"].(string); id != "" {
									t.lessons = append(t.lessons, id)
								}
							}
						} else if rec.Kind == journal.KindSend && t.reply == "" {
							t.reply, _ = v["text"].(string)
						}
					}
				}
				byConv[run.Conversation] = append(byConv[run.Conversation], t)
			}
			if len(runs) < 500 {
				break
			}
			f.Before = runs[len(runs)-1].CreatedAt
		}
	}
	out := map[string][]Use{}
	for _, turns := range byConv {
		for _, t := range turns {
			if len(t.lessons) == 0 {
				continue
			}
			// The next turn is the one whose earlier lines hold this
			// message, with the fewest earlier lines (clocks may tie).
			next, best := "", -1
			for _, u := range turns {
				if u.run.ID == t.run.ID || u.message == "" {
					continue
				}
				if contains(u.earlier, "user: "+t.message) && (best < 0 || len(u.earlier) < best) {
					next, best = u.message, len(u.earlier)
				}
			}
			for _, id := range t.lessons {
				out[id] = append(out[id], Use{Run: t.run.ID, At: t.run.CreatedAt, Message: o.Mask(t.message), Reply: o.Mask(t.reply), Next: o.Mask(next)})
			}
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].At.Before(out[id][j].At) })
	}
	return out, nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}

// SetMeta changes attributes of the given lessons' comments in a lessons
// file: status, reason, review... Values must have no spaces.
func SetMeta(file string, ids []string, set map[string]string) ([]string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var done []string
	out := lessonLine.ReplaceAllStringFunc(string(raw), func(line string) string {
		m := lessonLine.FindStringSubmatch(line)
		meta := knowledge.ParseMeta(m[1])
		if !want[meta["id"]] {
			return line
		}
		done = append(done, meta["id"])
		fields := strings.Fields(m[1])
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys) // new attributes in a stable order
		for _, k := range keys {
			v := set[k]
			v = strings.ReplaceAll(v, " ", "_")
			replaced := false
			for i, f := range fields {
				if strings.HasPrefix(f, k+"=") {
					fields[i], replaced = k+"="+v, true
				}
			}
			if !replaced {
				fields = append(fields, k+"="+v)
			}
		}
		return "<!-- " + strings.Join(fields, " ") + " -->"
	})
	if len(done) != len(want) {
		return done, fmt.Errorf("lessons not found: want %v, found %v", ids, done)
	}
	return done, os.WriteFile(file, []byte(out), 0o644)
}

// DisputeQuestion asks whether the user disputes the agent's previous
// reply; the host asks it live (dispute-001) and review afterwards.
const DisputeQuestion = "The user wrote MESSAGE, the agent answered REPLY, and the user then wrote NEXT. Does NEXT say that what REPLY stated is wrong, or dispute it with other information (\"não é verdade\", \"me disseram outra coisa\")? Thanks, agreement, or a new question are not that."

var lessonLine = regexp.MustCompile(`<!-- (lesson [^\n]*?) -->`)

// Renew sets the lessons' review date to now plus their kind's validity.
func Renew(file string, ids []string, now time.Time) ([]string, error) {
	lessons, err := ReadLessons(file)
	if err != nil {
		return nil, err
	}
	var done []string
	for _, l := range lessons {
		for _, id := range ids {
			if l.ID != id {
				continue
			}
			v := Validity[l.Meta["kind"]]
			if v == 0 {
				v = Validity["fact"]
			}
			if _, err := SetMeta(file, []string{id}, map[string]string{"review": now.Add(v).Format("2006-01-02"), "status": "active"}); err != nil {
				return done, err
			}
			done = append(done, id)
		}
	}
	if len(done) != len(ids) {
		return done, fmt.Errorf("lessons not found: want %v, renewed %v", ids, done)
	}
	return done, nil
}
