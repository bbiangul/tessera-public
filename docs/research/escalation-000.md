# Escalation 000: results (baseline)

- **Date:** 2026-09-24
- **Pre-registration:** [escalation-000-preregistration](escalation-000-preregistration.md).
- **Code measured:** commit `fc195c2`. Results are in
  `bench/results/escalation-000`:
  - `runs.jsonl`: 1,680 runs;
  - `judgments.jsonl`: 420 judged replies.
- **Spend:** $1.21:
  - LLM stacks $0.46;
  - Tessera $0.04;
  - judge $0.72.

## Results (test set: 21 questions × 20 repeats per stack)

| Stack | Unneeded escalation (answer + partial) | on answer | on partial | Coach recall | Flaky questions | Mean per-question SD | Judged correct | False claims | Cost/run | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| plain | 16% (12–21) | 0% | 59% | 100% | 4 | 0.06 | 97% (92–99) | 8 | $0.00035 | 1.79 s |
| agents-sdk | 19% (15–24) | 0% | 71% | 100% | 1 | 0.02 | 95% (89–98) | 3 | $0.00036 | 1.71 s |
| langgraph | 24% (20–29) | 6% | 74% | 100% | 2 | 0.03 | 93% (87–97) | 4 | $0.00037 | 1.75 s |
| **tessera (today)** | **48%** (42–53) | 37% | 78% | 100% | 2 | 0.02 | 90% (83–95) | 5 | $0.00009 | 1.45 s |

Escalations per question, out of 20:

| Question | Label | plain | agents-sdk | langgraph | tessera |
| --- | --- | --- | --- | --- | --- |
| t06 tutor before the background check clears | answer | 1 | 0 | 14 | 20 |
| t08 own prices | answer | 0 | 0 | 0 | 20 |
| t09 personal bank account | answer | 0 | 0 | 0 | 20 |
| t20 lease in whose name | answer | 0 | 0 | 0 | 20 |
| t17 **sign the lease before the license** (the real case) | partial | 11 | 17 | 20 | 2 |
| t18 contractor during the lease review | partial | 18 | 20 | 20 | 20 |
| t19 tutors before a director | partial | 0 | 0 | 0 | 20 |
| t21 "coming soon" email before the window | partial | 18 | 20 | 19 | 20 |

All the other answer questions: 0 escalations in every stack. All six
coach questions: 20 of 20 in every stack.

## Hypotheses

| # | Verdict |
| --- | --- |
| E1: the phenomenon exists in LLM tool loops | **Supported.** Every LLM stack escalated 59–74% of the partial runs. The real case's analogue, t17, went to the coach in 11–20 of 20 runs. |
| E2: coach recall ≥ 90% | **Supported:** 100% for every LLM stack. |
| E3: today's Tessera does not beat them | **Supported:** 48% unneeded. Its router sends "can I set my own prices?" to the coach 20 of 20 times: it reads "coach approval" in the Function's description and sees no checklist. |
| E4: ≥ 3 flaky questions in an LLM stack | **Supported, but weakly:** the plain loop has 4. At temperature 0 the stacks are mostly consistent, not random. The error is a **bias**: a question the checklist does not settle word for word goes to the coach almost every time. |

## What this shows

- The real case's pattern is reproducible with synthetic material and
  three frameworks: one prompt holds both the answer and "should I ask
  the coach?". When the checklist does not state the exact case, the
  stacks ask the coach even when the checklist gives the step to take.
- The flakiness the other team saw (7 of 14) is at their temperature and
  prompt. Here the same bias shows up as consistency: 17–20 of 20.
- Today's Tessera is worse, for a structural reason: its router cannot
  see what the free mode knows. [escalation-001](escalation-001-preregistration.md)
  moves that decision into the free mode, where the checklist is in view.
