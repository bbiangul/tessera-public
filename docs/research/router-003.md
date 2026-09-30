# Router 003: lodging misrouted to flights (#4), results

- **Date:** 2026-09-25
- **Pre-registration:** [router-003-preregistration](router-003-preregistration.md),
  with addendum 1.
- **Results:** `bench/results/router-003/baseline/`. The traveler router of
  main at `dfcc1ae`, Jev jev-1.13.0, 3 repeats, every set.
- **Spend:** about $0.02.

## Baseline

Right / wrong / false-route / unrouted, at the traveler's threshold of 0.5:

| Set | right | wrong | false-route | unrouted |
| --- | --- | --- | --- | --- |
| v2 (tuning) | 97.8% | 1.1% | 0.0% | 1.1% |
| holdout-v1 | 94.1% | 2.9% | 0.0% | 2.9% |
| holdout-v2 | 91.4% | 3.7% | 0.0% | 4.9% |
| holdout-v3 | 100% | 0 | 0 | 0 |
| holdout-v4 | 100% | 0 | 0 | 0 |
| holdout-v5 (new) | 93.9% | 0.0% | 6.1% | 0.0% |
| holdout-v6 (new, the misrouted pattern by design) | 91.7% | 8.3% | 0.0% | 0.0% |
| lodging-dev-v1 (new) | 78.6% | 7.1% | 14.3% | 0.0% |

**Every lodging message that went to flights**, out of more than 300
messages across the sets:

| Message | Set | Routed to |
| --- | --- | --- |
| "flat em sp pra 2 semanas" | holdout-v1 | flights 0.74–0.82 |
| "chale em campos do jordao pro inverno" | holdout-v2 | flights 0.83–0.85 |
| "chale em monte verde dia 20" | holdout-v6 | flights 0.87–0.89 |
| "quarto pra 2 em ouro preto sabado" | dev | flights 0.65–0.75 |

In the same shape, every other lodging message was right: studio, suite,
quarto duplo, flat in Copacabana, cabana, ap in Lisbon, casinha, loft,
kitnet, airbnb, glamping, apart hotel.

**The other misses** are out-of-scope questions routed to hotels:
- "hotel ou flat, qual compensa mais em sp?" (0.77–0.78);
- "quanto custa em media uma diaria em gramado?" (0.95).

## Hypotheses

| # | Verdict |
| --- | --- |
| Q1 (holdout-v5: ≥ 3 of 10 lodging wrong) | **Not supported:** 0 of 10. |
| Q1' (holdout-v6: ≥ 3 of 8 lodging wrong) | **Not supported:** 1 of 8 ("chalé"). |
| Q2–Q4 | Not run. The candidate was not built: the pre-registered rule required the problem to reproduce first. Q2's "+15 points" also has no room on holdout-v6, whose baseline is already 91.7%. |

## Decision

**#4 is real and narrow:** "chalé", and less often "flat" or "quarto",
with "em <place>" and a date read as a trip. Four messages in more than
300, confident and so not catchable by the threshold.

A general mechanism (`selector.check`, a typed yes/no per Function in the
same Jev request) is **not** built for it now:
- it would add a routing path and a regression surface for a rare error;
- the error's cost is a flight search where a lodging search was wanted.
  The user corrects it in the next message, and nothing irreversible
  happens.

Recommended: lower #4's priority. **Revisit when:**
- real traffic shows the pattern often; or
- the out-of-scope questions routed to hotels ("quanto custa em média uma
  diária…?") become a problem. That is a separate kind of miss, where the
  free mode, not a search, should answer (the traveler has no free mode
  today).

The sets written here (lodging-dev-v1, holdout-v5, holdout-v6) stay for
that measurement, and they are now used.
