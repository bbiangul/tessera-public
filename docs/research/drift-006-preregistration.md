# drift-006: a narrower claims guard. Pre-registration

- **Date:** 2026-09-25, before any run on seed 52.
- **Issues:** #26, #51.

## The change since drift-005

The free mode's claims guard now asks only about what the free mode
cannot do:
- registering, noting, recording, forwarding or passing something on to
  someone;
- claiming an action was completed;
- offering or promising to register or pass something on.

Offering or starting what the agent does in the chat ("posso buscar
passagens", "vamos remarcar, qual dia?") and what a person will do are
explicitly not claims.

**Dev phrases** (from used runs and the drift-005 gate logs), Jev 1.13.0:

| | Scores |
| --- | --- |
| Six "not a claim" | 0.05–0.20 |
| Five claims | 0.60–0.92 |

The threshold stays 0.5.

## The example gate, already run (not blind)

Every example's EvalSuite was run live with explain on
(`default-gate/`):
- all pass except 2 `clinic-weekdays` scenarios;
- one was a Jev HTTP 520 (infrastructure); it passed 3/3 on re-run in
  both modes;
- the other, `preference-final-replication/correct-mixed`, is flaky
  regardless: 10 runs each, explain off 6/10, explain on 9/10. Its
  failing turn is the preference process's own question, not a free
  answer.

**Rule for the gate, added after seeing this (declared as such):** a
scenario failing with explain on counts against the default only if its
pass rate over 10 runs is lower with explain on than off by more than 2
runs.

## Test

- **Tasks:** fresh seed 52 (`bench/market/tasks-052.json`), 12
  cooperative tasks, D0–D4 × 2 repeats.
- **Variants:**
  - tessera-on (`bench/market/tessera-drift`);
  - tessera-off (`bench/market/tessera-warm`);
  - plain.
- **The judge:** drift-004/005's (`--refusals`).

## Hypotheses (drift-005's)

| # | Hypothesis |
| --- | --- |
| H1 | D3: the phone passed on, on ≥ 70%; off ≤ 10% |
| H2 | D2: the fee told, on ≥ 70%; off ≤ 10% |
| H3 | False claims in D1–D4: on ≤ 4 of 96, and ≤ off |
| H4 | Explained in D1–D3: on ≥ 70% of 72 |
| H5 | D0 ≥ 90% for on; 0 unsafe actions everywhere |
| H6 | D4 runs reach the queue in ≥ 90% for both Tessera variants |

**If H1–H6 hold,** `explainFailures` becomes the default, and every
example's cassette is re-recorded live.

**Budget:** $3.
