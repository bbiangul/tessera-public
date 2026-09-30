# Market benchmark 002: after fixing the flaws market-001 found

- **Date:** 2026-09-23
- **Pre-registration:** [market-bench-002](market-bench-002-preregistration.md),
  on top of [market-bench-000](market-bench-000-preregistration.md).
- **Code measured:** commit `f5a2b36`. Raw results and every transcript
  are in `bench/results/market-002/runs.jsonl`.
- **Tasks:** held out (`tasks-002.json`, seed 22). The fixes were made on
  another seed.
- **Size:** 1,440 conversations, the same competitors and versions as
  market-001. Total spend $1.79.

## Results

C1–C3 count only the runs where the fault fired.

| Competitor | C0 success | C1 success | C1 runs with a duplicate move | C1 runs re-sending an applied move | C2 success | C3 success | Unconfirmed moves (after review) | Cost per successful C0 run (m$) | p50 turn |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Tessera** | **97%** (89–99) | **98%** (89–100) | **0 / 46** | **0 / 46** | 98% (89–100) | 96% (86–99) | **0** | **0.18** | **0.96 s** |
| plain loop | 100% (94–100) | 67% (53–78) | 19 / 48 | 40 / 48 | 100% | 100% | 0 | 1.12 | 2.1 s |
| langgraph | 100% (94–100) | 65% (50–77) | 19 / 48 | 46 / 48 | 98% (89–100) | 100% | 0 | 1.09 | 2.2 s |
| langgraph-sqlite | 100% (94–100) | 75% (61–85) | 15 / 48 | 46 / 48 | 100% | 100% | 0 | 1.12 | 2.2 s |
| agents-sdk | 100% (94–100) | 83% (70–91) | 11 / 48 | 47 / 48 | 96% (86–99) | 100% | 1 (C3) | 1.08 | 2.3 s |
| agents-sdk-sqlite | 100% (94–100) | 69% (55–80) | 18 / 48 | 45 / 48 | 100% | 100% | 1 (C3) | 1.11 | 2.3 s |

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| H4' | With the fixes Tessera is within 5 points of the best competitor in C0 | **Supported:** 97% against 100%, a 3-point gap (the intervals overlap widely). |
| H8 | The fixes keep the fault results | **Supported.** C1: 0 duplicates and 0 re-sends in 46 fired runs, against 23–40% duplicates and 83–98% re-sends for the others. C3: 0 duplicates. |
| H9 | The extra model call keeps the cost and latency advantage | **Supported, narrower.** $0.00018 against $0.00108–0.00112 per successful task (6 times less, was 8). p50 0.96 s against 2.1–2.3 s (was 0.32 s): the `titular` extraction is a DeepSeek call on the first turn. |
| H1, H3 | as in market-001 | **Still not supported.** No stack moved without a yes in C0. In-memory stacks recover from a crash because the patient repeats. |
| H2, H5, H7 | as in market-001 | **Supported again.** In C3 the Agents SDK read a retried message as the confirmation once more, in each variant. |

## What the fixes did

- **Someone else's appointment** (t15: son, husband, grandmother or
  mother): 12 of 12 runs correct, against 0 of 12 in market-001. Tessera
  refuses before listing slots, and nothing moves.
- **A change of mind at the confirmation** (t17–t20): no failure. The
  correction is confirmed and done ("Confirmo a remarcação da sua
  consulta de sexta 02/10 às 9h para terça 13/10 às 8h?").
- **Retried confirmations:** no "não entendi" and no second effect.

## What Tessera still loses: the long list of slots

The only Tessera failures are two tasks, t04 and t10, across conditions.
In both, the patient wants Tuesday 13/10 in the afternoon, where 17h is
the only slot, but opens with just "preciso remarcar". Tessera answers
with every slot of the next two weeks, more than 20 options. The
simulated patient then picked a morning slot, or confirmed and left early.
The LLM stacks asked for a preference or filtered the slots, and passed.

This is a real UX weakness of the Function, even though a simulated
patient makes the mistake. The fix belongs to the Function: ask for the
day or period first, or filter by what the patient said. It was not made
for this run.

## Limits

Those of market-001, plus:

- The hold-out varies slots, targets and the relative, but not the task
  templates. It shows the fixes are not tuned to the exact 20 tasks, not
  that they hold for new kinds of requests.
- H4' passes with a 3-point gap whose interval (89–99%) still reaches 11
  points below the competitors' 100%: "not worse" at this sample size is
  modest evidence.
