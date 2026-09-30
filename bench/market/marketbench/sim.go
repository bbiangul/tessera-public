package main

import (
	"context"
	"regexp"
	"strings"
)

// simulator plays the patient from the task's goal card (the same for
// every competitor): temperature 0, one message per turn, ###DONE### at the
// end.
type simulator struct {
	chat *chatClient
	goal string
}

const simPrompt = `Você é um paciente conversando por chat com a recepção de uma clínica odontológica.
Seu objetivo: %s
Hoje é sexta-feira, 02/10/2026.
Escreva SOMENTE a próxima mensagem do paciente: curta, natural, em português, sem aspas.
Não invente dados que o objetivo não dá. Siga o objetivo à risca.
Quando o objetivo estiver cumprido, ou for impossível, ou a recepção encerrar, responda exatamente ###DONE###.`

// next is the patient's next message given the transcript so far
// (agent lines are the other side).
func (s *simulator) next(ctx context.Context, transcript []line) (string, error) {
	msgs := []chatMessage{{Role: "system", Content: strings.Replace(simPrompt, "%s", s.goal, 1)}}
	if len(transcript) == 0 {
		msgs = append(msgs, chatMessage{Role: "user", Content: "(a conversa começa: escreva a primeira mensagem do paciente)"})
	}
	for _, l := range transcript {
		role := "user" // the agent speaks to the simulator
		if l.Who == "patient" {
			role = "assistant"
		}
		msgs = append(msgs, chatMessage{Role: role, Content: l.Text})
	}
	if len(transcript) > 0 && transcript[len(transcript)-1].Who == "patient" {
		msgs = append(msgs, chatMessage{Role: "user", Content: "(sem resposta da recepção)"})
	}
	r, err := s.chat.complete(ctx, "simulator", msgs, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Message.Content), nil
}

var yesRe = regexp.MustCompile(`(?i)(^|\W)(sim|confirmo|confirma|confirmado|pode (remarcar|confirmar|ser|fazer|marcar|agendar|mudar|trocar)|isso( mesmo)?|esse mesmo|perfeito|fechado|combinado|certo|ok|beleza|claro|com certeza|t[áa] bom|[óo]timo|quero sim)(\W|$)`)

// noRe vetoes a yes: a refusal or a change of mind, not any "não" ("não
// tem problema, pode remarcar" is a yes).
var noRe = regexp.MustCompile(`(?i)(^|\W)(não (quero|confirm|pode|precisa|vou)|nao (quero|confirm|pode|precisa|vou)|mudei de ideia|prefiro (outro|outra|o outro|a outra|na|no|às|as|para|pra)|na verdade)`)

// isYes: the patient's message confirms (a yes, and no refusal or change
// of mind).
func isYes(text string) bool { return yesRe.MatchString(text) && !noRe.MatchString(text) }

var dateRe = regexp.MustCompile(`\b\d{1,2}/\d{1,2}\b`)

// proposes: the agent's message proposes a date and asks (the moment a
// yes confirms something).
func proposes(text string) bool { return dateRe.MatchString(text) && strings.Contains(text, "?") }

// names says whether an agent message states a slot's day and hour; slot
// IDs are <dentist>-<MMDD>-<HH>.
func names(text, slotID string) bool {
	parts := strings.Split(slotID, "-")
	if len(parts) != 3 || len(parts[1]) != 4 {
		return false
	}
	mm, dd, hh := parts[1][:2], parts[1][2:], parts[2]
	day := regexp.MustCompile(`\b0?` + strings.TrimPrefix(dd, "0") + `/0?` + strings.TrimPrefix(mm, "0") + `\b`)
	hour := regexp.MustCompile(`\b0?` + strings.TrimPrefix(hh, "0") + `(h|:00| ?horas)`)
	return day.MatchString(text) && hour.MatchString(text)
}
