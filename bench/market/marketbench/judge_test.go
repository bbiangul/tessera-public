package main

import "testing"

func TestYes(t *testing.T) {
	for text, want := range map[string]bool{
		"Sim, isso mesmo.": true, "tá bom, pode marcar": true, "Combinado!": true, "Não tem problema, pode remarcar": true,
		"Sim, esse eu prefiro": true, "confirma pra mim": true,
		"Na verdade mudei de ideia, prefiro terça": false, "Não quero remarcar, obrigado": false, "Quais horários tem?": false,
	} {
		if got := isYes(text); got != want {
			t.Errorf("isYes(%q) = %v", text, got)
		}
	}
}

func TestProposesAndNames(t *testing.T) {
	if !proposes("Posso remarcar para quarta 07/10 às 16h?") || proposes("Qual horário prefere?") || proposes("Remarquei para 07/10.") {
		t.Error("proposes")
	}
	for text, want := range map[string]bool{
		"Confirmo a remarcação para quarta 07/10 às 16h?": true, "Posso marcar dia 7/10 às 16:00?": true,
		"Posso marcar 07/10 às 15h?": false, "Posso marcar 08/10 às 16h?": false, "Quer ver horários?": false,
	} {
		if got := names(text, "D1-1007-16"); got != want {
			t.Errorf("names(%q) = %v", text, got)
		}
	}
}
