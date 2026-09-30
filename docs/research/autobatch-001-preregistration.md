# Autobatch 001 — scoped Jev equivalence and execution boundaries

2026-09-25, base b0744f3. Freeze 12 synthetic new weekday messages before
provider calls. Reuse unchanged weekdaybench questions/model/scorer from
weekday-001 (8 questions, Jev 1.13.0, confidence .7). This is a new evaluation
set for an existing classifier, not broad authoring/conversation validation.

Run four sequential blocks: serial A, batch B, batch B, serial A, one repeat
of all 12 messages in each block. Each block uses the existing harness's four
concurrent samples. Total 24 samples per arm, 192 serial requests vs 24 batch
requests if no errors. Budget $0.05 per block, $0.20 total, metered. No Generate
or judge calls. Report correctness/unsafe counts, p50/p95 elapsed per sample,
reported cost, full-distribution deltas and threshold/value agreement by case
and repetition. ABBA reduces temporal confounding but is not full randomization.

Gate for retaining batch as the recommendation for declared independent groups:
each arm >=95% correct full masks, zero unsafe allowances, batch p50 at least
20% lower, no errors. Report every disagreement even if it passes. No tuning or
reruns to erase failure. No production change automatically follows this gate.

Separately inject low confidence in actual runtime sequential vs batch fixtures:
first question fails, second question uses the same state but is independent.
Measure how many questions reach the Decider. If batch sends a question serial
never sends, reject unconditional automatic merging even with equal final status.
Existing runtime tests cover invalid answers, human uncertainty policy, masking
and durable batch replay. No claim of crash-safe automatic fusion without an
implementation and recovery experiment.
