# RFC-0008: conhecimento, documentos e aprendizado com ciclo de vida

- **Estado:** aprovada, 2026-09-26. O dono delegou as decisões; as escolhas estão no fim.
- **Contexto:**
  - RFC-0001: o kind `KnowledgeBase` e a seção "Ciclo de aprendizado",
    nunca construídos;
  - RFC-0004: o modo livre;
  - RFC-0005: drift e promoção;
  - self-001 e guard-001..003;
  - o levantamento do loop artesanal do GovMachine (2026-09-26).

## O problema

**Hoje o Tessera não aprende e não lê documentos.** Medido no código em
2026-09-26:

- **O que o agente sabe** vem de três lugares:
  - as definições (`FACTS.self`, self-001);
  - as leituras do modo livre (`freeMode.reads`: ações de plugin fixas,
    chamadas a cada turno);
  - a busca na web (`freeMode.search`, websearch-001).

  Não há documentos, nem busca por trecho, nem citação de fonte.
- **O que o agente aprende:** nada que passe para a próxima conversa.
  - O conserto em tempo de execução (RFC-0005) vale só para aquela run.
  - O `tessera drift` propõe mudanças, mas uma pessoa aplica à mão.
  - A resposta de uma pessoa a uma escalação serve só àquela run.
  - Uma falha em produção não vira caso de teste.
- **O loop que existe na prática** é o nosso processo de pesquisa:
  manual, offline e alimentado por benchmarks sintéticos.

**O GovMachine mostra os dois lados de um loop artesanal:**
- **O que acerta:**
  - exemplos tirados das decisões do time;
  - um detector de regras que exige as palavras literais do usuário;
  - sugestões semanais com aceite humano.
- **O que quebra:**
  - aprende com as próprias decisões, e o erro se reforça;
  - uma regra aprendida descarta 200 itens sem aprovação;
  - rótulos errados (uma perda vira "descartar");
  - exemplos escolhidos por sobreposição de palavras: 0 a 4 de 8 eram
    do tema;
  - nenhuma medição de antes e depois;
  - nada expira;
  - dados reais de cliente dentro do repositório de testes.

## Princípios

1. **Conhecimento é dado, com origem.**
   - Todo trecho ou lição tem de onde veio (documento e versão, run,
     pessoa), quando, e para qual público vale.
   - Texto de documento ou de usuário nunca é instrução. É a regra da
     busca na web: citado, nunca obedecido.
2. **Aprender é propor, nunca agir.**
   - Uma lição não muda o comportamento até passar por um portão medido
     e por uma pessoa.
   - Efeitos continuam só em Functions.
   - Nenhuma lição varre um backlog.
3. **Só sinais de origem humana ou do mundo.**
   - Vale: respostas de pessoas, recusas com motivo, correções do
     usuário, o que a API respondeu.
   - Nunca as saídas do próprio agente como rótulo. É o erro do
     GovMachine.
4. **Tudo que é aprendido morre um dia.**
   - Cada item nasce com validade, dono e critério de revisão.
   - Aposentar é normal, auditado e reversível.
5. **Medido antes de ativar, vigiado depois.**
   - A ativação passa pelo mesmo portão que uma mudança de código:
     replay, `bench gate` e EvalSuite.
   - Depois de ativar, métricas com limites. A infraestrutura já existe
     em `Experiment` e guardrails.

## Parte A: documentos e RAG (`KnowledgeBase`)

### O recurso

```yaml
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: politicas, namespace: clinica }
spec:
  description: Políticas e FAQ da clínica.
  sources:
    - files: ["kb/*.pdf", "kb/*.md"]            # no repositório do agente
    - act: sharepoint/list-documents            # ou um plugin de leitura
      with: { site: "clinica" }
      refresh: 24h
  audiences: [paciente, equipe]                 # quem pode receber trechos
  mask: true                                    # dados pessoais mascarados na ingestão
  lifecycle: { reviewEvery: 180d }              # padrão para os trechos (parte C)
```

