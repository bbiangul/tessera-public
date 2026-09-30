# agency-008: discounts and rescheduling. J becomes the default

- **Date:** 2026-09-29.
- **Pre-registration:** `agency-008-preregistration.md` (85565f8), with its
  deviation (the judge sees the world's facts; `remarcar-voo` in dev
  round 2).
- **Data:** `bench/results/agency-008/`.
- **Variants:**
  - **I:** agency-007's default, `tessera-I2`;
  - **J:** d8b5c3c, `tessera-J2`, `bench/agency/j8`.
- **Spend:** about $5.

## What J changes

1. **`docs/agencia.md` matches the world and holds its general policies.**
   - Three statements were wrong:
     - Pix "na hora" (it confirms in up to 2 hours);
     - the team renting cars (the agency does not);
     - tours in any city (only in Natal and Maceió).
   - Added: discounts, the airlines' change and cancellation fees,
     minors, babies, pets, transfers, insurance and tours.
   - Per-hotel details stay with the team.
2. **`remarcar-voo`,** a flight leg rescheduled by code:
   - the booking is read from the back office;
   - the same airline is searched on the new day, one way;
   - new fare − fare paid + the airline's fee is computed in
     `scripts/reschedule.star`, with tests and mutation checks;
   - the estimate is the confirmation of `agency/request-change`
     (irreversible, keyed);
   - the team then gets the complete request.
3. **The team's task from `falar-com-agente`** carries the customer's
   bookings.

**Also:** the agent's instructions offer the team only for what the
agency does. A suite scenario for the weather question failed 1 in 3
times before this change and 0 in 5 after.

## Result (sealed test: 16 new personas × 3, judged with the world's facts)

| | I | J |
| --- | --- | --- |
| Goal met (of 48) | 28 (58%) | **38 (79%)** |
| Conversations with a false statement | 18 | **5** |
| False statements | 26 | 13 |
| Information lost | 23 | 22 |
| Redundant messages | 63 | 60 |
| Natural (1–5) | 2.94 | 3.48 |
| Trust (of 48) | 20 | 35 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0109 | $0.0087 |

| # | Hypothesis | Result |
| --- | --- | --- |
| V1 | J goals ≥ I + 6 | **Supported:** 38 against 28 |
| V2 | J conversations with a false statement ≤ I's | **Supported:** 5 against 18 |
| V3 | Rescheduling: ≥ 9 of 12 | **Not supported:** 6 of 12 |
| V4 | Discounts answered by the agent: ≤ 3 of 12 sent to the team | **Supported:** 0 of 12 |
| V5 | 0 invariant failures; median cost ≤ $0.02 | **Supported** |
| V6 | CI, lint, suites | **Supported:** 40/40 live; routing unchanged (traveler-routing-v1 98.8% before and after; route-dev 95%) |

**Decision (the pre-registered rule):** V1, V2 and V5 hold, so J becomes
the traveler's default.

**Most of I's false statements were its own document's errors,** now
judged against the world: "Pix na hora", car rental, tours everywhere.

**Per persona (goals of 3, I → J):**
- **Improved:**
  - a flexible ticket cancelled (v13: 0 → 3);
  - moving a return with a seat (v11: 1 → 3);
  - installments and a coupon (v16: 1 → 3).
- **Worse:** "something cheaper" for a hotel (v06: 3 → 1). The agent
  re-sent the same quote.

## Why rescheduling missed V3, fixed after the test (not in the numbers)

- **v02 (0/3): two bugs in `remarcar-voo`.**
  1. The route search took the first place found. "NAT" is inside
     "Guarulhos International", so it searched GRU→GRU and found no
     flight, eight days in a row. Now it takes the airport with that code.
  2. The customer had two bookings and named one by its locator. Each new
     message asked "qual reserva?" again. Now a locator written in the
     conversation picks the booking.

  Suite: `agency/reschedule-two-bookings`. It fails with the old airport
  pick and passes twice with the fix.
- **v15 (1/3):** changing a hotel's dates goes to the team, which the
  relay loops on.
- **v05 (2/3):** after the change, the customer asked for the payment
  link of the difference, and the agent showed the booking's old status.

## Still open

- "Something cheaper" re-sends the same quote (v06).
- Changing a hotel's dates has no process (v15).
- After a change, a payment link for the difference (v05).
