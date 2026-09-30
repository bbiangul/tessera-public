# Market benchmark 003: pre-registration

- **Date:** 2026-09-23, written before the market-003 run.
- **Rules:** [market-bench-000](market-bench-000-preregistration.md) and
  [market-bench-002](market-bench-002-preregistration.md) hold, except for
  what is listed here.

## Why

Market-002 left Tessera at 97% against 100% in C0. The failures were two
tasks where the Function listed two weeks of slots (20+ options) and the
simulated patient picked a slot outside the period it asked for.

## What changed in the benchmark's Tessera agent

- **Day first, then that day's times.** The Function asks for the day
  (about ten options), then the free times of that day (three at most).
  Both questions read the first message first, so a patient who says the
  day and the time goes straight to the confirmation.
- **The confirmation wording** is a plain question: "Posso remarcar sua
  consulta de X para Y? Responda sim para confirmar." The old "Confirmo a
  remarcação de X para Y?" also reads as "I confirm". On the development
  tasks, simulated patients took it as done and left before answering.

Development used seed 23 and tasks t04 and t10 of seed 22, which is
therefore no longer held out. On seed 23: 100% in C0 and C3 (40 runs
each), against 85% with an extraction-based filter that was tried first
and dropped.

## Held-out tasks

`bench/market/tasks-003.json`, seed 24, not looked at before this run.

## Hypotheses

The same H1–H9 as market-000 and market-002. H4' becomes:

| # | Hypothesis | Supported if |
| --- | --- | --- |
| H4'' | Tessera is not worse at success in C0 | Within 5 points of the best competitor, or better |

## Budget

$10 hard cap, one full run, the same competitors and versions.
