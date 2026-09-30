# Voice 005: replication of the warm voice on another seed, pre-registration

- **Date:** 2026-09-24, before the run.
- **Claim under test** ([voice-004](voice-004.md)): `voice.latitude: warm`
  raised the natural share from 16% to 55%, and clear from 9% to 26%, on
  seed 24. Every voice round used that seed.
- **Replication:** seed 22 (`tasks-002.json`, market-002's 20 tasks, with
  its competitors' C0 transcripts).
  - Tessera `tessera-rich` (strict) and `tessera-warm`, both on Flash;
  - C0, 3 repeats each;
  - each variant's transcripts judged against market-002's competitors,
    with the same judge and both orders.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| R1 | Warm beats strict on another seed | Warm's natural share ≥ strict's + 15 points |
| R2 | Warm is at least even with the LLM stacks | Warm's natural share ≥ 45% of decided pairs |
| R3 | Clarity | Warm's clear share ≥ strict's + 10 points |
| R4 | Nothing breaks | C0 ≥ 97% in both, 0 unconfirmed moves |

**If R1 and R2 hold,** `warm` becomes the default latitude, and the claim
"Tessera's conversation is as natural as the LLM stacks' on the same
model" is written with its limits: one domain, one judge, synthetic
patients.

**Budget:** $5.
