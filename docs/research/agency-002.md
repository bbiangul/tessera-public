# agency-002: the agency remembers the conversation

- **Date:** 2026-09-28.
- **Pre-registration:** `agency-002-preregistration.md` (commit 2681068).
- **Data:** `bench/results/agency-002/`.
- **Variants:**
  - **B:** `bench/agency/b1` on the platform of bde7a46;
  - **C:** d835b8b.
- **Spend:** about $4, the judge most of it.

## Result (sealed test: 12 new personas × 3)

| | B | C |
| --- | --- | --- |
| Goal met (of 36) | 19 (53%) | 19 (53%) |
| Information lost | 33 | 33 |
| Natural (1–5) | 2.39 | **2.94** |
| Trust (of 36) | 6 | **14** |
| Conversations with a false statement | 14 | 14 |
| False statements | 18 | 14 |
| … about a reservation, payment or emission (by the judge's list) | 6 | 7 |
| Redundant messages | 55 | 49 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0032 | $0.0035 |

| # | Hypothesis | Result |
| --- | --- | --- |
| K1 | Lost ≤ half of B's | **Not supported:** 33 and 33 |
| K2 | Goal ≥ 65% and ≥ B + 15 points | **Not supported:** 53% and 53% |
| K3 | Natural ≥ B + 0.5 | **Supported:** +0.55 |
| K4 | Not less true; no reservation statement the calls contradict | **Not supported:** conversations with a false statement 14 and 14; 7 flagged about the booking state |
| K5 | Invariants; median cost ≤ $0.02 | **Supported** |
| K6 | Regressions the same day | **Supported:** routing right 95.9% vs 95.8%, wrong 1.5% vs 1.5%, out of scope 0.7% vs 0.9%; fallback-001 set controls 36/36 both, overreach on real traps 3/27 both; suites 34/34 live; `make ci`, `make lint-extra` |

**Dev said otherwise.** On the 8 dev personas, one repeat, C met 7 goals
against B's 3, with 0 lost against 20 and natural 4.38 against 2.0. The
test does not repeat it. This is the second time dev overstated the
gain (agency-001 too). Dev personas were iterated on four times, and
their failures were fixed one by one. What is fixed there does not carry
to new customers as much as it looked.

## Why, from the test's transcripts

The failures that remain are mostly not the ones built for:

1. **A yes with a question, again and again** (u05).
   - The customer said "sim, pode reservar. E a mala?" at the
     confirmation, seven times. A yes with a question is not a yes, which
     is correct and measured (freemode-005).
   - The agent could not answer the bag's price at that moment: the free
     mode has no access to the flight's extras.
   - So the customer asked again, and the confirmation came back each
     time.
2. **The customer answers the team with a question** (u04). The team
   asked "Quer que eu cancele?" and the customer asked the team something
   back. The platform took the question as a side question for the free
   mode. It answered with an unrelated ANAC rule and repeated the team's
   message, three times.
3. **"Não tenho os dados, dá para reservar sem isso?"** (u06, u02). This
   is read as an answer to the travelers question. Nothing is extracted,
   and the same question comes back without the customer's doubt
   answered.
4. **Booking status** (u12). The free mode had the booking (locator,
   status, flight) and wrote it. The grounding check dropped two of three
   drafts, and the customer heard "não consigo confirmar". A booking's
   state is a system fact: it belongs in a Function with a template, not
   in the free mode.

**What did carry** is seen in natural (+0.55) and trust (14 against 6):
- fewer loops;
- the policy said instead of repeated demands;
- the team reached when a city is not found;
- earlier details used when the customer gave them before a request
  (u01, u09).

## What was built (d835b8b)

**Platform:**
- `extract.conversation`: a value may come from the user's earlier
  messages. Runs keep 16 lines from before. `format` inputs look there
  too, by pattern.
- `extract.future`: a date more than a week past moves a year on.
- The rest of a message a Function did not answer is answered first by
  the free mode, with the web and the documents.
- Preconditions are templates ("Entendi a ida como …, que já passou").
- The quote fallback quotes a web page, with its site, when there are no
  documents.
- The claims guard's retry no longer tells the model that the team reads
  the conversation.

**Traveler:**
- conversation-aware inputs and future dates;
- travelers and e-mail kept;
- the holding policy written in the agency's documents;
- picks by price and position, and a checked bag added to a booking;
- a conversation with the team (one reply round);
- the team looks for a city the search does not find;
- narrower selectors.

## Next (agency-003, with new personas again)

1. **The flight's extras visible while booking.** `book-trip` reads the
   chosen flight's extras. The free mode sees them at the confirmation
   (`runReads`), and the confirmation offers the bag with its price.
2. **Replies to the team go to the team.** A wait that relays a
   conversation with a person takes every reply as it is: no side-question
   classification.
3. **The rest of a reply**, not only of a request. A message answering a
   pending question and asking something the run's reply does not answer
   gets that answered.
4. **Booking status as a Function** (`minha-reserva`): the locator, the
   state, the flight and the next step, by template.
5. **Less dev tuning.** Four dev iterations overfit twice. Next time: two
   dev rounds at most, and a larger dev set.