### Ingestão, determinística e journalada

1. Ler e extrair o texto: PDF, DOCX, HTML e MD.
2. Cortar em trechos por estrutura (títulos, parágrafos), com o caminho
   do título.
3. Mascarar dados pessoais.
4. Indexar.

Cada trecho guarda o documento, a versão (hash), a posição e a data.
Documento novo ou mudado gera versão nova. Os trechos da versão antiga
saem do índice e ficam no histórico.

### Busca híbrida, mínima primeiro

- BM25 (SQLite FTS5, dentro do binário) e, opcionalmente, embeddings.
- O "menor primeiro" do dono vale aqui: medir BM25 sozinho antes de
  trazer vetores.
- Um reordenador (Jev, ou um modelo pequeno) só entra se a medição
  pedir.

### RAG agêntico, com limites

Não é um modelo com ferramentas livres. É o padrão da busca na web:
1. Jev decide se a mensagem precisa dos documentos.
2. O Generate escreve até 2 consultas, a partir da mensagem já
   mascarada.
3. Até 2 rodadas de busca: a segunda só se a primeira não cobriu.
4. A resposta cita os trechos usados.
5. Uma checagem de ancoragem, que é uma pergunta ao Jev: cada afirmação
   factual da resposta está num trecho citado? Senão, reescreve uma vez
   e depois diz que não sabe.

**Um trecho só chega a quem pode vê-lo:** o público da `KnowledgeBase`,
com as regras de audiência e de `sees` do RFC-0006. Dentro de uma
Function, um passo `retrieve` dá os trechos como dado tipado, com
origem.

### Segurança

- Trechos são dados de terceiros: vale a regra de injeção da busca na
  web.
- Um trecho nunca dá capacidade ao agente. "Ligue para X", ou um link
  presente só no documento, são repassados como citação, nunca como
  oferta do negócio.
- O direito à exclusão se propaga ao índice por crypto-shredding.

## Parte B: o loop de aprendizado

### Sinais, lidos do journal

| Sinal | De onde | Exemplo |
| --- | --- | --- |
| Resposta de uma pessoa | tarefa `answer` ou `input` | "o estacionamento é conveniado com o Parque X" |
| Recusa com motivo | tarefa `approve` negada, recusa de API | "não damos desconto acima de 30% em implante" |
| Correção do usuário | correção de input, confirmação recusada | "não, é a unidade Moema" |
| Beco sem saída | "não entendi", modo livre sem fato, repasse | 14 perguntas sobre estacionamento na semana |
| Falha | run falhou, efeito incerto | um erro repetido de `clinic/book` |
| Drift | `tessera drift` (RFC-0005) | um campo renomeado |

### Artefatos, sempre tipados e nunca prompt solto

1. **Lição**: um fato ou pergunta-e-resposta numa `KnowledgeBase` de
   lições, com origem nas runs e nas pessoas.
2. **Caso de EvalSuite**: vem de uma falha ou correção real, mascarado.
   A próxima versão é testada contra o que as pessoas já corrigiram
   (RFC-0001).
3. **Dado de roteamento**: mensagens rotuladas para `bench route` e
   propostas de `selector.description`.
4. **Proposta de regra**: uma `precondition` ou um `authorize` escrito
   como mudança no repositório, com o EvalSuite junto. Nunca ativada
   sozinha.
5. **Dado de calibração**: perguntas do Jev com o desfecho real, para
   `bench calibrate`.

### O caminho de uma lição

```
sinais → agrupar (Jev: "são a mesma pergunta?") → propor (com as runs de evidência)
  → portão (replay + EvalSuite + casos separados; não pode piorar nenhum cenário)
  → aprovação (tarefa `approve` num canal da equipe, com o texto exato e a evidência)
  → ativa (versionada, com validade)
  → vigiada (métrica do escopo; guardrail suspende)
```

- **Agrupar antes de propor.** Uma resposta isolada vira candidata só
  com N ocorrências ou com o aval de quem respondeu ("vale sempre?").
