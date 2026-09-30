# agency-007: the pending branch, with the simulated team. I becomes the default

- **Date:** 2026-09-29.
- **Pre-registration:** `agency-007-preregistration.md` (2cdfa90), with its
  deviation (the judge counts the team's answers as true).
- **Data:** `bench/results/agency-007/`.
- **Variants:**
  - **H:** the traveler default of c077a8d, `tessera-H3`;
  - **I:** 1bf3567, `tessera-I2`, after both dev rounds.
- **Instrument:**
  - the simulated team of team-001 answers what no persona scripts. It
    answered 83 tasks for H and 85 for I, and no conversation ended at
    an unanswered task;
  - the judge was changed before the test (the deviation).
- **Spend:** about $4.

## What I changes, while a process waits

1. **An unsure message that asks something** ("não tenho os dados
   agora… é o total pra dois?") is answered. It is not met with the
   pending question alone.
2. **Side answers search the web** in agent mode.
3. **A question with no answer that stands goes to the team.** The
   waiting run stops: it had done nothing yet, and the customer resumes
   it by asking again. One run waits at a time. Before, the agent
   offered "posso pedir para a equipe" and could not do it while a
   booking waited, so it looped.
4. **A dropped side answer** gets `messages.unsure` when there is no
   `whenUnsure`.

## Result (sealed test: 16 new personas × 3)

| | H | I |
| --- | --- | --- |
| Goal met (of 48) | 30 (62%) | **41 (85%)** |
| Information lost | 24 | **10** |
| Redundant messages | 92 | **44** |
| Conversations with a false statement | 6 | 5 |
| False statements | 14 | 5 |
| Natural (1–5) | 3.06 | 3.69 |
| Trust (of 48) | 23 | 35 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0112 | $0.0097 |

| # | Hypothesis | Result |
| --- | --- | --- |
| U1 | Lost ≤ 70% of H's | **Supported:** 10 against 24 |
| U2 | Redundant ≤ 80% of H's | **Supported:** 44 against 92 |
| U3 | Goals ≥ H's | **Supported:** 41 against 30 |
| U4 | Conversations with a false statement ≤ H's + 1 | **Supported:** 5 against 6 |
| U5 | 0 invariant failures; median cost ≤ $0.02 | **Supported** |
| U6 | CI, lint, suites; the classifier's prompt unchanged | **Supported:** 38/38 live; the interruption prompt is untouched |

**Decision (the pre-registered rule):** I becomes the traveler's default.
The changes are in the platform, and the traveler's configuration is
unchanged.

**Per persona:**
- I met more goals than H for 12 personas, fewer for none, and the same
  for 4.
- **The largest gains were the loops:**
  - "reenviar o voucher" (s13): 1 → 3 goals, lost 9 → 0, repeats 17 → 2;
  - a seat before confirming (s15): repeats 17 → 4;
  - a dog in the cabin (s05): 1 → 3.
- **Still weak:** a coupon (s10, 1/3 in both) and rescheduling (s04,
  1/3).

## Across the agency studies

These are different personas each time: the rows are not one scale.

| Study | Change | Goal (before → after) |
| --- | --- | --- |
| agency-001..003 | router and free mode | 39% → 58% |
| agency-004 | agent mode | 50% → 58% |
| agency-005 | agent mode's three gaps | 65% (router) → 77% |
| agency-006 | loops and details (not adopted as better) | 79% → 75% |
| agency-007 | the pending branch, with the simulated team | 62% → **85%** |

**agency-007 is the first measurement where a team answers**, as in a real
agency. Its instrument is the closest yet to the product the owner
described.
