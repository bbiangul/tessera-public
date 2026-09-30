# agency-006: agent mode's open points. Pre-registration

- **Date:** 2026-09-29, before measuring.
- **Follows:** agency-005. Agent mode became the traveler's default at
  77% of goals, but it lost more information than before, and more
  conversations ended at a question left with the team.

## Question

Do the fixes of 3681ff6 cut the information agent mode loses and the
questions it leaves with the team, without costing goals or truth?

**agency-005's diagnosis:**
- **Loops:** the same Function run again with the same inputs, so the
  same message went out 3–4 times.
- **Partial details held:** "meus dados são…, os da minha esposa
  amanhã" was held as "later", and the given details were lost.
- **Who travels:** a 15-year-old flying alone was counted with a parent.
- **Hand-offs after loops:** most questions left with the team were
  asked for by the customer after the agent went in circles.

**H has these fixes:**
- the same Function with the same inputs is not run twice in a row;
- a partly given answer reaches the process;
- who travels is counted by age 12 or more;
- evals fail a turn agent mode could not plan.

## Measure

- **Instrument:** as agency-005.
  - The simulator's stall line now names the queue and the task's kind.
  - `report.py` counts separately a question left with the team
    (`stalled_question`) and a request the team completes, such as a
    booking (`stalled_booking`).
- **Data:** `bench/agency/personas-006.json`, written now.
  - **dev:** 12 new personas;
  - **test:** 16 new personas, sealed.
  - Every persona whose goal ends in a booking has the team's booking
    answer, and every one whose goal needs a person has the team's
    answer.
  - At most two dev tuning rounds.
- **World:** seven new bookings.
- **Variants:**
  - **G:** agent mode as it shipped in agency-005 (`bench/agency/g5`,
    `tessera-G`);
  - **H:** the traveler and platform of 3681ff6 (`bench/agency/h6`).
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| R1 | Less lost | H's information lost ≤ 70% of G's |
| R2 | Fewer questions left with the team | H's `stalled_question` ≤ 60% of G's |
| R3 | No fewer goals | H's goals ≥ G's |
| R4 | Not less true | H's conversations with a false statement ≤ G's |
| R5 | Safe and affordable | 0 invariant failures in H; median agent cost ≤ $0.02 |
| R6 | Nothing else breaks | `make ci`, `make lint-extra`, the traveler suites live |

**Decision rule:** H replaces G as the traveler's default if R1, R3, R4
and R5 hold.

**Budget:** $5.
