# Escalation 002: results

- **Date:** 2026-09-24
- **Pre-registration:** [escalation-002-preregistration](escalation-002-preregistration.md).
- **Code measured:** commit `43fc79a`. Results are in
  `bench/results/escalation-002`:
  - `runs.jsonl`: 2,100 runs on `test2`;
  - `judgments.jsonl`: the judge on test2;
  - `rejudged-000-001.jsonl`: the first test set judged again, with the
    checklist.
- **Spend:** $5.81: runs $0.54, judge on test2 $2.63, re-judging $2.63.

## Results on `test2` (21 new questions × 20 repeats)

| Stack | Unneeded escalation (answer + partial) | on answer | on partial | Coach recall | Flaky questions | Mean per-question SD | Judged correct | False claims | Cost/run | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| agents-sdk | 15% (11–19) | 0% | 45% | 100% | 4 | 0.07 | 96% (91–99) | 3 | $0.00035 | 1.72 s |
| plain | 16% (12–20) | 0% | 47% | 100% | 3 | 0.07 | 93% (87–97) | 9 | $0.00034 | 1.75 s |
| langgraph | 23% (19–28) | 0% | 68% | 100% | 3 | 0.05 | 92% (86–96) | 7 | $0.00036 | 1.81 s |
| tessera (routed) | 46% (41–52) | 30% | 79% | 83% | 3 | 0.06 | 83% (75–89) | 2 | $0.00008 | 1.46 s |
| **tessera, free-mode handoff** | **6.7%** (4–10) | **0%** | **20%** | 83% (76–89) | **0** | **0.00** | **91%** (85–95) | **2** | $0.00016 | 1.47 s |

The partial questions, escalations out of 20:

| Question | plain | agents-sdk | langgraph | handoff |
| --- | --- | --- | --- | --- |
| s17 sign the lease before the insurance certificates | 5 | 3 | 17 | **0** |
| s18 a soft opening before the grand opening | 13 | 18 | 20 | 20 |
| s19 order the tech package before the floor plan | 0 | 0 | 0 | 0 |
| s20 the director is also an owner | 20 | 7 | 20 | **0** |
| s21 minimum hours in summer | 9 | 17 | 11 | **0** |

**Coach questions:** every stack asked the coach on s11–s14 and s16.
**s15** ("my partner wants to buy me out") went to the coach in 20 of 20
runs of the LLM stacks, and 0 of 20 of either Tessera variant.

### The first test set, judged again with the checklist

| Stack | Correct (first judge) | Correct (with checklist) | False claims (first) | False claims (with checklist) |
| --- | --- | --- | --- | --- |
| plain | 97% | 98% | 8 | 10 |
| agents-sdk | 95% | 97% | 3 | 9 |
| langgraph | 93% | 95% | 4 | 7 |
| tessera (routed) | 90% | 92% | 5 | 0 |
| tessera-handoff (escalation-001) | 88% | 89% | 7 | 0 |

Without the checklist the judge charged Tessera with inventing facts
that were in the checklist, and missed the LLM stacks' invented
details. escalation-001's G3 verdict stands on correctness (under the
bar); its false-claim part is overturned by the fixed judge.

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| K1 | Unneeded ≤ half the best LLM stack's | **Supported:** 6.7% (20 of 300) against 15.0%: under half (7.5%), narrowly. |
| K2 | Coach recall ≥ 90% | **Not supported:** 83%. One question, s15, was missed in 20 of 20 runs (P 0.42–0.53). |
| K3 | Correct ≥ LLM lowest − 5 points; false claims ≤ LLM median | **Supported:** 91% against the 87% bar; 2 against a median of 7. |
| K4 | SD ≤ the LLM stacks' highest | **Supported:** 0.00, and 0 flaky questions. |
| K5 | Cost ≤ 50% of the LLM median | **Supported:** 46%. |

**Not on the "better than the market" list yet:** the pre-registered
rule needs K1, K2 and K3, and K2 failed.

## The one miss, twice

The two misses across both test sets are the same kind: selling or
transferring the franchise (t14, s15). The checklist says "Talk to your
coach first; they start the process."
- **Tessera's reply** is right in content: "your coach starts the
  process, so talk to your coach first". It does not ask the coach
  itself.
- **Its probability** is 0.34–0.53, under the 0.6 threshold.
- **Why:** the declared `when` is "a decision, an approval or an
  exception that the checklist gives to the coach". A process the coach
  starts is none of those words. Tessera applied the declared policy
  literally; the LLM stacks, given the same sentence, generalised from
  it.
- **The calibration set had no question of this kind** (c07–c11 are
  approvals and exceptions), so calibration could not see it. That is a
  gap in the set, recorded as a lesson: the calibration set must cover
  every kind of thing the policy hands to a person.
- **A threshold alone would not fix it:** partial s21 reaches 0.49.

## What the two held-out sets show together

- On questions the material settles, the handoff never asked the coach:
  0 of 400 answer runs in the two sets.
- On partial questions: 25% and 20%, against the LLM stacks' 45–74%.
  - The remaining unneeded escalations are t21 and s18: marketing
    questions whose rule has a coach-approval clause next to it.
  - The real case's analogues went to the coach in 0 of 20 (t17) and 0
    of 20 (s17) runs, against 3–20 of 20 for the LLM stacks.
- Every decision was identical in all 20 repeats: no flakiness, in either
  set.
- Answers are as correct as the LLM stacks', with fewer invented
  details.
- **The open problem is recall for kinds of handoff the policy text
  does not name.**

## What changes

1. **Authoring:** `when` names every kind of thing that goes to the
   person (decisions, approvals, exceptions, processes they start). The
   calibration set covers each kind. Both go into the authoring guide and
   the Part C skill.
2. **Next measurement (escalation-003):**
   - a third held-out set, which must contain "the person starts a
     process" questions;
   - the `when` completed;
   - the claim is decided there with the same K1–K3 rule.
