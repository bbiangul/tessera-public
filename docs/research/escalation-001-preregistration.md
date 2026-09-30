# Escalation 001: pre-registration

- **Date:** 2026-09-24, before any run of the new variant on the test
  set.
- **Baseline:** [escalation-000](escalation-000.md) on the same test set:
  - the LLM stacks asked the coach without need in 16–24% of
    answer + partial runs: 0–6% on answer questions, 59–74% on partial
    ones;
  - their coach recall was 100%;
  - today's Tessera (a routed Function; the router does not see the
    checklist) was worse: 48%.

## What changed (branch `escalation-decision`)

- **`freeMode.functions`:** Functions the free mode may start instead of
  answering.
  - Before answering a message no Function was routed to, one Decide
    question (Jev) chooses between answering and each Function's `when`.
    It sees the free mode's reads, here the checklist.
  - The choice is separate from writing the answer.
  - Its probabilities are in the reply (`handoff`). The default
    threshold is 0.6.
- **The benchmark agent** (`bench/escalation/tessera-handoff`):
  - `ask-coach` has no selector; the free mode starts it;
  - its `when` is the policy sentence every stack gets.
- **Two fixes, found while building:**
  - The free mode answered one English question in Portuguese, because
    the rule text named Portuguese greetings. It now replies in the
    language of the message.
  - A run's voice now sees the previous run of the same conversation.

**Development:** dev set only, 13 questions × 3 repeats.
- 39 of 39 decisions matched the label: 0 unneeded escalations, all 12
  coach runs escalated.
- 0 replies in the wrong language.

## Test set and runs

- The same 21 test questions, 20 repeats: 420 runs of `tessera-handoff`.
- The competitors' and today's Tessera's runs are escalation-000's, not
  run again.
- The same judge, on repeats 1–5.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| G1 | Fewer unneeded escalations than the LLM stacks | Unneeded rate (answer + partial) at most half the best LLM stack's (16%), i.e. ≤ 8% |
| G2 | Without missing the coach | Coach recall ≥ 90% |
| G3 | Answers are not worse | Judged correct ≥ the LLM stacks' lowest (93%) minus 5 points; false claims no more than the LLM stacks' median |
| G4 | Stable | Mean per-question SD ≤ the LLM stacks' highest (0.06) |
| G5 | Cheaper | Cost per run ≤ 50% of the LLM stacks' median |

**Also reported, without a threshold:**
- the partial questions alone, including t17, the real case's analogue;
- the distribution of P(ask-coach) per label: whether a threshold other
  than 0.6 would separate the labels better. Any threshold picked from
  this test set is **not** a result: it would need a new held-out set.

**Budget:** $1.
