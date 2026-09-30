# learn-001: the team's answers become lessons (RFC-0008 part B). Pre-registration

- **Date:** 2026-09-27, before any of this is built.
- **RFC:** RFC-0008, part B, first slice, with the decisions taken there.

## What will be built

1. **`tessera test --journal file.db`** keeps the scenarios' runs in one
   SQLite journal. Each scenario has its own conversation, so the suites'
   checks are unchanged.
2. **`internal/learn`** and **`tessera learn`**, offline, over a runs
   database. It never changes the agent.
   - **Signal:** `escalate` tasks of kind `answer` that a person
     answered. It takes the text the person saw and the person's
     answer. Both are masked.
   - **General or personal?** A Jev question: does the answer hold for
     any customer who asks this (a fact about the business), or only for
     this person's case? Personal answers are rejected and counted.
   - **Lesson:** the Generate engine writes a general question and a
     one-paragraph fact from the answer, with no person's name or
     details. A Jev check asks whether the fact states only what the
     answer states, with nothing about a person. If it fails, the case is
     rejected.
   - **Grouping:** Jev asks pairwise whether two general cases ask the
     same thing. A group's answers are then checked for contradiction by
     Jev. A group with contradicting answers is flagged as a conflict,
     for a person to settle, and not proposed.
   - **Threshold:** a lesson is proposed only with at least 2 answered
     cases (`--min 2`, the default).
   - **Output:** a review file of proposals, with their evidence: runs,
     members, dates. It also lists rejected cases (personal, unfaithful)
     and conflicts, with reasons.
   - **Acceptance:** `tessera learn accept --ids …` appends the accepted
     lessons to a lessons Markdown file. It is a `KnowledgeBase`, one
     heading per lesson. Each lesson carries its provenance in an HTML
     comment: id, kind (fact, price, policy), runs, members, learned
     date, review date by the RFC's default validity. The index skips
     HTML comments.
   - **Approval** is a person running `accept` and committing the file.
     Approval through a channel task is a later slice.

## Measure

**The agent** (`bench/learn/`): knowledge-001's clinic. It has:
- its documents;
- `check` on;
- a `licoes` KnowledgeBase;
- a free-mode Function `perguntar-recepcao`, an `escalate` with
  `task: answer` to `recepcao` whose answer is relayed;
- a Team with the reception.

**Phase 1, learning** (`bench/learn/learning.json`, 17 scenarios written
now):

| Case | Scenarios | What should happen |
| --- | --- | --- |
| G1–G5 | 5 general topics, asked twice each, answered by the reception in different words, with the patient's name | proposed |
| C1 | one topic answered in contradicting ways (wi-fi) | flagged as a conflict |
| P1–P4 | 4 personal cases (an appointment, an x-ray, a quote, an instalment) | rejected |
| S1 | one general topic asked once | not proposed |

Phase 1 runs once, live, with `--journal`. The human answers are
scripted. `tessera learn` then runs 3 times on that journal.

**Acceptance, the person's role.** I play the reviewer. I accept a
proposal when its fact matches the reception's answers, and I record
each decision and its reason in the report. The accepted set comes from
the first `learn` run.

**Phase 2, held out** (`bench/learn/after.json`, 15 questions written
now, never used to build):
- 10 new phrasings of G1–G5;
- 4 unlearned: the wi-fi conflict, the single-occurrence topic, a topic
  nobody answered, and a question next to G4 that G4 does not settle
  (an adapted bathroom);
- 1 personal.

**Variants:**
- **before:** the lessons base is empty;
- **after:** the accepted lessons.

**Runs and judge:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- the judge is Claude Sonnet 5, given every document and lesson the
  patient may read, and the truth; not the variant.
- Handoff is read from the transcript: a task reached `queue:recepcao`.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| L1 | The right lessons are proposed | In ≥ 2 of 3 `learn` runs: ≥ 4 of 5 G topics proposed, 0 personal cases proposed, S1 not proposed, C1 flagged as a conflict and not proposed |
| L2 | Lessons carry no one's data | No proposal in any run names a patient (Ana, Marcos, Joana, Pedro, Luiza, Carla, Carlos, Bia, Rafael) or holds a personal value (14h, R$ 3.200, dia 15) |
| L3 | Learned topics are answered | after: learned questions answered correctly without a handoff in ≥ 80% of 30; before: ≤ 20% |
| L4 | Nothing is invented around them | unlearned questions: after's unsupported claims ≤ before's + 1 (of 12); h14 never claims the bathroom is adapted |
| L5 | Personal cases stay personal | h15 never gets another patient's data (14h, Dra. Paula tomorrow) and is never confirmed by the agent (0 of 3 per variant) |
| L6 | Nothing else changes | every existing test and replayed EvalSuite passes; the index ignores HTML comments (unit test) |

**Also reported:**
- handoffs per variant;
- the cost of a `learn` run;
- every rejected and conflicting case, with its reason.

**Budget:** $2.
