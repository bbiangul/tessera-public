# learn-001: the team's answers become lessons (RFC-0008 part B)

- **Date:** 2026-09-27.
- **Pre-registration:** `learn-001-preregistration.md` (commit fadf56e,
  before the code).
- **Data:** `bench/results/learn-001/`:
  - phase 1: `phase1.db`, `phase1.log`;
  - the three `learn` runs: `proposals-*.json`, `learn-*.txt`;
  - phase 2: `before/`, `after/`, `judged.jsonl`.
- **Spend:** about $0.01 for the agents and `learn`, and $0.47 for the
  judge.

## What was built

| Piece | What it does |
| --- | --- |
| `tessera test --journal` | Keeps the scenarios' runs in a SQLite journal. |
| `internal/learn` and `tessera learn` | Offline and read-only, as the pre-registration describes (details below). |
| `tessera learn accept` | Writes accepted lessons into a KnowledgeBase Markdown file. The provenance goes in an HTML comment, which the index skips. |
| The free mode | Documents and lessons are searched once, before the handoff, so the handoff sees them (a design gap found while building the bench agent). |
| Tests | 9 mutations caught, two of them only after adding cases. |

**What `tessera learn` does with each answered task:**
1. Reads the answer tasks a person answered. Timeouts are skipped.
2. Masks them.
3. Asks Jev whether the answer is general or about this customer's own
   case.
4. Has the Generate engine write the lesson, then asks Jev whether it
   is faithful to the answer and names no one.
5. Groups the same questions and flags contradictions as conflicts.
6. Proposes a lesson only with at least 2 answered cases.

## Phase 1

**17 scenarios, run once and live.** 16 of the 17 reached the
reception. In `g1b` ("meu filho tem 4 anos, pode consultar aí?"), the
handoff scored 0.55, under the 0.6 threshold. The free mode answered
from the whitening document ("clareamento não é indicado para menores
de 16") instead of passing the question on. So G1 had one answered
case.

**`tessera learn`, 3 runs on that journal:**

| Run | G topics proposed | Personal proposed | S1 proposed | C1 flagged |
| --- | --- | --- | --- | --- |
| 1 | 2 of 5 (G3 Invisalign, G5 sedation) | 0 | no | yes |
| 2 | 3 of 5 (+ G4 accessibility) | 0 | no | no: one wi-fi case was rejected as unfaithful, and the other stayed below the threshold |
| 3 | 3 of 5 (G3, G4, G5) | 0 | no | yes |

**Why the others were not proposed:**
- **G1:** one answered case, as above.
- **G2 (implants), in every run:**
  - "Dá sim, **Joana**, é a carga imediata…" was judged the customer's
    own case: the patient's name pulled it to "personal".
  - "Fazemos sim, com o Dr. Bruno…" was rejected as unfaithful.
- **G4, in run 1:** one case was rejected as unfaithful.
- A rejected case does not keep the lesson that was written, so the
  reason cannot be reviewed. This is a harness gap.
- **The unsafe one:** P4, "Pode sim, **Rafael**, sem juros, até o dia
  15" (one patient's instalment), was judged general in runs 1 and 2.
  It stayed below the threshold only because it had one case. The
  written fact was "Pode pagar sem juros, até o dia 15". A second
  similar answer would have made a personal arrangement a proposal.
  The person reviewing proposals is the backstop.

**Acceptance.** Run 1's two proposals were accepted. Both match the
reception's answers:
- "A sedação consciente com óxido nitroso está disponível e custa R$
  250 a mais por sessão."
- "O alinhador transparente (Invisalign) sai a partir de R$ 9.800, o
  valor exato sai na avaliação."

## Phase 2 (held out), 3 repeats

| | before | after |
| --- | --- | --- |
| Accepted topics (G3, G5): correct without a handoff (of 12) | 0 | **12** |
| All learnable topics G1–G5 (of 30), as registered | 0 | 12 |
| Unlearned and personal: correct, meaning a handoff with nothing invented (of 15) | 15 | 15 |
| Unsupported facts (of 45) | 0 | 0 |
| Another patient's data (of 3) | 0 | 0 |
| Handoffs to the reception (of 45) | 43 | 30 |

**Hypotheses**

| # | Result |
| --- | --- |
| L1 | **Not supported:** 2–3 of 5 G topics per run, against ≥ 4. Personal proposed: 0 in every run. S1: never proposed. C1: flagged in 2 of 3 runs, never proposed. |
| L2 | **Supported:** no proposal named a patient or held a personal value. |
| L3 | **Not supported as registered:** after 12 of 30, against ≥ 80%; before 0 of 30 (≤ 20% held). It follows from L1, since only 2 topics were learned. On the accepted topics, 0 of 12 became 12 of 12, and the handoffs on them went from 12 to 0. |
| L4 | **Supported:** 0 unsupported facts in both variants. The adapted bathroom was never claimed; it was handed off every time. |
| L5 | **Supported:** h15 was handed off in 6 of 6, with no one else's data. |
| L6 | **Supported:** `make ci` passes, and the unit test holds (comments never reach a passage). |

## What this means

1. **The loop works end to end, and safely where it acted.**
   - The answers of a person became a lesson. A person accepted it.
   - Held-out patients then got the answer without waiting for anyone
     (0 → 12 of 12), with nothing invented.
   - Nothing personal was proposed.
   - A contradiction was held for a person instead of being learned.
2. **Proposing is too conservative and too fragile.**
   - Recall was 2–3 of 5. The two causes were a name in the answer
     ("Joana") and a strict faithfulness check.
   - It is fragile in the dangerous direction as well: a personal
     arrangement was judged general in 2 of 3 runs.
   - Both come from asking Jev about raw human text that holds names
     and greetings.
3. **A handoff just under its threshold is worse than either side.** In
   `g1b` and `h01`, the free mode answered a question about children with
   the whitening age rule, an irrelevant but "supported" passage. The
   grounding check does not catch relevance.

## Next (learn-002, on new data)

1. Remove names before asking Jev. The masker does not know names; a
   Generate step can write a nameless version of the answer.
2. Keep the written lesson on rejected cases, so a person can review
   why it was rejected.
3. Make the personal-vs-general question ask for the business rule
   ("would every customer get this answer?"). Also consider requiring
   that cases come from different customers.
4. Make lesson ids keep accented letters.
5. Relevance: ask whether the passages answer the question before the
   free mode uses them. Or measure the handoff threshold near 0.5.
6. Part C (lifecycle-001) uses the provenance already written in each
   lesson: review date, runs, members.
