# Escalation 002: pre-registration

- **Date:** 2026-09-24, before any run on the `test2` questions.
- **Previous:** [escalation-001](escalation-001.md). The free-mode handoff
  cut unneeded escalations to 7% (the LLM stacks: 16–24%), but:
  - coach recall was 83%;
  - answers on partial questions deferred to the coach in words.

## What changed

- **The writer knows the decision.** When the handoff decides to answer,
  the free answer is told nobody else needs to decide: it answers with
  what the facts say or clearly imply, and does not send the user to a
  person. An example that came from a test question was removed from
  that instruction before any run.
- **The threshold stays at the default, 0.6.** It was calibrated on the
  dev and `cal` sets, 29 questions × 5 repeats:
  - partial and answer questions reached at most 0.53 (c15);
  - coach questions were at least 0.87 (c11);
  - so 0.6 makes no error there, and it is kept.

  The first test set had t14 at 0.43. That is known, and is exactly why
  this run uses new questions.
- **The judge sees the checklist.** Without it, it flagged true checklist
  facts ("10 business days", "every 2 years") as invented, so the
  escalation-000 and 001 false-claim counts are inflated. Their test runs
  are judged again with the fixed judge, reported next to the new ones.

**Development**, dev + cal with the new writer and the fixed judge:
- 56 of 58 judged correct;
- 3 false-claim flags, all real (d03 said the second owner pays; d13
  applied the closure rule to extra hours).

## Test set and runs

- **`test2`:** 21 new questions (10 answer, 6 coach, 5 partial), written
  and committed (`447bbca`) before any run. Nobody has run them.
- **Stacks:** plain, langgraph, agents-sdk, tessera (routed, as in
  escalation-000), tessera-handoff. 20 repeats each: 2,100 runs.
- **Judge:** repeats 1–5, with the checklist.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| K1 | Fewer unneeded escalations | tessera-handoff's unneeded rate (answer + partial) ≤ half the best LLM stack's |
| K2 | Without missing the coach | Coach recall ≥ 90% |
| K3 | Answers not worse | Judged correct ≥ the LLM stacks' lowest minus 5 points, and false claims ≤ the LLM stacks' median |
| K4 | Stable | Mean per-question SD ≤ the LLM stacks' highest |
| K5 | Cheaper | Cost per run ≤ 50% of the LLM stacks' median |

**The claim goes on the "better than the market average" list only if
K1, K2 and K3 all hold.** Otherwise it stays open, with the numbers.

**Budget:** $6: runs about $0.7, the judge on test2 about $2.6, the
re-judging of 000/001 about $2.6.
