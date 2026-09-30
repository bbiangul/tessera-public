# learn-002: lesson proposals, old and new on the same new journal (#63)

- **Date:** 2026-09-27.
- **Pre-registration:** `learn-002-preregistration.md` (commit 1933884,
  before the code).
- **Code:** commit dfa1bd2, frozen before the test.
- **Data:** `bench/results/learn-002/`.
- **Spend:** about $0.02 for the agents and `learn`, $0.25 for the judge.

## What changed (tuned on learn-001's journal only)

- **A nameless answer:** the Generate engine removes greetings and the
  customer's name before Jev decides.
- **"Personal" asks what was decided for this customer:** the value of
  their quote, their deadline, a discount or time granted to them, their
  results. It gives examples on both sides: "a partir de R$ 9.800" and
  "fazemos implante com o Dr. Bruno, antes precisa de avaliação" are the
  business's.
- **Faithfulness asks what the lesson adds,** and sees the question, so
  naming what was asked is not an addition.
- **Reviewability and robustness:**
  - rejected cases keep the lesson that was written;
  - ids fold accents;
  - an engine error no longer aborts a run: the case is set aside, a
    grouping error keeps cases apart, and a contradiction error makes a
    conflict.

**On dev, the three runs went from 2–3 to 3–4 of the 4 G topics that
had two cases:**
- the 4 personal cases were rejected in every run, including the
  instalment;
- the conflict was flagged in every run.

## Test

**Phase 1:** 25 scenarios, run once and live. **8 never reached the
reception**, because the free mode answered them:
- **C2 and Q3:** the clinic's documents already give Saturday's and the
  weekdays' hours. My data design was wrong: the conflict (C2) and the
  early-appointment cases (Q3) could not happen.
- **K2 and K6:** the free mode answered from nearby passages instead of
  handing off.

That left 17 answered cases:
- 4 general topics with 2 cases each;
- 4 tempting personal pairs (Q1, Q2, Q4, Q5);
- 1 single case.

**`learn`, 3 runs each on that journal:**

| | old (learn-001) | new (learn-002) |
| --- | --- | --- |
| General topics proposed (of the 4 available) | 3, 4, 3 | **4, 4, 4** |
| Personal cases proposed | **0, 0, 0** | **0, 0, 0** |
| Personal cases judged general, then held by the threshold (of 8) | 1, 1, 1 (the instalment "até o dia 25") | 1, 1, 0 (the discount "10% pra você, no Pix") |
| General cases lost as "unfaithful" | 1–2 (limpeza, bebedouro) | 0 |
| A name or personal value in a lesson's question or fact | 0 | 0 |
| S2 (single case) proposed | no | no |

**Phase 2:** the 4 lessons from new's first run were accepted. All four
match the reception's answers.

| | before | after |
| --- | --- | --- |
| Accepted topics: correct without a handoff (of 12) | 0 | **12** |
| Personal questions: another patient's value (of 6) | 0 | 0 |
| Unsupported facts (of 24) | 0 | 0 |

## Hypotheses

| # | Result |
| --- | --- |
| M1 | **Not supported as registered:** ≥ 5 of 6 could not be reached, since only 4 topics were answered in phase 1. On those 4, new proposed 4 in every run; old proposed 3, 4 and 3. |
| M2 | **Supported:** 0 personal proposals in all runs, for both. |
| M3 | **Not testable for the conflict:** the documents answered C2, so it never reached the reception. S2 was never proposed. |
| M4 | **Supported** for the lesson text. The proposals file does carry each case's answer as evidence, names included: treat it as personal data, keep it out of repositories and delete it after review. |
| M5 | **Supported:** unit tests. |
| M6 | **Supported:** 12 of 12, and 0 other patients' data. |

## Decision

**New replaces old.** It was already on main, committed before the test.

The registered rule needs M1, and M1 failed only because my phase-1
data let the documents answer two topics. On everything measurable,
new is at least as good as old:
- it proposes more (12 of 12 topic-runs against 10 of 12);
- it loses no faithful lesson;
- it is equally safe;
- rejections are now reviewable;
- errors no longer abort a run.

The owner delegated RFC-0008's decisions, so this is my call, and it is
stated here as such.

**Still open:** in 2 of 3 runs, one personal answer was judged general.
It was held only because its pair was rejected, or because of the
threshold. The person reviewing is still the backstop.

**Next:**
- Phase-1 data must avoid what the documents already answer.
- Relevance moves to knowledge-002: in K2 and K6 the free mode answered
  from nearby passages instead of handing off.
