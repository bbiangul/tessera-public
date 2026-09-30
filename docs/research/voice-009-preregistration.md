# Voice 009: dialogue: full after the two fixes, pre-registration

- **Date:** 2026-09-25, before any measured run.
- **Previous:** [voice-008](voice-008.md).
  - `voice.dialogue: full` was as safe as `marker` (0/108 promises,
    regressions 10/10, C0–C3 100%).
  - It was more natural on both seeds (+15 on 44, +6 on 45), short of the
    +10 replication bar.
  - It exposed two defects: bare "remarc" (5/120) and the repeated day
    list after "none of these is a Saturday".
- **Code:** branch `voice-context-fix`, `52b4a3a`:
  - the `keep` instruction lists the draft's own words, never a stem;
  - a question asked again after a reply that picked nothing first
    acknowledges that reply from the draft's options, then asks again
    with every option (not for confirmations).

  Both apply to `marker` too, i.e. to today's default.
- **Regression protection already done:**
  - new unit tests;
  - `make ci` and `make lint-extra` green;
  - the three clinic cassettes whose requests changed were re-recorded
    live, 63/63 passing, then replayed (102 replay scenarios in CI).

## Runs

1. **Safety:** voice-007's dev and replication cases through the
   production rewrite, both modes, and voice-007's judge.
2. **The #33/#34 EvalSuites,** live, with `dialogue: full` (10 scenarios).
3. **Fresh seed 46:** the five LLM competitors C0; `tessera-context`
   (marker) and `tessera-context-full` C0–C3, 3 repeats; each Tessera
   variant's C0 judged against the seed-46 competitors.
4. **A regression check of the default on seed 44:** `tessera-context`
   (marker) C0 with the new code, judged against market-bench-004's
   competitors. It is compared with market-bench-004's 32%.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W1 | Still safe | `full` ≤ 1 unsupported claim in 108; #33/#34 10/10 live; C0–C3 ≥ 95% with 0 duplicates and 0 unconfirmed moves for both modes on seed 46 |
| W2 | The default did not regress | New-code `marker` on seed 44 ≥ 25% natural (market-bench-004: 32%, lower bound 26%) |
| W3 | The stem defect is gone | 0 agent messages with a bare "remarc" across seed-46 and seed-44 runs of both modes |
| W4 | `full` beats `marker` on the fresh seed | Seed 46: `full` − `marker` ≥ 5 points of natural share |
| W5 | And across all three seeds | Decided pairs pooled over seeds 44, 45 and 46: `full` − `marker` ≥ 8 points. Seeds 44/45 are voice-008's runs (old code), 46 is the new code; the mix is stated |

**Decision rule, fixed now:** `full` becomes the default only if W1, W2,
W3, W4 and W5 all hold. If W2 fails, the two fixes are reverted and
analysed before anything else.

**Budget:** $10: safety about $0.6, runs about $1.2, three naturalness
judges about $6.6.
