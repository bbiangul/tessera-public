# Escalation 001: results

- **Date:** 2026-09-24
- **Pre-registration:** [escalation-001-preregistration](escalation-001-preregistration.md).
- **Code measured:** the `escalation-decision` branch (commit `d40021b`,
  rebased as `50a9413`). Results are in `bench/results/escalation-001`.
- **Spend:** $0.23: runs $0.07, judge $0.16.
- The comparison rows are [escalation-000](escalation-000.md)'s, on the
  same 21 test questions × 20 repeats.

## Results

| Stack | Unneeded escalation (answer + partial) | on answer | on partial | Coach recall | Flaky questions | Mean per-question SD | Judged correct | False claims | Cost/run | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| plain | 16% (12–21) | 0% | 59% | 100% | 4 | 0.06 | 97% | 8 | $0.00035 | 1.79 s |
| agents-sdk | 19% (15–24) | 0% | 71% | 100% | 1 | 0.02 | 95% | 3 | $0.00036 | 1.71 s |
| langgraph | 24% (20–29) | 6% | 74% | 100% | 2 | 0.03 | 93% | 4 | $0.00037 | 1.75 s |
| tessera (routed, 000) | 48% (42–53) | 37% | 78% | 100% | 2 | 0.02 | 90% | 5 | $0.00009 | 1.45 s |
| **tessera, free-mode handoff** | **7%** (4–10) | **0%** | **25%** | 83% (76–89) | **0** | **0.00** | 88% (80–93) | 7 | $0.00016 | 1.49 s |

The real case's analogue, **t17**, "can I sign the lease before my
license comes through?": the coach was asked 0 of 20 times, against 11,
17 and 20 of 20 for the LLM stacks. The reply gives the step: don't
sign yet; the draft lease goes to Franchise Administration for review
first.

### The decision's probabilities

P(ask-coach) from the handoff question, per question, over 20 repeats:

| Label | Questions | P(ask-coach) |
| --- | --- | --- |
| answer | 11 | 0.00 on every run |
| partial | t17, t18, t19 | 0.03 – 0.18 |
| partial | t21 ("coming soon" email before the window) | 0.87 – 0.92 |
| coach | t11, t12, t13, t15, t16 | 0.63 – 0.99 |
| coach | **t14** (selling the center) | **0.34 – 0.49**, under the 0.6 threshold |

- Each question's probability moves by at most ±0.08 across repeats, and
  no decision ever flipped.
- A threshold near 0.3 would have caught t14 without adding a single
  unneeded escalation. That threshold was read from the test set, so it
  is **not** a result. It is a hypothesis for a new held-out set.
- In t14 the reply still sends the owner to the coach: "the first move is
  to raise it with your coach". It does not ask the coach itself.

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| G1 | Unneeded rate ≤ 8% | **Supported:** 7% (4–10), against 16–24%. On partial questions: 25% against 59–74%. |
| G2 | Coach recall ≥ 90% | **Not supported:** 83%. t14 was missed in 20 of 20 runs (P 0.34–0.49). |
| G3 | Correct ≥ 88% and false claims ≤ the LLM median (4) | **Not supported:** 88% (92/105, under 88% by rounding) and 7 false-claim flags. |
| G4 | Mean per-question SD ≤ 0.06 | **Supported:** 0.00, and 0 flaky questions. |
| G5 | Cost ≤ 50% of the LLM stacks' median | **Supported:** 43%. |

## Why the answers lost points

All 13 incorrect replies are on three partial questions (t18, t19, t21).
They have the same shape: "the checklist doesn't say; ask your coach".
- The free mode's rules were written for a clinic's fixed facts: "if the
  answer is not there, say you do not know it and point to a person".
- The judge wanted the inference the checklist allows. For example, the
  Center Director is required *before opening*, so tutors can be hired
  first.
- The handoff had already decided that nobody else needs to decide. The
  writer was not told, so it deferred anyway, in words.

This is not a trade-off of the design: the decision and the writing are
separate, and the writing only needs to be told the decision.

## What this supports, and what not yet

**Supported, on held-out synthetic questions:** a declared, separate
decision that sees the material removes most unneeded escalations. It
did so on the real case's pattern, 0 of 20 against 11–20 of 20. The
decision is perfectly stable across repeats, costs less than half as
much, and its probabilities show where it is unsure.

**Not yet:**
- recall: one coach question fell under the threshold;
- answer quality on partial questions.

The claim for the "better than the market" list needs a run where G2
and G3 hold as well.

## What changes (escalation-002, on a new held-out set)

1. The free answer is told the handoff's decision: "no one else needs to
   decide; answer with what the facts say or imply". Its rules allow
   reasoning from the facts, not only quoting them.
2. The threshold: set from the dev set and a new calibration set, never
   from these test questions.
3. New test questions, the same labels, written before the run.
