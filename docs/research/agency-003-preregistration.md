# agency-003: loops at the confirmation, replies to the team, booking status. Pre-registration

- **Date:** 2026-09-28, before any change for this study.
- **Follows:** agency-002: natural 2.94, trust 14/36, goal 53%. The
  failures left were:
  - a yes with a question at the confirmation, looping;
  - replies to the team taken as side questions;
  - "não tenho os dados, dá para reservar sem?" answered with the same
    question;
  - booking status dropped by the grounding check.

## What will be built

1. **The flight's extras visible while booking.**
   - `book-trip` reads the chosen flight's extras.
   - The free mode sees them at the confirmation (`runReads`).
   - The confirmation offers the bag with its price.
2. **Replies to the team are relayed as they are.** A wait that relays a
   conversation with a person takes every reply, with no side-question
   classification (platform).
3. **The rest of a reply.** A message that answers a pending question and
   asks what the run's next message does not answer gets it answered
   (platform, as agency-002's rest of a request).
4. **Booking status as a Function** (`minha-reserva`): locator, state,
   flight and next step, by template, from the agency's system.

## Measure

- **Instrument:** as agency-001 and 002 (llm simulator, world sandboxes,
  blind Claude Sonnet 5 judge, `bench/agency/report.py`).
- **Data:** `bench/agency/personas-003.json`, written now.
  - **dev:** 12 new personas. At most **two** tuning rounds after the
    first measurement (agency-001 and 002 overfit their dev sets over
    four rounds).
  - **test:** 12 new personas, sealed.
  - The world gains eight existing bookings for the personas that ask
    about or change one.
- **Variants:**
  - **C:** the traveler and platform of be2a4d7 (`bench/agency/c2`,
    binary built at that commit);
  - **D:** after the changes.
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| M1 | More goals | D goal ≥ 65% and ≥ C + 15 points |
| M2 | Fewer loops | D's redundant messages ≤ 60% of C's |
| M3 | More natural | D mean natural ≥ C + 0.4 |
| M4 | Not less true | D's conversations with a false statement ≤ C's |
| M5 | Safe and cheap | 0 invariant failures; median agent cost ≤ $0.02 |
| M6 | Nothing else breaks | routing and fallback-001 set the same day against C (as agency-002 K6); suites live; `make ci`, `make lint-extra` |

**Also reported:** lost, trust, per persona, and agency-001's bars
(goal 75%, natural 4.0).

**Budget:** $6.
