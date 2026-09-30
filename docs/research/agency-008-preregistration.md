# agency-008: discounts and rescheduling. Pre-registration

- **Date:** 2026-09-29, before J is built.
- **Follows:** agency-007.
  - Coupon: 1/3 goals.
  - Rescheduling: 1/3 goals.

## Diagnosis (agency-007 test)

- **Rescheduling.** The team got "quanto fica remarcar?" with no booking
  and no new fare, and asked the customer for the airline. The agent
  never searched the new date and waited forever.
- **Coupons.** The agent did not know the discount policy. It sent the
  question to the team, and re-sent the same quote to "algo mais
  barato".
- **The agency's document (`docs/agencia.md`) contradicts the world:**
  - it says Pix is approved "na hora" (the world: up to 2 hours);
  - it says the team rents cars (the world: it does not);
  - it says the team sells tours in any city (the world: only in Natal
    and Maceió).

## Variants

- **I:** the traveler default (agency-007), `tessera-I2`,
  `bench/agency/h6`.
- **J:** I with the following:
  - `docs/agencia.md` aligned with the world and completed with its
    general policies: discounts, airline fees for changes and
    cancellations, minors, babies, pets, transfers, insurance and tours.
    Per-hotel details stay with the team;
  - the free mode's instructions: how to reschedule (the booking, the
    airline's fee, the new date's fare searched, a confirmation, then the
    team with everything);
  - `falar-com-agente`: the team's task carries the customer's bookings.

## Measure

- **Instrument:** as agency-007, with the simulated team.
- **World:** the team's sheet gains how it computes a reschedule's total
  and hotel date changes, and 12 bookings are added. Both variants see
  them.
- **Data:** `bench/agency/personas-008.json`, written now.
  - **dev:** 12 new personas;
  - **test:** 16 new personas, sealed. 11 of the 16 are about discounts,
    rescheduling or cancellation.
  - At most two dev tuning rounds.
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| V1 | More goals | J's goals ≥ I's + 6 (of 48) |
| V2 | Not less true | J's conversations with a false statement ≤ I's |
| V3 | Rescheduling works | J's goals on the reschedule personas (v02, v05, v11, v15: 12 conversations) ≥ 9 |
| V4 | Discounts are answered by the agent | in J, the team is asked about coupons or discounts in ≤ 3 of the 12 conversations of v01, v03, v14 and v16 |
| V5 | Safe and affordable | 0 invariant failures in J; median agent cost ≤ $0.02 |
| V6 | Nothing else breaks | `make ci`, `make lint-extra`, the traveler suites |

**Decision rule:** J becomes the traveler's default if V1, V2 and V5
hold.

**Budget:** $5.

## Deviation (recorded before any test conversation of J was run or any test was judged)

**Judge:** it counted the agency's policies as unsupported. In agency-008
dev, "Pix confirma em até 2 horas" was flagged, because the judge saw only
system calls and the team's answers. Now it gets the world's facts
(`bench/agency/world/team-facts.md`) as the truth about policies, rules
and fees:
- a statement that contradicts them is false;
- one they support is true.

This is the same sheet the simulated team answers from, and it applies to
both variants. The dev rounds are judged again with it.

**J's dev round 2 adds `remarcar-voo`,** after dev round 1 showed the
planner pricing a round trip, picking another airline and never
computing the total:
- code reads the booking, searches the same airline on the new date,
  and computes new fare − fare paid + fee;
- the customer confirms;
- then the team gets the complete request.

It comes with the back-office action `agency/request-change`
(irreversible, keyed), added to both worlds' sandboxes.
