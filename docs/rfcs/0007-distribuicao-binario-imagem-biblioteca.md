# RFC-0007: distribuição: binário, imagem Docker e biblioteca

- **Estado:** aprovada pelo dono em 2026-09-25, com as decisões no fim 
- **Data:** 2026-09-25

## O problema

O dono quer construir um agente **em cima** do Tessera sem mexer no
repositório do Tessera, de duas formas:
- **como produto pronto:** um binário ou uma imagem Docker, onde o agente
  é uma pasta de definições;
- **como biblioteca:** um serviço Go que embute o motor e registra as
  próprias ferramentas em código.

Hoje, medido no repositório em 2026-09-25:
- **Não há imagem Docker nem release.** O binário `tessera` só existe
  compilando o código-fonte (`make build`); o CI não publica nada.
- **Nada é importável.** Todo o código Go está em `internal/`, que o Go
  proíbe importar de fora do módulo. Um serviço externo não pode usar o
  motor como biblioteca.
- **Ferramentas próprias** hoje são:
  - plugins declarativos (`http` para APIs REST, `sql` com banco próprio);
  - Starlark em sandbox;
  - handlers Go em processo (`host.Options.Handlers`), só de dentro deste
    repositório.

  Uma ferramenta em Python ou TypeScript, ou código Go de outro
  repositório, não tem caminho.
- **A superfície já existe por dentro.** `host.Options` recebe store
  (journal), Decide, Generate, segredos, handlers de ações, cliente HTTP e
  um gancho de observação. `tessera serve` já expõe a API HTTP completa
  (`/v1/openapi.json`), o console, webhooks e o agendador.

## O que não muda

- **As garantias vêm da plataforma, em qualquer forma de uso.** Um handler
  em Go, um plugin HTTP ou um processo externo passam pelo mesmo caminho:
  - classe de efeito;
  - confirmação;
  - chave de idempotência;
  - journal;
  - verificação.

  Quem embute a biblioteca não ganha um atalho que pule o runtime.
- **A definição é o contrato:** as mesmas pastas de YAML, a mesma
  validação (`tessera validate`) e os mesmos testes (`tessera test`), nas
  três formas.

## Proposta

### 1. O binário e a imagem: "o agente é uma pasta"

- **Releases versionados** (`goreleaser` no CI):
  - binários para linux e darwin, amd64 e arm64;
  - uma imagem `ghcr.io/bbiangul/tessera:<versão>`, distroless e sem
    root, com `/data` como volume do banco.
- **Um projeto de agente é um repositório do cliente:**

  ```
  meu-agente/
    agent.yaml  functions/  plugins/  evals/  scripts/
    AGENTS.md  .claude/skills/tessera-author/   # gerados por tessera init
    Dockerfile
  ```

  ```dockerfile
  FROM ghcr.io/bbiangul/tessera:0.9
  COPY . /agent
  CMD ["serve", "/agent", "--namespace", "clinica", "--db", "/data/runs.db"]
  ```

  Os segredos vêm por variável de ambiente (`TESSERA_SECRET_*`, como
  hoje).
- **`tessera init <pasta>`** cria o esqueleto: um Agent, uma Function de
  exemplo, uma EvalSuite, o `AGENTS.md` e a skill de autoria. O "agente de
  um prompt" (RFC-0004, parte C) começa daqui, fora do repositório do
  Tessera. O kit de autoria de fonte fechada dos authoring-003/004 é o
  protótipo disso.
- **`tessera validate` e `tessera test` no CI do cliente,** com cassetes:
  offline e sem custo, como no nosso CI.

### 2. Ferramentas em qualquer linguagem: o plugin por processo

Um terceiro tipo de plugin, além de `http` e `sql`: **`process`**, um
servidor do cliente (Python, TypeScript, Go) que implementa ações por um
protocolo simples. A proposta é HTTP com JSON: `POST /actions/{nome}`
com os argumentos e o cabeçalho de idempotência, e um `GET /verify` para a
sonda.

- O manifesto declara as ações, os tipos e as **classes de efeito**, como
  qualquer plugin. A admissão valida e o runtime aplica as garantias.
- **Kits pequenos** em Python e TypeScript: um decorator por ação, a
  verificação da assinatura do Tessera, e testes com o sandbox.
- É o mesmo contrato que os canais (RFC-0006: `ApprovalSurface`,
  `IdentityProvider`) podem usar quando forem de terceiros.

