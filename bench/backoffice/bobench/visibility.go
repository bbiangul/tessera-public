package main

// audiences-002 (docs/research/audiences-002-preregistration.md): what a
// member sees of the conversation behind a task, the audit of every view,
// and approve-actions for the dentists.

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const (
	patientEmail = "ana@exemplo.com"
	patientPhone = "+55 11 98888-7777"
	withContact  = "quero um desconto de %d%%, meu email é " + patientEmail + " e meu fone " + patientPhone
)

// deliverAll sends the open tasks on WhatsApp and Slack.
func (w *world) deliverAll() {
	ctx := context.Background()
	if _, err := w.wa.DeliverTasks(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err := w.sl.DeliverTasks(ctx); err != nil {
		log.Fatal(err)
	}
}

// posted is what a member got on a channel's task messages.
func posted(list []string, to string) string {
	var out []string
	for _, p := range list {
		if strings.HasPrefix(p, to+"|") {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n")
}

// audits are the view records of a run: "member/channel" -> visibility.
func (w *world) audits(runID string) map[string]string {
	recs, _ := w.store.Records(context.Background(), runID)
	out := map[string]string{}
	for _, r := range recs {
		if i := strings.Index(r.Site, "/viewed/"); i >= 0 {
			v, _ := journal.Decode(r.Value)
			m, _ := v.(map[string]any)
			out[r.Site[i+len("/viewed/"):]] = fmt.Sprint(m["visibility"])
		}
	}
	return out
}

func init() {
	scenarios = append(scenarios, []scenario{
		{"vis-reception-masked", func(w *world) Outcome { // V1, V4
			o := Outcome{}
			r := w.say("c1", patient, fmt.Sprintf(withContact, 20))
			w.deliverAll()
			card := posted(w.slPosts, ritaSlack) + "\n" + posted(w.buttons, ritaWA)
			if !strings.Contains(card, "Conversa:") {
				o.Unsupported++
				o.Note = "no conversation shown: " + card
				return o
			}
			if strings.Contains(card, patientEmail) || strings.Contains(card, "98888-7777") {
				o.Leaks++
				o.Note = card
			}
			if !strings.Contains(card, "desconto") {
				o.Unsupported++
			}
			a := w.audits(r.RunID)
			if a["rita/slack"] != "masked" || a["rita/whatsapp"] != "masked" {
				o.OutcomeNotShown++ // a view without its audit record
				o.Note = fmt.Sprint("audits ", a)
			}
			return o
		}},
		{"vis-owner-full", func(w *world) Outcome { // V2
			o := Outcome{}
			r := w.say("c1", patient, fmt.Sprintf(withContact, 60))
			w.deliverAll()
			card := posted(w.buttons, doraWA)
			if !strings.Contains(card, patientEmail) {
				o.Unsupported++
				o.Note = "the owner does not see it in full: " + card
			}
			if w.audits(r.RunID)["dora/whatsapp"] != "full" {
				o.OutcomeNotShown++
			}
			return o
		}},
		{"vis-none", func(w *world) Outcome { // V3, V4
			o := Outcome{}
			r := w.say("c1", patient, fmt.Sprintf(withContact, 20))
			t, err := w.h.Runtime.PendingTask(context.Background(), r.RunID)
			if err != nil {
				o.Unsupported++
				return o
			}
			bruno, ok := w.h.Runtime.Member("clinica-bo", "bruno")
			if !ok {
				o.Unsupported++
				o.Note = "no dentist in the Team"
				return o
			}
			if view := w.h.Runtime.TaskView(context.Background(), t, bruno, "slack"); view != t.Text {
				o.Leaks++
				o.Note = view
			}
			if _, seen := w.audits(r.RunID)["bruno/slack"]; seen {
				o.OutcomeNotShown++ // an audit for a view that showed nothing
			}
			return o
		}},
		{"vis-free-mode-masked", func(w *world) Outcome { // V5
			o := Outcome{}
			w.say("c1", patient, fmt.Sprintf(withContact, 60))
			w.waText(1, doraWA, "tenho algo para aprovar?")
			w.mu.Lock()
			defer w.mu.Unlock()
			if len(w.taskConv) == 0 {
				o.Unsupported++
				o.Note = "no conversation in the model's facts"
				return o
			}
			for _, c := range w.taskConv {
				if strings.Contains(c, patientEmail) || strings.Contains(c, "98888-7777") {
					o.Leaks++ // the model got it unmasked
					o.Note = c
				}
			}
			return o
		}},
		{"au2-dentist-approve-actions", func(w *world) Outcome { // A1
			o := Outcome{}
			w.say("c2", dentist1, "bloqueia minha sexta")
			r := w.say("c2", dentist1, "sim")
			if len(w.effects) != 0 {
				o.UnauthorizedEffects++ // it happened before a person approved
			}
			link, t := w.link(r.RunID)
			if link == "" || !strings.Contains(t.Text, "clinic/block") {
				o.Unsupported++
				o.Note = fmt.Sprint("no approval task: ", t.Text)
				return o
			}
			w.approveByLink(link, "rita", "")
			expectOne(w, &o, "the approved block")
			return o
		}},
		{"au2-dentist-denied", func(w *world) Outcome { // A1
			o := Outcome{}
			w.say("c2", dentist1, "bloqueia minha sexta")
			r := w.say("c2", dentist1, "sim")
			link, _ := w.link(r.RunID)
			if link == "" {
				o.Unsupported++
				return o
			}
			w.answerByLink(link, "rita", "", "deny")
			afterDenial(w, &o)
			if !strings.Contains(strings.Join(userTexts(w), "\n"), teamDenied) {
				o.MessageMismatch++
			}
			return o
		}},
	}...)
}
