# Voice 006: shorter warm messages lose clarity and naturalness

- **Date:** 2026-09-24.
- **Pre-registration:** [voice-006-preregistration](voice-006-preregistration.md).
- **Result:** neither candidate passes; retain the voice-005 warm default.
- **Evidence:** `bench/results/voice-006/{A,B}`: 480 conversations and
  600 judged pairs (each judged in both orders), all complete.
- **Code measured:** `486083e` plus `bench/results/voice-006/candidate.patch`.
  Hashes are in `candidate.sha256`; B uses `bench/market/tessera-warm2`.
- **Reported spend:** $4.5238: market ledgers $0.1376 + $0.1282;
  judges $2.1279 + $2.1301. Below the $10 cap. The per-run costUSD
  sums are separately reported by the summarizer; those list-price
  estimates differ from the harness's shared-ledger total.

## Comparison (seed 24)

Control is the historical voice-004 warm run. A changes the warm prompt
and repeated-question instructions while retaining every fact. B adds
the single-time-in-the-requested-period explanation. Both use Flash.

| Metric | Control | A: wording | B: wording + explanation |
| --- | --- | --- | --- |
| Natural W / L / T | 86 / 70 / 144 | 58 / 130 / 112 | 53 / 124 / 123 |
| Natural share of decided | 55.1% | 30.9% | 29.9% |
| Clear W / L / T | 25 / 70 / 205 | 10 / 69 / 221 | 9 / 71 / 220 |
| Clear share of decided | 26.3% | 12.7% | 11.3% |
| Efficient W / L / T | 11 / 42 / 247 | 23 / 35 / 242 | 26 / 31 / 243 |
| Efficient share of decided | 20.8% | 39.7% | 45.6% |
| Mean agent message, characters | 174.1 | 136.6 | 136.0 |
| C0 p50 turn | 1.96 s | 1.88 s | 1.93 s |
| Distinct transcripts flagged for facts | 3 | 2 | 2 |

The messages are 21.5–21.8% shorter. Efficiency improves, but the
naturalness loss is large and clarity does not improve. Clarity losses
are nearly unchanged (70 → 69/71); wins become ties. The small A/B
difference provides no evidence to promote the extra explanation.

## Safety and hypotheses

Each candidate has 60 C0 runs and 60 runs per fault condition. Faults
actually fire in 48 runs per condition. Both achieve 60/60 success in C0
and 48/48 in each fired C1–C3 condition, with zero duplicate effects,
unconfirmed moves, foreign-patient moves or re-sends. No harness errors.

| Hypothesis | A | B |
| --- | --- | --- |
| C1: clarity +10 points | Not supported (−13.7) | Not supported (−15.1) |
| C2: efficiency +10 points | Supported (+18.9) | Supported (+24.9) |
| C3: natural ≥45% and length −10% | Not supported: natural 30.9% | Not supported: natural 29.9% |
| C4: success ≥97%, zero unsafe moves | Supported | Supported |
| C5: no new voice factual error among reviewed flags | Supported within this review | Supported within this review; a free-mode gap remains |

The raw judge flags and their manual review are preserved in
`bench/results/voice-006/factual-review.md`. Three flagged transcripts
have incorrect calendar objections. B/t13/repeat 2 exposes a real
free-mode information gap: it cannot answer from slots the Function has
already read ([#31](https://github.com/bbiangul/tessera/issues/31)).
The harness's successful final state for this no-move task does not mean
the conversation answered the user's question correctly.

## Decision and next step

Neither candidate advances to seed 22, as pre-registered. The measured
warm default is restored; the candidate patch and incoming local patch
are kept as research artifacts, and unit regressions protect facts on
repeated questions and confirmations. `tessera-warm2` remains an
experimental definition, not the recommended agent.

The next quality experiment should address information and flow, starting
with #31, rather than broadly shortening the voice. Supply only relevant,
authorized, masked reads to the free answer, reproduce the conversation
in an EvalSuite, and measure on a fresh off-script set. Keep the current
voice as the control. This is a proposed next step, not implemented here.

## Limits and reproduction

No unplanned live runs or tuning between A and B. C0 judging ran while
the remaining fault runs completed, after checking that all 60 C0 rows
were present. Historical control and competitor transcripts mean
provider drift is not isolated. Seed 24 is a reused development set,
not a held-out test; one synthetic domain, one text model family and one
automatic judge remain the limits. No replication claim is made.

Recompute the table with:

```bash
python3 bench/results/voice-006/summarize.py
```

To reproduce the candidate, use an isolated checkout of `base-commit.txt`
and apply `candidate.patch`; use the definitions and commands in the
pre-registration and handover. Do not apply the incoming patch on top:
it is a preservation artifact and contains an unmeasured fact-lock
relaxation that was deliberately not used.