### 3. A biblioteca Go: uma fachada pública e estável

Um pacote público (`github.com/bbiangul/tessera/tessera`), fino, sobre o
que hoje é `internal/host`, com semver próprio. `internal/` continua
interno.

```go
eng, err := tessera.Open(tessera.Config{
    Definitions: os.DirFS("agent"),          // ou embed.FS
    Namespace:   "clinica",
    Store:       tessera.SQLite("runs.db"),   // Postgres depois
    Decide:      tessera.Jev(os.Getenv("TYPESAFE_API_KEY")),
    Generate:    tessera.OpenRouter(os.Getenv("OPENROUTER_API_KEY"), "deepseek/deepseek-v4.1-flash"),
})
eng.Handle("clinic/reschedule", func(ctx context.Context, call tessera.Call) (any, error) { … }) // efeito com as garantias do runtime
reply, err := eng.Message(ctx, conversation, caller, text)  // e Ack, Reply (humano), Tick (agendador)
http.Handle("/tessera/", eng.HTTPHandler())                  // a mesma API e console do serve, montados no seu servidor
```

- **Os testes do cliente** rodam as EvalSuites dele com `go test`
  (`tessera.RunEvals(t, "agent")`), com cassetes.
- **Scripts isolados:** o `serve` roda cada chamada Starlark num processo
  filho (o próprio binário), para que um script que aloca demais mate só
  o filho (starmem-001, #6). Quem embute a biblioteca chama
  `tessera.RunWorkerIfRequested()` no começo do `main` para ter o mesmo
  isolamento. Sem isso, os scripts rodam no processo, só com o watchdog.
- **O que fica fora da fachada:** o runtime, a admissão, o journal e os
  engines por dentro. Mudam sem quebrar o cliente.

### 4. Outras linguagens sem embutir: a API HTTP

Quem não usa Go roda o binário (ou a imagem) e fala com ele pela API
HTTP, que já existe. **Clientes gerados** do `/v1/openapi.json` para
Python e TypeScript: mensagens, outbox com ack, respostas humanas e
leitura dos runs.

## Estabilidade e versões

- **A API das definições** (`agents.tessera.dev/v1alpha1`) passa por
  `v1beta1` e depois `v1`, com conversão automática entre versões e
  `tessera migrate` para reescrever as pastas.
- **A fachada Go e a API HTTP** seguem semver. Um teste de compatibilidade
  roda as definições e as EvalSuites de cada versão publicada contra a
  seguinte.
- **O journal migra sozinho.** Um banco de uma versão abre na versão
  seguinte; isso já é testado nas migrações.

## Como medir antes de construir

**A medida é o tempo e o atrito do primeiro agente fora do repositório,**
do jeito dos authoring-003/004:
- um autor novo, sem o código-fonte, com só a imagem (ou só o módulo Go) e
  a documentação, recebe um brief;
- mede-se o tempo até um agente rodando, validado e testado, as
  intervenções, e o que ele não conseguiu fazer;
- a linha de base é o kit de fonte fechada de hoje.

**Hipótese:** a imagem com `tessera init` reduz o tempo e zera as
intervenções. Os problemas que os autores encontrarem viram a lista do
que falta na fachada.

## Ordem proposta

1. Releases e a imagem Docker, com `tessera init`. É o menor passo e
   destrava o uso "o agente é uma pasta".
2. O plugin `process`, com os kits Python e TypeScript. Destrava
   ferramentas próprias sem Go.
3. A fachada Go pública, com `RunEvals`. Destrava embutir num serviço.
4. Clientes HTTP gerados.
5. A API das definições em `v1beta1`, com `tessera migrate`.

## Decisões do dono (2026-09-25)

1. **A imagem:** publicada no GHCR, junto com os binários.
2. **O primeiro caso real** usa o **binário** (e a imagem); o cliente
   é indiferente. A ordem fica: o binário e a imagem antes da
   biblioteca.

   **Passo 1 feito em 2026-09-25:**
   - `tessera init`;
   - a imagem distroless sem root (~10 MB);
   - o `.goreleaser.yaml` e o workflow de release.

   O release de teste (`--snapshot`) construiu os binários e as imagens
   amd64 e arm64. Um agente criado pelo `init` rodou no container: anotou
   um recado no plugin sql em `/data` e o leu de volta, ao vivo.

   Publicar é uma tag `v*`.
3. **Postgres:** depois, na fase de cluster. Uma réplica com SQLite
   atende os casos atuais.
