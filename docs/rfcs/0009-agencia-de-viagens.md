# RFC-0009: a agência de viagens completa

- **Estado:** primeira fatia construída e medida em 2026-09-28 (agency-001): melhor que antes em tudo, ainda abaixo do nível registrado. Próxima: agency-002. O dono pediu ("resolve tudo") e
  delegou as decisões; as escolhas estão no fim.
- **Contexto:**
  - RFC-0001, "Caso de referência": o traveler;
  - RFC-0002: o simulador `llm`, planejado para a fase 2;
  - RFC-0004 (modo livre), RFC-0006 (equipe e aprovações),
    RFC-0008 (documentos e lições).

## O problema

**O objetivo do Tessera não é um chatbot.** É um agente que substitui uma
pessoa: fala de um jeito natural e merece a confiança de um software
determinístico. O agente de viagens deve ser uma agência completa num
agente.

**Hoje ele é um buscador.** Numa conversa real sobre Natal (2026-09-28,
Jev e DeepSeek reais, Booking.com simulada), ele falhou onde uma agente
humana não falharia:

1. **Perde o que a pessoa disse.** "15 de novembro, volto dia 22" virou
   uma busca só de ida. A resposta à pergunta "Qual o dia da ida?" só
   preenche a data de ida (medido no journal).
2. **Não junta voo e hotel.** "E hotel em Natal nessas datas?" perguntou
   o check-in de novo. "Pra nós 3" (casal e um filho de 4 anos) virou
   3 adultos.
3. **Não entende refinamento.** "Tem algo mais barato?" perguntou a
   cidade. "Busca de novo" perguntou o destino.
4. **Repete a pergunta pendente.** Depois de uma resposta fora do
   roteiro, a mesma pergunta aparece duas vezes.
5. **Não fecha negócio.** "Pode reservar?" recebe "fale com a equipe",
   sem ninguém avisado.
6. **Não aconselha.** "Vale ir pra Pipa?", "precisa de vacina?": "não
   tenho essa informação".
7. **Não faz pacote.** "Quanto custa um pacote com hotel?" cai na busca
   de voo.
8. **Não entende "SP".**

## O que é "substituir uma pessoa com confiança"

Uma agente de viagens boa:
1. **Nunca pede de novo o que já ouviu,** e diz o que está assumindo
   ("para os 3, de 15 a 22/11"), para a pessoa corrigir.
2. **Monta a viagem inteira:** voo, hotel, total e por pessoa, e o que
   está incluso.
3. **Refina** quando a pessoa pede: mais barato, direto, outro dia, outro
   bairro.
4. **Aconselha com fonte:** documentos, vacinas, época, passeios. Diz de
   onde tirou e nunca decide o caso da pessoa (websearch-002).
5. **Fecha:**
   - reúne os dados dos passageiros e confirma o total com a pessoa;
   - passa o pedido para quem emite, com o dossiê completo;
   - devolve o localizador e o link de pagamento;
   - confirma a emissão só quando ela aconteceu.
6. **Acompanha:** lembra o check-in antes do voo.
7. **Sabe o que não sabe:** alteração, cancelamento, carro, seguro vão
   para uma pessoa com o contexto. A resposta volta para o cliente, e a
   equipe ensina o agente (lições, RFC-0008).
8. **Fala a verdade sobre o estado:** reservado, pago e emitido vêm dos
   sistemas, nunca do modelo.

## O que muda

### Na plataforma (serve a todo agente)

1. **O simulador `llm` nas EvalSuites** (RFC-0002). Uma persona (objetivo,
   estilo, o que sabe) conversa com o agente até o objetivo, com as
   respostas das pessoas da equipe em ordem. Sem ele, não dá para medir
   uma conversa natural: um roteiro fixo quebra quando o agente pergunta
   outra coisa. O custo do simulador fica à parte.
2. **Uma resposta preenche mais de uma entrada.** Ao ler a resposta a uma
   pergunta, a mesma chamada também lê as entradas da Function que ainda
   não foram resolvidas ("15/11, volto dia 22" dá a ida e a volta).
3. **Sem a pergunta pendente em dobro.** Quando a resposta livre já pede
   o que a pergunta pendente pede, a pergunta não é repetida.
4. **As chamadas das EvalSuites guardam o resultado,** para um juiz
   conferir os fatos.

### No agente (`examples/traveler`)

1. **A viagem em andamento** (plugin `trip`, sql): destino, origem,
   datas e grupo, por conversa. Toda busca lê e grava a viagem. Toda
   resposta começa pelo que foi assumido.
