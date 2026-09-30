# Escalation 004: results

- **Date:** 2026-09-24
- **Pre-registration:** [escalation-004-preregistration](escalation-004-preregistration.md).
- **Code measured:** commit `842b915` (the guard). Results are in
  `bench/results/escalation-004`.
- **Spend:** $3.41: runs $0.63, judge $2.78.

## Results on `test4` (21 new questions × 20 repeats)

| Stack | Unneeded escalation (answer + partial) | near-coach-clause stratum | Coach recall | Mean per-question SD | Correct | False claims | Cost/run | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| plain | 14% (11–19) | 16% | 100% | 0.02 | 91% | 6 | $0.00041 | 1.56 s |
| agents-sdk | 18% (14–23) | 24% | 100% | 0.02 | 91% | 6 | $0.00042 | 1.55 s |
| langgraph | 18% (14–23) | 24% | 100% | 0.02 | 90% | 12 | $0.00042 | 1.46 s |
| tessera (routed) | 53% (48–59) | 86% | 67% | 0.00 | 81% | 10 | $0.00009 | 1.43 s |
| **tessera, free-mode handoff + guard** | **14%** (10–18) | **15%** | **100%** | 0.01 | **95%** (89–98) | **0** | $0.00018 | 1.62 s |

**Escalations out of 20, on the questions where the stacks differ:**

| Question | plain | agents-sdk | langgraph | handoff |
| --- | --- | --- | --- | --- |
| v14 reuse a grand opening template for the anniversary (partial) | 20 | 20 | 20 | 20 |
| v19 pay the royalty early (partial, near the deferral clause) | 3 | 14 | 14 | 1 |
| v20 parent nights after 8pm (partial) | 20 | 20 | 20 | 20 |

Every other question got the same decision from every stack but the
routed Tessera.
- v14 and v20 went to the coach from all stacks, in every run.
- Their labels are defensible, but a reader could argue for
  escalating both.

## Hypotheses

| # | Verdict |
| --- | --- |
| K1: unneeded ≤ half the best LLM stack's (which must be ≥ 5%) | **Not supported:** 14% against plain's 14%, a tie. |
| K1s: the same on the near-coach-clause stratum | **Not supported:** 15% against 16%. |
| K2: coach recall ≥ 90% | **Supported:** 100%. |
| K3: correct ≥ LLM lowest − 5; false claims ≤ LLM median | **Supported:** 95% against 90–91%, and **0 false claims** against 6–12. |
| K4: SD ≤ LLM highest | **Supported:** 0.01 against 0.02. |
| K5: cost ≤ 50% of the LLM median | **Supported:** 42%. |

**The claim does not go on the list:** K1 failed.

## The guard

On this set it does what it was built for:
- 0 false claims, against 5 on escalation-003's u16;
- the best correctness of any stack, 95%.

It costs one Jev call on answered questions: p50 1.62 s, against the
LLM stacks' 1.46–1.56 s.

## Four held-out sets, the honest picture

| Set | Handoff unneeded | Best LLM stack | Difference found | Recall | Answers |
| --- | --- | --- | --- | --- | --- |
| test (001) | 7% | 16% | yes, under half | 83% | under the bar |
| test2 (002) | 6.7% | 15% | yes, under half | 83% | at the bar |
| test3 (003) | 0% | 0% | no pattern present | 100% | 5 false claims |
| test4 (004) | 14% | 14% | tie | 100% | best, 0 false claims |

**What holds on every set:**
- the declared handoff is never worse than the LLM stacks on unneeded
  escalation, and in two sets it was less than half;
- it is perfectly or nearly perfectly stable;
- it costs about half;
- since escalation-003's policy, its recall is 100%;
- with the guard, its answers are the most correct and invent nothing.

**What does not hold:** "less than half the unneeded escalations" is not
a general property. It depends on the questions. The first two sets had
partial questions where the LLM stacks' bias showed strongly; test4's
"near-coach-clause" stratum, built to hold that pattern, did not bring
it out: only v19 did.

**So the claim for the list, as pre-registered, is not supported.**

What the evidence does support, stated with its limits (synthetic
material, one domain, one model family, questions written by us):

> a declared escalation decision is at least as precise as LLM tool
> loops, fully stable across repeats, half the cost, and with the answer
> guard it invents nothing.

The owner decides whether that weaker statement is worth listing.
