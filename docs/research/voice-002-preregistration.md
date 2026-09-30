# Voice 002: naturalness ablation, pre-registration

- **Date:** 2026-09-24, before the measured runs.
- **Why:** [freemode-002](freemode-002.md) found the blind judge's
  "natural" share falling from 20% (voice-001) to 13%, then 7%. The judge
  is stable: a re-judge of the same pairs gave 19%. So the change is in
  Tessera's conversations. Candidate causes:
  - the greeting rules (greet once, and remove a later greeting with the
    name);
  - content: the competitors name the dentist and both dates, structure
    the confirmation, and offer only the times of the period asked for.

  This run tests them one at a time, instead of choosing style by taste.

## Variants

All four run on seed 24 (market-003's tasks), C0, 3 repeats, 60
conversations each, with the code of this branch. The branch adds
`voice.greeting: first|any`, the language fix and the previous-run
context.

| Variant | Definitions | What differs |
| --- | --- | --- |
| **A, control** | `bench/market/tessera` | Today: greeting `first` |
| **B, greetings free** | `bench/market/tessera-greet` | `voice.greeting: any`: no rule, nothing removed (voice-001's behaviour) |
| **D, richer content** | `bench/market/tessera-rich` | Authoring only, greeting `first`:<br>• the confirmation and the final message name the dentist and both dates;<br>• the patient's period (manhã/tarde) narrows the times offered;<br>• the persona allows short lines in a confirmation |

A variant that keeps the name when a greeting is removed is left for
later, if B wins.

**Development** (seed 23, 1 repeat): D had 20/20 successes. Its
transcripts show afternoon-only times for "de tarde", and a direct
confirmation when the period leaves one time.

## Measure

- Each variant's C0 transcripts are paired with the competitors' market-003
  transcripts: 300 pairs, the same judge, both orders.
- Success, duplicates and unconfirmed moves in C0.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W1 | The greeting rule cost naturalness | B's natural share of decided pairs ≥ A's + 10 points |
| W2 | Content costs naturalness and clarity | D's natural share ≥ A's + 10 points, **or** D's clear share ≥ A's + 10 points |
| W3 | No variant breaks the process | C0 success ≥ 97% in every variant, 0 unconfirmed moves |

10 points is about the width of one variant's 95% interval at these
sizes. Smaller differences are reported, not called.

**What happens next:** the winning settings become the defaults (the
greeting), or go into the authoring guide (content). They are then
checked on a new seed before any claim.

**Budget:** $7: 3 × (runs ~$0.03 + judge ~$2.1).
