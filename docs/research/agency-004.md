# agency-004: agent mode against the router

- **Date:** 2026-09-28.
- **Pre-registration:** `agency-004-preregistration.md`, with its
  deviations (44cbfc3).
- **RFC:** RFC-0010; ADR-0007.
- **Data:** `bench/results/agency-004/`. The runs used are `D-dev2`,
  `E-dev2`, `D-test2` and `E-test`, and the judgments `test-judged*.jsonl`.
  The first D runs and E's dev round 1 used the old instrument and are
  kept only as a record.
- **Variants:**
  - **D:** the traveler of c06884c (`bench/agency/d4`, router and free
    mode), binary `tessera-D4b`: c06884c plus only the simulator fix;
  - **E:** the same traveler with `freeMode.agent: {maxSteps: 5}`
    (`bench/agency/e4`), platform 276de1d, planner DeepSeek V4.1 Flash.
- **Spend:** about $4.5.

## Result (sealed test: 16 new personas × 3)

| | D | E |
| --- | --- | --- |
| Goal met (of 48) | 24 (50%) | 28 (58%) |
| Conversations with a false statement | 18 | 11 |
| False statements | 31 | 12 |
| Information lost | 62 | 44 |
| Redundant messages | 81 | 58 |
| Natural (1–5) | 2.69 | 3.10 |
| Trust (of 48) | 16 | 22 |
| Invariant failures | 0 | 0 |
| Ended at a team task the scenario did not script | 15 | 21 |
| Median agent cost | $0.0032 | $0.0055 |

| # | Hypothesis | Result |
| --- | --- | --- |
| P1 | E goal ≥ 70% and ≥ D + 10 points | **Not supported:** 58% and +8 points |
| P2 | Natural ≥ D + 0.5 | **Not supported:** +0.41 |
| P3 | Conversations with a false statement ≤ D's | **Supported:** 11 and 18 (false statements 12 and 31) |
| P4 | 0 invariant failures; effects only after their confirmation | **Supported** |
| P5 | Median agent cost ≤ $0.02 | **Supported:** $0.0055 |
| P6 | With agent mode off, nothing else changes | **Supported:** `make ci`, `make lint-extra`, traveler suites 36/36 live. The router and fallback code are unchanged (`theRest` only moved), so the routing and fallback-001 sets were not re-run. |

**Decision (the pre-registered rule):** P1 fails, so agent mode stays
experimental and off by default.

**Per persona:**
- E met more goals than D for 6 personas, fewer for 2, and the same for
  8. A sign test gives p ≈ 0.29.
- **Where E gained:** questions in the same message as a request (w01
  installments, w13 three destinations, w03 a budget) and w10 "já paguei".
- **Where E lost:** w15 (Lisboa, 3/3 → 1/3) and w05 (2 → 3 people).

## What E does better

- **It is more truthful:** less than half the false statements (12 vs
  31). The planner writes from what its tools returned in the same turn,
  and each reply still passes the unsupported and claims checks.
- **It repeats and loses less:** a request with a question gets the
  process's answer and then the rest, in one turn.

## Where E fails (for a next study)

1. **A dropped rest.** A process answered part of a message and the
   planner's add-on reply failed a check. The rest of the message was
   then lost silently (w15: the documents question after the quote).
2. **The team instead of the web.** A public-information reply the
   checks drop goes to `whenUnsure`, so the team is asked what a web page
   answers (w15, w08). E ended 21 conversations at a team task the
   scenario did not script, against D's 15.
3. **A pending process still loops.** With a run pending, the Jev path
   is unchanged. "I don't have the names now" gets the same question
   again (g03, g06). The RFC's planner for the pending branch is not
   built yet.

## Instrument changes (deviations, before the test)

- Four personas whose goal only a person meets got team answers.
- The simulator gives a task a scripted answer of its shape (`fitting`).
- Both changes applied to D and E alike. D was re-run whole.

**Also seen:** the judge counts what the scripted team says as
unsupported (g07, g10), as in agency-003.
