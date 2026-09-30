// Command weekdaybench compares serial and batched Jev weekday classification.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/ledger"
)

type sample struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Allowed []int  `json:"allowed"`
	Clarify bool   `json:"clarify,omitempty"`
}
type result struct {
	ID               string                   `json:"id"`
	Mode             string                   `json:"mode"`
	Repeat           int                      `json:"repeat"`
	Calls            int                      `json:"calls"`
	Millis           int64                    `json:"millis"`
	Cost             float64                  `json:"costUSD"`
	Correct          bool                     `json:"correct"`
	Unsafe           bool                     `json:"unsafe"`
	EffectiveAllowed []int                    `json:"effectiveAllowed,omitempty"`
	Clarified        bool                     `json:"clarified,omitempty"`
	Answers          map[string]decide.Answer `json:"answers"`
	Error            string                   `json:"error,omitempty"`
}

func questions() map[string]decide.Question {
	days := []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	out := map[string]decide.Question{}
	for i, day := range days {
		out[fmt.Sprint(i)] = decide.Question{Kind: decide.KindNoul, Instructions: "Considere somente restrições de DIA DA SEMANA para o NOVO horário desejado na mensagem. Um horário em " + day + " atende a essas restrições? Ignore horário do dia, saudações, agenda antiga e dúvidas sobre funcionamento. Se nenhum dia da semana restringir o pedido, todos os dias atendem. Respeite exclusões, alternativas e grupos: fim de semana é sábado/domingo; dias úteis são segunda a sexta. Não deduza dia da semana de data numérica. Se o pedido escolher dias específicos, os outros NÃO atendem.", IfTrue: "Esse dia é permitido pelo pedido.", IfFalse: "Esse dia é excluído pelas restrições do pedido."}
	}
	out["explicit"] = decide.Question{Kind: decide.KindNoul, Instructions: "A mensagem especifica alguma restrição de DIA DA SEMANA para o NOVO horário desejado, nomeando dias (sábado, segunda etc.) ou grupos (dias úteis, fim de semana), inclusões ou exclusões? Uma data numérica como 08/10, horário, saudação, o dia da consulta antiga ou pergunta sobre funcionamento não é restrição de dia da semana para a nova vaga.", IfTrue: "Há restrição de dia da semana solicitada para a nova vaga.", IfFalse: "Não há essa restrição; uma data numérica sozinha não conta."}
	return out
}
func score(s sample, answers map[string]decide.Answer) (bool, bool) {
	if len(answers) != 8 {
		return false, false
	}
	good, unsafe := true, false
	meta, present := answers["explicit"]
	if !present || (meta.Noul > .3 && meta.Noul < .7) {
		return false, false
	}
	if meta.Noul <= .3 {
		return len(s.Allowed) == 7, len(s.Allowed) != 7
	}
	allowed := map[int]bool{}
	for _, d := range s.Allowed {
		allowed[d] = true
	}
	for i := 0; i < 7; i++ {
		a, ok := answers[fmt.Sprint(i)]
		yes := a.Noul >= .5
		p := a.Noul
		if !yes {
			p = 1 - p
		}
		if !ok || p < .7 || yes != allowed[i] {
			good = false
		}
		if ok && p >= .7 && yes && !allowed[i] {
			unsafe = true
		}
	}
	return good, unsafe
}
func main() {
	data := flag.String("dataset", "", "JSON cases")
	out := flag.String("out", "", "JSONL results")
	mode := flag.String("mode", "batch", "batch or serial")
	repeats := flag.Int("repeats", 2, "repeats")
	cap := flag.Float64("max-cost", .2, "USD budget")
	policy := flag.String("ambiguity-policy", "", "empty (legacy), baseline, A, B, C, D, E, F, G or H")
	flag.Parse()
	if *policy != "" && *policy != "baseline" && *policy != "A" && *policy != "B" && *policy != "C" && *policy != "D" && *policy != "E" && *policy != "F" && *policy != "G" && *policy != "H" {
		panic("invalid ambiguity policy")
	}
	if *policy != "" && *mode != "batch" {
		panic("ambiguity study requires batch")
	}
	if *mode != "batch" && *mode != "serial" {
		panic("invalid mode")
	}
	raw, err := os.ReadFile(*data)
	must(err)
	var samples []sample
	must(json.Unmarshal(raw, &samples))
	l := ledger.New(ledger.Budget{MaxCostUSD: *cap})
	d := &decide.Metered{Inner: &decide.TypeSafe{APIKey: os.Getenv("TYPESAFE_API_KEY"), Model: "jev-1.13.0"}, Ledger: l}
	f, err := os.Create(*out)
	must(err)
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	for rep := 0; rep < *repeats; rep++ {
		for _, s := range samples {
			wg.Add(1)
			sem <- struct{}{}
			go func(s sample, rep int) {
				defer wg.Done()
				defer func() { <-sem }()
				r := result{ID: s.ID, Mode: *mode, Repeat: rep, Answers: map[string]decide.Answer{}}
				start := time.Now()
				qs := questions()
				if *policy == "G" || *policy == "H" {
					qs = structuredQuestions(*policy)
				} else {
					for id, q := range familyScopeQuestions(*policy) {
						qs[id] = q
					}
				}
				call := func(q map[string]decide.Question) bool {
					r.Calls++
					resp, e := d.Decide(context.Background(), decide.Request{State: map[string]any{"message": s.Message}, Questions: q})
					if resp != nil {
						r.Cost += resp.Usage.CostUSD
						for k, a := range resp.Answers {
							r.Answers[k] = a
						}
					}
					if e != nil {
						r.Error = e.Error()
						return false
					}
					return true
				}
				if *mode == "batch" {
					call(qs)
				} else {
					for _, k := range []string{"explicit", "0", "1", "2", "3", "4", "5", "6"} {
						if !call(map[string]decide.Question{k: qs[k]}) {
							break
						}
					}
				}
				r.Millis = time.Since(start).Milliseconds()
				r.Correct, r.Unsafe = score(s, r.Answers)
				if *policy == "G" || *policy == "H" {
					r.Correct, r.Unsafe, r.Clarified, r.EffectiveAllowed = scoreStructured(s, r.Answers, *policy)
				} else if *policy != "" {
					r.Correct, r.Unsafe, r.Clarified = scoreScope(s, r.Answers, *policy)
				}
				if r.Error != "" {
					r.Correct = false
				}
				mu.Lock()
				must(json.NewEncoder(f).Encode(r))
				mu.Unlock()
			}(s, rep)
		}
	}
	wg.Wait()
	must(f.Close())
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
