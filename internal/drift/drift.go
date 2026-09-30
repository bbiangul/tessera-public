// Package drift reads the journal for signs that an agent's systems
// changed under it (RFC-0005 steps 1 and 4): refusals that say what the
// system now wants, reads that broke their declared type, and the repairs
// the agent made. It reports them per action for the developer, with
// proposals to update the definitions. It never changes anything.
package drift

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Store is what the report reads.
type Store interface {
	Runs(ctx context.Context, f journal.RunFilter) ([]journal.Run, error)
	Records(ctx context.Context, runID string) ([]journal.Record, error)
}

// Signal is one kind of finding, counted.
type Signal struct {
	Key         string    `json:"key"`
	Count       int       `json:"count"`
	First, Last time.Time `json:"-"`
	ExampleRun  string    `json:"exampleRun"`
}

// Repair is one kind of action repair and how it went.
type Repair struct {
	Fields    string `json:"fields"`
	Asked     string `json:"asked,omitempty"`
	Agree     string `json:"agree,omitempty"`
	Proposed  int    `json:"proposed"`
	Approved  int    `json:"approved"`
	Succeeded int    `json:"succeeded"`
}

// Proposal is a suggested change to the definitions, for a person or the
// authoring skill to review.
type Proposal struct {
	Kind string `json:"kind"` // binding-output, author-note
	Text string `json:"text"`
	YAML string `json:"yaml,omitempty"`
}

// Action is everything found about one plugin action.
type Action struct {
	Action    string     `json:"action"`
	Refusals  []Signal   `json:"refusals,omitempty"`
	Contracts []Signal   `json:"contracts,omitempty"`
	Renames   []Signal   `json:"renames,omitempty"`
	Repairs   []Repair   `json:"repairs,omitempty"`
	Proposals []Proposal `json:"proposals,omitempty"`
}

// Report is a namespace's drift over a window.
type Report struct {
	Namespace string    `json:"namespace"`
	Since     time.Time `json:"since"`
	Runs      int       `json:"runs"`
	Actions   []*Action `json:"actions"`
}

// Analyze reads the runs of ns created since since, in every store.
// Plugins (by name) let it propose binding changes; nil skips proposals.
func Analyze(ctx context.Context, stores []Store, ns string, since time.Time, manifests map[string]*plugins.PluginManifest) (*Report, error) {
	r := &Report{Namespace: ns, Since: since}
	byAction := map[string]*collector{}
	get := func(a string) *collector {
		if byAction[a] == nil {
			byAction[a] = &collector{action: a, refusals: map[string]*Signal{}, contracts: map[string]*Signal{}, renames: map[string]*Signal{}, repairs: map[string]*Repair{}}
		}
		return byAction[a]
	}
	for _, st := range stores {
		f := journal.RunFilter{Namespace: ns, Limit: 500}
		for {
			runs, err := st.Runs(ctx, f)
			if err != nil {
				return nil, err
			}
			for _, run := range runs {
				if run.CreatedAt.Before(since) {
					continue
				}
				r.Runs++
				recs, err := st.Records(ctx, run.ID)
				if err != nil {
					return nil, err
				}
				scanRun(run, recs, get)
			}
			if len(runs) < 500 {
				break
			}
			f.Before = runs[len(runs)-1].CreatedAt
		}
	}
	names := make([]string, 0, len(byAction))
	for a := range byAction {
		names = append(names, a)
	}
	sort.Strings(names)
	for _, a := range names {
		c := byAction[a]
		out := c.report()
		out.Proposals = propose(out, manifests)
		r.Actions = append(r.Actions, out)
	}
	return r, nil
}

type collector struct {
	action                       string
	refusals, contracts, renames map[string]*Signal
	repairs                      map[string]*Repair
}

func count(m map[string]*Signal, key, run string, at time.Time) {
	s := m[key]
	if s == nil {
		s = &Signal{Key: key, First: at, ExampleRun: run}
		m[key] = s
	}
	s.Count++
	if at.After(s.Last) {
		s.Last = at
	}
	if at.Before(s.First) {
		s.First = at
	}
}

