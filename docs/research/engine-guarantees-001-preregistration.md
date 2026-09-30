# Engine guarantees 001 — failure semantics before automatic batching

2026-09-25. Owner direction: enforce universally beneficial behavior in the
engine, retain author freedom where design changes meaning.

Hypothesis: single `decide` currently treats provider errors as uncertainty,
can accept absent yes/no answers as confident false, and can panic on nil
responses. `decideBatch` already rejects these. This is a correctness study,
not a model-quality or latency experiment. Provider budget: $0, scripted only.

Before changing production, add fault-injection regressions for nil/missing,
malformed answers and provider errors under continue/fail/human policies.
Acceptance: all failures stop the step without continuing or asking a human;
valid uncertainty retains author policy. Run race tests, CI and static checks.
Record the failing baseline and final results in bench/results/engine-guarantees-001.

Do not automatically merge sequential steps: even equal independent states
can have different failure, human-wait and journal semantics. Explicit batches
already make one Jev request. Automatic scheduling requires a separate contract
and experiment; no speedup or universal equivalence claim in this change.
