# Voice 005: replication of the warm voice, results

- **Date:** 2026-09-24
- **Pre-registration:** [voice-005-preregistration](voice-005-preregistration.md).
- **Results:** `bench/results/voice-005/{strict,warm}`. Spend: $4.28 for
  the two judges, and about $0.06 for the runs.

## Results (seed 22, market-002's tasks and competitor transcripts; C0, 60 conversations per variant)

| | Strict (D) | **Warm (D + latitude warm)** |
| --- | --- | --- |
| Natural W / L / T | 28 / 183 / 89 | **95 / 79 / 126** |
| Natural, share of decided | 13% (9–19) | **55% (47–62)** |
| Clear, share of decided | 3% (1–7) | **34% (26–43)** |
| Efficient, share of decided | 33% | 16% (9–26) |
| Tessera transcripts flagged for a fact | 5 | 1 |
| C0 success, unconfirmed moves | 100%, 0 | 100%, 0 |
| p50 turn | 2.16 s | 2.16 s |

## Hypotheses

| # | Verdict |
| --- | --- |
| R1: warm ≥ strict + 15 points (natural) | **Supported:** +42 points. |
| R2: warm ≥ 45% natural | **Supported:** 55%. |
| R3: clear ≥ strict + 10 points | **Supported:** +31 points. |
| R4: nothing breaks | **Supported:** 100% in both, 0 unconfirmed moves. |

## The claim, with its limits

> On the same model (DeepSeek V4.1 Flash), Tessera's conversation is as
> natural as the LLM stacks' in a blind judge's eyes.
>
> It wins 55% of the decided pairs on two seeds, where the strict voice
> won 13–16% and the templates 0%. Its facts stay locked by the
> platform's checks, and the fault guarantees are unchanged: 0 duplicates
> under act-then-timeout, crash and retried delivery (market-003,
> freemode-002).

**Limits:**
- one domain;
- one judge (Claude Sonnet 5, stable on re-judging);
- synthetic patients (DeepSeek);
- 20 tasks per seed.

**Still behind the LLM stacks:**
- clarity: 26–34% of decided pairs;
- efficiency: 16–21%. The warm messages are longer.

`warm` is now the default latitude (this commit). `strict` stays for texts
that must go out exactly as written.
