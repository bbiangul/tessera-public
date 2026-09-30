# RFC-0005: agentes que não envelhecem

- **Estado:** aprovada pelo dono em 2026-09-25, com as decisões no fim
- **Data:** 2026-09-24
- **Issue:** [#26](https://github.com/bbiangul/tessera/issues/26)

## O problema

Um agente bem escrito hoje envelhece. O traveler, por exemplo, sabe
buscar voos por categoria, com criança, com bagagem. Um dia a companhia
muda uma regra e ninguém atualiza as definições. O agente continua
seguindo o caminho antigo, com confiança.

Um loop de LLM com ferramentas (e com código) se adapta sozinho: lê a
resposta nova da API e improvisa. O Tessera estrito não. O dono quer que
o desenvolvedor possa deixar essa porta aberta, para o produto evoluir
junto com os modelos.

## A regra que não muda

> Nenhum efeito acontece fora das proteções da plataforma.

Toda a vantagem medida do Tessera vem de proteções que a plataforma
impõe, e não que o modelo lembra de seguir:
- 0 ações duplicadas contra 30–50%;
- confirmação que nomeia tudo;
- verificação depois de timeout.

Improvisar é permitido; improvisar sem essas proteções não.

E uma observação que muda o desenho: **o modelo também está
desatualizado.** A regra nova chega por uma fonte viva (a resposta da
API, a documentação, uma busca), não pelo modelo. Quem declara essas
fontes é o desenvolvedor.

## Quatro degraus, do mais barato ao mais poderoso

### 1. Perceber que o agente envelheceu

O journal já tem os sinais:
- mensagens parecidas que caem no modo livre ou no repasse a uma pessoa,
  em número crescente;
- Functions falhando mais, ou com erros novos;
- respostas de API com um formato que não bate com o que o plugin
  declara (um campo novo, um campo sumido);
- usuários corrigindo o agente, e confirmações recusadas.

**Saída:** um relatório de deriva para o desenvolvedor (`tessera drift`,
e no console). Por exemplo: "14 pessoas perguntaram sobre pet na cabine
esta semana e nenhuma Function cobre isso; 3 exemplos".

É o degrau mais barato e o de maior valor: o agente não improvisa nada,
mas o dono fica sabendo.

### 2. Improvisar leitura e cálculo

O modo livre pode:
- combinar as leituras declaradas numa ordem que nenhuma Function
  previu;
- escrever código em sandbox (Starlark, sem rede e sem relógio) em tempo
  de execução, para calcular o que a pergunta pede.

O código gerado fica no journal, então reexecutar dá o mesmo resultado.
Nada muda no mundo, por isso não há risco de efeito.

### 3. Improvisar ações, por uma Function escrita na hora

- O modelo escreve a definição de uma Function em tempo de execução.
- **A mesma admissão de sempre a valida:**
  - classes de efeito;
  - confirmação que nomeia tudo;
  - chave de idempotência da plataforma;
  - verificação.
- Ela roda no journal como qualquer run.
- Só pode usar as ações que o desenvolvedor liberou
  (`improvise.allowActs`), e pede aprovação conforme a política do
  desenvolvedor: do usuário, de uma pessoa, ou das duas.

É o poder do loop de LLM com as proteções do Tessera.

### 4. Promover o que funcionou

Uma improvisação que deu certo várias vezes vira uma **proposta de
Function**: um PR escrito pela skill de autoria (RFC-0004, parte C), com
EvalSuite. O desenvolvedor revisa em vez de reescrever, e o agente
evolui.

## Como medir antes de construir

Um benchmark de **deriva**, no harness de mercado: mudar regras no
servidor de ferramentas sem tocar nas definições. Por exemplo:
- uma taxa nova;
- um campo obrigatório novo numa ação;
- uma política nova ("remarcação com menos de 24 h só por telefone").

Comparar o Tessera estrito, o Tessera com cada degrau e os loops de LLM,
medindo:
- sucesso;
- ações erradas ou duplicadas;
- se o desenvolvedor foi avisado, e em quanto tempo;
- o custo.

As hipóteses são pré-registradas, como nas outras rodadas.

## Ordem proposta

1. O benchmark de deriva, com a linha de base (Tessera estrito e loops de
   LLM).
2. O degrau 1 (perceber). Mede-se o tempo até avisar e os falsos alarmes.
3. O degrau 2 (ler e calcular).
4. O degrau 3 (ações com Function na hora). Depende de a validação rodar
   em tempo de execução e da política de aprovação.
5. O degrau 4 (promover). Depende da skill de autoria estar madura.

## Decisões do dono (2026-09-25)

1. **O relatório de deriva** fica no console (`tessera drift`). Um resumo
   semanal sai por um canal plugável: Slack, e-mail, Teams ou WhatsApp,
   os mesmos canais do RFC-0006. Abrir issue no repositório do agente é
   opcional.
2. **O degrau 3** (ações por uma Function escrita na hora) vem
   **desligado por padrão**. Quando ligado, quem aprova é uma pessoa da
   equipe, pela fila e pelos canais do RFC-0006, nunca o próprio usuário.
3. **Busca na web entra como fonte, por plugins prontos (built-in)** que
   o desenvolvedor só liga, sem escrever binding.
   - **Provedores prontos:**
     - `tinyfish`, o padrão: busca e fetch gratuitos, sem cartão, 30
       buscas por minuto e 500 por hora;
     - `searxng`: auto-hospedado, sem custo de API;
     - `tavily` e `brave`: camadas gratuitas com chave.

     O contrato é o mesmo para todos: `web/search` (consulta → títulos,
     URLs, trechos) e `web/fetch` (URL → texto), classe de efeito `read`.
   - **Desligado por padrão**, ligado por agente:

     ```yaml
     freeMode:
       reads:
         - action: web/search
           public: true
     ```

     e o plugin, `plugins: [{ builtin: web, provider: tinyfish }]`.
   - **As garantias não mudam:**
     - O conteúdo da web é **dado, nunca instrução**. Ele entra como
       fato, marcado com a origem (`source: web`, a URL), e passa pelo
       mesmo guarda de resposta.
     - O agente **cita a origem** ("segundo o site da companhia, …") e
       não trata a web como a palavra da empresa.
     - Consultas **não levam dados pessoais**: a busca sai com os
       valores mascarados. Uma consulta com um nome, CPF ou telefone é
       recusada antes de sair.
     - O custo e as chamadas entram no ledger como as de qualquer
       plugin.
   - **Medido antes de virar recomendação:** o conjunto fora do roteiro
     do viajante e da clínica, com e sem web. A hipótese é que a web tire
     os becos sem saída de "dados atuais" (bagagem, visto, melhor época)
     sem aumentar as afirmações falsas. Também se mede quanto conteúdo
     hostil na página muda a resposta (injeção de prompt).
