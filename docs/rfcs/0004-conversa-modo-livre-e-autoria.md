# RFC-0004: conversa natural, modo livre seguro e agente criado por um prompt

- **Estado:** aprovada (2026-09-24) e implementada: A (voz), B (modo livre, interrupções, repasse) e C (skill de autoria). Resultados em `docs/research/` (voice-*, freemode-*, escalation-*, history-001, authoring-*).
- **Data:** 2026-09-23
- **Contexto:** [market-bench-001](../research/market-bench-001.md),
  [002](../research/market-bench-002.md) e [003](../research/market-bench-003.md);
  feedback do dono sobre essas rodadas.

## O problema

Três benchmarks com o mesmo modelo, os mesmos pacientes simulados e as
mesmas falhas contra LangGraph, OpenAI Agents SDK e um loop simples
mostraram o seguinte:

- **O núcleo tem valor.** O Tessera fez zero ações duplicadas quando a
  infraestrutura falha; os outros fizeram 30–50%. Custa cerca de 6× menos
  e responde cerca de 4× mais rápido. Na rodada 003 teve o mesmo sucesso
  em conversa normal (100%).
- **A conversa é pior.** As mensagens são templates fixos, secos. Os
  agentes baseados em LLM escrevem melhor. Nas palavras do dono: "senão
  vira um chatbot de 2015".
- **Fora do escopo, a resposta é ruim.** Um pedido que nenhuma Function
  cobre, ou uma pergunta no meio de um processo ("tem estacionamento?"),
  recebe "Desculpe, não entendi".
- **Construir o agente dá trabalho.** A Function de remarcação precisou
  de três redesenhos para chegar a 100%: pedir o dia primeiro, mudar a
  redação da confirmação, tratar o pedido para outra pessoa. Os LLMs
  chegaram lá com um prompt.

## Objetivos

1. **Conversa no nível de um agente LLM**, sem abrir mão da garantia de
   que os fatos ditos (valores, horários, o que vai ser feito) são os que
   o sistema tem.
2. **Fora do escopo, atendimento humano:** responder dúvidas, conversar,
   consultar informação, usar código em sandbox. Processos estritos
   continuam estritos.
3. **Um agente nasce de um prompt para o Claude Code**, com segurança e
   testes, sem o autor precisar conhecer o YAML.
4. **Tudo medido**, antes e depois, no mesmo benchmark.

## A regra que não muda

> **Nenhum efeito acontece fora de uma Function declarada.**

O modo livre conversa, lê e calcula. Para agir (remarcar, enviar, cobrar,
apagar), ele só pode **iniciar a Function correspondente**, que segue o
caminho estrito: confirmação, idempotência, journal, verificação. Código
gerado roda em sandbox, sem rede de escrita.

Toda a vantagem medida do Tessera vem de os efeitos passarem por um
caminho declarado. Um modo livre que agisse sozinho recriaria o problema
que os benchmarks mostraram nos outros frameworks (até 51% de ações
duplicadas).

## Parte A: a voz (conversa natural com fatos travados)

- **`Agent.spec.voice`:** persona, tom, idioma e exemplos. Quem escreve é
  o autor, ou o Claude Code por ele.
