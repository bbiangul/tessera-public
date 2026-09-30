# drift-003: say plainly it was not done. Pre-registration

- **Date:** 2026-09-25, before any run on seed 49.
- **Issues:** #26, #51.
- **After drift-002:** the explanation passed the phone and the fee on,
  but judged false claims rose to 41/96. Two causes:
  - it was appended after a "a equipe vai confirmar" escalation message;
  - the free mode said the team "will continue with the rescheduling".

## Changes (tuned on seed 44 only)

1. **The failure's message is rewritten**, not appended to. The rewrite:
   - opens by saying the action was not done;
   - passes on what the system said, with the user's own next step when
     the system asks them to act;
   - keeps what the message says a person will do.

   **Checks:**
   - every number of the system's words and of the message must survive,
     digit for digit;
   - a Jev question drops a rewrite that says the action was done or is
     on its way;
   - on any failed check, the original message goes.
2. **The claims guard covers offers** ("já repasso", "se me contar, eu
   anoto").
3. **While a person handles a request,** the free mode says it is with
   them and they will reply, never what they will decide or do with it.

**Dev (seed 44), not evidence:**
- D1: 0/12 false claims, 12/12 explained;
- D2: 1/12 false, 11/12 explained, fee 12/12;
- D3: 0/12 false, phone 12/12;
- D0: 12/12.

## Test

- **Tasks:** fresh seed 49 (`bench/market/tasks-049.json`), 12
  cooperative tasks, D0–D4 × 2 repeats.
- **Competitors:**
  - `tessera` (`bench/market/tessera-drift`, `explainFailures: true`);
  - `plain` as the reference.
- **The judge:** the same as drift-001/002.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| F1 | D3: the phone passed on ≥ 70% (24 runs) |
| F2 | D2: the fee told ≥ 70% |
| F3 | Judged false claims in D1–D4 ≤ 4 of 96 (drift-001/002's strict-Tessera level was 3–4) |
| F4 | Judged "explained" in D1–D3 ≥ 50% of 72 |
| F5 | D0 ≥ 90%; 0 unsafe actions everywhere |

**The default flip.** If F1–F5 hold, `explainFailures` becomes the
default only if every example's EvalSuite also passes live with it on.
The claims guard then runs on every free answer, and the cassettes are
re-recorded. Any failure there keeps it opt-in and is reported.

**Budget:** $2.
