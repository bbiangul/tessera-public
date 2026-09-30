# Market benchmark 002: pre-registration

- **Date:** 2026-09-23, written before the market-002 run.
- **Issue:** [#23](https://github.com/bbiangul/tessera/issues/23)
- **Rules:** everything in [market-bench-000](market-bench-000-preregistration.md),
  including its deviations, holds except for what is listed here.

## Why

Market-001 found three Tessera flaws that cost it success in normal
conditions (92% against 100%):

1. the confirmation did not name the appointment;
2. a change of mind at the confirmation cancelled the request;
3. a retried confirmation got "não entendi".

They were not fixed before that run. They are fixed now, and this run
checks the fixes on tasks that were not used to make them.

## What changed in Tessera (the platform)

- **A no that carries a correction** ("mudei de ideia, prefiro X") ends
  the run without acting. The host starts the same Function again from
  that reply, which resolves and confirms the new value (`Result.Restart`,
  `TestCorrectionAtTheConfirmation`). This costs one extra Jev question on
  a no.
- **Admission warns** when a confirmation does not mention every input
  the action acts on (`CONFIRM_INCOMPLETE`).
- **`POST /v1/messages` and `Host.HandleOnce` take the channel's message
  ID**, so a retry with the same ID is not handled again.

## What changed in the benchmark's Tessera agent (`bench/market/tessera`)

- The confirmation names the current appointment and the new slot.
- A `titular` input, extracted by DeepSeek from the message, recognizes a
  request about someone else's appointment. The Function then refuses
  before listing slots. This adds one generate call per conversation.
- The not-understood message also works as an answer to small talk.

## What changed in the harness

- **Held-out tasks:** `bench/market/tasks-002.json`, seed 22, 20 tasks of
  the same kinds with other slots, targets and goal wording (the relative
  varies: son, husband, grandmother, mother). The fixes were developed on
  a separate set, seed 23, which is not committed. Seed 21 (market-001)
  and seed 22 were not used during development.
- Appointments have a label in the tool server's data, the same for every
  competitor.
- Every patient message carries an ID; a C3 retry repeats it. Only
  Tessera uses it, because the other stacks have no documented way to
  (the pre-registered rule: no Tessera patterns added to them).

## Hypotheses

The same H1–H7 as market-000. What this run adds:

| # | Hypothesis | Supported if |
| --- | --- | --- |
| H4' | With the fixes Tessera is not worse at success in C0 | Within 5 points of the best competitor |
| H8 | The fixes do not cost the fault results | Tessera still 0 duplicates and 0 re-sends in C1, 0 duplicates in C3 |
| H9 | The extra model call does not erase the cost and latency advantage | Cost per successful C0 task ≤ 50% of the median competitor; p50 turn below the competitors' |

## Budget

$10 hard cap, one full run, the same competitors and versions as
market-001.
