# Testing

Trust is one of the project's foundations, so tests are part of every change,
not a follow-up. For distributed CLI authoring, see [partner testing notes](partner-testing.md)
for output locations, successful effect counts and cassette handling.

## Levels

| Level | What | How to run | Needs network |
| --- | --- | --- | --- |
| Unit | Every package. Network code against `httptest` fakes that also assert on the request sent. | `make test` | No |
| Integration | Real providers (TypeSafe, OpenRouter). Build tag `integration`. Each test skips when its key is missing. | `make test-integration` | Yes |
| Replay | Recorded model calls replayed through the real code; a changed prompt fails with the new request. | part of `make test` | No |
| Conversations (replay) | Every EvalSuite scenario replayed from recorded Jev answers: offline, free, deterministic. | `make test-evals` (part of `make ci`) | No |
| Conversations (live) | The same scenarios against the real engines, budgeted per scenario and per run. | `tessera test <dir>` | Yes |
| Benchmark | Model comparisons, calibration, latency, cost on labelled datasets. | `tessera bench decide`, `tessera bench extract`, `tessera bench route` | Yes |

All tests run with the race detector.

The full strategy, including conversation simulations with personas and the
CI tiers, is [RFC-0002](../rfcs/0002-testing-agents.md). Re-recording
cassettes is described in [bench/README.md](../../bench/README.md).

## Rules

- **Test the contract, including the failure modes.** Malformed provider
  answers, non-label replies, missing logprobs, retries and cancellation all
  have tests. A backend that silently patches bad output is a bug.
- **Fakes check what we send, not only what we receive.** Example: the
  TypeSafe test asserts option order in the raw request body.
- **Hand-computed expectations for metrics.** `internal/bench` tests compare
  Brier, ECE, Wilson intervals and percentiles against values worked out by
  hand in the test comments.
- **Datasets are validated like code.** `bench.ReadDataset` rejects unknown
  fields, invalid questions, gold labels that are not outcomes, and duplicate
  ids; a test loads every shipped dataset.
- **Integration tests never print secrets** and never assume a model's answer
  beyond an unambiguous case.

## Looking inside a run

`tessera serve --dev` gives a web console at `/` and the same as JSON under `/v1`
(`/v1/openapi.json` describes it): talk to the agent, list runs, and open one to see
every record explained, from the router's probabilities to the final message; fire a
schedule by hand and move the clock forward to test waits. Without a server,
`tessera runs list|show --db <runs.db>` reads the same from the database.

## Writing an EvalSuite

An `EvalSuite` (see [the travel example](../../examples/traveler/evals/flights.evalsuite.yaml))
plays whole conversations:

- **Sandboxes replace plugins.** A Starlark file with one function per
  action (`list-upcoming-appointments` → `list_upcoming_appointments`),
  receiving `state` (a dict it may change) and the action's arguments.
  `definite(code, message)` and `ambiguous(message)` fail like the real
  system; an ambiguous failure after changing `state` is how you test
  "the clinic applied it but the answer never came back". An action with
  no sandbox fails the scenario: tests never reach a real system. A
  scenario's `state: { <plugin>: {...} }` replaces top-level keys of that
  sandbox's state for the scenario only: an outage (`down: true`), an
  empty agenda.
