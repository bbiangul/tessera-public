# Free mode 002: results

- **Date:** 2026-09-24
- **Pre-registration:** [freemode-002-preregistration](freemode-002-preregistration.md).
- **Code measured:** commit `af028c6`. Results are in
  `bench/results/freemode-002`.
- **Spend:** $3.65:
  - off-script runs $0.33, their judge $1.13;
  - market runs $0.12;
  - naturalness judge $2.06.

  The judge re-check below cost $2.10 more.

## Results

### Off-script, held out (seed 33, 360 conversations)

| Competitor | Success (95% CI) | False claims | Dead ends | Humane (1–5) |
| --- | --- | --- | --- | --- |
| **tessera** | **100%** (94–100) | **0** | **0** | 4.43 |
| plain | 100% (94–100) | 0 | 0 | 4.67 |
| langgraph | 100% (94–100) | 0 | 0 | 4.75 |
| langgraph-sqlite | 100% (94–100) | 0 | 0 | 4.70 |
| agents-sdk | 98% (91–100) | 1 | 0 | 4.73 |
| agents-sdk-sqlite | 100% (94–100) | 0 | 0 | 4.77 |

- Tessera succeeded on every kind: new-request-midway was 6/6, up
  from 3/6 in freemode-001.
- A side question now gets its answer, then the pending question as a
  continuation. For example: "E sobre a remarcação: posso remarcar sua
  consulta de sexta 02/10 às 9h para terça 06/10 às 10h?"
- The Agents SDK's miss: it moved an appointment the patient had not
  confirmed.

**Later greetings:** 2 of 60 Tessera runs, down from 21. Both open a
new run in the same conversation (a greeting, then a reschedule; a
reschedule, then a request for the family). A run's voice saw only its
own lines. This is fixed on the next branch: a run records the previous
run's last lines when it starts.

### Market (seed 24, Tessera, 240 conversations)

| Condition | Fired | Success | Duplicates | Unconfirmed moves | p50 / p95 turn |
| --- | --- | --- | --- | --- | --- |
| C0 | 60 | 100% (94–100) | 0 | 0 | 2.01 / 4.34 s |
| C1 | 48 | 100% (93–100) | 0 | 0 | 3.15 / 22.0 s |
| C2 | 48 | 100% (93–100) | 0 | 0 | 1.86 / 3.34 s |
| C3 | 48 | 100% (93–100) | 0 | 0 | 1.68 / 3.52 s |

### Naturalness (300 pairs, both orders)

Tessera's wins / losses / ties:

| | voice-001 | freemode-001 | **freemode-002** |
| --- | --- | --- | --- |
| Natural | 38 / 154 / 108 | 26 / 179 / 95 | **16 / 206 / 78** |
| Clear | 0 / 159 / 141 | 0 / 161 / 139 | **1 / 173 / 126** |
| Efficient | 18 / 59 / 223 | 18 / 32 / 250 | **22 / 29 / 249** |
| Natural, share of decided | 20% (15–26) | 13% (9–18) | **7% (4–11)** |

The factual flags (18 distinct transcripts) are the same kinds as
before: the period relevance defect, and the judge's date errors.

## Is the judge stable?

The fall from 20% to 7% could be the judge. It was checked by judging
voice-001's 300 pairs again, the same inputs:

| | First run | Second run | Same verdict |
| --- | --- | --- | --- |
| Natural, share of decided | 20% | 19% | 262 / 300 |
| Clear | 0% | 1% | 249 / 300 |
| Efficient | 23% | 30% | 280 / 300 |

**The judge is stable on "natural".** The decline comes from the
transcripts, not from noise.

What changed in Tessera's C0 messages:

| | market-003 (templates) | voice-001 | freemode-001 | freemode-002 |
| --- | --- | --- | --- | --- |
| Messages with an emoji | 0% | 60% | 45% | 30% |
| Messages opening with a greeting | 0% | 41% | 36% | 37% |
| Mean length (chars) | 96 | 115 | 111 | 108 |
| Messages sent in the voice | — | 95% | 97% | 98% |

Fallbacks to the template did not grow, so they do not explain it.
Between voice-001 and freemode-002:
- the voice gained a rule to greet only in the first message;
- the voice now removes a greeting, and the user's name with it
  ("Oi, Bruno! …");
- the pending question is re-asked as a continuation.

Emoji use halved, and "warmth" is named in 54% of lost pairs. These
runs cannot tell those causes apart. That needs an ablation on the same
tasks: see "What changes".

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| F1 | ≤ 2 of 60 off-script runs greet again | **Supported:** 2, down from 21. |
| F2 | 0 effects outside Functions, 0 false claims | **Supported:** state right 60/60, 0 flags. |
| F3 | Off-script success ≥ 90%, dead ends ≤ 5% | **Supported:** 100%, 0. |
| F4 | Market C0 ≥ 97%, 0 duplicates in C1 and C3, 0 unconfirmed moves | **Supported:** 100% everywhere, 0, 0. |
| F5 | Natural ≥ 20% of decided pairs | **Not supported:** 7% (4–11). The pre-registration expected this might fail, but not that it would get worse. |
| F6 | p50 turn ≤ 2.0 s in C0 | **Not supported, by 10 ms:** 2.01 s in the market's C0. Off-script C0: 1.78 s. |

## Conclusion

Off-script handling is now at the LLM stacks' level on held-out tasks:
100% success, no false claims, no dead ends. It keeps what the others
lack: 0 duplicates and 0 unconfirmed moves under faults.

The blind judge finds Tessera's conversations less natural than
before, and the judge is stable. Style rules chosen by intuition (greet
once, no name) cost warmth in its eyes. From here, style changes are
decided by an ablation, not by taste.

## What changes

1. **The later-run greeting:** fixed on the next branch, with a test.
2. **Naturalness ablation (voice-002),** Tessera on seed 24 C0 against
   the same competitors' transcripts, pre-registered. One variant per
   suspected cause:
   - greetings allowed as in voice-001;
   - the name kept when a greeting is removed;
   - richer templates: the full date, the dentist.
3. **Latency:** the voice rewrite and the re-ask each add a model call.
   The p50 hovers at 2.0 s.
