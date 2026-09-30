# Market benchmark 003: after narrowing the slot question

- **Date:** 2026-09-23
- **Pre-registration:** [market-bench-003](market-bench-003-preregistration.md),
  on top of 000 and 002.
- **Code measured:** commit `bcd61f0`. Held-out tasks: `tasks-003.json`,
  seed 24. Raw results are in `bench/results/market-003/runs.jsonl`.
- **Size:** 1,440 conversations, the same six competitors and versions.
  Total spend $1.61.

## Results

C1–C3 count only the runs where the fault fired.

| Competitor | C0 success | C1 success | C1 runs with a duplicate move | C1 runs re-sending an applied move | C2 success | C3 success | Unconfirmed moves (after review) | Cost per successful C0 run (m$) | p50 turn |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Tessera** | **100%** (94–100) | **100%** (93–100) | **0 / 48** | **0 / 48** | **100%** (93–100) | **100%** (93–100) | **0** | **0.15** | **0.58 s** |
| plain loop | 100% (94–100) | 85% (72–93) | 24 / 47 | 45 / 47 | 98% (89–100) | 100% | 0 | 1.00 | 2.2 s |
| langgraph | 100% (94–100) | 83% (70–91) | 20 / 47 | 46 / 47 | 96% (86–99) | 100% | 0 | 0.97 | 2.2 s |
| langgraph-sqlite | 100% (94–100) | 83% (70–91) | 20 / 48 | 46 / 48 | 100% | 100% | 0 | 1.00 | 2.1 s |
| agents-sdk | 98% (91–100) | 88% (75–94) | 17 / 48 | 45 / 48 | 96% (86–99) | 100% | 2 (C3) | 0.99 | 2.2 s |
| agents-sdk-sqlite | 98% (91–100) | 88% (75–94) | 22 / 48 | 46 / 48 | 100% | 100% | 1 (C3) | 1.01 | 2.3 s |

## Hypotheses

| # | Verdict |
| --- | --- |
| H4'' (not worse at C0 success) | **Supported:** 100% against 98–100%. |
| H2, H8 (no duplicates under act-then-timeout) | **Supported for the third time.** 0 of 48 against 36–51% duplicates and 94–98% re-sends. |
| H5, H9 (cost) | **Supported:** $0.00015 against $0.00097–0.00101 per successful task, about 6.5 times less. |
| H6 (speed) | **The opposite of the hypothesis, again:** p50 0.58 s against 2.1–2.3 s. |
| H7 (retries) | **Supported:** Tessera 0. The Agents SDK took a retried message as the confirmation in 3 runs, as in 001 and 002. |
| H1, H3 | **Still not supported:** no stack moved without a yes in C0, and in-memory stacks recover from a crash because the patient repeats. |

## What the transcripts show

- **Tessera:** no failure in any condition. A patient who gives the day
  and the time goes straight to the confirmation. One who does not is
  asked the day, then that day's times.
- **C1, the same trap as before.** Every LLM stack re-sent the applied
  move, told the patient the slot "was taken", and moved them again. Three
  of these second moves were flagged as unconfirmed by the automatic
  judge. In each, the patient had explicitly asked for that slot, so after
  review they count as 0.
- **C3.** The Agents SDK treated the duplicated message as the yes to the
  question it had just asked: 3 real unconfirmed moves.

## Across the three runs

| | 001 (as shipped) | 002 (flaws fixed) | 003 (question narrowed) |
| --- | --- | --- | --- |
| Tessera C0 success | 92% | 97% | **100%** |
| Best competitor C0 success | 100% | 100% | 100% |
| Tessera C1 duplicates | 0 | 0 | 0 |
| Competitors' C1 duplicates (range) | 29–48% | 23–40% | 36–51% |
| Tessera cost per success, relative | ~1/8 | ~1/6 | ~1/6.5 |
| Tessera p50 turn | 0.32 s | 0.96 s | 0.58 s |

## Limits

- Everything in market-001's limits still applies: one domain, one
  language, one model family, synthetic tasks, and an LLM playing the
  patient.
- The Tessera Function was improved three times, each time by looking at
  failures. The runs used tasks held out each time, but the task
  *templates* stayed the same. A new kind of request would test the
  approach, not these fixes.
- Two of the changes are wording and question design in one Function.
  They are good practice written into the authoring checklist, not
  platform guarantees. The platform-level results are the fault results
  (C1, C3) and the new admission warning for incomplete confirmations.
