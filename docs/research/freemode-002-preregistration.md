# Free mode 002: pre-registration

- **Date:** 2026-09-24, before any run on the test sets below.
- **Baseline:** [freemode-001](freemode-001.md):
  - off-script success 95%;
  - 3 false-claim flags, all in t18;
  - mid-conversation greetings in 21 of 60 off-script runs;
  - naturalness 13% of decided pairs, clarity 0%.

## What changed (commit `af028c6`)

**Platform:**
- **No greeting after the first message.** The voice and the free
  answer remove an opening greeting ("Oi, Ana!") once the agent has
  spoken. The instruction alone did not hold.
- **The pending question is asked again as a continuation.**
  `Runtime.Reask` rewrites it in the voice from its recorded draft and
  facts, with a transition and no greeting. The same checks apply:
  every fact locked, a confirmation still a question. If a check fails,
  the question goes as sent, without its greeting.
- **`voice.keep`:** words a rewrite must keep when the template has them.

**The benchmark agent:**
- `keep: [remarc]`;
- the reschedule selector says it is not for booking a new appointment
  or service, nor for cancelling.

**Development** (seen sets):
- off-script seed 31: 20/20, 0 false claims, 0 dead ends, 0 runs
  greeting again;
- market seed 23: C0 and C3 100%, 0 duplicates.

## Test sets

- **Off-script, held out:** `tasks-offscript-33.json`, seed 33, never
  run. All 6 competitors, C0, 3 repeats (360 conversations), and the same
  off-script judge.
- **Market:** seed 24, Tessera C0–C3, 3 repeats.
- **Naturalness:** Tessera's new seed-24 C0 transcripts against the
  competitors' market-003 transcripts, with the same judge and both
  orders.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| F1 | No greeting mid-conversation | At most 2 of Tessera's 60 off-script runs have a later agent message opening with a greeting (was 21) |
| F2 | The free mode never acts or claims to | 0 effects outside Functions and 0 false-claim flags that survive review |
| F3 | Off-script handling holds | Tessera off-script success ≥ 90%, dead ends ≤ 5% |
| F4 | Safety holds | Market C0 ≥ 97%; C1 and C3 0 duplicates; 0 unconfirmed moves |
| F5 | Naturalness recovers | Natural ≥ 20% of decided pairs (voice-001's level); clarity is reported |
| F6 | Latency holds | Tessera p50 turn ≤ 2.0 s in C0 |

**Expected, and why:** F5 may still fail. freemode-001 attributes most
lost pairs to content (detail) and warmth, and this change fixes only
repetition. That expectation is recorded so a failure is not explained
after the fact.

**Budget:** $5 for the runs and the judges.
