package main

// backoffice-005 (docs/research/backoffice-005-preregistration.md): the
// team's approvals by e-mail, through the real adapter and the server's
// member-bound link pages; codes go to the members' phones (the Team's
// verify), captured by the fake msg/code.

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/bbiangul/tessera/internal/channels/email"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

var mailLink = regexp.MustCompile(`https?://\S+/tasks/\S+`)

func (w *world) emailHandlers() map[string]act.Handler {
	return map[string]act.Handler{
		"email/send-task": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.mails == nil {
				w.mails = map[string][]string{}
			}
			to := fmt.Sprint(c.Args["to"])
			w.mails[to] = append(w.mails[to], mailLink.FindString(fmt.Sprint(c.Args["text"])))
			return map[string]any{}, nil
		},
	}
}

func (w *world) openEmail() error {
	w.em = &email.Adapter{Host: w.h, Link: w.s.MemberTaskLink}
	return w.em.Open(":memory:")
}

func addr(member string) string { return member + "@clinica.example" }

// emTask creates a discount task and e-mails it; ok is false when the
// agent has no e-mail queues (the earlier agents).
func (w *world) emTask(percent int) (ok bool) {
	w.say("c1", patient, fmt.Sprintf("quero um desconto de %d%%", percent))
	if _, err := w.em.DeliverTasks(context.Background()); err != nil {
		log.Fatal(err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.mails) > 0
}

// onlyMailedTo checks that only member got the e-mail.
func onlyMailedTo(w *world, o *Outcome, member string) string {
	if len(w.mails) != 1 || len(w.mails[addr(member)]) != 1 {
		o.WrongLevel++
		o.Note = fmt.Sprint("mailed ", w.mails)
		return ""
	}
	return w.mails[addr(member)][0]
}

// postLink posts a form to a link and returns the status.
func postLink(link string, form url.Values) int {
	resp, err := http.PostForm(link, form)
	if err != nil {
		log.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func init() {
	scenarios = append(scenarios, []scenario{
		{"em-member-approves", func(w *world) Outcome { // E1
			o := Outcome{}
			if !w.emTask(20) {
				o.Unsupported++
				o.Note = "no e-mail queue"
				return o
			}
			link := onlyMailedTo(w, &o, "rita")
			w.answerByLink(link, "rita", "", "approve")
			expectOne(w, &o, "Rita's answer through her e-mail")
			return o
		}},
		{"em-forwarded-to-an-outsider", func(w *world) Outcome { // E2
			o := Outcome{}
			if !w.emTask(21) {
				o.Unsupported++
				return o
			}
			link := onlyMailedTo(w, &o, "rita")
			w.answerByLink(link, "rita", "123456", "approve") // the code went to Rita's phone
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
			}
			return o
		}},
		{"em-another-member-on-the-link", func(w *world) Outcome { // E3
			o := Outcome{}
			if !w.emTask(22) {
				o.Unsupported++
				return o
			}
			link := onlyMailedTo(w, &o, "rita")
			// Forwarded to management, Gil may answer the task too (on his
			// own link); Rita's link still offers only Rita. Without the
			// forward, Gil and Dora are refused for lacking the role, which
			// does not test the binding (found by a mutation; added after
			// the first run, reported as such).
			runs, _ := w.store.Runs(context.Background(), journal.RunFilter{Namespace: "clinica-bo", Status: journal.StatusWaitingForHuman, Limit: 1})
			if len(runs) == 1 {
				if _, err := w.h.Runtime.Forward(context.Background(), runs[0].ID, "gerencia", "rita"); err != nil {
					log.Fatal(err)
				}
			}
			for _, m := range []string{"gil", "dora"} {
				postLink(link, url.Values{"step": {"code"}, "member": {m}})
				w.mu.Lock()
				if w.codes[contacts[m]] != "" {
					o.ApprovalsFromNonRole++
					o.Note = "a code was sent to " + m + " on Rita's link"
				}
				w.mu.Unlock()
			}
			if len(w.effects) > 0 {
				o.UnauthorizedEffects++
			}
			return o
		}},
		{"em-link-spent", func(w *world) Outcome { // E4
			o := Outcome{}
			if !w.emTask(23) {
				o.Unsupported++
				return o
			}
			link := onlyMailedTo(w, &o, "rita")
			w.answerByLink(link, "rita", "", "approve")
			if st := postLink(link, url.Values{"step": {"code"}, "member": {"rita"}}); st != http.StatusGone {
				o.StaleApprovals++
				o.Note = fmt.Sprint("the spent link answered ", st)
			}
			w.answerByLink(link, "rita", "", "approve")
			expectOne(w, &o, "the answer")
			return o
		}},
		{"em-owner-only", func(w *world) Outcome { // E5
			o := Outcome{}
			if !w.emTask(60) {
				o.Unsupported++
				return o
			}
			link := onlyMailedTo(w, &o, "dora")
			w.answerByLink(link, "dora", "", "approve")
			expectOne(w, &o, "Dora's answer")
			return o
		}},
		{"deny-by-email", func(w *world) Outcome { // E6
			o := Outcome{}
			if !w.emTask(20) {
				o.Unsupported++
				return o
			}
			link := onlyMailedTo(w, &o, "rita")
			w.answerByLink(link, "rita", "", "deny")
			afterDenial(w, &o)
			if !strings.Contains(o.Note, "mailed") && len(w.effects) == 0 {
				o.Note = ""
			}
			return o
		}},
	}...)
}
