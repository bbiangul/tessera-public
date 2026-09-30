# Voice 004: a warmer voice, pre-registration

- **Date:** 2026-09-24, before the run.
- **Why:** [voice-003](voice-003.md) showed a larger model returned text
  close to the template, because it followed the voice's rules literally
  ("say what DRAFT says and nothing more; keep it short"). The rules lock
  the warmth together with the facts.
- **What changes:** `voice.latitude: warm`. The instructions keep:
  - every fact character for character;
  - "add no facts: no dates, times, options, prices, names, promises or
    steps";
  - the question asked the same.

  They allow what a friendly person adds around the facts:
  - an acknowledgement of what the user said;
  - a short transition;
  - a brief closing offer of help;
  - facts on short lines;
  - not shortening the draft.

  The checks are unchanged: facts, placeholders, a confirmation still a
  question, `keep`. The default stays `strict` until this is measured.
- **Variant:** `bench/market/tessera-warm` = voice-002's D +
  `latitude: warm`, on Flash (the default model).

## Run

Seed 24, C0, 3 repeats. The C0 transcripts are judged against the
competitors' market-003 transcripts, with the same judge and both orders.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| L1 | Warmth is the lever | Natural share ≥ D's 16% + 10 points |
| L2 | Clarity follows | Clear share ≥ D's 9% + 10 points |
| L3 | The facts stay locked | The judge flags at most D's 5 + 5 Tessera transcripts for a fact, each checked by hand; 0 wrong dates, times or actions |
| L4 | Nothing breaks | C0 ≥ 97%, 0 unconfirmed moves |

**If L1 holds,** `warm` becomes the default. It is then checked on another
seed before any claim.

**Budget:** $2.5.
