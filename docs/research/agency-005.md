# agency-005: agent mode's three gaps fixed. It becomes the traveler's default

- **Date:** 2026-09-28.
- **Pre-registration:** `agency-005-preregistration.md` (77cd73a), with its
  deviation.
- **Data:** `bench/results/agency-005/`.
- **Variants:**
  - **D:** the router, `tessera-D4b`;
  - **E:** agent mode as in agency-004, `tessera-E2`;
  - **F:** E with the three fixes, `tessera-F3` at 162ba52.
- **Spend:** about $7.5.

## The three fixes (F)

1. **A dropped draft is never silence.** After a Function answered part
   of a message, the free answer covers the rest. If no answer stands,
   `messages.unsure` says so.
2. **A dropped reply tries the web and the free answer** (its retry and
   quoted fallback) before the team.
3. **"Mando depois" keeps a process waiting.** A separate Jev question
   (`later`) is asked only when the classifier took the message as the
   answer. The run waits, the question is not asked again, and
   `messages.later` is sent.

## Result (sealed test: 16 new personas × 3)

| | D (router) | E | F |
| --- | --- | --- | --- |
| Goal met (of 48) | 31 (65%) | 33 (69%) | **37 (77%)** |
| Conversations with a false statement | 7 | 8 | 7 |
| False statements | 9 | 10 | 7 |
| Information lost | 31 | 10 | 20 |
| Redundant messages | 53 | 27 | 40 |
| Natural (1–5) | 3.06 | 3.46 | 3.48 |
| Trust (of 48) | 23 | 24 | 31 |
| Invariant failures | 0 | **2** | 0 |
| Ended at a team task not scripted | 6 | 15 | 13 |
| Median agent cost | $0.0033 | $0.0059 | $0.0056 |

| # | Hypothesis | Result |
| --- | --- | --- |
| Q1 | F goal ≥ 70% and ≥ E + 8 points | **Supported:** 77.1%, +8.3 |
| Q2 | F lost ≤ 70% of E's | **Not supported:** 20 against 10 |
| Q3 | F's conversations with a false statement ≤ E's | **Supported:** 7 and 8 |
| Q4 | F's unscripted team endings ≤ 60% of E's | **Not supported:** 13 against 15 |
| Q5 | 0 invariant failures in F; median cost ≤ $0.02 | **Supported:** 0; $0.0056 |
| Q6 | With agent mode off, nothing else changes | **Supported:** `make ci`, `make lint-extra`, traveler suites 36/36 live |

**Decision (the pre-registered rule):**
- Q1, Q3 and Q5 hold.
- F's goal is D's plus 12.5 points.

**So agent mode becomes the traveler's default.** It stays opt-in for
every other agent (ADR-0007).

**Per persona (goals of 3; D / E / F):**
- **F gained most on:**
  - a coupon question, then a quote (m12: 0/0/2);
  - "I paid yesterday" (m14: 0/2/3);
  - a 15-year-old flying alone (m13: 1/0/2).
- **F lost on:** Lisboa with insurance (m08: 3/1/1) and the cat (m04:
  3/2/2).

## What the test found, fixed after it (not in the numbers above)

1. **Duplicate effects in agent mode (E, m08).**
   - In one turn the planner ran the hotel search, then the full quote,
     which searches hotels again.
   - Both saved under the same idempotency key. The traveler's keys
     assume one run per message.
   - F shares the path; it was not hit in 48 conversations.
   - **Fix:** agent mode runs at most one Function a turn.
   - Dev round 2 had shown the same class of bug: a Function run twice
     with other inputs. That was reverted then (the deviation).
2. **A redundant add-on.** After a Function answered all a message asked,
   the planner still restated it ("essas são as opções que te mostrei").
   Now the add-on is sent only when the measured "rest" question finds
   something unanswered.
3. **A traveler bug** that agent mode exposed. In `search-hotels`, the
   source computing the check-out from a day of the month ("saída no dia
   2") sat under `childrenAges`, so the run failed whenever it applied.
   It was moved to `checkOut`. Suite: `hotels/check-out-by-day`.
4. **The eval harness now knows agent mode:**
   - a turn's `says` include the Functions' messages sent before its
     result;
   - `routed` names the Function the turn ran;
   - `handoff` counts the planner's choice.
5. **CI tested the router without anyone knowing.** The traveler's
   recorded answers had no planner calls, so agent mode fell back to the
   router in replay. They were re-recorded with agent mode on: 37/37
   live, 37/37 replayed, 100 planner calls.

**The build that ships (G: F plus 1–5) was checked on all 60 persona
conversations:**
- 0 invariant failures;
- the traveler suites pass 37/37 live with agent mode on.

**For information only, not a result:** G's goals on the test were 41/48
(85%), with 7 conversations with a false statement. The test was no
longer sealed for me when G was built.

## Still open

- **Unscripted team endings stay high** (F 13, D 6). Agent mode hands
  questions to the team more readily than the router does, and a real
  team would answer them. Scripted teams do not.
- **Information lost: F 20 against E 10.** Q2 was not supported, and why
  F loses more than E is not explained yet.
- **A cassette miss is silent.** When a recorded answer is missing in
  replay, agent mode falls back to the router without any sign. CI should
  fail instead.
