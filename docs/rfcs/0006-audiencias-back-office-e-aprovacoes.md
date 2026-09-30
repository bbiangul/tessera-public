# RFC-0006: audiências, back office e aprovações por canal plugável

- **Estado:** aprovada pelo dono em 2026-09-25, com as decisões no fim. Sem quórum por ora; identidade da equipe e canais de aprovação plugáveis, com opções prontas.
- **Data:** 2026-09-25
- **Contexto:**
  - o modelo de front desk e back office comum em agentes de atendimento;
  - RFC-0001, seções "Humano no circuito por padrão" e "Autorização não é
    probabilística";
  - issue #13 (filas com SLA e aprovação autenticada).

## O problema

Um agente de atendimento tem dois lados:
- **O front desk:** o cliente final (o paciente).
- **O back office:** a equipe (a atendente, o dentista, o dono da clínica).

Cada papel via contextos e ferramentas diferentes, e escalava para o
outro lado de formas diferentes. Dentro do back office havia níveis:
a atendente sobe para a gerência, que sobe para o dono.

O Tessera tem parte disso:
- **Identidade de quem fala:** o canal entrega o `caller`, com `role`,
  `patientId` etc.
- **`authorize` por Function e por ação:** uma expressão CEL sobre o
  `caller`. É o que impede um paciente de mexer na consulta de outro.
- **Filas humanas (`HumanQueue`):**
  - SLA com lembrete;
  - escalonamento para outra fila (`escalateTo`);
  - timeout com ação padrão;
  - horário comercial;
  - aviso à equipe por qualquer plugin.
- **Tarefas humanas:** aprovar, responder, preencher um dado, assumir a
  conversa.
- **Caminhos até o humano:** `escalate` numa Function, `onError` que escala
  um efeito incerto, e o repasse do modo livre (`freeMode.functions`).

O que falta, medido no código em 2026-09-25:

1. **Uma persona para todos.**
   - A admissão aceita **um Agent por namespace**, então há uma voz, um
     modo livre e um conjunto de mensagens para o paciente e para a equipe.
   - Hoje, ter "recepção" e "assistente da equipe" exige dois namespaces,
     que não compartilham Functions nem contexto.
2. **O cardápio não depende do papel.**
   - O roteador oferece todas as Functions roteáveis, e o `authorize` só
     barra depois ("não posso fazer isso por você").
   - O modo livre lê as mesmas fontes para qualquer pessoa.
