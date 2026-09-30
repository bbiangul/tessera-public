# Tessera

> **Status: pre-alpha.** Nothing here is stable. The name is provisional
> ([ADR-0001](docs/adr/0001-provisional-name.md)).

Tessera is an open-source, declarative platform for AI agents that **act on
behalf of a company** — book appointments, send WhatsApp messages, update a
CRM — where a mistake has a cost.

Frameworks help you *write* an agent. Tessera makes sure an agent that acts in
the real world does it once, with permission, with a human when needed, and
that you can prove why it did it.

## Design in one screen

- **Declarative resources** (`Agent`, `Function`, `Workflow`, `Trigger`,
  `Decision`, `Guardrail`, `HumanQueue`, …) with `spec`/`status`, validated
  before anything runs.
- **Four engines, never mixed in one call:**
  - **Decide** — typed decisions with probabilities (choice, yes/no, score).
  - **Generate** — LLM text, validated by schema.
  - **Compute** — CEL expressions and Starlark, sandboxed and deterministic.
  - **Act** — plugins with declared effect classes and verification modes.
- **Honest effect guarantees**: idempotency where the external system allows
  it, verification by probe where it doesn't, and a human when nobody can know.
- **Human-in-the-loop by default**: queues, SLAs, authenticated approvals,
  takeover.
- **Durable, replayable execution**: every non-deterministic result is an event.

The full design is [RFC-0001](docs/rfcs/0001-plataforma-declarativa-de-agentes.md)
(Portuguese; an English version will follow).

## What exists today

