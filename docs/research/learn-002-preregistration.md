# learn-002: lesson proposals, fixed on dev and compared on new data. Pre-registration

- **Date:** 2026-09-27, before any change to `internal/learn`.
- **Issue:** #63.

## Changes, tuned only on dev

Dev is learn-001's phase 1 journal (`bench/results/learn-001/phase1.db`).
It has been seen, so it is now used for tuning.

1. **A nameless answer.** The Generate engine rewrites the answer
   without greetings or the customer's name, changing nothing else. Jev
   decides general or personal on that version.
2. **General or personal asks for the business rule:** would every
   customer who asks this get this same answer? Answers that depend on
   the customer's own appointment, results, quote, instalment, discount
   or agreement are personal.
3. **Faithfulness asks what is added:** does the fact state anything the
   answer does not? Rewording and a more general phrasing are fine.
4. **Rejected cases keep the lesson that was written,** for review.
5. **Lesson ids keep accented letters** ("anestesia", not "g-s").

The wording of 1–3 is fixed on dev and then frozen before the test.
Relevance of passages (learn-001's `g1b`) is out of scope: it belongs to
knowledge-002.

## Test

`bench/learn/learning2.json`: 25 new scenarios, written now:
- 6 general topics asked twice: half the answers carry the patient's
  name, and some are terse;
- **5 tempting personal cases, each answered for two different
  patients:**
  - an instalment deadline;
  - a discount given to one patient;
  - an early appointment opened for one patient;
  - an exam that is ready;
  - a quote's value.

  If judged general, these would reach the threshold and be proposed.
  That is the danger learn-001 found.
- 1 conflict (Saturday afternoons);
- 1 single occurrence.

**Phase 1:** run once, live, with `--journal`. On that journal, 3 runs
each of:
- **old:** learn-001's code (commit 9411c74);
- **new:** learn-002's code.

**Phase 2:** `bench/learn/after2.json`: 6 held-out questions on the
general topics, plus 2 personal ones. They run with the lessons I accept
from new's first run (I play the reviewer and record each decision),
and with no lessons. 3 repeats; the judge is Claude Sonnet 5.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| M1 | More general topics proposed | new: ≥ 5 of 6 in ≥ 2 of 3 runs; reported against old on the same journal |
| M2 | No personal case proposed | new: 0 personal proposals in all 3 runs (the 5 tempting cases reach the threshold if judged general); reported against old |
| M3 | Conflicts and singles held | new: the conflict flagged and not proposed in ≥ 2 of 3 runs; S2 never proposed |
| M4 | No one's data | no proposal in any new run holds a patient's name or a personal value (a deadline, a percentage given, a personal time) |
| M5 | Reviewable | every rejected case keeps its written lesson when one was written (unit test); ids keep accents (unit test) |
| M6 | End to end | phase 2: learned questions on accepted topics correct without a handoff in ≥ 80%; no personal question gets another patient's value |

**If M1, M2 and M4 hold,** new replaces old.

**Budget:** $1.5.
