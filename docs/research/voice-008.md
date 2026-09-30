# Voice 008: the warm voice with its dialogue back (#40), results

- **Date:** 2026-09-25
- **Pre-registration:** [voice-008-preregistration](voice-008-preregistration.md).
- **Code measured:** branch `voice-context-fix`, `3fc4bb8`: `voice.dialogue:
  full` against `marker`, today's default. Results are in
  `bench/results/voice-008`.
- **Spend:** about $8.8:
  - safety runs $0.02, safety judge $0.59;
  - live regressions $0.005;
  - market runs about $0.9;
  - three naturalness judges $6.6.

## Results

### Safety (voice-007's promise cases, through the production rewrite)

| Set | Mode | Unsupported claims | Invented facts | Content lost | Fallbacks | p95 |
| --- | --- | --- | --- | --- | --- | --- |
| dev (24 × 3) | marker | 0 / 72 | 0 | 0 | 0 | 1410 ms |
| dev (24 × 3) | **full** | **0 / 72** | 0 | 0 | 0 | 1539 ms |
| replication (12 × 3) | marker | 0 / 36 | 0 | 0 | 0 | 1508 ms |
| replication (12 × 3) | **full** | **0 / 36** | 0 | 0 | 0 | 1430 ms |

### Regressions (#33, #34), live with `dialogue: full`

- `examples/clinic-confirmation`: 7/7, including
  `unsupported-service-promise` (#34).
- `examples/clinic-context`: 3/3.
- One live run each.

### Pairwise naturalness against the competitors (blind, both orders, 300 pairs per row)

| Seed | Mode | Natural W / L / T | **Natural, share of decided** | Clear | Efficient |
| --- | --- | --- | --- | --- | --- |
| 44 | marker (market-bench-004) | 55 / 116 / 129 | 32% (26–39) | 15% | 22% |
| 44 | **full** | 78 / 89 / 133 | **47% (39–54)** | 31% | 13% |
| 45 (fresh) | marker | 43 / 122 / 135 | 26% (20–33) | 12% | 28% |
| 45 (fresh) | **full** | 54 / 115 / 131 | **32% (25–39)** | 21% | 28% |

The seed-45 competitors are fresh from this round: 100% C0 each, $0.0012
per run, p50 2.50–2.65 s.

### Faults, seed 44, `full`

C0–C3: 100% each, 0 duplicates, 0 unconfirmed moves. p50 2.16 s in C0,
$0.0006 per run.

## Hypotheses

| # | Verdict |
| --- | --- |
| V1: `full` ≤ 1 unsupported claim in 108 | **Supported:** 0 in 108, the same as `marker`. |
| V2: the regressions hold | **Supported:** 10/10 live. |
| V3: seed 44, `full` ≥ 45% natural | **Supported:** 47% (39–54), up from 32%. |
| V4: seed 45, `full` − `marker` ≥ 10 points | **Not supported:** +6 points (26% → 32%). The direction replicates; the size does not. |
| V5: the guarantees hold | **Supported:** 100%, 0, 0. |

**By the pre-registered rule, `marker` stays the default:** V4 failed.

## What the transcripts show

The judge flagged 6 `full` transcripts on seed 45 for a fact (0 for
`marker`). Checked by hand, none is a fact the voice invented:
- **t04:** 9h and 11h offered for an afternoon request. The period filter
  missed "de tarde"; that is content.
- **t14 (×2):** the list of days repeated after "none of these is a
  Saturday". The repeated question sends the whole list again; that also
  happens with `marker`.
- **t11:** the voice wrote **"te ajudo com o remarc"** and **"Seu remarc
  foi feito"**.
  - `keep: [remarc]` is a stem meant to match "remarcar" and
    "remarcação". The instruction shows it to the model as a word to keep,
    and with the dialogue in view the model sometimes writes it bare.
  - Bare "remarc" appears in 5 of 120 `full` conversations (both seeds)
    and 0 of 120 `marker` conversations.
  - It is a defect of the `keep` instruction's wording, which `full`
    exposes.

## Reading

- **Giving the dialogue back is safe:** 0/108 promises, and the #34
  scenario passes live.
- **It helps on both seeds:** natural +15 and +6, clear +16 and +9.
- **But the pre-registered replication bar was not met,** and on the
  fresh seed even `full` (32%) is far from parity.

Seed 45 is harder for Tessera in general: `marker` scores 26% there,
against 32% on seed 44. So the gap to parity is not only voice-007. On
seed 45 the losses also come from the repeated day list and the period
filter.

## What next

1. **Fix the `keep` instruction:** show stems as prefixes ("words starting
   with remarc-"), so the model never writes a bare stem.
2. **Fix the repeated question:** a repeated question sends the whole
   list again. freemode rounds and voice-005's t14 show the same
   behaviour.
3. **Then measure `full` again,** after those two fixes, on another fresh
   seed (46), pooled with 44 and 45, and decide the default there.

The owner may also decide now: the effect is in the same direction on
both seeds, with no safety cost. That is a decision against the
pre-registered bar, so it should be written down as such.
