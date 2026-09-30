# Voice 004: a warmer voice, results

- **Date:** 2026-09-24
- **Pre-registration:** [voice-004-preregistration](voice-004-preregistration.md).
- **Results:** `bench/results/voice-004`. Spend: judge $2.19, runs about
  $0.04.

## Results (seed 24, C0, 60 conversations; 300 judged pairs)

| | D, strict (voice-002) | **D, warm** |
| --- | --- | --- |
| Natural W / L / T | 32 / 168 / 100 | **86 / 70 / 144** |
| Natural, share of decided | 16% (12–22) | **55% (47–63)** |
| Clear, share of decided | 9% (5–17) | **26% (19–36)** |
| Efficient, share of decided | 33% | 21% (12–33) |
| Tessera transcripts flagged for a fact | 5 | 3 |
| C0 success, unconfirmed moves | 100%, 0 | 100%, 0 |
| p50 turn | 2.02 s | 1.96 s |
| Mean message length | 133 chars | 174 |
| Messages with an emoji / on lines | 25% / 16% | 75% / 77% |

**For the first time Tessera wins more "natural" pairs than it loses,**
against the LLM stacks on the same model.

Example (t06):
1. "Oi, Fábio! Claro, te ajudo a remarcar. Na terça 06/10 tenho: / 15h,
   16h / Qual prefere? Depois eu confirmo com você."
2. "Perfeito, Fábio! Só confirmando: posso remarcar sua consulta com o
   dentista D3 de sexta 02/10 às 9h para terça 06/10 às 15h? Responda sim
   para confirmar."
3. "Pronto, Fábio! Sua consulta com o dentista D3 foi remarcada: / - De:
   sexta 02/10 às 9h / - Para: terça 06/10 às 15h / Qualquer coisa, é só
   me chamar por aqui 😊"

**The three factual flags, checked by hand:**
- one is the judge's date error: 2026-10-02 is a Friday;
- two are the period filter: "tarde" was not extracted in one task, so 9h
  was offered. That is the content, not the voice.

The voice introduced no wrong date, time or action.

## Hypotheses

| # | Verdict |
| --- | --- |
| L1: natural ≥ 26% | **Supported:** 55%. |
| L2: clear ≥ 19% | **Supported:** 26%. |
| L3: facts stay locked | **Supported:** 3 flags, none caused by the voice. |
| L4: nothing breaks | **Supported:** 100%, 0. |

## Caveat, and what is next

Seed 24 is the set every voice round was measured on, and variant D's
content was chosen on it. As pre-registered, `warm` becomes the default
only if it holds on another seed: voice-005, seed 22 (market-002's
tasks and competitor transcripts), with warm and strict run side by side.

Efficiency losses rose: the messages are longer. That is the trade the
judge makes visible, and it is reported, not optimised away.
