# agency-009: documents up front and gated quotes. K is not adopted

- **Date:** 2026-09-29.
- **Pre-registration:** `agency-009-preregistration.md` (a2f866c). It was
  written after K was built, before any conversation was run.
- **Data:** `bench/results/agency-009/`.
- **Variants:**
  - **J:** 0fec50b, the default;
  - **K:** 51b6553, the documents searched when every agent-mode turn
    starts, and a document quote sent only if it answers.
- **Spend:** about $5.

## Result (sealed test: 16 new personas × 3)

| | J | K |
| --- | --- | --- |
| Goal met (of 48) | **39 (81%)** | 36 (75%) |
| Information lost | **8** | 16 |
| Robotic "Pela nossa política, \"…\"" messages | 5 | 4 |
| Conversations with a false statement | 8 | 9 |
| Redundant messages | 43 | 52 |
| Natural (1–5) | 3.83 | 3.69 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0106 | $0.0103 |

| # | Hypothesis | Result |
| --- | --- | --- |
| W1 | Robotic quotes ≤ 50% of J's | **Not supported:** 4 against 5; rare in both |
| W2 | Lost ≤ 80% of J's | **Not supported:** 16 against 8 |
| W3 | Goals ≥ J's | **Not supported:** 36 against 39 |
| W4 | False ≤ J's + 1 | **Supported:** 9 against 8 |
| W5 | 0 invariant failures; cost ≤ $0.02 | **Supported** |

**Decision (the pre-registered rule):** K is not adopted, and 51b6553 is
reverted (333ddf3). The default stays J.

## What the study says

- **The robotic quote of the demo run is rare:** 5 in J's 48 test
  conversations. One demo conversation made it look larger than it is.
- **K's losses come from traveler processes, not from K's change.**
  Across the three runs they swing either way in both variants:
  - **Travel insurance promised and not booked (z16: J 1/3, K 0/3).**
    `book-trip` adds only a checked bag. The agent confirmed a total with
    insurance and the booking went without it. **This is a bug of the
    booking: an extra the customer confirms must be booked, or the agent
    must not confirm it.**
  - **A child under 12 flying alone** is quoted with an adult (z13). The
    people-count rule counts 12 and older as adults, and a younger child
    alone is not handled.
  - **A booking that never reached the team (z04),** and a hotel's
    breakfast question left with the team (z07).

## Next

- **`book-trip` adds the insurance and the flexible ticket** like the bag,
  from the flight's extras. Until then the agent must not confirm them.
- **A child under 12 flying alone:** one passenger, the unaccompanied-minor
  service and its fee. The customer is told the airline's age limits.