- **A aprovação é hash-bound,** como as de hoje: aprova o texto exato.
- **Uma lição pode entrar como `Experiment`:** metade das conversas com
  ela e metade sem, com guardrail. A promoção continua sendo de uma
  pessoa.

## Parte C: aprendizado velho, morto e errado

Cada lição ou trecho tem um ciclo de vida explícito:

```
proposta → ativa → (em revisão) → suspensa → aposentada
                       ↑______________|   (restaurável; nunca apagada em silêncio)
```

**Metadados obrigatórios:**
- `origin`: runs, pessoa, documento e versão;
- `createdAt`;
- `owner`: a fila ou o papel que revisa;
- `scope`: público e sujeito;
- `validUntil` ou `reviewEvery`;
- `evidence`: contagem e última ocorrência;
- `supersedes`.

### Como algo fica velho, e o que acontece

| Gatilho | Detecção | Ação |
| --- | --- | --- |
| **Tempo** | passou `validUntil` ou `reviewEvery` (padrão por tipo: preço 30 dias, política 180 dias, fato estável 365 dias) | vai para revisão: a fila dona recebe "isto ainda vale?"; sem resposta no SLA, é suspensa |
| **A fonte mudou** | o hash do documento mudou; a leitura em que a lição se apoia mudou de forma (drift); a Function citada mudou de versão | suspensa na hora e reproposta contra a fonte nova |
| **Contradição** | lição nova conflita com uma ativa (Jev compara os pares do mesmo escopo) | nenhuma das duas ativa até uma pessoa decidir; a vencedora `supersedes` a outra |
| **Morta** | não é recuperada há 90 dias, ou nunca foi | candidata a aposentar no resumo semanal; aposentada se ninguém objetar |
| **Nociva** | recuperada e seguida de correção, repasse ou "não era isso" acima da taxa de base | suspensa pelo guardrail; vai para revisão com as conversas |
| **Modelo novo** | troca de Jev ou do modelo de texto | os EvalSuites de lições rodam de novo; as que falham são suspensas |
| **Exclusão de titular** | crypto-shredding | lições que só vêm de conversas daquele titular saem; com outras fontes, perdem aquela evidência |

**Limites:**
- número máximo de lições ativas por escopo;
- a recuperação pesa evidência e idade;
- uma lição sem evidência nova perde prioridade antes de expirar.

**Visível:** `tessera knowledge` lista por estado, idade, uso e dono, e
um resumo semanal vai pelo canal da equipe. É o "weekly digest" do
RFC-0005.

## Medição, cada parte pré-registrada antes de construir

1. **knowledge-001:** documentos sintéticos de uma clínica e perguntas
   escritas antes.
   - Mede:
     - acerto;
     - citação correta;
     - afirmações sem apoio;
     - injeção num documento;
     - vazamento entre públicos.
   - Compara BM25 com vetores, e com e sem a checagem de ancoragem.
2. **learn-001:** respostas de pessoas repetidas viram lições. Em
   mensagens separadas, mede:
   - quanto a taxa de repasse cai;
   - fatos falsos (nenhum a mais);
   - quantas lições propostas as pessoas recusam.
3. **lifecycle-001:** mudar preços e políticas no meio do caminho e medir
   respostas velhas com e sem validade e reproposta. Plantar uma lição
   nociva e medir quanto tempo leva até a suspensão.

## Fora de escopo

- Fine-tuning.
- Reescrita automática de prompts ou instruções.
- Functions escritas por modelo e ativadas sem revisão.
- Lições que agem: disparar efeitos, varrer filas.

## Ordem proposta

1. A (BM25 e citação) com knowledge-001.
2. B (lições a partir de respostas de pessoas, e casos de EvalSuite a
   partir de falhas) com learn-001.
3. C (ciclo de vida) com lifecycle-001.
4. Vetores e reordenador, só se knowledge-001 pedir.

## Decisões do dono

