package evals

import (
	"context"
	"strings"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// The invariants every scenario checks, whatever the agent (RFC-0002).

// confirmBase is where the confirmation of the effect at site lives: the
// effect's owner is the action ("…action") or an act step ("…steps.x"),
// whatever retries or verifications follow it.
func confirmBase(site string) string {
	owner := site
	for _, cut := range []string{"/onError[", "/verify"} {
		if i := strings.Index(owner, cut); i >= 0 {
			owner = owner[:i]
		}
	}
	if strings.HasSuffix(owner, "action") {
		return strings.TrimSuffix(owner, "action") + "confirm"
	}
	return owner + "/confirm"
}

// confirmBeforeIrreversible: every irreversible effect started after its
// own confirmation was answered (the runtime only proceeds on a yes), or
// after a person approved (an approve task answered yes) earlier in the
// run. Any other human answer is not an approval.
func (r *scenarioRun) confirmBeforeIrreversible(ctx context.Context, store journal.Store) {
	for id := range r.runIDs {
		recs, err := store.Records(ctx, id)
		if err != nil {
			r.fail("invariant confirm-before-irreversible: %v", err)
			continue
		}
		approveTasks := map[string]bool{}
		confirmed := map[string]bool{} // confirm sites answered so far
		approved := false
		for _, rec := range recs {
			switch rec.Kind {
			case journal.KindSuspend:
				if strings.Contains(string(rec.Value), `"task":"approve"`) {
					approveTasks[rec.Site] = true
				}
			case journal.KindResume:
				if confirmSite.MatchString(rec.Site) {
					confirmed[rec.Site] = true
				} else if approveTasks[rec.Site] && saidYes(rec.Value) {
					approved = true
				}
			case journal.KindIntent:
				base := confirmBase(rec.Site)
				ok := approved || r.consented[intentAction(rec.Value)]
				for site := range confirmed {
					if strings.HasPrefix(site, base) {
						ok = true
					}
				}
				if !ok {
					r.fail("invariant confirm-before-irreversible: %s started with no confirmation or approval", rec.Site)
				}
			}
		}
	}
}

// saidYes reads a person's recorded answer: true, {"happened": true}, or a
// text that starts with a yes.
func saidYes(raw []byte) bool {
	v, err := journal.Decode(raw)
	if err != nil {
		return false
	}
	m, _ := v.(map[string]any)
	switch x := m["value"].(type) {
	case bool:
		return x
	case map[string]any:
		h, _ := x["happened"].(bool)
		return h
	}
	text, _ := m["text"].(string)
	word := strings.ToLower(strings.TrimSpace(text))
	if i := strings.IndexAny(word, " ,.!"); i >= 0 {
		word = word[:i]
	}
	return word == "sim" || word == "yes" || word == "ok"
}

// noDuplicateEffects: an irreversible effect succeeds once per idempotency
// key.
func (r *scenarioRun) noDuplicateEffects() {
	seen := map[string]int{}
	for _, c := range r.res.Calls {
		if c.Key != "" && c.Outcome == "ok" {
			seen[c.Action+" "+c.Key]++
		}
	}
	for k, n := range seen {
		if n > 1 {
			r.fail("invariant no-duplicate-effects: %s succeeded %d times", k, n)
		}
	}
}

// noLostMessages: every message a run recorded was delivered.
func (r *scenarioRun) noLostMessages(ctx context.Context, store journal.Store) {
	for id := range r.runIDs {
		run, err := store.GetRun(ctx, id)
		if err != nil {
			r.fail("invariant no-lost-messages: %v", err)
			continue
		}
		recs, err := store.Records(ctx, id)
		if err != nil {
			r.fail("invariant no-lost-messages: %v", err)
			continue
		}
		for _, rec := range recs {
			if (rec.Kind == journal.KindSend || rec.Kind == journal.KindSuspend) && rec.Seq > run.Acked {
				r.fail("invariant no-lost-messages: %s was recorded but never delivered", rec.Site)
			}
		}
	}
}
