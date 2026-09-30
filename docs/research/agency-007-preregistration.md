# agency-007: the pending branch, with the simulated team. Pre-registration

- **Date:** 2026-09-29, before the fixes are built.
- **Follows:** agency-006 and team-001.

## Question

While a process waits (a booking asking for the travellers' details), do
the fixes below stop the agent from repeating the pending question
instead of answering what the customer asks?

In agency-006 (p11, p09), "não tenho os dados agora… me confirma se é o
total pra dois?" got only the pending question, three times.

**Causes:**
- the classifier was unsure, and an unsure message only repeats the
  pending question;
- a side answer the checks dropped also left only the pending question.

**Variant I has three fixes:**
- an unsure message that asks something (a "?") is handled as a
  question: it is answered, and the pending question is not forced
  again;
- a dropped side answer becomes `messages.unsure`, not silence;
- in agent mode, side answers also get the web search.

## Measure

- **Instrument:** as agency-006, with the simulated team (team-001):
  what no persona scripts, the team answers from
  `bench/agency/world/team-facts.md`.
- **Data:** `bench/agency/personas-007.json`, written now.
  - **dev:** 12 new personas;
  - **test:** 16 new personas, sealed.
  - Several goals combine a pending booking with a question.
  - At most two dev tuning rounds.
- **World:** five new bookings.
- **Variants:**
  - **H:** the current traveler default (`bench/agency/h6`,
    `tessera-H3`, c077a8d, simulated team included);
  - **I:** H with the fixes above.
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| U1 | Less lost | I's information lost ≤ 70% of H's |
| U2 | Fewer repeats | I's redundant messages ≤ 80% of H's |
| U3 | No fewer goals | I's goals ≥ H's |
| U4 | Not less true | I's conversations with a false statement ≤ H's + 1 |
| U5 | Safe and affordable | 0 invariant failures in I; median agent cost ≤ $0.02 |
| U6 | Nothing else breaks | `make ci`, `make lint-extra`, the traveler suites; the interruption classifier's prompt unchanged |

**Also reported:**
- the simulated team's answers per variant;
- conversations still ending at an unanswered task.

**Decision rule:** I becomes the traveler's default if U1 or U2 holds,
together with U3, U4 and U5.

**Budget:** $5.

## Deviation (recorded before any test conversation was judged)

The judge's prompt counted what the team answered as unsupported. With
the simulated team, that happened often. Examples: a booking's locator,
or a price given by the team and relayed by the agent.

**Now the prompt says:**
- what the team ("human") answered is true about what it did and said;
- a task in "queue" shows the agent passed the request on.

The dev round is judged again with it, and the test is judged only with
it.
