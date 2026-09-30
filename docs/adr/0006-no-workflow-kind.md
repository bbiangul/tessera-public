# ADR-0006: no separate Workflow kind; a Function with durable waits is the workflow

- **Status:** Accepted
- **Date:** 2026-09-23
- **Context:** RFC-0001 lists `Workflow` as a resource: "composição
  durável de passos com espera, desvio e compensação", with runs per
  subject and events other than replies. Phase 2 was to build it. By the
  time it came up, the Function had grown every one of those parts, and
  the reference workflow (an outbound sales cadence: template, wait two days for
  the reply or the meeting, follow-up, wait three days, nurture) runs as
  Functions in `testdata/cadencia`, tested on a simulated clock.

## Decision

There is no `Workflow` kind. A Function is the unit of durable work:

| What RFC-0001 asks of a Workflow | Where it is |
| --- | --- |
| Durable steps pinned to the version they started with | Every run (RFC-0003, generations) |
| Wait for time, a reply or another system's event | `wait { for, until, orReply, events }` (ADR-0005, this change) |
| Branch, call, compose | `branch`, `call` (a called Function runs in the same run and journal) |
| Compensation | `onError: [{ compensate }]` with each action's declared compensation |
| Runs per subject, started by other systems | Webhook `Trigger`s with `key` (one run per key, e.g. per lead) and `conversation` |
| Per-step status | The run's timeline (`GET /v1/runs/{id}`), one record per step |

## Why

- **One concept less for authors.** Two kinds with the same steps would
  differ only in how they start and what they may wait for: authors would
  have to choose, and admission, the console, the evals and the docs would
  handle both.
- **Nothing is missing.** Each part of the RFC's list has a test: waits
  (`TestWaitCrashAtEveryAppend`), replies (`TestWaitOrReply`), events
  (`TestEventEndsTheWait`, `TestEventCrashAtEveryAppend`), one run per
  key (`TestEventEndsTheWait`), and the whole cadence
  (`testdata/cadencia`, five scenarios).
- **The RFC's own example is a Function now.** Its `wait: { event:
  message.received }` is `orReply`, its CRM stages are `events`.

## What would change this

A composition that Functions cannot express: parallel branches that wait
at the same time, or a run that must survive a change of its own
definition (a new version taking over a running workflow). Neither is
needed by the reference tenants; both would be a new step type or a
generation migration, not a new kind.
