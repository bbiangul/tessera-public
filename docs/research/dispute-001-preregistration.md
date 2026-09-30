# dispute-001: a disputed lesson goes to a person (#66). Pre-registration

- **Date:** 2026-09-27, before any change.
- **Issue:** #66.

## The problem

**In lifecycle-001,** after a patient said "me disseram que é a partir de
3 anos", the free mode answered "pela nossa política, atendemos a partir
de 1 ano". It defended a wrong learned lesson.

**Lessons carry less authority than documents.** They are the team's past
answers, and they may be outdated or wrong. The review suspends a lesson
only after repeated disputes, so the first patients are misled.

## What will be built (variant D)

1. **Detecting the dispute.** Before a free turn, if the conversation's
   previous free turn was given lessons (the journal records their ids),
   one Jev question asks whether the new message disputes what the
   previous reply stated. It is the review's dispute question.
2. **On a dispute:**
   - the disputed lessons are not given to this turn;
   - FACTS gain `disputed` (the lessons' facts), with a rule: do not
     repeat or defend them, say it will be checked with a person, and
     never claim a person was already asked;
   - the handoff question is told that a disputed lesson needs a person;
   - the turn journals `disputedLessons` (ids), which the review counts.
3. **Unchanged:**
   - document passages (a dispute of a document is not this);
   - what the review suspends (still 2 disputes and 30%).

## Measure

**Setup:**
- `bench/dispute/`: learn-001's clinic with a lessons file of 4 lessons.
  One is wrong ("crianças a partir de 1 ano").
- **Conversations:** two turns, written now, split once:

  | | dev | test |
  | --- | --- | --- |
  | A lesson disputed in turn 2 | 4 | 8 |
  | Benign follow-ups (thanks, a related question) | 3 | 6 |
  | A document fact disputed | — | 2 |

- **Variants:** **base** (today's binary) and **D**.
- **Runs:** 3 repeats; Jev 1.13.0, DeepSeek V4.1 Flash.

**Deterministic checks on the reply to turn 2:**
- **insists:** it repeats the disputed claim (a per-conversation regex:
  "1 ano", "250", "40 min", "dia do pagamento");
- **handoff:** a task reached `queue:recepcao`;
- **disputed-flag:** D's journal records the lesson id.

The wording of the dispute rule is tuned on dev only, then frozen.

## Hypotheses (test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| D1 | Disputed lessons are not defended | D insists in ≤ 2 of 24; base's count reported (expected ≥ 8) |
| D2 | They reach a person | D hands off ≥ 18 of 24 lesson disputes |
| D3 | Benign follow-ups are left alone | D's handoffs on benign follow-ups ≤ base's + 2 (of 18), and flagged as disputed in ≤ 2 of 18 |
| D4 | Documents unchanged | document disputes: D's handoffs and insistence within ±2 of base's (of 6) |
| D5 | Recorded for the review | every D turn that dropped a lesson journals its id (unit test); `make ci` passes |

**If D1, D3 and D5 hold,** D ships on by default.

**Budget:** $1.
