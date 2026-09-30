package main

import (
	"fmt"
	"github.com/bbiangul/tessera/internal/engines/decide"
)

const operandPolicy = "Analise somente a preferência ATUAL para a NOVA vaga. Ignore saudações, perguntas sobre funcionamento, datas numéricas, horas e dias da consulta antiga. Uma correção explícita ('corrigindo', 'na verdade', 'agora', 'esqueça') substitui a preferência antiga. Distinga o grupo-base de dias nomeados individualmente: dias úteis=segunda a sexta, fim de semana=sábado/domingo, qualquer dia=todos. Grupos não são menções individuais de seus membros. Expanda intervalos de dias nomeados (de terça a quinta menciona terça, quarta e quinta). Uma exclusão NÃO é inclusão. Extraia os operandos; não calcule quais dias sobram."

func structuredQuestions(policy string) map[string]decide.Question {
	qs := map[string]decide.Question{"scope": familyScopeQuestions("E")["scope"]}
	qs["base"] = decide.Question{Kind: decide.KindChoice, Instructions: operandPolicy + " Qual grupo-base é mencionado na preferência atual? Se houver um grupo e dias individuais adicionais ou excluídos, mantenha o grupo como base. Se só houver dias nomeados/intervalo, a base é explicit. Ausência de restrição é all. Se houver 'dias úteis OU fim de semana', a base é all.", Options: []decide.Option{
		{Key: "all", Description: "Qualquer dia, todos os dias, ausência de restrição ou união de dias úteis e fim de semana."},
		{Key: "weekdays", Description: "Grupo dias úteis, eventualmente com dias adicionados/excluídos."},
		{Key: "weekend", Description: "Grupo fim de semana, eventualmente com dias adicionados/excluídos."},
		{Key: "explicit", Description: "A preferência usa só dias nomeados ou intervalo entre nomes; não usa um grupo-base."},
	}}
	days := []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	for i, day := range days {
		id := fmt.Sprint(i)
		if policy == "G" {
			qs[id] = decide.Question{Kind: decide.KindChoice, Instructions: operandPolicy + " Qual é o papel explícito de " + day + " na preferência atual? Não infira o papel pela pertença a um grupo. 'Fim de semana menos domingo': domingo=exclude, sábado=none. 'Dias úteis ou sábado': sábado=include e demais dias=none.", Options: []decide.Option{{Key: "include", Description: "Dia nomeado positivamente, inclusive em intervalo entre nomes."}, {Key: "exclude", Description: "Dia nomeado para excluir ou explicitamente indisponível."}, {Key: "none", Description: "Dia não nomeado na preferência atual; pertença a grupo não conta."}}}
		} else {
			qs["include"+id] = decide.Question{Kind: decide.KindNoul, Instructions: operandPolicy + " " + day + " é nomeado POSITIVAMENTE na preferência atual, inclusive em intervalo entre nomes? Não conte pertença implícita a um grupo, exclusão ou preferência antiga.", IfTrue: "Inclusão individual positiva.", IfFalse: "Sem inclusão individual positiva."}
			qs["exclude"+id] = decide.Question{Kind: decide.KindNoul, Instructions: operandPolicy + " " + day + " é nomeado para EXCLUSÃO na preferência atual? Não deduza exclusão da escolha de outro dia/grupo; procure uma negação ou indisponibilidade explícita para esse dia.", IfTrue: "Exclusão individual explícita.", IfFalse: "Sem exclusão individual explícita."}
		}
	}
	return qs
}

func certainChoice(a decide.Answer) (string, bool) {
	return a.Choice, a.Kind == decide.KindChoice && a.Probabilities[a.Choice] >= .7
}
func certainBool(a decide.Answer) (bool, bool) {
	return a.Noul >= .5, a.Kind == decide.KindNoul && (a.Noul <= .3 || a.Noul >= .7)
}

// structuredMask composes only confidently identified operands; unknown never means all.
func structuredMask(answers map[string]decide.Answer, policy string) ([]int, bool) {
	if !scopeClear(answers, "E") {
		return nil, false
	}
	base, ok := certainChoice(answers["base"])
	if !ok {
		return nil, false
	}
	mask := [7]bool{}
	switch base {
	case "all":
		for i := range mask {
			mask[i] = true
		}
	case "weekdays":
		for i := 1; i <= 5; i++ {
			mask[i] = true
		}
	case "weekend":
		mask[0], mask[6] = true, true
	case "explicit":
	default:
		return nil, false
	}
	for i := 0; i < 7; i++ {
		var include, exclude bool
		if policy == "G" {
			role, ok := certainChoice(answers[fmt.Sprint(i)])
			if !ok {
				return nil, false
			}
			switch role {
			case "include":
				include = true
			case "exclude":
				exclude = true
			case "none":
			default:
				return nil, false
			}
		} else {
			var incOK, excOK bool
			include, incOK = certainBool(answers[fmt.Sprint("include", i)])
			exclude, excOK = certainBool(answers[fmt.Sprint("exclude", i)])
			if !incOK || !excOK || (include && exclude) {
				return nil, false
			}
		}
		mask[i] = (mask[i] || include) && !exclude
	}
	out := []int{}
	for i, yes := range mask {
		if yes {
			out = append(out, i)
		}
	}
	return out, true
}

func scoreStructured(s sample, answers map[string]decide.Answer, policy string) (bool, bool, bool, []int) {
	mask, known := structuredMask(answers, policy)
	if !known {
		return s.Clarify, false, true, nil
	}
	if s.Clarify {
		return false, true, false, mask
	}
	want := map[int]bool{}
	for _, i := range s.Allowed {
		want[i] = true
	}
	correct, unsafe := len(mask) == len(want), false
	for _, i := range mask {
		if !want[i] {
			correct, unsafe = false, true
		}
	}
	return correct, unsafe, false, mask
}
