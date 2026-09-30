# router-004: general travel questions routed to searches. Pre-registration

- **Date:** 2026-09-25, before any change.
- **Issue:** #48.
- **The problem:**
  - In traveler-freemode-001, 11 of the 14 remaining dead ends were
    general questions ("qual bairro é melhor", "hotel ou flat", "quanto
    tempo de voo") that the router sent to `search-hotels` or
    `search-flights`. The search then asked "Qual o dia do check-in?"
    with no word on why.
  - In websearch-002, "quanto tempo de voo de Recife pra Lisboa?" was
    routed to the flight search 6 of 6 times, so web search never saw it.

## The change (tuned on `dev` only)

The two searches' `selector.description` also say what they are not
for: general questions about a destination or a trip that ask for no
options. Examples of such questions:
- how long a flight takes;
- which neighbourhood;
- hotel or flat;
- which airport;
- whether a place is expensive;
- which airline flies there.

Those go to the free mode, which answers them (with web search when it
is on). Nothing else changes.

## Sets

- **Development:** `bench/datasets/traveler-routing-general-dev-v1.jsonl`,
  12 messages (7 `none`, 5 searches).
- **Test:** `bench/datasets/traveler-routing-general-test-v1.jsonl`, 18
  messages written now and sealed (sha256 prefix `eb2104ec34a893a3`):
  - 10 general questions (`none`);
  - 8 real searches, near misses of the same places and topics.

  Labels follow one rule: a message asking for options or prices of a
  trip or a stay, with or without dates, is a search. A question whose
  answer does not depend on a date or availability is `none`.
- **Regression:** the used routing sets (`traveler-routing-v1`, `v2`,
  holdouts `v1`–`v6`, `lodging-dev-v1`), 302 messages. They are used for
  regression only, never for tuning.

## Runs

- `tessera bench route`, Jev 1.13.0, 3 repeats.
- The traveler's threshold (0.5).
- **A** is `examples/traveler`; **B** is A with the new descriptions.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| R1 | On test `none`, B routes to a search in ≤ 10% of the 30 runs; A is reported alongside |
| R2 | On test searches, B routes correctly in ≥ 90% of the 24 runs, and ≥ A − 1 run |
| R3 | On the regression sets, B's correct routes are ≥ A's − 1 point, and its wrong routes are ≤ A's + 1 point |

**Budget:** $1.