// scanRun collects one run's signals.
func scanRun(run journal.Run, recs []journal.Record, get func(string) *collector) {
	bySite := map[string]journal.Record{}
	for _, rec := range recs {
		bySite[rec.Site+"|"+string(rec.Kind)] = rec
	}
	decode := func(rec journal.Record) map[string]any {
		var m map[string]any
		_ = json.Unmarshal(rec.Value, &m)
		return m
	}
	for _, rec := range recs {
		m := decode(rec)
		switch {
		case rec.Kind == journal.KindError:
			action, _ := m["action"].(string)
			definite, _ := m["definite"].(bool)
			code, _ := m["code"].(string)
			if action == "" || !definite {
				continue
			}
			if code == "CONTRACT" {
				msg, _ := m["message"].(string)
				msg = strings.TrimPrefix(msg, action+": ")
				if i := strings.Index(msg, " (has "); i > 0 {
					msg = msg[:i] // the fields present vary by item; the missing ones are the finding
				}
				count(get(action).contracts, msg, run.ID, rec.At)
				continue
			}
			if detail, _ := m["detail"].(string); detail != "" {
				count(get(action).refusals, code+": "+detail, run.ID, rec.At)
			}
		case rec.Kind == journal.KindResult && strings.HasSuffix(rec.Site, "/repaired"):
			action, _ := m["action"].(string)
			rename, _ := m["rename"].(map[string]any)
			if action == "" || len(rename) == 0 {
				continue
			}
			pairs := make([]string, 0, len(rename))
			for to, from := range rename {
				pairs = append(pairs, fmt.Sprintf("%s ← %v", to, from))
			}
			sort.Strings(pairs)
			count(get(action).renames, strings.Join(pairs, ", "), run.ID, rec.At)
			// A repaired read broke its declared type first (drift-009).
			typ, _ := m["type"].(string)
			missing := make([]string, 0, len(rename))
			for to := range rename {
				missing = append(missing, to)
			}
			sort.Strings(missing)
			count(get(action).contracts, fmt.Sprintf("the answer did not match %s: missing %s (repaired by renaming)", typ, strings.Join(missing, ", ")), run.ID, rec.At)
		case rec.Kind == journal.KindResult && strings.HasSuffix(rec.Site, "/repair/proposal"):
			action, _ := m["action"].(string)
			p, _ := m["proposal"].(map[string]any)
			if action == "" || p == nil || p["possible"] != true {
				continue
			}
			base := strings.TrimSuffix(rec.Site, "/proposal")
			key, asked, agree := repairKey(p)
			c := get(action)
			rp := c.repairs[key]
			if rp == nil {
				rp = &Repair{Fields: key, Asked: asked, Agree: agree}
				c.repairs[key] = rp
			}
			rp.Proposed++
			if ans, ok := bySite[base+"/approve|"+string(journal.KindResume)]; ok && approved(decode(ans)) {
				rp.Approved++
			}
			if _, ok := bySite[base+"/call|"+string(journal.KindResult)]; ok {
				rp.Succeeded++
			}
		}
	}
}

func repairKey(p map[string]any) (fields, asked, agree string) {
	var names []string
	if add, ok := p["add"].(map[string]any); ok {
		for k, v := range add {
			names = append(names, fmt.Sprintf("%s=%v", k, v))
		}
	}
	if ask, ok := p["askUser"].(map[string]any); ok {
		if f, _ := ask["field"].(string); f != "" {
			names = append(names, f+"=(asked)")
		}
		asked, _ = ask["question"].(string)
	}
	sort.Strings(names)
	agree, _ = p["userMustAgree"].(string)
	return strings.Join(names, ", "), asked, agree
}

func approved(m map[string]any) bool {
	if v, ok := m["value"].(bool); ok {
		return v
	}
	t, _ := m["text"].(string)
	t = strings.ToLower(strings.TrimSpace(t))
	return t == "yes" || t == "sim" || strings.HasPrefix(t, "yes") || strings.HasPrefix(t, "sim") || t == "approve"
}

func (c *collector) report() *Action {
	sorted := func(m map[string]*Signal) []Signal {
		out := make([]Signal, 0, len(m))
		for _, s := range m {
			out = append(out, *s)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return out[i].Key < out[j].Key
		})
		return out
	}
	a := &Action{Action: c.action, Refusals: sorted(c.refusals), Contracts: sorted(c.contracts), Renames: sorted(c.renames)}
	for _, rp := range c.repairs {
		a.Repairs = append(a.Repairs, *rp)
	}
	sort.Slice(a.Repairs, func(i, j int) bool { return a.Repairs[i].Proposed > a.Repairs[j].Proposed })
	return a
}
