# Traveler (reference tenant)

**Status:** the port is done except the onboarding questions on the first
message, which need phase 2 `Trigger`s. 24 conversations (13 flights, 6
hotels, 5 profile), 3 live runs green on Jev, replayed in CI.

The travel assistant from the legacy platform, rebuilt on Tessera (RFC-0001,
"Casos de referência"). It is being ported piece by piece; each piece is
checked against the original.

| Piece | Status | Evidence |
| --- | --- | --- |
| `scoreFlights` → `scripts/scoring.star` | Ported | `scoring_test.go`: parity with the original bash on 14 synthetic fixtures, mutation-checked |
| Seat availability → `scripts/seats.star` | Ported | parity on 4 fixtures; bassinet seats counted once (the original counted a row per bassinet seat) |
| `scoreHotels` → `scripts/hotels.star` | Ported | parity on 7 fixtures; two bugs fixed (long stays never got the apartment bonus; a hotel with no price ranked as the cheapest) |
| Booking.com flight search as an `http` plugin | Done (`plugins/booking.yaml`) | the key is a secret reference; admission refuses a literal one |
| Flight search Function | Done (`functions/search-flights.yaml`) | one Generate call extracts the request; only missing details are asked |
| Flights EvalSuite | 6 scenarios (`evals/flights.evalsuite.yaml`) | pass live with Jev + DeepSeek V4.1 Flash for $0.0007; replayed in CI |
| Follow-up questions ("e a bagagem do segundo?") | Done (`functions/flight-details.yaml`, `plugins/memo.yaml`) | the shown flights are stored with the built-in sql plugin; Jev picks the flight: 96.9% on 32 references (`docs/research/decide-followups-001.md`) |
| Hotel search Function | Done (`functions/search-hotels.yaml`) | the original's destination choice and kind of lodging by length of stay, in `scripts/hotels.star`; prices compared per night (the API's price is the whole stay) |
| Hotels EvalSuite | 6 scenarios (`evals/hotels.evalsuite.yaml`) | 36/36 in 3 live runs with Jev + DeepSeek V4.1 Flash, about $0.001 a run; replayed in CI |
| Traveler profile (`plugins/profile.yaml`, `functions/set-preferences.yaml`) | Done | preferences told in conversation, saved with the sql plugin; a save only changes what was said; both searches read it (home city, avoided airlines, loyalty, price or time) |
| Profile EvalSuite | 3 scenarios (`evals/profile.evalsuite.yaml`) | 45/45 over 3 live runs of the whole traveler suite; a planted defect (searches ignoring the profile) fails 2 of them |
| Airport-change risk (`scripts/connections.star`, `plugins/routes.yaml`) | Done | the legacy platform's 04b: detection and risk rules with parity on 9 fixtures (one bug fixed: every drive time was marked estimated); Google Routes' route matrix answers every pair in one call (the original: one call per pair); without Routes the drive is estimated and the risk still shown |
| Flights EvalSuite, airport changes | 2 more scenarios (8 in `evals/flights.evalsuite.yaml`) | the risky cheapest flight drops to third with a warning; also with Routes down (`state: { routes: { down: true } }`) |
| Currency (`scripts/currency.star`) | Done | the legacy platform's determineCurrency with parity on 16 fixtures; two bugs fixed (short words matched anywhere: "Pernambuco" was AUD; Portuguese city names unknown) |
| the legacy platform's bugs as scenarios | BUG-002 (named airport), BUG-003 (round trip is one search), BUG-005 (children's ages), BUG-007 (currency) | checked on what was sent (`expect.effects[].where`); a planted regression of BUG-005 and BUG-007 fails them |
| Add-ons (`functions/flight-extras.yaml`, `scripts/extras.star`) | Done | the legacy platform's AnalyzeBaggageOptions and AnalyzeFlexibleTicket: checked-bag, flexible-ticket and insurance prices from one getSeatMap call, read by code (the original had a model read the response); 2 scenarios, including the API down |
| Routing with five Functions | 100% on the extras holdout, no false routes on 6 sets | follow-ups guarded by `selector.after`; threshold 0.5 set from the benchmark ([router-002](../../docs/research/router-002.md), addendum 2) |
| Onboarding questions on the first message | Next | needs phase 2 Triggers |

Run it:

```bash
tessera test examples/traveler --decide typesafe:jev-1.13.0 \
  --generate openrouter:deepseek/deepseek-v4.1-flash          # live, budgeted
make test-evals                                                # replayed, offline
tessera chat examples/traveler --namespace traveler --caller userId=me
```

The chat needs `TESSERA_SECRET_BOOKING_RAPIDAPI_KEY` for real searches and
`TESSERA_SECRET_GOOGLE_ROUTES_API_KEY` for drive times between airports.

## Parity with the original

`testdata/score_flights/*.input.json` are synthetic inputs
(`testdata/gen_score_flights.py`); `*.legacy.json` is what the legacy platform's
bash + jq printed for them, recorded once with a parity script that read
the legacy code from its own checkout. The script and the legacy code are
not part of this repository; the recorded outputs are.
Intended differences (bugs fixed by the port) are listed at the top of
`scripts/scoring.star` and asserted in the test.
