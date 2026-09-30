# AGENTS.md

Instructions for coding agents (Claude Code, Codex, Cursor) and for people
working in this repository. Keep this file short and true; if it is wrong, fix
it in the same change.

**Picking up the work?** The design is in `docs/rfcs/` and `docs/adr/`; the evidence is in `docs/research/`; the guides are in `docs/guides/`.

## What this project is

Tessera is a declarative platform for AI agents that act on behalf of
companies. The design is in `docs/rfcs/0001-plataforma-declarativa-de-agentes.md`
(RFC-0001). Architecture decisions are in `docs/adr/`. Read the RFC section
for the area you touch before changing behavior.

## Commands

| Task | Command |
| --- | --- |
| Build | `make build` |
| Unit tests (race detector on) | `make test` |
| Integration tests (real providers, needs keys) | `make test-integration` |
| Lint and file-size check | `make lint` |
| Static analysis (staticcheck, errcheck, govulncheck; pinned) | `make lint-extra` |
| Everything CI runs | `make ci` |
| Validate resources | `./bin/tessera validate <files-or-dirs>` (`-o json` for machines) |
| Document any field | `./bin/tessera explain Function.spec.inputs` |
| Regenerate JSON Schemas after changing `api/` | `make schema` |
| Decide benchmark | `./bin/tessera bench decide --help` |
| Extraction benchmark (Generate) | `./bin/tessera bench extract --help` |
| Routing benchmark (a namespace's router) | `./bin/tessera bench route --help` |
| Conversation tests (EvalSuites) | `./bin/tessera test <dir>` live, `make test-evals` replayed |
| A/B test verdict from a runs database | `./bin/tessera metrics experiment <dir> --namespace <ns> --experiment <name> --db <runs.db>` |
| Business funnel from a runs database | `./bin/tessera metrics funnel <dir> --namespace <ns> --funnel <name> --db <runs.db>` |
| Run an agent as a service (HTTP API, webhooks, scheduler) | `TESSERA_API_TOKEN=... ./bin/tessera serve <dir> --namespace <ns> --db runs.db` |
| Model providers | `--generate openrouter:<model>` pins a measured model to its measured provider (`deepseek/deepseek-v4.1-flash` → DeepSeek, `internal/engines/generate/providers.go`); `@any` lets OpenRouter choose, `@X,Y` pins explicitly (provider-001) |
| Talk to an agent | `./bin/tessera chat <dir> --namespace <ns>` (budgeted; `/human` answers human tasks) |

## Building an agent from a request (for coding agents)

Follow `.claude/skills/tessera-author/SKILL.md`: the loop (understand → map the systems → Functions → Agent → EvalSuite → gates), the rules the benchmarks taught, and the gates that must pass before an agent is called done. It works for any coding agent, not only Claude Code.

## Testing and troubleshooting an agent (for coding agents)

Two ways, the same information:

- **A running agent:** `TESSERA_API_TOKEN=$(openssl rand -hex 32) ./bin/tessera serve <dir> --namespace <ns> --db runs.db --dev`.
  Read `GET /v1/openapi.json` first. The loop: `GET /v1/agent` (what it can do),
  `POST /v1/messages` (talk as a user), `GET /v1/runs?conversation=...`, `GET /v1/runs/{id}`
  (the timeline: the router's choice with probabilities, each read, each Jev decision with its
  question and options, questions and replies, the intent before each effect, results,
  failures, milestones, messages). Test time without waiting: `POST /v1/triggers/{name}/fire`,
  `POST /v1/dev/clock {"advance":"48h"}` (only with `--dev`). People's tasks:
  `POST /v1/runs/{id}/reply {"from":"human", ...}`. Watch everything as it happens:
  `GET /v1/activity?after=<next>` (poll with the cursor it returns) or
  `curl -N .../v1/activity/stream` (Server-Sent Events): every record of every run, explained,
  in the order written. The web console at `/` is a client of the same API.
- **A runs database, no server:** `./bin/tessera runs list --db runs.db [--status Failed]` and
  `./bin/tessera runs show <id> <dir> --db runs.db [-o json]`, and
  `./bin/tessera runs follow <dir> --namespace <ns> --db runs.db [--once] [-o json]` to watch
  records as a running `serve` or `chat` writes them.

To reproduce a problem, write it as an EvalSuite scenario (`docs/guides/testing.md`) and run
`./bin/tessera test <dir>`, or `POST /v1/evals/run {"only": "suite/scenario"}` on a running server
(it re-reads the files: edit, re-run, no restart): a fix is done when the scenario passes live
and replayed.

## Rules

- **Keep the backlog true.** The backlog is the GitHub issues
  (labels `phase-2`, `finding`, `needs-owner`, `backlog`; milestone per
  phase). A change that finishes, advances or finds work closes, comments
  on or opens the issue in the same change.
- **Measure before believing.** A claim about models, latency, cost or
  behavior needs a benchmark or a test in the repo. Record results under
  `bench/results/` and write findings in `docs/research/`.
- **Decisions use Jev** (ADR-0003). Do not add LLM-based decision paths;
  `llm-constrained` exists only for measurement and fallback.
- **Start with the smallest model** for LLM text generation: evaluate
  `deepseek/deepseek-v4.1-flash` first; move to larger models only when the
  measurements show the small one fails, and record where it fails.
- **Every package has unit tests.** Network code is tested with
  `httptest` fakes. Tests run with `-race`.
- **Integration tests** that call real providers use the build tag
  `integration` and skip cleanly when the API key is missing.
- **No file over 800 lines** (checked by `scripts/check-file-size.sh`).
- **No implicit state in `context.Context`** beyond cancellation and tracing.
  Pass dependencies explicitly.
- **Check every error you can act on.** errcheck runs in CI; its only
  exclusions (`.errcheck-excludes`) are writes to the CLI's output and closing
  files that were only read. Closing a file you wrote is checked.
- **Fail loudly.** Unknown fields, unresolved references and malformed model
  output are errors, never silently ignored or patched.
- **Model calls in tests are either replayed or budgeted** (RFC-0002). Use
  `decidetest.Scripted` in unit tests, cassettes for replay, and
  `decide.Metered` with a ledger budget for anything live.
- **Expressions follow `internal/expr`**: never build CEL, SQL or text by
  string concatenation of values; use `CompileValue` or `CompileTemplate`.
- **API types are documentation.** Doc comments in `api/` become the JSON
  Schema descriptions and `tessera explain` output; write them for users,
  then run `make schema`.
- **Every file under `examples/` must pass `tessera validate`** (checked in
  CI). New validation rules come with a test in `internal/admission` that
  plants the defect in the example and asserts the code and path.
- **Never hard-code the project name.** Use `internal/project`.
- **Secrets** come from the environment; never commit keys, and never print
  them in logs or test output.
- Keep comments at the density of the surrounding code; exported identifiers
  have doc comments.

## Layout

```
cmd/tessera        CLI
api/agents/v1alpha1    agent resource types; doc comments ARE the reference docs
api/plugins/v1alpha1   plugin manifest type
api/schemas        generated JSON Schemas (embedded); never edit by hand
internal/admission load + schema-validate + semantic checks (tessera validate)
internal/explain   field documentation from the schemas (tessera explain)
internal/project   project naming (provisional name lives here only)
internal/httpx     JSON HTTP client with bounded retries
internal/engines/decide   Decide engine: contract + typesafe + llm-constrained
internal/engines/decide/decidetest   scripted Decider for unit tests
internal/engines/act      Act engine contract: plugin calls, definite vs ambiguous failures
internal/engines/compute  Compute engine: sandboxed, deterministic Starlark (CEL is internal/expr)
internal/runtime   executes Functions over a journal (RFC-0003): asks, confirm, effects, onError, wait/timers
internal/runtime/journal  run store, records, typed codec; Memory store for tests
internal/mask      masks personal data before anything reaches a model
internal/engines/generate  Generate engine: text or schema-validated objects (OpenRouter)
internal/plugins/http      built-in http plugin: manifest-declared REST actions, secrets by name
internal/plugins/sql       built-in sql plugin: the agent's own database, literal SQL + named params
internal/host      loads resources, wires engines and plugins, routes messages to runs, Tick, webhooks
internal/server    HTTP API of tessera serve: messages, outbox, ack, replies, signed webhooks
internal/channels  channel adapters for tessera serve (whatsapp: Meta webhook in, outbox out)
internal/inspect   runs explained record by record, conversation transcripts
internal/evals     runs EvalSuites: sandboxes, scripted turns, invariants, budgets
internal/expr      CEL expressions, ${ } values, {{ }} templates (RFC-0001 rules)
internal/ledger    cost ledger, budgets, kill switch
internal/metrics   business numbers from milestones: funnels by cohort
internal/cron      five-field cron expressions for schedule Triggers
internal/bench     benchmark harness: datasets, runner, metrics, calibration, gate
internal/cassette  record/replay of model calls (shared by Decide and Generate)
internal/tools     genschema (make schema), timerbench (ADR-0005 measurements)
bench/datasets     labelled datasets (JSONL); synthetic, no personal data
bench/results      benchmark outputs, committed as evidence
bench/baselines    the run CI gates against, per pinned model version
bench/cassettes    recorded model calls for free, offline replay tests
docs/rfcs docs/adr docs/research docs/guides
spikes/            throwaway code behind a decision, each its own Go module
```
