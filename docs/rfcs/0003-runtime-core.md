# RFC-0003 — Runtime core: executing a Function

- **Status:** Accepted for phase 1 (RFC-0001 roadmap)
- **Date:** 2026-09-23
- **Depends on:** RFC-0001 ("Execução durável, efeitos e estado"), RFC-0002

## Summary

The runtime executes an admitted `Function` for one event: it checks
`authorize`, resolves `inputs`, checks `preconditions`, asks for confirmation,
performs the `action`, and renders the `output`. Every result that could differ
between two executions is written to a **journal** first. Execution is a
**deterministic interpreter over that journal**: whenever something new happens
(a user reply, a human answer, a restart), the run is executed again from the
top, and every step that is already in the journal returns its recorded result
instead of running again.

This is the same model as Temporal and Restate (replay over an event history),
kept small enough to run on SQLite in a single binary. The engine choice for
long waits (phase 2) stays open; the journal interface is the seam.

## Goals

- **Resume never repeats a model call or an effect.** A user who confirmed a
  slot gets that slot, even if the process died between the confirmation and
  the booking.
- **Effects follow their class** (RFC-0001): reads retry freely,
  idempotent writes retry with the same input, irreversible actions are
  confirmed, keyed and never repeated without verification. An ambiguous
  failure of an action nobody can verify stops in `Indeterminate` and goes to
  a human.
- **Suspension is a value, not a thread.** Waiting for the user or a human
  costs one row, not a goroutine.
- **Testable without a network.** Plugins, Decide and humans are interfaces
  with fakes; a run can be driven step by step in a unit test.

## Non-goals (this RFC)

- `Workflow`s and `Trigger`s (phase 2). The `wait` step and durable timers
  are specified below ("wait"), added at the start of phase 2.
- Multi-node execution and leases (phase 4). One process owns a run.
- The Generate engine. Output is a template; `guardrails` are checked by
  name only once the Guardrail resource exists.

## Model

### Run

A `Run` is one execution of one `Function` generation for one event:

| Field | Meaning |
| --- | --- |
| `id` | Random, recorded at creation |
| `namespace`, `function`, `generation` | What runs; the generation is fixed for the life of the run |
| `conversation`, `caller`, `event` | The scopes of RFC-0001; `event.time` is recorded at creation and never read from the clock again |
| `status` | See the state machine below |
| `journal` | Ordered records |

