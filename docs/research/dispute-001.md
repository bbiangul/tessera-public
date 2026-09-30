# dispute-001: a disputed lesson goes to a person (#66)

- **Date:** 2026-09-27.
- **Pre-registration:** `dispute-001-preregistration.md` (commit 0b9523f,
  before the code).
- **Data:** `bench/results/dispute-001/`:
  - `base/` and `d/`: transcripts;
  - `d-journal/`: variant D rerun with journals, to count the flags.
- **Spend:** about $0.03 (agents and Jev). No judge: the checks are
  deterministic.

## What was built

**When the conversation's previous free turn was given lessons,** one Jev
question asks whether the new message disputes that turn's reply. It is
the same question `learn review` asks. On a dispute:
- the disputed lessons are not given to this turn;
- `FACTS.disputed` holds their facts, with a rule: do not repeat or
  defend them, say the reception confirms it, and never claim it was
  passed on;
- the handoff is told that a disputed lesson needs a person;
- the lesson ids are journaled as `disputedLessons`, on the free turn or
  on the handoff's route.

**Document passages are unchanged.** It costs one Jev call, and only
after a turn that used a lesson.

**Unit tests:**
- a disputed lesson is not given again;
- a follow-up that is not a dispute keeps the lesson;
- the dispute is journaled on both paths;
- 5 mutations caught.

**Two gaps were found while measuring:**
- the unit test did not cover exclusion, until the dispute message
  itself retrieved the lesson;
- a handed-off dispute journaled nothing, because the first code wrote
  the ids only on the free-answer path.

Both are fixed and tested. The second was found when the journal rerun
counted 0 flags.

## Result

Dev (7 conversations) needed no tuning: D insisted 0 of 12 times,
against 9 for base.

**Test:** 16 conversations, 3 repeats. D was run twice: the transcripts
run, and the journal run after the fix. Their counts match.

| | base | D |
| --- | --- | --- |
| Lesson disputes: the disputed claim repeated (of 24) | 5 | **0** |
| Lesson disputes: handed to the reception (of 24) | 19 | **24** |
| Lesson disputes: flagged in the journal (of 24) | — | 18 |
| Benign follow-ups: handed off (of 18) | 9 | 9 |
| Benign follow-ups: flagged as disputes (of 18) | — | **0** |
| Document disputes: handed off / claim repeated (of 6) | 0 / 6 | 0 / 6 |

| # | Result |
| --- | --- |
| D1 | **Supported:** 0 of 24. Base repeated the claim in 5 (fewer than the ≥ 8 expected), because base's handoff already took most disputes to the reception. |
| D2 | **Supported:** 24 of 24. |
| D3 | **Supported:** benign follow-ups were handed off 9 times with D and 9 with base, and flagged 0 of 18. |
| D4 | **Supported:** document disputes are unchanged. The agent restates the document ("não abrimos aos domingos"; "a avaliação custa R$ 120" against a Google claim that it is free), which is right for the business's own documents. |
| D5 | **Supported after the fix:** unit tests on both paths; `make ci` passes. |

**Shipped on:** it applies whenever the free mode has knowledge.

## What this means

- A disputed lesson is no longer defended. Every dispute reached a
  person, and none was repeated.
- **Detection found 18 of 24 disputes.** The other 6 reached the
  reception through the ordinary handoff, so they are not flagged.
  `learn review` still finds them, because it reads the next message
  independently.
- **Harm still needs two disputes to suspend a lesson** (lifecycle-001).
  What changed is that no patient is misled twice by the same
  conversation.
