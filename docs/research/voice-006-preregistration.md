# Voice 006: clearer, shorter warm messages, pre-registration

- **Date:** 2026-09-24, before live calls.
- **Baseline:** voice-004 on seed 24; voice-005 warm on seed 22.
- **Starting tree:** `486083e` plus pre-existing local voice/re-ask edits
  and `bench/market/tessera-warm2`. The incoming patch is preserved
  locally before editing. Those edits have not been measured.

## Candidates

Keep Flash, Jev, the simulator, judge and all effect guards unchanged.
The runtime candidate preserves every interpolated fact, including on
repeated questions. Unlike the incoming patch it never clears FACTS.
Warm messages may be shorter than the draft, use at most one brief
acknowledgement, and omit a closing offer while asking a question.
Use lines for a from/to change or a list longer than three items.
Repeated questions omit introductory padding, not facts.

- **A:** candidate runtime, existing `tessera-warm` definitions.
- **B:** same runtime, `tessera-warm2`: explain that only one time fits
  the requested period, only when it actually does. This isolates the
  added content from the runtime wording change.

Seed 24 (`tasks-003.json`) is development, not a held-out test. Run each
variant through C0–C3, three repeats (240 conversations each). Judge
the 60 C0 conversations against the frozen market-003 competitors:
300 pairs per variant, both orders, same judge and aggregation as
voice-004/005. This uses historical controls and cannot isolate provider
drift; report that limitation.

## Hypotheses and selection

Shares below count only pairs whose two orders agree on a winner;
report wins, losses and ties as well as shares.

| ID | Supported if |
| --- | --- |
| C1 | Clear share rises at least 10 percentage points over voice-004 warm |
| C2 | Efficient share rises at least 10 points over voice-004 warm |
| C3 | Natural share is at least 45%, and mean agent-message length falls at least 10% |
| C4 | Success is at least 97% in each fired condition, with zero duplicates, unconfirmed or foreign moves |
| C5 | Manual review of the judge's flagged transcripts finds no new factual error introduced by the voice |

Advance only a candidate passing C1–C5. If both pass, prefer higher
clarity, then efficiency, then A (fewer definition changes). Replicate
the selected candidate, unchanged, on seed 22 (`tasks-002.json`), with
market-002 competitors and voice-005 warm as control. Seed 22 was used
before and is replication, not a fresh holdout. Apply the same bars
relative to its control. Do not tune between the two seeds.

If neither passes, report the failure; retain the measured warm default
and keep candidate changes as reproducible research artifacts rather
than silently promoting them. Unit regressions must cover repeated
question facts, confirmation facts, fallback and replay before live runs.

## Budget and outputs

Maximum $10: up to three market runs capped at $0.50 each and three
judges capped at $2.80 each ($9.90 total requested caps). The existing
parallel Python judge can exceed its check by in-flight requests; stop
and report any overrun. No new dependency installations.

Outputs: `bench/results/voice-006/{A,B,replication}`, excluding per-run
databases; final report `docs/research/voice-006.md`. Record code and
definition hashes, actual spend, incomplete pairs and deviations.
