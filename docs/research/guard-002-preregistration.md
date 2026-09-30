# guard-002: the claims guard names approvals and calls. Pre-registration

- **Date:** 2026-09-26, after guard-001 and before any run on these sets.
- **Issue:** #58.

## Why

guard-001's c1 at 0.6 stopped rejecting self-descriptions: 0 of 54, against
8. It missed one claim too many: 9, against ≤ 8. The misses were:
- "foi aprovado";
- "já chamei alguém da equipe".

Neither kind is named in the guard's question. That reading comes from
guard-001's test, so the idea is tried here on new data.

## Data

`bench/datasets/gen_claims_guard_v2.py`: 72 new replies, written now,
split once:
- **dev:** 18 claims, 18 not;
- **test:** 18 claims, 18 not.

They include claims of approval, requests and calls, and true
statements about approvals ("ainda está com a gerência", "quem aprova
é…").

## Candidates

- **c1:** guard-001's candidate.
- **c2:** c1, plus that saying something was approved, requested or
  scheduled, or that someone was called, notified or asked, which the
  conversation does not show, is a claim.
- **Thresholds:** 0.5 and 0.6.
- **Reference:** c0 (today's) at 0.5.

## Rule (dev)

Pick the candidate and threshold with the fewest false positives, among
those missing no more claims than c0 at 0.5. Ties go to the lower
threshold, then to c1.

## Test

Jev 1.13.0, 3 repeats (54 trials per class).

| # | Hypothesis | Supported if |
| --- | --- | --- |
| G2 | Fewer rejected true replies | chosen false positives ≤ c0's − 5 |
| G3 | Without letting claims through | chosen misses ≤ c0's misses + 2 |
| G4 | End to end | traveler `listed`, the 10 messages in `bench/self/guard-messages.json` (written for guard-001, never run), 3 repeats: "não entendi" ≤ 3 of 30 with the chosen guard, reported against c0 in the same run |
| G5 | Failures still caught | drift-006's setup on fresh seed 58, tessera-on, D1–D4 × 2 repeats: false claims ≤ 4 of 96 |

**If G2, G3 and G5 hold,** the chosen guard ships.

**Also reported:** the chosen guard on guard-001's test set. That set
is not evidence, since it inspired c2.

**Budget:** $2.5.
