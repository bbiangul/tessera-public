# Voice 009: dialogue: full after the two fixes, results

- **Date:** 2026-09-25
- **Pre-registration:** [voice-009-preregistration](voice-009-preregistration.md).
- **Code measured:** branch `voice-context-fix`, `a2ffdf6`. Results are in
  `bench/results/voice-009`.
- **Spend:** about $8.5:
  - safety runs and judge $0.6;
  - regressions and re-recordings $0.08;
  - market runs about $1.1;
  - three naturalness judges $6.6.

## Results

| # | Hypothesis | Result | Verdict |
| --- | --- | --- | --- |
| W1 | Still safe | Safety: 0/108 unsupported claims for both modes, and 0 invented facts, lost content or fallbacks. #33/#34: 10/10 live with `full`. Seed 46 C0–C3: both modes 100%, 0 duplicates, 0 unconfirmed moves | **Supported** |
| W2 | The default did not regress | New-code `marker`, seed 44: 30% (24–37) natural, against market-bench-004's 32% | **Supported** (bar 25%) |
| W3 | The stem defect is gone | 0 bare "remarc" in 1,406 agent messages (seed 46, both modes, C0–C3; seed 44 marker) | **Supported** |
| W4 | `full` beats `marker` on fresh seed 46 | `full` 44% (36–53), `marker` 33% (26–42): **+11** | **Supported** (bar +5) |
| W5 | Pooled over seeds 44, 45 and 46 | `full` 194 / 476 = **40.8%**, `marker` 142 / 468 = **30.3%**: **+10.4** | **Supported** (bar +8) |

Per seed (natural wins / losses against the five LLM stacks, blind, both
orders):

| Seed | marker | full |
| --- | --- | --- |
| 44 | 55 / 116 (32%) | 78 / 89 (47%) |
| 45 | 43 / 122 (26%) | 54 / 115 (32%) |
| 46 | 44 / 88 (33%) | 62 / 78 (44%) |

**Clarity**, reported, with no threshold:
- `full`: 31%, 21%, 26%;
- `marker` on the new code: 8% on seeds 44 and 46. market-bench-004's old
  code had 15% on seed 44, with overlapping intervals.

With `full` as the default, the lower clarity of `marker` does not reach
users unless an agent opts in. It is recorded, and it is the first thing
to look at if `marker` is ever made the default again.

One seed-46 judgment was malformed (a missing field). It is counted as a
tie on "natural" and "clear".

## Decision

**Every condition of the pre-registered rule holds, so `voice.dialogue:
full` is now the default** (commit after `a2ffdf6`). `marker` remains
available as an opt-in.

**What made the change safe to ship:**
- the default's regression check (W2);
- the safety cases through the production path;
- the #33/#34 scenarios live;
- the fault conditions;
- the clinic cassettes whose requests changed, re-recorded live twice (for
  the fixes, then for the default: 63/63 each time) and replayed in CI
  (102 scenarios).

## Limits

- One domain (the clinic).
- One judge (Claude Sonnet 5).
- Synthetic patients.
- Seeds 44 and 45 were measured with voice-008's code, and seed 46 with
  this one.
- Parity with the LLM stacks (≥ 45%) is reached on seed 44 only. The
  pooled 40.8% is still below. The rest of the gap is content and flow
  (voice-008): the period filter, the questions.
