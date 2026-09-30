package main

import (
	"fmt"
	"github.com/bbiangul/tessera/internal/engines/decide"
)

func scopeQuestion(policy string) decide.Question {
	const instruction = "Avalie somente o escopo dos dias da semana para a NOVA vaga. Peça esclarecimento se o usuário usa apenas expressões vagas como 'durante a semana' ou 'meio da semana', sem delimitar dias, ou se impõe restrições contraditórias sem corrigir a anterior. Dias úteis significa segunda a sexta; fim de semana sábado e domingo; intervalos de dias nomeados são claros. Ausência de restrição, data numérica, horário do dia, dia da consulta antiga e perguntas sobre funcionamento NÃO exigem esclarecer os dias da semana. Correção explícita substitui a preferência antiga. Não confunda excluir dias específicos com contradição."
	if policy == "A" {
		return decide.Question{Kind: decide.KindNoul, Instructions: instruction + " É necessário esclarecer o escopo dos dias?", IfTrue: "Escopo ambíguo ou contraditório: esclarecer.", IfFalse: "Escopo claro ou sem restrição de dia da semana."}
	}
	return decide.Question{Kind: decide.KindChoice, Instructions: instruction + " Classifique o escopo.", Options: []decide.Option{{Key: "clear", Description: "Dias claros ou ausência de restrição; não precisa esclarecer."}, {Key: "ambiguous", Description: "Escopo vago ou contraditório; precisa esclarecer."}}}
}

func scoreScope(s sample, answers map[string]decide.Answer, policy string) (correct, unsafe, clarified bool) {
	if policy != "baseline" {
		clear := scopeClear(answers, policy)
		if !clear {
			return s.Clarify, false, true
		}
	}
	meta, ok := answers["explicit"]
	if !ok || (meta.Noul > .3 && meta.Noul < .7) {
		return s.Clarify, false, true
	}
	allowed := map[int]bool{}
	for _, day := range s.Allowed {
		allowed[day] = true
	}
	mask := map[int]bool{}
	for i := 0; i < 7; i++ {
		if meta.Noul <= .3 {
			mask[i] = true
			continue
		}
		a, ok := answers[fmt.Sprint(i)]
		if !ok || (a.Noul > .3 && a.Noul < .7) {
			return s.Clarify, false, true
		}
		mask[i] = a.Noul >= .5
	}
	if s.Clarify {
		return false, true, false
	}
	correct = true
	for i := 0; i < 7; i++ {
		if mask[i] != allowed[i] {
			correct = false
		}
		if mask[i] && !allowed[i] {
			unsafe = true
		}
	}
	return correct, unsafe, false
}