2. **O grupo com crianças:** "nós 3" menos as crianças já conhecidas.
3. **Ida e volta de verdade:** as duas pernas e o total do grupo.
4. **Refinamentos:** mais barato, só direto, outro dia, outra
   hospedagem, sobre a mesma viagem.
5. **A cotação completa** (`quote-trip`): o melhor voo, a melhor
   hospedagem, o total e o valor por pessoa. "Pacote" cai aqui.
6. **A reserva** (`book-trip`):
   - escolhe o voo e o hotel mostrados;
   - pede nome completo e data de nascimento de cada passageiro, e um
     e-mail;
   - confirma o total;
   - registra o pedido no sistema da agência (`agency`);
   - uma tarefa vai para a fila `emissao` (Team com papéis, WhatsApp,
     Slack e o resto já prontos);
   - quem emite responde o localizador e o link de pagamento, ou por que
     não deu;
   - espera o evento do pagamento (webhook) e então a emissão;
   - lembra o check-in 2 dias antes do voo.

   Marcos: `cotou`, `pediu-reserva`, `pagou`, `emitiu` num funil `vendas`.
7. **Falar com uma agente** (`falar-com-agente`, pelo modo livre):
   alteração, cancelamento, reembolso, carro, seguro, pedidos
   especiais. A pergunta vai com o contexto e a resposta volta ao
   cliente.
8. **O modo livre de agência:**
   - busca na web (TinyFish);
   - os documentos da agência (`KnowledgeBase`: pagamento, parcelamento,
     taxas, cancelamento, como funciona a reserva);
   - as lições da equipe (RFC-0008);
   - as reservas do cliente como leitura privada ("já emitiu?").
9. **A voz:** uma agente calorosa e objetiva que mantém os verbos da ação
   (reserv, emiss).
10. **A equipe como audiência:** as tarefas abertas, a conversa mascarada
    por trás de cada tarefa.

## Como vamos saber (agency-001)

Pré-registro em `docs/research/agency-001-preregistration.md`:
- personas `dev` para ajustar e `test` selado;
- o traveler de hoje contra o novo;
- um juiz cego (Claude Sonnet 5) com os fatos dos sistemas;
- objetivo atingido, afirmações falsas, informação perdida,
  naturalidade;
- as invariantes e o custo.

## Decisões (delegadas)

| # | Decisão | Por quê |
| --- | --- | --- |
| D1 | A reserva passa por uma pessoa que emite; o agente não compra sozinho | A API da Booking.com (RapidAPI) não reserva; uma agência pequena emite por consolidadora; a confiança vem da pessoa no ponto irreversível e do sistema dizendo o estado |
| D2 | Dados dos passageiros ficam no sistema da agência, não no texto da tarefa | A tarefa vai para canais (WhatsApp, Slack); o guia pede que dados pessoais fiquem fora |
| D3 | A viagem em andamento é por conversa e vale 14 dias | Uma conversa de WhatsApp dura meses; uma viagem de março não deve preencher uma de novembro |
| D4 | Reaproveitar datas só quando a mensagem não fala de datas | "E pra Maceió?" quer as mesmas datas; "Paris em dezembro" não |
| D5 | Web: TinyFish, e nas medições uma sandbox com páginas fixas | Reprodutível; o comportamento com a web real já foi medido (websearch-001/002) |
| D6 | O alerta de preço fica para depois | Uma Function não tem laço; precisa de um desenho próprio (issue) |

## Progresso

**agency-001** (`docs/research/agency-001.md`), teste selado com 12
personas × 3:

| | Antes | Depois | Meta |
| --- | --- | --- | --- |
| Objetivo atingido | 39% | 50% | ≥ 75% |
| Conversas com algo falso | 14 | 8 | ≤ 2 |
| Informação perdida | 114 | 45 | ≤ 38 |
| Naturalidade (1–5) | 1,69 | 2,56 | ≥ 4,0 |
| Invariantes | 0 falhas | 0 falhas | 0 |

**Falta, em ordem (agency-002, com personas novas):**
1. a extração lê o que foi dito antes na conversa;
2. segurar e adiar a reserva sem insistir, guardando os dados dos
   viajantes;
3. responder pela web quando ela responde;
4. conversa com a equipe (a réplica do cliente volta à mesma tarefa);
5. escolhas por preço, ordem e extras ("o de 1500", "com a mala").
