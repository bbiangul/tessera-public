package main

// #53 (backoffice-003 addendum): a denied approval must not act, nor tell
// the user it was approved. Denials on the link, WhatsApp and Slack.

import (
	"context"
	"strings"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// afterDenial counts the effects, and the messages to users saying
// "aprovado", once a task was denied.
func afterDenial(w *world, o *Outcome) {
	o.DeniedButDone = len(w.effects)
	ctx := context.Background()
	runs, err := w.store.Runs(ctx, journal.RunFilter{Namespace: "clinica-bo", Limit: 100})
	if err != nil {
		panic(err)
	}
	for _, run := range runs {
		recs, _ := w.store.Records(ctx, run.ID)
		for _, r := range recs {
			v, err := journal.Decode(r.Value)
			if err != nil || r.Kind != journal.KindSend {
				continue
			}
			m, _ := v.(map[string]any)
			if text, _ := m["text"].(string); m["to"] == "user" && strings.Contains(strings.ToLower(text), "aprovado") {
				o.ToldApproved++
			}
		}
	}
}

func init() {
	scenarios = append(scenarios, []scenario{
		{"deny-on-the-link", func(w *world) Outcome { // D1
			r := w.say("c1", patient, "quero um desconto de 20%")
			link, _ := w.link(r.RunID)
			o := Outcome{}
			w.answerByLink(link, "rita", "", "deny")
			if run, _ := w.store.GetRun(context.Background(), r.RunID); run.Status == journal.StatusWaitingForHuman {
				o.Unsupported++
				o.Note = "the denial did not answer"
			}
			afterDenial(w, &o)
			return o
		}},
		{"deny-on-whatsapp", func(w *world) Outcome { // D2
			id, ok := w.waTask(20)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.waTap(1, ritaWA, strings.TrimSuffix(id, ":approve")+":deny")
			afterDenial(w, &o)
			return o
		}},
		{"deny-on-slack", func(w *world) Outcome { // D3
			id, ok := w.slTask(20)
			o := Outcome{}
			if !ok {
				o.Unsupported++
				return o
			}
			w.slClick(workspace, ritaSlack, "1.1", strings.TrimSuffix(id, ":approve")+":deny", 0, slackSecret)
			afterDenial(w, &o)
			w.shown(&o, "1.1", "Recusado por Rita")
			return o
		}},
	}...)
}
