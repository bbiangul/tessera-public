package main

import "github.com/bbiangul/tessera/internal/engines/decide"

const scopePolicy = "Julgue a preferência de dias para a NOVA vaga, ignorando saudações, pedidos como 'filtre o calendário', a consulta antiga e perguntas sobre funcionamento. Uma expressão de grupo com limites vagos (durante a semana, começo, meio ou final da semana) exige esclarecer quais dias pertencem ao grupo. Isso continua valendo se o usuário excluir dias nomeados: 'durante a semana, menos terça' ainda não diz se sábado/domingo pertencem ao grupo. Só um conjunto ou intervalo explicitamente delimitado resolve esses limites: dias úteis=segunda a sexta, fim de semana=sábado e domingo, de segunda a quarta=segunda/terça/quarta. Restrições simultâneas incompatíveis precisam de esclarecimento; uma correção explícita substitui a preferência antiga e não é contradição. Não ter preferência por dia, dar somente data numérica/hora ou mencionar dia na consulta antiga/FAQ NÃO exige esclarecer dias da semana."

func familyScopeQuestions(policy string) map[string]decide.Question {
	if policy == "baseline" || policy == "" {
		return nil
	}
	if policy == "A" || policy == "B" {
		return map[string]decide.Question{"scope": scopeQuestion(policy)}
	}
	if policy == "E" || policy == "F" {
		base := "C"
		if policy == "F" {
			base = "D"
		}
		qs := familyScopeQuestions(base)
		for id, q := range qs {
			q.Instructions = exclusionPolicy + " " + q.Instructions
			qs[id] = q
		}
		return qs
	}
	if policy == "C" {
		return map[string]decide.Question{"scope": {
			Kind: decide.KindNoul, Instructions: scopePolicy + " A preferência exige esclarecer dias por limites vagos ou restrições incompatíveis?",
			IfTrue: "Sim, precisa esclarecer antes de permitir qualquer dia.", IfFalse: "Não: limites explícitos e compatíveis, ou nenhuma restrição de dia da semana.",
		}}
	}
	return map[string]decide.Question{
		"vague":    {Kind: decide.KindNoul, Instructions: scopePolicy + " Há um grupo de dias com limites vagos na preferência atual, ainda não delimitado explicitamente? Avalie só a vagueza dos limites, não contradições.", IfTrue: "Grupo com limites vagos, mesmo se excluir dias específicos.", IfFalse: "Sem grupo vago: limites explicitamente definidos ou nenhuma restrição de dia."},
		"conflict": {Kind: decide.KindNoul, Instructions: scopePolicy + " Há restrições simultâneas de dias que não podem ser atendidas juntas, sem uma correção explícita? Avalie só incompatibilidade, não vagueza.", IfTrue: "Restrições incompatíveis e não corrigidas.", IfFalse: "Restrições compatíveis, corrigidas ou ausentes."},
	}
}

func scopeClear(answers map[string]decide.Answer, policy string) bool {
	if policy == "B" {
		a, ok := answers["scope"]
		return ok && a.Kind == decide.KindChoice && a.Choice == "clear" && a.Probabilities["clear"] >= .7
	}
	ids := []string{"scope"}
	if policy == "D" || policy == "F" {
		ids = []string{"vague", "conflict"}
	}
	for _, id := range ids {
		a, ok := answers[id]
		if !ok || a.Kind != decide.KindNoul || a.Noul > .3 {
			return false
		}
	}
	return true
}

const exclusionPolicy = "Regras para distinguir conjuntos claros de grupos vagos: 'qualquer dia', 'todos os dias' e 'sem restrição' definem TODOS os sete dias. Excluir dias nomeados desse conjunto é claro: 'qualquer dia menos sábado' permite domingo a sexta, sem precisar esclarecer. Excluir de um grupo definido também é claro: 'dias úteis menos terça' ou 'fim de semana menos domingo'. Não confunda uma exclusão com contradição. Já 'durante a semana menos terça' tem limites vagos, pois excluir terça não define os demais dias do grupo. Quando a pessoa diz 'corrigindo', 'na verdade', 'mudei de ideia', 'esqueça o anterior' ou equivalente, avalie somente a preferência corrigida; não combine a antiga com a nova. Uma restrição positiva incompatível com outra sem correção explícita exige esclarecer."
