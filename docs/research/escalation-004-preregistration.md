# Escalation 004: pre-registration

- **Date:** 2026-09-24, before any run on `test4`.
- **Previous:** [escalation-003](escalation-003.md):
  - recall reached 100%;
  - the writer contradicted the decision on one question (5 false
    claims), so K3 failed;
  - the test set had no question of the over-escalation pattern, so K1
    held without a difference.

## What changed

- **A guard on the free answer.** When the handoff decided to answer,
  one Decide yes/no asks whether the reply sends the user to a person or
  says something needs approval. On a yes (p ≥ 0.6) the reply is written
  once more: it is told its draft deferred, and nobody else needs to
  decide. Otherwise the first reply stands. It is tested
  (`TestSettledAnswerDoesNotDefer`).
- **Nothing else:** the policy sentence, the checklist, `minConfidence:
  0.55` and every competitor are as in escalation-003.

**Development** (dev + cal, 2 repeats):
- 66 of 66 judged correct, 0 false claims;
- escalation errors only on the two borderline calibration questions (c15,
  c17), as calibrated;
- p50 2.1 s, up from 1.5 s: the guard is one more Jev call on answered
  questions.

## Test set

- **`test4`:** 21 new questions, written and committed (`269d7e5`)
  before any run:
  - 6 answer;
  - 6 coach;
  - 9 partial, 7 of them in a **declared stratum, "near-coach-clause"**:
    the rule that answers them sits next to a clause that gives something
    to the coach, e.g. "a 15% discount" next to "above 20% needs approval".
    This is the real case's pattern, present by design this time.
- **Stacks:** plain, langgraph, agents-sdk, tessera (routed),
  tessera-handoff. 20 repeats: 2,100 runs.
- **Judge:** with the checklist, repeats 1–5.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| K1 | Fewer unneeded escalations | tessera-handoff's unneeded rate (answer + partial) ≤ half the best LLM stack's, and the best LLM stack's rate is at least 5% (a difference exists to find) |
| K1s | On the pattern itself | On the near-coach-clause stratum, tessera-handoff ≤ half the best LLM stack's |
| K2 | Without missing the coach | Coach recall ≥ 90% |
| K3 | Answers not worse | Correct ≥ the LLM stacks' lowest − 5 points; false claims ≤ the LLM median |
| K4 | Stable | Mean per-question SD ≤ the LLM stacks' highest |
| K5 | Cheaper | Cost per run ≤ 50% of the LLM median |

**If K1, K2 and K3 hold, the claim goes on the list.** If the best LLM
stack's rate is under 5%, K1 cannot be decided, and it is reported that
way.

**Budget:** $3.5.
