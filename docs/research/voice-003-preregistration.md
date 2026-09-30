# Voice 003: a larger model for the voice, pre-registration

- **Date:** 2026-09-24, before the run.
- **Why now:** the rule is to start with the smallest model and move up
  only when measurement shows it fails. Four rounds measured the voice on
  `deepseek/deepseek-v4.1-flash`:
  - voice-001: 20% natural;
  - freemode-001/002: 13% and 7%;
  - voice-002 variant D, the best content: 16%;
  - authoring-002: 4%.

  The competitors are around 80–90% of decided pairs. That is the
  failure the rule asks for.
- **The candidate:** `deepseek/deepseek-v4-pro`.
  - The same family as every competitor, and not the judge's (Claude
    Sonnet 5), so the judge cannot prefer its own style.
  - It costs about 6× Flash for input and 3× for output.

## Run

- `bench/market/tessera-rich` (voice-002's D), unchanged. Tessera's
  Generate model is `deepseek/deepseek-v4-pro@DeepSeek`: the voice, the
  free mode and extraction. Decide stays Jev.
- Seed 24, C0, 3 repeats. The C0 transcripts are judged against the
  competitors' market-003 transcripts (all on Flash), with the same judge
  and both orders.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| M1 | The voice's model is a lever | Natural share ≥ D-on-Flash's 16% + 10 points |
| M2 | And for clarity | Clear share ≥ D-on-Flash's 9% + 10 points |
| M3 | Nothing breaks | C0 ≥ 97%, 0 unconfirmed moves |

Also reported: cost per conversation, and p50 per turn.

**If M1 fails,** the model is not the lever at this size either. The gap
is in what Tessera says (the flow and the templates), not in how it
phrases it, and the next step is structural, not a bigger model.

**Budget:** $3.