1. **Onde fica o índice:** SQLite (FTS5, e sqlite-vec se vierem vetores)
   dentro do binário, ou Postgres com pgvector quando o agente já usa
   Postgres.
2. **Embeddings:** por provedor (OpenRouter, Azure), local, ou nenhum até
   a medição pedir.
3. **Quem aprova as lições:** a fila `humanInTheLoop` ou uma fila própria
   de conhecimento, e por quais canais.
4. **Validades padrão por tipo**, e se lição morta se aposenta sozinha
   depois do resumo ou só com aceite.

### Decididas (2026-09-26, por delegação do dono)

1. **Índice:** SQLite FTS5 em memória, dentro do binário, reconstruído
   quando o agente carrega.
   - Os documentos estão no repositório do agente, então reconstruir
     custa pouco e não há estado a migrar.
   - Postgres só se um agente precisar de índice persistente e grande.
2. **Embeddings:** nenhum até o knowledge-001 mostrar que BM25 não
   basta.
3. **Quem aprova lições:** uma fila `knowledge`, se o agente declarar
   uma; senão, `humanInTheLoop`. Pelos canais da fila.
4. **Validades padrão:** preço 30 dias, política 180 dias, fato 365
   dias.
   - Lição morta **não** se aposenta sozinha: o resumo semanal propõe e
     uma pessoa aceita.
   - Lição nociva é **suspensa** automaticamente, o que é reversível.

## Progresso

- **Parte A, primeira fatia (2026-09-26, knowledge-001):**
  - construídos:
    - o kind `KnowledgeBase` (Markdown e texto);
    - o FTS5 em memória;
    - a reescrita da consulta;
    - `freeMode.knowledge` com checagem de ancoragem, ligada por padrão;
    - o filtro por audiência;
    - o registro dos trechos no journal.
  - Recall@4: 96%.
  - Respondíveis: 54 de 54.
  - 0 fatos sem apoio em 90 (segunda análise).
  - 0 injeções obedecidas.
  - 0 vazamentos.
  - **Faltam:**
    - PDF e DOCX, e fontes por plugin;
    - o passo `retrieve` dentro de Functions;
    - vetores, se um corpus maior pedir (knowledge-002).
- **pdf-001 (2026-09-27):** PDF e DOCX são lidos dentro do binário.
  - Com os mesmos documentos em PDF e DOCX, o resultado ficou igual ao do
    Markdown:
    - recall de 23 e 22 de 24;
    - respondíveis 53 e 54 de 54;
    - 0 injeções obedecidas e 0 vazamentos.
  - Um PDF escaneado é recusado na admissão (sem OCR).
  - PDFs em duas colunas saem embaralhados; o modelo ainda acertou 5 de
    6 perguntas.
