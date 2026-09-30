# Experiments 001: why the A/B verdict is sequential

- **Date:** 2026-09-23
- **Question:** RFC-0001 says an experiment can be read every day. Does a
  plain two-proportion z test survive that, and does the sequential test
  we chose (mSPRT, Johari et al., "Always Valid Inference", normal
  approximation, mixing variance 0.01) keep its promise with enough power?
- **Setup:** `internal/metrics/experiment_test.go` (`TestPeekingSimulation`),
  fixed seed: 400 simulated experiments, each looked at 20 times, 100 new
  decided subjects per arm between looks (2,000 per arm at the end),
  alpha 0.05.

| Case | z test, read at every look | mSPRT |
| --- | --- | --- |
| A/A (10% vs 10%): declared a winner | 99/400 (24.8%) | 6/400 (1.5%) |
| A/B (10% vs 15%): found the difference | — | 379/400 (94.8%) |

## Findings

1. Reading a fixed-horizon test every day makes it declare a winner in a
   quarter of experiments where there is no difference: five times alpha.
2. The mSPRT stays under alpha while read at every look, and finds a
   5-point difference 95% of the time by 2,000 subjects per arm.
3. a common A/B design sends traffic to the strategy with the most
   positives and counts unanswered tests as negatives; neither exists here: assignment is a fixed hashed split, and
   subjects inside the conversion window are pending, not failures.

## Decision

The verdict is the mSPRT, shown only after `minSamplePerVariant` decided
subjects per arm; the z test's p-value is reported for reference, labelled
as valid only for a single read. A sample ratio mismatch check (chi-square
of exposures against weights) flags a broken split: in the test, dropping a
third of one arm's exposure logs is caught (p < 0.001) and a correct hashed
split is not (p = 0.26).
