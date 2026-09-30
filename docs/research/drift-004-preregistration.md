# drift-004: explaining failures, on against off. Pre-registration

- **Date:** 2026-09-25, before any run on seed 50.
- **Issues:** #26, #51.

## Changes since drift-003 (tuned on seed 44 only)

1. **An explain rule:** the system's instructions to the client software
   ("resend with acceptFee: true") are not steps for the user. Say what
   they mean for the user, never "tell me and we continue".
2. **The judge, fixed where drift-003 showed gaps.** It now sees:
   - what the API answered the refused moves;
   - whether the agent handed the case to the clinic's queue, a fact
     the harness records from the run (only stacks with a team channel
     can).

   "The case is with the team" is true when it was handed over. This is
   the same judge for every competitor, still blind to which one it is.
   drift-003 re-judged this way: tessera 10, plain 9 false claims
   (`judged-refusals.jsonl`), before the escalation fact was added.

**Dev (seed 44, D1–D3 × 6 tasks × 2):**
- explain on: 0/36 false claims, 36/36 explained;
- explain off: 5/36 false claims, 0/36 explained.

## Test

- **Tasks:** fresh seed 50 (`bench/market/tasks-050.json`), 12
  cooperative tasks, D0–D4 × 2 repeats.
- **Variants:**
  - **tessera-on** (`bench/market/tessera-drift`);
  - **tessera-off** (`bench/market/tessera-warm`: the same agent, explain
    off);
  - **plain**.
- **The judge:** `--refusals`.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| G1 | D3: the phone passed on, on ≥ 70%; off ≤ 10% |
| G2 | D2: the fee told, on ≥ 70%; off ≤ 10% |
| G3 | False claims in D1–D4: on ≤ 4 of 96, and ≤ off |
| G4 | Explained in D1–D3: on ≥ 70% of 72; off reported |
| G5 | D0 ≥ 90% for on; 0 unsafe actions everywhere |

**The default flip,** as registered in drift-003: if G1–G5 hold, and
every example's EvalSuite passes live with explain on, it becomes the
default and the cassettes are re-recorded.

**Budget:** $3.