- **plugin-001 (2026-09-27):** documentos vindos de um plugin.
  - Uma ação lista os documentos e outra busca cada um. O índice é
    atualizado periodicamente, e uma falha mantém o que já tinha sido
    lido.
  - `response: bytes` com redirecionamentos seguros.
  - `tessera add sharepoint` (Microsoft Graph).
  - Paridade com arquivos: 54 de 54.
  - O SharePoint real não foi validado (#56).
- **retrieve-001 (2026-09-27):** o passo `retrieve` dentro das
  Functions.
  - Mesmo público, ciclo de vida e relevância do modo livre.
  - O resultado fica gravado na run, para o replay.
  - Num orçamento: preços da tabela 18 de 18 com o passo, contra 0 sem
    ele; 0 preços inventados.
- **knowledge-003 (2026-09-28, #65):** o agente não conclui além do
  trecho.
  - Mudanças:
    - a checagem de ancoragem passa a contar uma conclusão para um caso
      não nomeado;
    - o modo livre ganha a regra: dizer o que o documento diz e que a
      recepção confirma.
  - Resultado:
    - conclusões indevidas caíram de 4 para 0 em 36;
    - as diretas continuam 18 de 18;
    - custo: 3 de 6 controles viraram cautela ou "não entendi" (issue).
- **Parte B, primeira fatia (2026-09-27, learn-001):**
  - construídos:
    - `tessera learn`, que propõe lições a partir das respostas da
      equipe, com a lição escrita, a checagem de fidelidade, o
      agrupamento e os conflitos;
    - `tessera learn accept`, que grava o arquivo de lições com a origem
      de cada uma;
    - `test --journal`;
    - os documentos entram antes do repasse.
  - **Resultado:**
    - nos tópicos aceitos, as respostas corretas sem repasse foram de 0
      para 12 de 12, com nada inventado e nada pessoal;
    - a proposta ficou conservadora demais: 2 a 3 de 5 tópicos gerais;
    - houve uma quase-falha perigosa, uma parcela pessoal julgada geral
      em 2 de 3 rodadas.
  - **Próximo:** learn-002.
- **learn-002 (2026-09-27):**
  - Mudanças:
    - a resposta vai ao Jev sem o nome do cliente;
    - "pessoal" passa a significar "decidido para este cliente";
    - a checagem de fidelidade pergunta o que a lição acrescenta;
    - as lições rejeitadas ficam guardadas para revisão;
    - um erro de motor não aborta mais a rodada.
  - Resultado no mesmo journal:
    - o novo propôs 4 de 4 tópicos em todas as rodadas; o antigo, 3 a 4;
    - 0 casos pessoais propostos pelos dois;
    - na fase 2, 12 de 12 respostas corretas sem repasse.
- **knowledge-002 (2026-09-27):** o filtro de relevância passou a ser
  ligado por padrão.
  - Um pedido ao Jev diz quais trechos respondem à mensagem.
  - Em dados novos:
    - respostas enganosas caíram de 14 para 7 em 42;
    - repasses subiram de 19 para 27;
    - as respondíveis se mantiveram (18 de 18).
  - Resta a inferência além do trecho (um adolescente de 17 anos, o
    carnaval).
- **learn-003 (2026-09-28):** lições aprovadas no canal da equipe.
  - Toda semana o Tick propõe as lições. Cada uma vira uma tarefa
    `approve` com o texto exato; uma contradição vem com "Substitui:".
  - Aprovada, a lição é gravada no arquivo do servidor e usada na hora.
  - Negada, não volta.
  - Ao vivo: 4 tarefas, e as 4 perguntas novas respondidas pelas lições.
- **renew-001 (2026-09-28):** renovação pelo canal.
  - Uma lição com revisão em até 14 dias vira a tarefa "Esta lição
    continua valendo?", uma por data de revisão.
  - Aprovada, é renovada e volta a ser usada. Negada, vence.
  - Ao vivo: 4 tarefas e 4 de 4 respostas depois de renovar. A menção à
    resposta repetida saiu em 2 de 4.
- **Parte C, primeira fatia (2026-09-27, lifecycle-001):**
  - construídos:
    - as lições vencidas, suspensas ou aposentadas deixam de ser usadas;
    - `learn review` marca as que estão para vencer, as vencidas, as
      mortas e as nocivas, e suspende as nocivas com `--apply`;
    - `renew`, `retire` e `accept --supersede`;
    - `learn --lessons` segura contradições.
  - **Resultado:**
    - preço vencido: dito em 9 de 9 respostas antes, 0 de 9 depois;
    - lição nociva suspensa em 3 de 3 rodadas e depois não usada;
    - a lição boa nunca foi suspensa;
    - lição morta encontrada;
    - contradição retida em 3 de 3.
- **dispute-001 (2026-09-27):** uma lição contestada pelo usuário não é
  repetida nem defendida.
  - A pergunta vai para uma pessoa, e a contestação fica registrada.
  - Resultado:
    - a afirmação contestada foi repetida em 5 de 24 casos antes, 0
      depois;
    - repasses subiram de 19 para 24 de 24;
    - continuações normais não mudaram;
    - documentos continuam tendo autoridade. A origem de cada lição já está gravada.
