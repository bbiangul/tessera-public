# lifecycle-001: lessons that expire, die, contradict and harm (RFC-0008 part C). Pre-registration

- **Date:** 2026-09-27, before any of this is built.
- **RFC:** RFC-0008, part C, with the decisions taken there (validity by
  kind; dead lessons retire only on a person's accept; harmful ones are
  suspended automatically).

## What will be built

1. **Lesson metadata reaches the index.** The HTML comment of each lesson
   (id, kind, learned, review, status, supersedes) is parsed into its
   passages; it is never passage text.
2. **Expired lessons are not used.**
   - A lesson whose `review` date has passed (by the agent's clock) is
     not retrieved.
   - Neither is one with `status=suspended` or `status=retired`.
   - The question then goes to a person, whose answer can teach the
     lesson again.
3. **`tessera learn review --db … --namespace … --lessons file [--apply]`**
   reports, for each lesson:
   - **due:** review within 14 days, or already past;
   - **dead:** not used for 90 days, or never used and learned more than
     90 days ago. Usage is read from the journal: the passages given to
     each free turn, now with the lesson id;
   - **harmful:** of the free turns that used it, how many were followed,
     in the same conversation, by a user message disputing the answer
     (Jev). It is a candidate when disputed at least twice and in at least
     30% of its uses.

   `--apply` suspends the harmful candidates in the file, with the reason
   and the date. Dead and due lessons are only reported.
4. **`tessera learn renew --ids …` and `tessera learn retire --ids …`**
   update the file:
   - `renew` sets the review date to today plus the kind's validity;
   - `retire` sets `status=retired` with a reason.
5. **`tessera learn --lessons file`** compares each proposal with the
   active lessons (Jev: the same question?).
   - The same question with a contradicting answer is reported as
     **contradicts <id>**, not proposed.
   - The same question with a consistent answer is reported as
     **reconfirms <id>**, a renewal for a person to accept.
   - `accept --supersede` retires the old lesson (`superseded-by`).

## Measure

Fixtures are in `bench/lifecycle/`, written now. The lessons file starts
with:
- an **expired price** (Invisalign, review 2026-07-01);
- a **valid price** (sedation);
- a **wrong fact planted as harmful** (children "a partir de 1 ano"; the
  truth is 3);
- a lesson **never used** (bebedouro, learned 2026-03-01).

The eval clock is 2026-09-27. The agent is learn-001's clinic with this
lessons file.

| Part | What runs |
| --- | --- |
| **Expiry** | 6 questions (3 Invisalign, 3 sedation), 3 repeats. **off:** today's binary, which ignores dates. **on:** the new binary. |
| **Usage and harm** | 6 two-turn conversations, live, with `--journal`: 3 use the wrong children lesson and the patient disputes it; 3 use the sedation lesson and the patient is satisfied. Then `learn review --apply`, 3 runs, each on a fresh copy of the lessons file. Then 2 questions after the suspension, 3 repeats. |
| **Contradiction** | 4 answered scenarios: 2 say Invisalign is now R$ 11.500, 2 reconfirm sedation at R$ 250. Then `learn --lessons`, 3 runs. |

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| X1 | Expired lessons stop being said | on: 0 of 9 Invisalign replies state R$ 9.800; off: reported (expected ≥ 5 of 9) |
| X2 | Valid lessons keep being used | on: sedation replies state R$ 250 without a handoff in ≥ 8 of 9 |
| X3 | Harmful lessons are suspended, good ones are not | in ≥ 2 of 3 review runs: the children lesson is a harmful candidate and suspended by `--apply`; the sedation lesson never is (0 of 3) |
| X4 | Suspended lessons are not used | after `--apply`: 0 of 3 replies to a1 say "1 ano"; a2 still answers R$ 250 in ≥ 2 of 3 |
| X5 | Dead lessons are found | bebedouro reported dead in every review run; sedation and children never |
| X6 | Contradictions are held | in ≥ 2 of 3 `learn --lessons` runs: the R$ 11.500 answers are reported as contradicting the Invisalign lesson and not proposed; the sedation answers are reported as reconfirming it |
| X7 | Nothing else changes | knowledge-001's and learn-002's unit tests and `make ci` pass; the index never returns metadata text (unit test) |

**Budget:** $1.5.