- **Tudo que sai para o usuário passa pela voz.** O template da Function
  continua existindo como **conteúdo**: o que dizer e quais fatos. O
  Generate reescreve no tom da persona. Após voice-007, a voz warm recebe
  o texto aprovado, fatos, nome e apenas a indicação de que o agente já
  falou, sem as falas brutas: pedidos paralelos induziam promessas indevidas
  (#34). Roteamento e modo livre mantêm seu contexto; a voz strict continua
  recebendo diálogo mascarado. A decisão foi comparada e replicada antes
  da implementação; não é uma garantia universal contra alucinações.
- **Fatos travados:** cada valor interpolado (`{{ inputs.newSlot.label }}`)
  é um fato. O texto reescrito precisa conter cada fato **literalmente**.
  Se não contiver, sai o template. Em confirmações, além disso, o texto
  tem que continuar sendo uma pergunta e nomear tudo que a ação altera
  (a regra `CONFIRM_INCOMPLETE`, agora checada também no texto final).
- **Custo:** uma chamada ao Generate por mensagem. Estimativa: a latência
  mediana vai de ~0,6 s para ~1,5 s, ainda abaixo dos concorrentes
  (2,1–2,3 s). Isso é **hipótese** e será medido.
- **Desligável** por Function ou por mensagem, para quem quer texto exato
  (textos jurídicos, templates aprovados do WhatsApp).

## Parte B: o modo livre e as interrupções

**Modo livre** (`Agent.spec.freeMode`), configurado pelo autor:

- `instructions`: o que o agente sabe e como se comporta.
- `reads`: ações de leitura que ele pode chamar (horário da clínica,
  preços, FAQ, base de conhecimento).
- `runReads` (implementação de #31): ações de leitura cujos resultados
  já registrados nas fontes `fromAction` dos inputs da Function atual
  podem responder a uma dúvida paralela. Opt-in, mesma identidade e
  conversa, sempre mascarados; não executa novamente as leituras nem
  compartilha escopos arbitrários. Não inclui passos ou Functions
  chamadas. São snapshots, não uma verificação atual de disponibilidade.
- `functions`: as Functions que ele pode **propor iniciar**. Iniciar
  passa pelo roteador e pela autorização de sempre.
- `compute`: código em sandbox (Starlark hoje; outra linguagem se medirmos
  a necessidade), sem rede.
- O modo livre roda com o modelo da voz e registra tudo no journal, como
  qualquer run.

**Quando ele entra:**

- **Mensagem sem Function** (o "não entendi" de hoje): o modo livre
  responde.
- **Interrupção no meio de um processo:** quando um run espera uma
  resposta, a mensagem é classificada primeiro, por uma pergunta ao Jev,
  barata. As opções:
  - responde a pergunta pendente → segue como hoje;
  - é uma dúvida paralela → o modo livre responde e **repete a pergunta
    pendente**; um "sim, mas antes..." com pergunta ou condição não
    autoriza a ação. Falha na classificação ou na resposta não libera
    o processo, e classificação incerta repete a pergunta;
  - é um pedido novo → o modo livre responde ou redireciona e repete a
    pergunta pendente, sem iniciar a ação extra. Mesmo com um sim junto,
    exige nova confirmação após responder (#35, freemode-005). Dúvida e
    pedido novo compartilham handler e somam suas probabilidades para
    atingir 0,6; essa soma nunca inclui a categoria que autoriza seguir;
  - é cancelamento → encerra.
- **Confirmação tardia (#33):** com modo livre, o roteador recebe até
  oito falas recentes, mascaradas e da mesma identidade/conversa. Uma
  confirmação do trabalho já relatado como concluído é reconhecida sem
  reiniciar; um pedido explícito de nova alteração continua roteável.
- **Sem modo livre configurado:** a mensagem do Agent vira uma resposta
  humana, gerada pela voz: explica o que o agente faz e oferece a fila
  humana (HIL).

## Parte C: o agente nasce de um prompt

O autor escreve para o Claude Code algo como *"quero um agente que remarca
consultas no sistema de agenda, com confirmação, fala como a recepção da clínica,
avisa a equipe quando não consegue"*. O Claude Code entrega um agente
validado e testado. Para isso:

1. **Uma skill de autoria no repositório** (`.claude/skills/tessera-author`,
   também legível por Codex, Cursor e outros via `AGENTS.md`). O ciclo:
   1. entender o pedido;
   2. levantar as APIs;
   3. gerar plugins e Functions;
   4. escrever a EvalSuite;
   5. rodar;
   6. ler as falhas no inspetor;
   7. corrigir;
   8. repetir até os portões passarem.

   A skill conhece as armadilhas que os benchmarks ensinaram: confirmação
   que nomeia tudo, pergunta em vez de afirmação, listas estreitadas,
   pedido para outra pessoa.
2. **`tessera plugin import <openapi>`:** gera o manifest do plugin a
   partir da especificação da API. A classe de efeito das ações de escrita
   **não é adivinhada**: a ferramenta propõe e o Claude Code precisa
   confirmar com o autor. Errar essa classe tiraria a confirmação e a
   verificação de uma ação irreversível.
3. **Usuário simulado no `tessera test`:** cenários com objetivo em
   linguagem natural (`simulator: llm`, `goal: ...`), o mesmo simulador do
   benchmark. O autor, ou o Claude Code, escreve "o paciente quer remarcar
   a consulta da mãe" em vez de roteirizar cada fala.
4. **Portões obrigatórios antes de declarar pronto:**
   - `validate` sem erros e sem avisos não lidos;
   - EvalSuite verde;
   - os cenários de falha padrão (ação aplicada com timeout, mensagem
     repetida, queda) gerados automaticamente para toda ação
     irreversível.

   A segurança sai do processo, não da memória do autor.
5. **Métrica de esforço:** o tempo, o custo e as intervenções humanas
   para ir do prompt ao agente verde passam a ser medidos. A Parte C só
   está pronta quando esses números caírem.

## Como vamos medir (antes de construir)

Três extensões do benchmark de mercado, rodadas **primeiro no Tessera de
hoje** (linha de base) e depois de cada parte:

| Extensão | O que mede | Como |
| --- | --- | --- |
| **Naturalidade às cegas** | Se a conversa do Tessera está no nível dos LLMs | Pares de conversas da mesma tarefa, sem identificar o competidor, julgados por um modelo maior e por amostra humana: qual soa mais natural, qual é mais claro, qual erra fato |
| **Pedidos fora do roteiro** | Humanização fora do escopo | Tarefas novas: dúvidas soltas, pergunta no meio do processo, pedido que nenhuma Function cobre, pedido perigoso fora do escopo ("cancela tudo") |
| **Autoria por prompt** | Se criar um agente deixou de dar trabalho | O Claude Code recebe só o briefing de uma tarefa nova e trabalha até o agente passar. Mede-se tempo, custo, intervenções e, depois, o desempenho do agente produzido no benchmark completo com falhas |

Os critérios de sucesso de cada parte são pré-registrados antes da rodada,
como nas anteriores:

- **Parte A:** naturalidade às cegas empatada ou melhor contra o melhor
  LLM, **zero fatos errados** e as garantias de C1/C3 intactas.
- **Parte B:** nas tarefas fora do roteiro, nenhum "não entendi" em pedido
  que dava para responder, retorno correto à pergunta pendente e **zero
  efeitos fora de Function**.
- **Parte C:** um agente novo do prompt ao verde, sem o autor editar YAML,
  com desempenho no benchmark igual ao de um agente escrito à mão.

## Ordem proposta

1. **Linha de base:** as três extensões no Tessera de hoje.
2. **Parte A (voz):** mais visível para o usuário final, e a que mais
   afeta a percepção de "chatbot de 2015".
3. **Parte B (modo livre e interrupções):** usa a voz.
4. **Parte C (autoria):** a skill precisa conhecer A e B, e o simulador
   por objetivo e o `plugin import` já ajudam antes disso.

## Fora do escopo

- Modo livre com efeitos diretos: nunca, pela regra acima.
- Trocar o Jev por um LLM nas decisões: os benchmarks sustentam o Jev
  para decisões tipadas.
- Um editor visual: o caminho de autoria é o Claude Code e outros agentes
  de código, via API e CLI, como pedido no #20.

## Perguntas abertas para o dono

1. **Modelo da voz:** DeepSeek V4.1 Flash (o mesmo, barato) ou um maior se
   a naturalidade às cegas pedir? A proposta é começar pelo pequeno e
   subir só se a medição mostrar necessidade.
2. **Julgamento humano da naturalidade:** alguém da equipe avalia uma
   amostra (umas 50 comparações), ou ficamos só com o juiz automático?
3. **Tarefa da autoria:** a primeira tarefa nova para o Claude Code
   construir deve ser um processo real de empresa ou sintética?
