# Traveler free mode 001: does the free mode generalise to a second domain? Pre-registration

- **Date:** 2026-09-25, before any run.
- **Why:**
  - The free mode was measured only on the clinic (freemode-001..005).
  - router-003 found traveler questions that are not searches ("quanto
    custa em média uma diária em Gramado?", "hotel ou flat, qual
    compensa?") routed to hotel searches or unrouted. The traveler has no
    free mode, so an unrouted message gets "Desculpe, não entendi".
  - The owner asked to test the free mode on this use case.
- **Variants** (`bench/traveler-freemode/`, copies of `examples/traveler`):
  - **baseline:** today's traveler.
  - **freemode:** the same plus `freeMode.instructions`:
    - what the assistant does (search flights and lodging, keep
      preferences) and does not (bookings, packages, cars, insurance,
      visas);
    - when a question depends on current data, say so and offer a search.

    It has no reads: the traveler has no knowledge source. By the
    platform's rules the free mode answers only from its facts, so it
    should redirect rather than give general advice. Measuring that is part
    of the point.
- **Set:** 16 off-script messages written now (`messages.json`), never run
  on any variant:
  - general questions (baggage, flight time, best season, visas, pets,
    booking in advance);
  - requests the assistant does not serve (car, package, insurance,
    cancelling elsewhere);
  - small talk.
- **Runs:**
  - each variant's `offscript` EvalSuite, live, 3 repeats (96
    conversations);
  - Jev for decisions, DeepSeek V4.1 Flash (pinned) for text;
  - the judge is Claude Sonnet 5, blind to the variant.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| T1 | Today's traveler dead-ends off-script | baseline: dead ends ≥ 50% of the 48 conversations |
| T2 | The free mode removes the dead ends | freemode: dead ends ≤ 10% |
| T3 | Without inventing | freemode: false claims ≤ 2 of 48, and not more than baseline + 1 |
| T4 | And it is more helpful | freemode: helpful ≥ baseline + 30 points |
| T5 | Searches are unchanged | The traveler's own EvalSuites (flights, hotels, profile) pass live with the free mode; the router is the same, and only unrouted messages reach the free mode |

**Also reported:**
- which messages started a search in each variant (routing is shared, so
  the same ones should);
- which kinds the free mode cannot help with (general advice without
  facts).

**Budget:** $1.5.
