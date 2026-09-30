# Experiments 002: when a guardrail stops an experiment

- **Date:** 2026-09-23
- **Question:** RFC-0001 says guardrails (opt-out, complaints, escalations)
  stop an experiment when a variant makes them worse than a limit. The
  scheduler checks every few minutes, so the rule is read hundreds of
  times. Which rule stops real harm quickly without stopping experiments
  that did no harm?
- **Setup:** `internal/metrics/experiment_test.go`
  (`TestGuardrailSimulation`), fixed seed, 400 simulated experiments per
  case, a 2-point margin (`maxIncrease: 0.02`), alpha 0.05, 100 decided
  subjects per arm between looks (20 looks, 2,000 per arm) or 10 (200
  looks). A throwaway run compared two rules on the same cases.

| Case | Rule A: mSPRT significant and increase > margin | Rule B: mSPRT on (increase − margin) |
| --- | --- | --- |
| A/A 3%, 20 looks | 0/400 | 0/400 |
| A/A 10%, 20 looks | 1/400 | 0/400 |
| +1 point (3%→4%, inside the margin) | 28/400, median 1,000 per arm | 0/400 |
| +2 points (at the margin) | 198/400, median 1,200 | 1/400 |
| +5 points (3%→8%) | 400/400, median 400 | 346/400, median 900 |
| +10 points (3%→13%) | 400/400, median 200 | 400/400, median 200 |

The committed test runs rule A (seed and looks differ from the table
above):

| Case | Stopped | Median subjects per arm at the stop |
| --- | --- | --- |
| A/A 3%, 20 looks | 0/400 | — |
| A/A 3%, 200 looks | 1/400 | 310 |
| A/A 10%, 200 looks | 4/400 | 410 |
| +1 point, inside the margin | 37/400 | 1,300 |
| +5 points | 400/400 | 400 |
| +10 points | 400/400 | 200 |

## Findings

1. Both rules almost never stop an experiment that did no harm, even read
   200 times: the significance part is the always-valid mSPRT.
2. Rule B treats the margin as the null hypothesis: it is right about
   "worse by more than the margin" but slow, and misses a 5-point harm in
   one experiment out of eight by 2,000 subjects per arm.
3. Rule A stops real harm twice as fast and never missed it; its cost is
   stopping some variants that are worse by less than the margin (7–9% at
   +1 point, half at exactly the margin).

## Decision

Rule A. A guardrail protects people (opt-outs are leads lost for good,
complaints are harm), while a wrongly stopped variant costs an experiment:
the variant was worse anyway, just within the tolerance. On a breach the
platform stops the experiment (subjects exposed after get the control and
leave the comparison) and starts `onBreach` once; resuming is a new
Experiment, a person's decision.

Not covered: cost guardrails (cost per subject needs model and plugin
cost recorded per run, which the journal does not have yet), and guardrails
on rates other than "had milestone X within a window".