- **Turns** are `user:` messages, `human:` answers to a waiting task,
  `advance: 48h` (moves the scenario's clock and runs the scheduler:
  waits, schedules, deadlines) or `trigger: { name, payload }` (a
  schedule or webhook Trigger, unsigned in tests; a run it starts in
  another conversation becomes the scenario's conversation), each with
  optional `expect` (status, routed Function, texts said, queue, and
  `handoff`: the free mode's decision, `answer` or the Function of
  `freeMode.functions` it started. The decision exists only when the Agent
  has `freeMode.functions`; without them no turn has one).
- **A simulated user** (`simulator: llm`, RFC-0002): a model plays a
  `persona` (`goal`, `style`, `knows`) until the goal is met, for up to
  `maxTurns` messages (default 12), after the scenario's `turns` if any.
  - It reads only what the agent told the user, and never invents facts
    outside `knows`.
  - `humans: [...]` answer, in order, the tasks the agent hands to a
    person. When they run out, the scenario ends at the next task.
  - The clock moves a minute per message.
  - Its model is the `--generate` backend; its cost is reported apart
    (`simulator` in `--out`). Recorded in a cassette, it replays like the
    agent's calls.
  - Use it for what a script cannot: the agent asks something else, and
    the user still answers like a person. The transcript and every call
    with its `output` are in `--out`, for a judge.
  - `bench/agency` plays 18 travel customers this way (agency-001).

  ```yaml
  - id: natal
    simulator: llm
    maxTurns: 14
    persona:
      goal: "Planejar uma viagem para Natal: voo e hotel, e quanto fica tudo."
      style: "WhatsApp, informal, conta os detalhes só quando perguntam."
      knows: { origem: "São Paulo", ida: "15 de novembro", volta: "22 de novembro" }
    humans: [ { reserved: true, locator: "NAT7Q2" } ]
  ```
- **Private sandbox helpers** should start with `_` (for example `_ordinal`).
  Public functions are treated as action handlers; a helper without this prefix
  gets an UNUSED_FUNCTION warning because it matches no plugin action.
- **Sandboxes** receive an action's parameters as keyword arguments:
  `def find_slots(state, dentistId, days, **kw)`, after the suite's `state`. A parameter named like a Starlark
  keyword (`from`) is read from `kw["from"]`.
- **Web search** (`freeMode.search`) is tested like any plugin: a sandbox
  for `web` with `def search(state, query, **kw)` returns fixed results,
  so a scenario can:
  - check the answer cites them;
  - put a hostile snippet among them, to check it is not obeyed;
  - assert `never: [web/search]` for messages that must not search.

  A live test against the real provider needs its key, as in production.
- **Calibrating a handoff:** write labelled questions as scenarios with
  `expect.handoff`. `tessera test -v` prints each decision's
  probabilities, and `--out results.json` has them under `handoffs`. Pick
  `minConfidence` from those, never from your test questions
  (escalation-003).
- **Final expectations**: status, successful effect counts, actions that
  must never be called, texts, and a budget (cost, Decide and Generate
  calls). An effect count with `where` counts only the calls whose
  arguments meet a CEL condition on `inputs`
  (`where: "inputs.children == '5,8'"`): test what was sent, not only
  that something was.
- **Invariants run on every scenario**: `confirm-before-irreversible`
  (each irreversible effect after its *own* confirmation, or after a
  person answered yes to an `approve` task), `no-duplicate-effects`,
  `no-lost-messages`. Skipping one needs a reason in the scenario's
  description.
- **A fixed clock** (`spec.now`), so "amanhã" means the same day on every
  run.

Record once with `--cassette <file> --cassette-mode record` and commit the
cassette; CI replays it. The runner is itself tested against planted
defects (`internal/evals`): a missing confirmation, a sandbox bug, a wrong
count, an exceeded budget.

New recordings isolate identical requests by scenario; see
[scenario-scoped recordings](#scenario-scoped-recordings) below. The old
per-scenario-file workaround for [#32](https://github.com/bbiangul/tessera/issues/32)
is only relevant to legacy unscoped recordings. Re-record the complete
suite to migrate; do not weaken scenario assertions to hide a replay miss.

## Adding a Decide backend

1. Implement `decide.Decider`.
2. Unit tests with a fake server: happy path, every malformed answer, retry,
   and the `Calibrated == false` default.
3. An integration test behind the `integration` tag.
4. A benchmark run on a labelled dataset against the existing targets
   (`tessera bench decide`), with findings in `docs/research/`.

### Scenario-scoped recordings

New `tessera test --cassette ... --cassette-mode record` recordings isolate
Decide and Generate calls by namespace, suite and scenario. One file can
hold a whole suite, including different answers to identical requests.
`--only` replays the same response sequence as a full run; scenario order
does not affect it. This identity is not sent to a model. Extra occurrences
or changed requests still miss.

Wholly unscoped legacy targets remain supported with their original reset
behavior. They cannot recover scenario identity retroactively; re-record
the complete suite to migrate. If a target mixes scoped and legacy entries,
scoped runs never fall back to legacy entries: re-record any missing
scenarios. Recording one existing scoped scenario replaces the recorded
sequence for each request it makes and preserves other scenarios. Requests
not encountered during re-recording remain in the file, as before; delete
the old cassette first when an exact fresh snapshot is required.

The two clinic suites now use `contexto-evals.jsonl` and
`clinic-confirmation-evals.jsonl` in CI. Older per-scenario files remain as
historical measurement evidence; they are no longer needed by CI.

When testing preference capture, include a single message with both a day
preference and a factual question, followed by a search acknowledgment that
does not repeat the day. Assert both the factual answer and the actual dates
returned by the sandbox, plus no effect. Test later corrections, repeated
side questions, unclear replies, timeout and withdrawal separately. Keep
business-hours facts consistent with sandbox availability; a contradictory
fixture can create an apparent conversational failure independently of
preference retention. Preserve lexical assertion failures alongside manual
semantic analysis rather than silently weakening the assertion.
