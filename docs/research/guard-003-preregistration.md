# guard-003: confirming c2 at 0.5 on new data. Pre-registration

- **Date:** 2026-09-26, after guard-002 and before any run on this set.
- **Issue:** #58.

## The candidate

There is one candidate, fixed before this set was written:
**c2 at 0.5**.

guard-002 found that c2 names approvals, requests and calls. On
guard-002's test it had 4 false positives and missed 2 claims. c0 had 6
false positives and missed 15.

That choice was made after seeing guard-002's test, so it is confirmed
here on new data.

## Data

`bench/datasets/gen_claims_guard_v3.py`: 60 new replies, test only:
- 30 claims;
- 15 self-descriptions and invitations (#58's target);
- 15 other true replies.

Jev 1.13.0, 3 repeats. That is 90 claim trials, and 45 trials for each
not-claim group.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| G2a | Self-descriptions survive | c2's false positives on n01–n15 ≤ c0's − 5 (of 45) |
| G2b | No worse on other true replies | c2's false positives on n16–n30 ≤ c0's + 2 (of 45) |
| G3 | Claims caught at least as well | c2's misses ≤ c0's + 2 (of 90) |
| G4 | End to end | traveler `listed`, `bench/self/guard-messages.json`, 3 repeats: "não entendi" ≤ 3 of 30 with c2, reported against c0 in the same run |
| G5 | Failures still caught | drift-006's setup on fresh seed 58, tessera-on, D1–D4 × 2 repeats: false claims ≤ 4 of 96 |

**If G2a, G2b, G3 and G5 hold,** c2 at 0.5 ships.
