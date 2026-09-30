# Router 003: lodging misrouted to flights (#4), pre-registration

- **Date:** 2026-09-25, before any run on the new sets.
- **Problem** ([router-002](router-002.md), finding 2):
  - "flat em sp pra 2 semanas" and "chale em campos do jordao" route to
    search-flights at 0.8–0.94 (Jev): a place and a length of stay read as a
    trip.
  - No threshold catches it.
  - Two rewrites of the descriptions left the misses and made the clean
    holdout worse.
- **Sets:**
  - `traveler-routing-lodging-dev-v1` (14, for building);
  - `traveler-routing-holdout-v5` (22, written now, never run): lodging
    in unusual words (loft, chalé, airbnb, kitnet, glamping, apart hotel),
    trips described as a place and a stay, follow-ups and out-of-scope
    questions;
  - the earlier sets v2 and holdout-v1..v4, for regressions.

## Candidate

`selector.check`: an optional yes/no question a routable Function
declares ("does the message ask for a flight or airline ticket?").
- The router asks every candidate's check **in the same Decide request**
  as the route (one Jev call, as the owner asked for independent
  decisions).
- A route is taken only if the chosen Function's check says yes (p ≥ 0.5).
- Otherwise the next-best option whose check passes and whose probability
  clears the threshold is taken, or nothing is routed (the user is asked
  again: the cheap error).

For the traveler, only search-flights and search-hotels get a check:
- **flights:** "Does the message ask to travel by plane or for an airline
  ticket, not for a place to stay?"
- **hotels:** "Does the message ask for a place to stay (hotel, pousada,
  flat, apartment, room, any lodging)?"

Both texts are fixed now, before any run on holdout-v5. Only the dev set
may change them, and any change is recorded.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| Q1 | The problem reproduces | Baseline: ≥ 3 of holdout-v5's 10 lodging messages go wrong (to flights) |
| Q2 | The check removes it | Candidate: wrong routes on holdout-v5 fall by ≥ 70%, and its right rate ≥ baseline + 15 points |
| Q3 | No regression | On v2 and holdout-v1..v4, wrong + false-route ≤ baseline + 1 point, and right ≥ baseline − 2 points (each set) |
| Q4 | Cost | p50 routing latency ≤ baseline + 150 ms (one request, more questions) |

**Budget:** $1 (Jev, 3 repeats).

## Addendum 1 (after the baseline, before any candidate run)

**The baseline did not reproduce the problem on holdout-v5:** its 10
lodging messages went 100% to search-hotels, so Q1 is not supported
there. The misses are still in the older sets and in dev:

| Message | Routed to |
| --- | --- |
| "flat em sp pra 2 semanas" (holdout-v1) | search-flights, 0.74–0.82 |
| "chale em campos do jordao pro inverno" (holdout-v2) | search-flights, 0.83–0.85 |
| "quarto pra 2 em ouro preto sabado" (dev) | search-flights, 0.65–0.75 |

**The pattern is narrower than holdout-v5 assumed:** an ambiguous lodging
noun, "em <city>", and a date or a stay length. holdout-v5's lodging
messages named lodging more explicitly (airbnb, pousada, hospedagem).

**Holdout-v6** (`traveler-routing-holdout-v6.jsonl`, 12 messages, written
now) holds that pattern by design:
- 8 lodging messages (studio, suite, quarto, flat, cabana, ap, casinha,
  chalé, each "em <place>" plus dates);
- 4 terse trips with the same shape ("recife sexta, volto domingo").

**Q1' (replaces Q1):** baseline ≥ 3 of the 8 lodging messages go wrong on
holdout-v6. **Q2** is then judged on holdout-v6 instead of v5 (wrong routes
−70%, right +15 points). Q3 and Q4 are unchanged, with v5 added to Q3's
regression sets.
