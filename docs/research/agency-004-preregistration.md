# agency-004: agent mode against the router. Pre-registration

- **Date:** 2026-09-28, before agent mode is built.
- **RFC:** RFC-0010; ADR-0007.

## Question

Does an LLM planning each turn over the traveler's Functions and reads
(agent mode) serve new customers better than the router and the free mode,
with the same safety?

## Measure

- **Instrument:** as agency-001..003.
  - the llm simulator (DeepSeek V4.1 Flash);
  - the world's sandboxes;
  - the blind Claude Sonnet 5 judge with the same prompt;
  - `bench/agency/report.py`.
- **Data:** `bench/agency/personas-004.json`, written now.
  - **dev:** 12 new personas;
  - **test:** 16 new personas, sealed. That is 48 conversations per
    variant, for more power than the 36 before.
  - At most two dev tuning rounds.
  - The world gains seven existing bookings.
- **Variants:**
  - **D:** the traveler and platform of c06884c (`bench/agency/d4`, binary
    built at that commit): router and free mode;
  - **E:** the same traveler with `agentMode` on, planner DeepSeek V4.1
    Flash.
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| P1 | More goals | E goal ≥ 70% and ≥ D + 10 points |
| P2 | More natural | E mean natural ≥ D + 0.5 |
| P3 | Not less true | E's conversations with a false statement ≤ D's |
| P4 | Safe | 0 invariant failures in E; every effect after its own confirmation or approval |
| P5 | Affordable | E's median agent cost per conversation ≤ $0.02 |
| P6 | Nothing else breaks | with agent mode off, suites and CI as before; the routing and fallback-001 regressions unchanged (the router path is untouched) |

**Decision rule:** agent mode becomes the traveler's default if P1, P3 and
P4 hold. Otherwise it stays experimental, and the report says where it
lost.

**Also reported:**
- lost, redundant and trust;
- latency per turn;
- per persona;
- agency-001's bars (goal 75%, natural 4.0).

**Budget:** $8.

## Deviations (recorded 2026-09-28, before any test conversation of E was run or any test conversation was judged)

Dev round 1 showed two flaws in the instrument.

1. **Personas whose goal only a person meets had no team answers:**
   - g04 and w04 (resend a voucher);
   - g10 and w10 (a payment was sent).

   A handoff, the right move, ended the conversation unanswered. Each now
   has one team answer, written from the goal and the booking in
   `agency.state.json` only.
2. **Scripted team answers were given in order to any task.** A booking's
   reply (a value) went to a question for the team and broke its run
   (g09). Now an answer task takes the next text, and an input task the
   next value (`fitting`, internal/evals/simulate.go).

**What changes:**
- D is re-run whole, dev and test, with its frozen commit plus only this
  simulator patch (`tessera-D4b`). Results go to `D-dev2` and `D-test2`.
  The first D runs are kept and not used.
- E's dev round 1 was run with the old instrument. Dev round 2 (`E-dev2`)
  and the test use the new one.
- Also reported: conversations that end at a team task the scenario has
  no answer for, per variant. Such a task is a handoff that a scripted
  team did not foresee.
