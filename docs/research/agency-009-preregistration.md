# agency-009: answers that the checks dropped. Pre-registration

- **Date:** 2026-09-29.
- **Order:** written after K was built (51b6553) and before any
  conversation of J or K was run on these personas. The code came first
  this time; the measurement is still sealed.

## Question

Do K's two changes turn the robotic or missing answers of the demo run
into plain ones, without making the agent less true?

**What the demo run showed:**
- a question asked while a booking waited ("o total fica R$ 3.920? dá
  pra pagar no Pix?") got a quote of an unrelated sentence of the
  agency's document;
- a minor's travel rule, which the document has, was dropped and handed
  to the team.

**Cause (the recorded Decide calls):**
- the unsupported check drops right answers at 0.53–0.78 over one small
  extra clause;
- the documents fallback then quotes the document without checking the
  sentence answers the question;
- in agent mode, the planner often does not search the documents, so the
  check does not see the rule.

**K's changes:**
1. Every agent-mode turn searches the documents for the message when it
   starts. The planner and the checks see them, and the passages are
   journaled.
2. A document quote is sent only when Jev says it answers the question,
   as web quotes already were (agency-006).

   fallback-001's case still gets its quote in 10 of 10 runs.

## Measure

- **Instrument:** as agency-008:
  - the simulated team;
  - the judge with the world's facts.
- **Data:** `bench/agency/personas-009.json`, written now.
  - **dev:** 12;
  - **test:** 16, sealed.
  - Mostly questions asked while a booking waits (the total, payment,
    cancelling, the bag, pets, assistance), and the agency's rules
    (minors, babies, children at hotels, Portugal, insurance, transfers,
    discounts).
  - At most two dev tuning rounds.
- **World:** two bookings added.
- **Variants:**
  - **J:** the traveler default at 0fec50b (`bench/agency/j9`,
    `tessera-J3`);
  - **K:** 51b6553 (`bench/agency/k9`, `tessera-K`).
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W1 | Fewer robotic quotes | K's agent messages that begin "Pela nossa política, \"" ≤ 50% of J's |
| W2 | Fewer questions left unanswered | K's information lost ≤ 80% of J's |
| W3 | No fewer goals | K's goals ≥ J's |
| W4 | Not less true | K's conversations with a false statement ≤ J's + 1 |
| W5 | Safe and affordable | 0 invariant failures in K; median agent cost ≤ $0.02 |
| W6 | Nothing else breaks | `make ci`, `make lint-extra`, the traveler suites; fallback-001's case 10/10 |

**Decision rule:** K becomes the default if W3, W4 and W5 hold, together
with W1 or W2.

**Budget:** $5.
