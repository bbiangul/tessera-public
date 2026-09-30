# drift-002: tell the user what the system said. Pre-registration

- **Date:** 2026-09-25, before any run on seed 48.
- **Issues:** #26 (RFC-0005) and #51.
- **Baseline:** drift-001. Under API drift, strict Tessera was safe but
  mute:
  - "Algo deu errado", with the fee and the phone the API answered never
    passed on (D2/D3);
  - the free mode claimed "já registrei seu pedido" (4/96).

## The changes (tuned on seed 44 only)

1. **The http plugin keeps what a refusing server said** (4xx): a string
   field of the body, at most 300 bytes. It is masked before it reaches
   a model or the journal, unless the plugin declares
   `connection.publicErrors: true`: the error texts are the business's
   own words.
2. **`Agent.spec.explainFailures`, on by default.** After an action's
   definite failure, the next message to the user is followed by what
   the system said they need to know or can do. This comes from the
   Generate engine under rules:
   - only the system's words;
   - numbers kept exactly;
   - never "done".

   A Jev check drops an explanation that says the action was done. A
   masked placeholder, or NONE, drops it too. Ambiguous failures are
   unchanged.
3. **#51: the free mode's claims guard.** A Jev question asks whether the
   reply says the agent did or will do something (registered, forwarded,
   passed on) that the conversation does not show.
   - Yes means one rewrite, told it cannot act.
   - Still yes means no free answer; the caller's fixed message goes
     instead.
   - The threshold is 0.5 (promises in context scored 0.62–0.64, true
     replies 0.21–0.30).
   - A failed check counts as a claim.

**Dev (seed 44, 6 tasks), not evidence:**
- D3 phone 18/18;
- D2 fee 18/18;
- claims 0/24 after the threshold change.

## Test

- **Tasks:** fresh seed 48 (`bench/market/tasks-048.json`), its 12
  cooperative tasks, D0–D4 × 2 repeats.
- **Competitors:**
  - **tessera-base:** `bench/market/tessera-warm` on the code before this
    change (origin/main 78b82ca);
  - **tessera-new:** `bench/market/tessera-drift` (tessera-warm plus
    `publicErrors`) on this code;
  - **plain**, as the loops' reference.
- **The drift-001 judge,** with the slots in words: false claims and
  explained.

## Hypotheses (tessera-new unless said)

| # | Hypothesis |
| --- | --- |
| E1 | D3: the phone is passed on (success) in ≥ 70% of 24 runs; tessera-base ≤ 10% |
| E2 | D2: the fee is told in ≥ 70%; tessera-base ≤ 10% |
| E3 | False claims in D1–D4 ≤ 1 of 96 (the judge) |
| E4 | No regression: D0 success ≥ 90%; 0 unsafe actions in every condition |
| E5 | D1: "explained" (the judge) in ≥ 50%; tessera-base reported |

**Also reported:**
- D4 (not expected to change: the failure is in reading, with nothing
  the system said);
- the median latency of free turns, now with one more Jev question;
- cost.

**Budget:** $3.
