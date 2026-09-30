# Voice 003: a larger model for the voice, results

- **Date:** 2026-09-24
- **Pre-registration:** [voice-003-preregistration](voice-003-preregistration.md).
- **Results:** `bench/results/voice-003`. Spend: $2.08 for the judge, and
  about $0.04 for the runs.
- **Harness deviation:** `-model` also set the patient simulator's model.
  The first attempt was stopped before any judging. A new
  `-tessera-model` flag changes only Tessera's Generate model (commit
  after `cde873e`). The simulator and the competitors stayed on Flash.

## Results (seed 24, C0, 60 conversations; 300 judged pairs)

| | D on Flash (voice-002) | **D on V4 Pro** |
| --- | --- | --- |
| Natural W / L / T | 32 / 168 / 100 | 9 / 226 / 65 |
| Natural, share of decided | 16% (12–22) | **4%** (2–7) |
| Clear, share of decided | 9% (5–17) | 3% (1–8) |
| C0 success | 100% | 100% |
| p50 turn | 2.02 s | **3.47 s** |
| Mean message length | 133 chars | 123 |
| Messages with an emoji | 25% | 10% |
| Messages on several lines | 16% | 9% |

The same task (t06), last two messages:
- **Flash:** "Perfeito, Fábio! Só confirmando: posso remarcar … 😊", then
  "Pronto, Fábio! Sua consulta com o dentista D3 foi remarcada: / - De:
  … / - Para: …"
- **Pro:** "Posso remarcar sua consulta com o dentista D3 de … para …?
  Responda sim para confirmar.", then "Pronto, Fábio! Sua consulta … foi
  remarcada de … para …."

## Hypotheses

| # | Verdict |
| --- | --- |
| M1: natural ≥ 26% | **Not supported:** 4%, worse than Flash. |
| M2: clear ≥ 19% | **Not supported:** 3%. |
| M3: nothing breaks | **Supported:** 100%. |

## What this says

The larger model follows the voice's rules more literally:
- "Say what DRAFT says and nothing more";
- "Keep it short";
- "at most light formatting".

So it returns something close to the template. Flash takes more liberty,
which is exactly what the judge rewards: acknowledgements, transitions,
an emoji, the facts on lines.

**The limit is the voice's instructions, not the model's size.** They were
written to lock the facts, and they also lock the warmth. The next round
keeps the fact lock and allows the rest, on Flash.
