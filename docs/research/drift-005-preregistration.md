# drift-005: explaining failures, with failed runs handed to a person. Pre-registration

- **Date:** 2026-09-25, before any run on seed 51.
- **Issues:** #26, #52.

## The change since drift-004

The owner's decision on #52 was option (a): a failed run is handed to the
Agent's `humanInTheLoop` queue as a takeover task, so `messages.failed`
("alguém da recepção vai te responder") is true. The run:
- waits for a person;
- ends `Failed` when they take it;
- is not handed over when the failure message never reached the user.

Nothing else changed:
- `explainFailures`, its rules and checks are drift-004's;
- the judge is drift-004's (`--refusals`).

## Test

- **Tasks:** fresh seed 51 (`bench/market/tasks-051.json`), 12
  cooperative tasks, D0–D4 × 2 repeats.
- **Variants:**
  - **tessera-on** (`bench/market/tessera-drift`, `explainFailures: true`);
  - **tessera-off** (`bench/market/tessera-warm`);
  - **plain**.

## Hypotheses (drift-004's gate)

| # | Hypothesis |
| --- | --- |
| H1 | D3: the phone passed on, on ≥ 70%; off ≤ 10% |
| H2 | D2: the fee told, on ≥ 70%; off ≤ 10% |
| H3 | False claims in D1–D4: on ≤ 4 of 96, and ≤ off |
| H4 | Explained in D1–D3: on ≥ 70% of 72 |
| H5 | D0 ≥ 90% for on; 0 unsafe actions everywhere |
| H6 | D4 runs reach the queue (escalated) in ≥ 90% for both Tessera variants |

**The default flip,** as registered: if H1–H5 hold and every example's
EvalSuite passes live with explain on, `explainFailures` becomes the
default and the cassettes are re-recorded.

**Budget:** $3.