3. **As filas não têm membros.**
   - Não existe "a fila do dentista só é respondida por dentistas".
   - Quem responde uma tarefa pelo link assinado **digita o próprio nome**,
     sem verificação (`internal/server/links.go`). O RFC-0001 pedia que
     aprovar fosse um ato autenticado, com o papel conferido na fila; isso
     não foi feito (#13).
4. **O escalonamento entre níveis é só por tempo.** O `escalateTo` dispara
   quando o SLA estoura. Não há uma pessoa repassando ao nível de cima, nem
   uma regra do tipo "desconto acima de 50% vai direto ao dono".
5. **A equipe não conversa com o agente como usuária.** Um dentista
   perguntando "como está minha agenda amanhã?" ou pedindo "bloqueia minha
   sexta à tarde" não tem ferramentas próprias, contexto próprio nem
   garantias próprias.
6. **Aprovar pelo canal que a pessoa já usa não existe.**
   - A aprovação acontece no console ou no link.
   - Um botão no Slack ou no WhatsApp da gerente, com a identidade do
     canal verificada, não está implementado.
   - Cada canal novo teria que ser feito à mão.

## A regra que não muda

> **Quem pode fazer o quê vem de identidade verificada e de regras
> declaradas, nunca de um modelo.**

- O roteador e o modo livre podem **escolher** entre o que o papel pode
  fazer; nunca **conceder** permissão.
- **Uma aprovação é um ato autenticado:**
  - de alguém cuja identidade o canal atesta e que tem o papel exigido;
  - presa ao ID da tarefa e ao hash do que vai ser executado. Se o conteúdo
    mudar, a aprovação perde a validade.
- Texto livre ("pode seguir" num grupo) nunca aprova: vira, no máximo, uma
  sugestão que ainda pede o ato explícito.

## Proposta

### 1. Audiências dentro do Agent

Um Agent passa a ter **audiências**. Cada uma diz quem é (uma condição
sobre a identidade verificada) e o que essa pessoa vê:

```yaml
kind: Agent
metadata: { name: clinica, namespace: clinica }
spec:
  audiences:
    - name: paciente                     # front desk
      who: "caller.role == 'patient'"
      voice: { persona: "Recepcionista acolhedora…" }
      functions: [remarcar-consulta]     # o cardápio do roteador para este papel
      freeMode:
        instructions: "Dúvidas da clínica…"
        reads: [ { act: clinic/info, public: true } ]
        functions: [ { function: falar-com-recepcao, when: "…" } ]
    - name: recepcao                     # back office
      who: "'recepcao' in caller.roles"
      voice: { persona: "Assistente da equipe, direto e objetivo." }
      functions: [agenda-do-dia, encaixar-paciente, aprovar-excecao]
      freeMode: { reads: [ { act: clinic/agenda-do-dia } ] }
    - name: dentista
      who: "'dentista' in caller.roles"
      functions: [minha-agenda, bloquear-horario]
      freeMode: { reads: [ { act: clinic/minha-agenda, with: { dentistId: "${ caller.dentistId }" } } ] }
    - name: dono
      who: "'dono' in caller.roles"
      functions: [relatorio, aprovar-desconto]
```

- **O roteador só oferece as Functions da audiência.** Quem não vê não
  pede, e quem não pode não recebe "não posso" depois de escolhido.
- O `authorize` de cada Function continua valendo, como segunda barreira.
- A voz, o modo livre, as mensagens e o nível de autonomia (`draft`,
  `approve-actions`, `autonomous`, do RFC-0001) passam a ser por audiência.
- **O que cada audiência vê do contexto de outra é declarado.** Por
  exemplo, a recepção vê a conversa do paciente mascarada; o dentista vê o
  histórico clínico, a recepção não. Tudo fica no journal, como auditoria.

### 2. Identidade e equipe

**Tudo aqui é plugável, com opções prontas (built-in).** A plataforma
define dois contratos, e um provedor novo é um plugin que implementa um
deles:
- **`IdentityProvider`:** transforma o que o canal atesta (um telefone, um
  user ID, um login) em uma pessoa com papéis e atributos.
- **`ApprovalSurface`:** apresenta uma tarefa, recebe a resposta e atesta
  quem respondeu (seção 4).

Os provedores de identidade prontos:

| Provedor | O que atesta |
| --- | --- |
| `team` | o recurso `Team` (abaixo): pessoas, papéis e contatos declarados no repositório |
| `oidc` | login no console por SSO/OIDC (Google, Microsoft, Okta, Keycloak) |
| `whatsapp` | o número verificado pela Meta no webhook assinado |
| `slack` | o user ID do Slack, no request assinado |
| `email-otp` | um e-mail confirmado por código de uso único |
| `directory` (plugin) | qualquer diretório externo por uma ação de plugin: a API de profissionais da clínica, o Google Workspace, um banco |

Uma identidade verificada chega de três jeitos:
- **pelo canal:** o número do WhatsApp, o user ID do Slack, o e-mail
  confirmado;
- **pelo console:** login com SSO ou OIDC;
- **por um diretório:** uma ação de plugin que mapeia a identidade do
  canal para uma pessoa e seus papéis, por exemplo a API de profissionais
  da clínica.

Um recurso `Team` declara pessoas e papéis quando não há diretório
externo:

```yaml
kind: Team
metadata: { name: equipe, namespace: clinica }
spec:
  directory: { act: clinic/profissionais }    # opcional: a fonte de verdade externa
  members:
    - { id: ana,   roles: [recepcao],        contacts: { whatsapp: "+55…", slack: U123 } }
    - { id: bruno, roles: [dentista],        attributes: { dentistId: D2 } }
    - { id: carla, roles: [recepcao, gerente] }
    - { id: dono,  roles: [dono] }
  delegation: true                             # ausências: quem cobre quem, com prazo
```

O `caller` que chega às Functions passa a ter `roles` e `attributes`
verificados. Um papel nunca é inferido de texto.

### 3. Filas com membros, níveis e regras

```yaml
kind: HumanQueue
metadata: { name: excecoes, namespace: clinica }
spec:
  answeredBy: [recepcao]                  # só quem tem o papel responde
  levels:                                 # a escada do back office
    - { name: recepcao, roles: [recepcao], sla: { remind: 30m, escalate: 2h } }
    - { name: gerencia, roles: [gerente],  sla: { escalate: 4h } }
    - { name: dono,     roles: [dono],     sla: { timeout: 1d, onTimeout: { answer: false } } }
  route:                                  # regras: em que nível a tarefa começa
    - { when: "task.inputs.desconto > 0.5", level: dono }
    - { when: "task.inputs.desconto > 0.2", level: gerencia }
  forward: true                           # um membro pode subir de nível, com uma nota
  channels: [console, slack, whatsapp]    # por onde a fila recebe e responde (seção 4)
```

Uma tarefa sobe de nível de três jeitos, e todos ficam no journal:
- **por tempo:** o SLA de hoje;
- **por regra:** `route`, avaliada em CEL sobre a tarefa;
- **por uma pessoa:** "encaminhar para a gerência", com uma nota.

### 4. Aprovações por canal plugável

Hoje uma tarefa só é respondida no console ou num link assinado, sem
identidade verificada. A proposta é um **contrato de superfície de
tarefas** que qualquer plugin de canal pode implementar, como hoje
implementa mensagens:

| O canal faz | A plataforma faz |
| --- | --- |
| **Apresentar** a tarefa: texto, opções, botões (Slack interativo, WhatsApp com botões ou template fora da janela de 24h, e-mail com link) | Monta o conteúdo, o ID da tarefa, o hash do que será executado e a validade |
| **Receber** a resposta: o clique, o botão, a resposta no fio | Confere a assinatura do canal (a verificação do webhook do Slack ou da Meta) |
| **Atestar a identidade** de quem respondeu: user ID, telefone | Mapeia para um membro do `Team`, confere o papel exigido no nível atual, e que a tarefa ainda está aberta e com o mesmo hash |
| — | Registra quem, quando, por qual canal e com qual hash, e continua o run |

Formas de aprovação declaráveis:
- **Uma pessoa com o papel exigido no nível atual aprova.** Quórum ("dois
  gerentes") fica fora por ora, por decisão do dono (2026-09-25). O
  contrato não impede acrescentá-lo depois.
- **Quem pediu não aprova o próprio pedido:** separação de funções.
- **Validade:** a aprovação expira com a tarefa; um conteúdo diferente
  invalida.
- **Negar com motivo:** o motivo volta ao run, que decide o que dizer ao
  paciente.

**As superfícies de aprovação prontas (built-in),** em ordem de
implementação:
1. console, com login (`oidc` ou `team`);
2. WhatsApp: botões, e template aprovado fora da janela de 24h;
3. Slack: botões interativos, request assinado;
4. Microsoft Teams: Adaptive Cards com botões, pelo Bot Framework, com o
   request assinado;
5. e-mail, **Outlook** (Microsoft Graph) e **Gmail** (Gmail API): link de
   uso único mais código. A resposta ao e-mail só vale com o link, porque
   o remetente de um e-mail se falsifica;
6. webhook: o sistema do cliente apresenta e devolve a resposta assinada.

Qualquer outro canal (Telegram, um app próprio) é um plugin que
implementa `ApprovalSurface` com as mesmas garantias: a plataforma, não o
plugin, confere o papel, o hash e se a tarefa está aberta.

O link assinado de hoje passa a exigir identidade (código por e-mail ou
SSO). Se aceitar nome digitado, é só para `answer` e `input`, que não
autorizam efeito.

**Em grupos**, uma mensagem livre ("pode seguir") não aprova. O Decide
pode ler a intenção e o canal responde com o botão para o ato explícito.

### 5. O back office conversando com o agente

A mesma plataforma serve a equipe no canal dela, com as mesmas garantias:
- **o dentista** pergunta "minha agenda amanhã" (uma leitura da
  audiência) e pede "bloqueia sexta à tarde", uma Function com
  confirmação, idempotência e journal;
- **a recepção** vê as tarefas pendentes na própria conversa ("você tem 2
  exceções para aprovar") e responde ali mesmo, pelo contrato da seção 4;
- **o dono** pede um relatório.

Os efeitos continuam só em Functions: o back office conversa com as
mesmas regras do front desk.

## Como medir antes de construir

**Um benchmark de vários papéis (`backoffice-000`),** sintético, na
clínica, com cenários como:
- o paciente pede uma exceção (encaixe, desconto) → a tarefa vai para a
  recepção → aprovação pelo canal → o efeito acontece uma vez;
- a regra manda o desconto de 60% direto ao dono;
- alguém sem o papel tenta aprovar: pelo link, pelo Slack, "pode seguir"
  num grupo;
- o conteúdo muda depois da aprovação: a aprovação não pode valer;
- o SLA estoura e sobe de nível; e o encaminhamento manual;
- o paciente tenta usar uma Function da equipe, e o dentista tenta ler a
  agenda de outro dentista;
- o dentista conversa com o agente: leitura da agenda, bloqueio com
  confirmação.

**Medidas:**
- efeitos não autorizados, que precisam ser 0;
- aprovações aceitas de quem não podia, também 0;
- aprovações velhas aceitas depois de o conteúdo mudar, também 0;
- Functions expostas à audiência errada;
- a tarefa chegando ao nível certo;
- o tempo até a aprovação;
- a qualidade da conversa enquanto espera, com o juiz de sempre;
- o custo.

**A linha de base:**
- o Tessera de hoje, com os contornos atuais (dois namespaces,
  `caller.role`, filas sem membros);
- um loop de LLM e o LangGraph com `interrupt()` (a pausa para humano da
  documentação dele), com as mesmas ferramentas e a checagem de papel que
  a documentação de cada um recomenda.

## Ordem proposta

1. O benchmark e a linha de base.
2. Identidade e equipe (`Team`, diretório plugável), filas com membros e
   aprovação autenticada no console e no link. Isso fecha o #13.
3. O contrato de superfície de tarefas: primeiro o WhatsApp, depois
   Slack, Teams e e-mail (Outlook e Gmail).
4. Níveis, regras (`route`) e encaminhamento manual.
5. Audiências no Agent: voz, modo livre, cardápio e contexto por papel.
6. O back office conversacional.

## Fora do escopo

- **Papéis inferidos por modelo:** nunca.
- **Um editor visual de fluxos de aprovação:** a autoria continua pelo
  Claude Code e pela skill (RFC-0004, parte C).

## Andamento

- **2026-09-26: `backoffice-000`** (a linha de base) e **`backoffice-001`**.
  - Construídos: `Team`, `roles` nas filas, página de tarefa autenticada
    por código de uso único, `Reply.By` conferido, encaminhamento,
    `escalate.route` e `selector.who`.
  - Todas as medidas de segurança do benchmark ficaram em 0.
  - **Faltam:**
    - as superfícies de aprovação por canal (WhatsApp primeiro);
    - as audiências completas (voz e modo livre por papel);
    - o back office conversacional;
    - a linha de base com LLM.

- **2026-09-26: as superfícies de aprovação (passos 3 e 4).**
  - Construídas: WhatsApp (botões e template fora da janela), Slack,
    Teams e e-mail (Outlook e Gmail), com o papel e o hash conferidos
    pela plataforma.
  - Medidas: backoffice-002 a -005, e uma revisão adversarial com 21
    achados, corrigidos.
  - Correção de #53: uma recusa encerra a Function.
  - Publicado na v0.2.0.
- **2026-09-26: audiências (passo 5), `audiences-001`.**
  - Construídos: `Agent.spec.audiences` (cardápio, voz, modo livre e
    mensagens por papel) e a identidade vinda do `Team` pelo número que o
    WhatsApp atesta (`caller.roles`, `caller.attributes`).
- **2026-09-26: o back office conversacional (passo 6), `backoffice-006`.**
  - Construídos:
    - a equipe conversa com o agente também no Slack (DM, Events API) e
      no Teams (chat pessoal), com a identidade do Team;
    - as respostas seguem por uma entrega única para os canais;
    - `freeMode.tasks`: o membro pergunta pelas próprias tarefas;
    - o relatório do dono é uma Function de leitura.

- **2026-09-26: a linha de base com LLM, `backoffice-baseline-001`.**
  - Os mesmos 7 cenários rodaram no Tessera, no LangGraph (com o
    middleware de aprovação) e num loop simples, com o mesmo modelo.
  - Quando todos se comportam, as três stacks resolvem.
  - Quando alguém sem o papel aprova, uma aprovação chega duas vezes ou
    o pedido é do nível do dono, as stacks de LLM aceitam em toda
    execução, e o Tessera recusa em toda execução.

- **2026-09-26: visibilidade de contexto e níveis de autonomia, `audiences-002`.**
  - `audiences[].sees` declara o que um papel vê da conversa por trás de
    uma tarefa, mascarada ou completa, e cada visualização fica no
    journal.
  - `autonomy` (`autonomous`, `approve-actions`, `draft`) vale por Agent e
    por audiência.
  - Com isso, todas as partes do RFC-0006 estão construídas e medidas.

## Decisões do dono (2026-09-25)

1. **Identidade da equipe:** os dois. O recurso `Team` no repositório vem
   primeiro, e um diretório externo (a API da clínica, o Google Workspace,
   o Microsoft Entra) pode ser ligado depois pela mesma interface.
2. **Canais de aprovação prontos:** WhatsApp, Slack, Teams e e-mail
   (Outlook e Gmail), além do console e do webhook, nessa ordem de
   implementação.
3. **"Quem pediu não aprova":** uma opção por Function, desligada por
   padrão. Não há quórum.
4. **O que o paciente vê:** o papel de quem aprovou ("a gerência
   aprovou"), nunca o nome.
5. **LGPD entre audiências:** cada cliente define o que o back office vê,
   e o padrão é restritivo: só o que a tarefa precisa.