States used in this RFC: `Resolving`, `WaitingForUser`, `WaitingForHuman`,
`Confirming`, `Acting`, `Indeterminate`, `Succeeded`, `Failed`. Only one
event per conversation is processed at a time (RFC-0001, "Entrega de eventos
por conversa"); the runtime enforces it with a per-conversation lock.

### Journal

Each record has a **site**: a stable name for the place in the Function that
produced it, derived from the definition, never from a counter:

```
authorize
inputs.appointment/upcoming          fromAction result
inputs.appointment/pick              decide answer, or the question asked
inputs.appointment/pick#reply        the user's reply
preconditions[0]
confirm                              question sent / reply
action#intent                        about to call, with args and key
action#result                        the outcome
onError[1]#attempt2                  a retry
output                               the rendered response
```

Record kinds: `result` (a value), `error` (a classified error), `suspend`
(waiting for someone, with what was sent), `resume` (the answer that ended a
suspension), `intent` (an effect is about to start).

Because sites come from the definition and expressions are pure (no `now()`,
RFC-0001), executing the same generation over the same journal takes the same
path. A site reached that has no record is new work: it runs and is appended.
Records are looked up by (site, kind) and the store refuses a second record
for the same pair, so a bug that reaches a site twice fails loudly instead of
overwriting history.

Recorded values go through a typed codec (`internal/runtime/journal`) that
keeps `int` apart from `float`, and timestamps, durations and bytes as
themselves. Plain JSON would turn `2` into `2.0` and a timestamp into a
string, and a replayed run could take another path. The first execution
also uses the decoded value, so both see exactly the same data.

### Resolving an input

Sources are tried in order; a source whose `when` is false is skipped, and
every source that runs exposes `sources.<id>.output` to the ones after it.
What resolves the input:

- `fromAction`: only when the action's declared output type is the input's
  type. Otherwise its output is an intermediate value (a list the next
  source picks from, as in the reschedule example).
- `value`, `ask`, `fromFunction`: any non-null value. It must conform to the
  input's type, or the run fails: that is a bug in the definition.
- A read that fails moves on to the next source; the error is kept for the
  failure message.

Inputs are resolved after the inputs they read and otherwise in the order
they were written, so the user is asked in the author's order.

- `extract` takes the value from the user's message with the Generate
  engine. Every `extract` source of a Function is answered by **one** model
  call at the start of the run, with a schema built from the inputs' types
  and descriptions; a request with seven details costs one call. Every
  field may be null ("the message does not say"), and a value that does
  not fit the type (an impossible date) counts as not said, so the next
  source, usually a question, takes over.
- `ask` without `choices` is a free-text question: the reply is extracted
  as the input's type with one Generate call per reply, and asked again up
  to `maxAskRounds` times; `onLowConfidence: human` then asks a person.
- A caller's `with` value of null gives nothing: the callee resolves that
  input from its own sources, and may ask the user for it.
- `date` is a basic type: a `YYYY-MM-DD` string that must be a real day.

When no source resolves the input, `onUnresolved` runs (`fallback`,
`escalate`, `respond`, `fail`); without it, the run fails and the user gets
the runtime's failure message. A `fromFunction` call runs the other Function
nested in the same run (its `with` values become its inputs) and yields its
action's output, or its rendered output when it has no action, unless
the callee sets `output.value`, which is then what the caller gets.

### Effects

For an action with effect class `irreversible` or `external-message`:

1. Append `action#intent` with the arguments and the idempotency key.
2. Call the plugin with the key.
3. Append `action#result`.

On replay, `intent` without `result` means the process died during the call:
the outcome is unknown. Recovery follows the action's `verify` mode:

| Verify | Recovery |
| --- | --- |
| `native` | Call again with the same key; the system deduplicates |
| `probe` | Call the probe action declared in the manifest; its `happened` expression decides; if it shows the effect, its `output` expression becomes the result, otherwise the chain continues (a retry is now safe) |
| `none` | `Indeterminate`, and a `HumanTask` asks a person what happened. Never retried |

The same classification applies when the plugin itself returns an ambiguous
error (a timeout after the request was sent). Plugins return one of three
outcomes: success, a **definite** failure (nothing happened: validation error,
4xx), or an **ambiguous** failure (it may have happened).

`read` and `idempotent-write` actions skip the intent record: repeating them
is safe by definition.

The manifest describes the probe, because only the plugin knows how to
check its own effects:

```yaml
reschedule:
  effect: irreversible
  verify:
    mode: probe
    with: list-upcoming-appointments
    args: { patientId: "${ caller.patientId }" }
    happened: "sources.probe.output.exists(a, a.id == inputs.appointmentId && a.slotId == inputs.slotId)"
    output: "sources.probe.output.filter(a, a.id == inputs.appointmentId)[0]"
```

`inputs` are the action's arguments and `sources.probe.output` is the probe's
result. A probe that fails, or whose expressions cannot be evaluated, proves
nothing: the verdict is "unknown" and the run goes on as if there were no
verification.

The manifest's `authorize` rule is evaluated by the runtime before every
call of the action, with the action's arguments as `inputs`. A refusal is a
definite failure (`FORBIDDEN`): nothing was called.

### onError

`onError` strategies run in order after a definite or ambiguous failure of the
action (or of an input's source, for `onUnresolved`):

| Strategy | Behavior |
| --- | --- |
| `probe` | Ambiguous failure only: run the verification above; if the effect happened, continue as success |
| `retry` | Up to `attempts` more calls with backoff; only after a definite failure, or after an ambiguous failure the probe resolved as "did not happen", or for classes that are safe to repeat |
| `fallback` | Call another function or action (checked at admission like the main action), or use null |
| `escalate` | Create a `HumanTask` on the queue; send `whileWaiting`; the run waits in `WaitingForHuman`. The human's answer is recorded as a `resume` record |
| `respond` | Send the template and end the run as `Failed` (handled) |
| `fail` | End the run as `Failed` |

Backoff sleeps are recorded as timer records so a resumed run does not sleep
again. If no strategy handles the failure the run ends `Failed` with the error.

### ask

An `ask` source resolves a choice from the conversation:

1. **Decide** (site `…/pick`): a Choice question whose options are the
   `choices` list (label: the item's `label` field if it has one, otherwise
   its canonical JSON) plus `NONE`, over the masked recent conversation. If the
   top answer is not `NONE` and its probability is at least `minConfidence`, it
   is the input's value.
2. Otherwise `onLowConfidence`:
   - `ask-user`: send `text`, suspend in `WaitingForUser`; the reply is
     recorded, and Decide runs again on the reply (site `…/pick#1`). After
     `maxAskRounds` (default 2) unanswered rounds the input is unresolved.
   - `human`: escalate an `answer` task with the choices.
   - `escalate-model`: ask the namespace's escalation backend, when one is
     configured; without one it behaves as `human`, the safe default.

The decide answer is always recorded, including probabilities, so replay and
audit see why the run acted.

### confirm

When an irreversible action has `confirm`, the runtime sends the rendered
question and suspends in `Confirming`. The reply is classified by Decide as a
yes/no (Noul) question. At or above 0.9 probability of yes the action runs; at
or below 0.1 the run ends `Succeeded` with a "cancelled by the user"
outcome; in between it asks once more, then escalates.

### wait

A `wait` step sleeps durably: `for: 36h` (from when the step is first
reached) or `until:` a timestamp expression, e.g. the next business day at
10h, `atTime(addBusinessDays(event.time, 1, tz), '10:00', tz)`.

- The wake time is computed once and recorded as the step's result, so a
  restart, a clock change or a new definition never moves a timer that is
  running. A time already past does not sleep.
- The run records a `sleep` (not a message, so the outbox never lists it) and goes to
  `WaitingForTime` with `WakeAt` on the run header. A sleeping run is not
  the conversation's active run: the user's messages start other runs.
- `Runtime.WakeDue(limit)` finds the due runs (an index on `wake_at` that
  holds only sleeping runs) and `Wake`s each: a resume record at the
  pending site, then execution. `Wake` does nothing to a run that is not
  sleeping or not due, so several schedulers, retries and crashes never
  send twice (`TestWakeIsIdempotent`, `TestWaitCrashAtEveryAppend`).
- The step's output is the time it woke.
- `orReply: true` also ends the wait when the user replies: the run
  waits as `WaitingForUser` with `WakeAt`, so it is the conversation's
  active run and the reply is delivered to it, not routed; if the time
  comes first, `Wake` resumes it with a timer record. The output is
  `{reply, timedOut}`. Whichever comes first wins; the other finds the run
  no longer waiting (`TestWaitOrReply`, crash-tested at every write).
- `events: [{name, key}]` also end the wait when another system reports
  one of them about the same key (a webhook `Trigger` with `event`). The
  keys are evaluated once and recorded; the wait registers a watch
  (namespace, event, key → run, site) in the store. `DeliverEvent` looks
  the watches up, and under the conversation's lock appends a resume
  record with the event and its payload at the pending site, then
  executes; a watch whose run no longer waits at that site is forgotten,
  and a final run forgets all of its watches. The output is
  `{reply, timedOut, event, payload}`. The first outcome recorded wins
  (a timer, a reply or an event): the append at the same site fails for
  the others (`TestEventEndsTheWait`, `TestEventCrashAtEveryAppend`).

The spike that chose this over Temporal and Restate is ADR-0005.

## Interfaces

```go
// Actor calls plugin actions (the Act engine).
type Actor interface {
    Call(ctx context.Context, call ActCall) (any, error) // error: *DefiniteError or *AmbiguousError
}

// Journal stores records per run. Append is atomic per record.
type Journal interface {
    Load(ctx context.Context, runID string) ([]Record, error)
    Append(ctx context.Context, runID string, r Record) error
}
```

`decide.Decider` is used as it is.

Messages are never sent by the runtime. Every message (a question, a reply,
a task for a human queue) is a journal record first. A `Result` carries all
the run's messages that were not acknowledged yet, each with a stable `ID`
(run and site) and a `Seq`; the channel layer delivers them and calls
`Ack(runID, seq)`. If anything fails in between, the messages come back on
the next execution, and the channel drops IDs it already delivered. This
is the transactional outbox: no message is lost when an execution fails
after recording it, and none is sent twice by the runtime.

## Storage

Phase 1 uses SQLite through `modernc.org/sqlite` (pure Go, so the binary stays
static and cross-compiles). One table of records keyed by `(run_id, seq)`, one
table of runs. Postgres comes with cluster mode (phase 4). An in-memory
journal implements the same interface for tests.

## Measurements

| What | Target | Measured (2026-09-23, Apple M-series laptop) |
| --- | --- | --- |
| Crash injection: fail the store at every append of the reschedule conversation, recover, compare with an uninterrupted run | Same final state, one reschedule, same messages | Passes for every append (`TestCrashAtEveryAppend`) |
| Crash during the irreversible call (intent recorded, no outcome), effect applied or not | Verified by the probe, never repeated blindly | Passes both cases (`TestCrashDuringTheEffect`) |
| Replay of the reschedule run waiting at the confirmation (11 records: CEL, templates, asks) | < 1 ms | 84 µs (`BenchmarkReplay`) |
| Replay of a run with 51 records | < 1 ms | 125 µs (`BenchmarkReplay50`) |

Replay from the top is not a performance concern at phase 1 sizes. The
first version of `BenchmarkReplay50` reported 159 ns: it measured a finished
run that returns at once. Numbers are only trusted after checking that the
benchmark does the work it claims.