| Piece | State |
| --- | --- |
| Runtime ([RFC-0003](docs/rfcs/0003-runtime-core.md)): Functions run over a journal; resume never repeats a model call or an effect; irreversible effects are confirmed, keyed, verified (probe/native) or handed to a person; transactional outbox; SQLite store | Working; crash-injection tested at every journal write |
| Inputs: extraction from the message (one model call for all fields), free-text and choice questions, plugin reads, other Functions, `onUnresolved` | Working |
| Steps: `compute` (CEL, Starlark), `act`, `call`, `decide`, `extract`, `generate`, `ask`, `respond`, `escalate`, `branch` | Working |
| Engines: Decide (Jev per [ADR-0003](docs/adr/0003-jev-is-the-decide-engine.md); `llm-constrained`), Generate (OpenRouter, schema-validated, small model first), Compute (sandboxed Starlark), Act (plugins) | Working, measured ([research](docs/research/)) |
| Built-in plugins: `http` (manifest-declared REST actions, secrets by reference, egress allowlist) and `sql` (the agent's own SQLite, migrations, parameterized statements) | Working |
| `Agent`: a namespace's own messages in its users' language, and its queue | Working |
| `tessera validate`, `explain`, `chat`, `test`, `bench decide/extract/route/calibrate/gate` | Working |
| EvalSuites (RFC-0002 level 4): whole conversations against Starlark sandboxes, invariants, budgets; replayed in CI | Working |
| Reference agent: the [traveler](examples/traveler), a whole travel agency: flights, follow-up questions, extras, hotels, profile, airport-change risk, currency; scripts proven against the original | **Phase 1 done.** Traveler EvalSuite: 24 conversations (including known bugs of an earlier implementation), 3 live runs green on Jev; on the fallback backend it passed 3/3 until DeepSeek drifted mid-day ([decide-followups-001](docs/research/decide-followups-001.md)). Onboarding questions wait for `Trigger`s (phase 2) |
| Routing: `selector.description`, follow-ups guarded by `selector.after`, threshold per namespace (`Agent.spec.routing`) | Measured on 6 datasets ([router-002](docs/research/router-002.md)) |
| **Phase 2, in progress.** Business metrics: `Subject`, `milestone` on steps and actions (recorded once, when the effect is confirmed), `Funnel` by cohort with time per stage and stuck subjects (`onStuck` starts a Function once per stuck subject, e.g. a task for the team), `tessera metrics funnel`; example: a retail chain's opening funnel ([testdata/lojas](testdata/lojas)) | Working, with the built-in `metrics` plugin for reports and alerts (`respond` to a queue); export next ([#10](https://github.com/bbiangul/tessera/issues/10)) |
| Channels: a plugin declares a channel's rules (WhatsApp: 24h free-text window, consent) and the platform enforces them at run time and at admission; `wait orReply`; EvalSuite `advance` and `trigger` turns (a simulated clock). Reference tenant 2: [testdata/cadencia](testdata/cadencia), an outbound cadence with an A/B first message | Working, with the WhatsApp Cloud API adapter (`tessera serve --whatsapp`: Meta's signed webhook in, deduplicated; outbox out through the channel plugin, a refused send skipped and a failed one retried without holding other conversations). Tested against Meta's payloads, not yet against a real WABA ([#12](https://github.com/bbiangul/tessera/issues/12)) |
| People: `HumanQueue` with an SLA (remind, escalate to another queue, timeout with a safe answer or failure; business hours in the Agent's zone), the inbox (`GET /v1/tasks`, console), approvals bound to the task's hash, who answered recorded; pluggable notifications (`notify`: any plugin action, e.g. Slack in [testdata/lojas](testdata/lojas), WhatsApp in [testdata/cadencia](testdata/cadencia)) with signed task links (`--public-url`, 7 days, their own key) | Working; tested end to end in Go, not yet against real Slack or Meta; the console's inbox page has no automated test ([#13](https://github.com/bbiangul/tessera/issues/13)) |
| Console and inspection: `tessera serve` serves a web console and the same as a JSON API (`/v1/openapi.json`) for people and coding agents: talk to the agent, list runs, see each run's timeline (the router's probabilities, Jev's decisions with the question and options, plugin calls, intents, results, milestones), fire triggers, move a development clock, run EvalSuites, follow every run live (`GET /v1/activity`, its SSE stream, the console's Live panel); `tessera runs list|show|follow` reads the same from a database | Working ([#20](https://github.com/bbiangul/tessera/issues/20)) |
| Market benchmark 001 ([report](docs/research/market-bench-001.md), pre-registered): the same task, model and simulated patients for Tessera, LangGraph (2 variants), the OpenAI Agents SDK (2 variants) and a plain loop; 1,440 conversations with faults | Tessera: 0 duplicate or re-sent moves under act-then-timeout (the others: 29–48% duplicates, 90–100% re-sends), about 8× cheaper, p50 0.3 s against 2.2–2.8 s; worse at success in normal conditions (92% against 100%) from three product flaws. After fixing them ([market-002](docs/research/market-bench-002.md): 97%) and narrowing the slot question ([market-003](docs/research/market-bench-003.md), held-out tasks): **100% against 98–100%**, still 0 duplicates (others 36–51%), about 6.5× cheaper, p50 0.6 s against 2.1–2.3 s ([#21](https://github.com/bbiangul/tessera/issues/21), [#23](https://github.com/bbiangul/tessera/issues/23)) |
| A/B tests: `Experiment` (hashed, stable assignment; exposure recorded once), the `experiment` step, `tessera metrics experiment` with a sequential verdict and a sample-ratio check ([experiments-001](docs/research/experiments-001.md)) | Working, with guardrails that stop a harmful variant and tell the team (`guardrails`, `onBreach`; [experiments-002](docs/research/experiments-002.md)); cost guardrails need per-run cost recording ([#11](https://github.com/bbiangul/tessera/issues/11)) |
| `tessera serve`: HTTP API (messages, a durable outbox poll with ack, human replies), signed webhook `Trigger`s (idempotent by key), the scheduler; schedule `Trigger`s (cron in the Agent's time zone, once per scheduled time across schedulers, `maxDelay` after downtime); one process executes the runs of a database (a lock refuses a second: runs are serialized per conversation inside a process, cluster mode is phase 4) | Working, with events: a webhook Trigger delivers an event to the runs waiting for it by key (`wait.events`), and `webhook.key` makes one run per subject; milestone triggers start a Function when a milestone is recorded; no separate Workflow kind ([ADR-0006](docs/adr/0006-no-workflow-kind.md)) ([#8](https://github.com/bbiangul/tessera/issues/8), [#9](https://github.com/bbiangul/tessera/issues/9)) |
| Durable timers: the `wait` step (`for`, or `until` a time such as the next business day at 10h; `orReply` ends it on the user's reply, for cadences that stop when the lead answers), `WaitingForTime` runs woken by `WakeDue`; chosen over Temporal and Restate by a measured spike ([ADR-0005](docs/adr/0005-durable-timers-on-our-journal.md)) | Working; crash-tested; a million sleeping runs measured |
| `Workflow`, `Trigger`s, human queues with SLA, WhatsApp, business metrics, cluster mode | Not started (phase 2 and later) |

What is next, and what needs a decision from the owner, is the
[backlog](https://github.com/bbiangul/tessera/issues) (milestone per phase;
`needs-owner` for decisions).

## Try it

```bash
make build
./bin/tessera validate examples                   # check resources before they run
./bin/tessera explain Function.spec.inputs        # documentation for any field
make test-evals                                   # both reference suites, replayed offline

export TYPESAFE_API_KEY=...      # Jev (Decide): a paid API by TypeSafe
export OPENROUTER_API_KEY=...    # DeepSeek V4.1 Flash (Generate)
# Without a TypeSafe account, Decide can run on a model through OpenRouter:
#   --decide llm:deepseek/deepseek-v4.1-flash
# The measurements in docs/research were made with Jev; the replayed suites
# (make test-evals) need no key at all.
./bin/tessera test examples/traveler --generate openrouter:deepseek/deepseek-v4.1-flash
./bin/tessera chat examples/traveler --namespace traveler --caller userId=me
```

Start with the [authoring guide](docs/guides/authoring.md).

## A new agent, in its own repository

- **Create the repository:**

  ```bash
  tessera init minha-agencia --namespace agencia
  ```

  It holds a small working agent, `AGENTS.md`, the authoring skill for
  Claude Code, the guides and reference examples (`.tessera/`), and a
  `Dockerfile`.
- **Build it with Claude Code:** open Claude Code there and describe the
  agent you want.
- **Ship it:** `docker build` on the Tessera image, then `docker run` with
  `/data` as a volume.
- **The image:**
  - `make image` builds `tessera:dev` locally;
  - a tag `v*` publishes the binaries and `ghcr.io/bbiangul/tessera`
    (`.goreleaser.yaml`, `.github/workflows/release.yml`; RFC-0007).

## Principles for contributors

1. **Measure, don't assume.** Design claims come with a benchmark or a test.
   See [ADR-0002](docs/adr/0002-measure-before-believing.md).
2. **Trust is a foundation.** Unit tests for every package, integration tests
   against real providers behind a build tag, `-race` always.
3. **Documentation ships with the code**, for people and for coding agents
   ([AGENTS.md](AGENTS.md)).
4. **Start small.** Model evaluations start with the smallest model and move
   up only when measurements say the small one is not enough.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/guides/testing.md](docs/guides/testing.md).

## License

Apache-2.0 (provisional; see [ADR-0001](docs/adr/0001-provisional-name.md)).
