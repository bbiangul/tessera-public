# Voice 008: give the warm voice its dialogue back, safely (#40), pre-registration

- **Date:** 2026-09-25, before any measured run.
- **Why:**
  - [market-bench-004](market-bench-004.md) found that voice-007
    (`1d07796`) cut the warm voice's pairwise naturalness against the
    competitors from 53% to 33% on seed 44. voice-007 removed the dialogue
    from the warm rewrite's context, to stop false promises (#34).
  - voice-007's own selection also measured variant B, which keeps the
    dialogue and adds a boundary rule ("DRAFT is the sole authority …;
    CONVERSATION provides tone and continuity only"). B had 0/72
    unsupported claims and naturalness 4.986 against C's 5.000.
  - C won by cost (8.5% cheaper). B was never replicated, and neither was
    judged pairwise against the competitors.
- **Code:** branch `voice-context-fix` (`7b81eaf`, from main `814dd34`):
  - adds `voice.dialogue: marker|full`. `marker` is today's behaviour and
    stays the default; `full` is B, with voice-007's rule word for word.
  - `bench/market/tessera-context-full` = `tessera-context` + `dialogue: full`.
  - `bench/scripts/market.sh` starts its Python servers with `exec` (#41).

## Measurements

1. **Safety:** voice-007's cases (`bench/voice-safety/dev.json`, 24;
   `replication.json`, 12), 3 repeats each, through the **production**
   rewrite in both modes (`TestVoiceSafetyProduction`). Scored with
   voice-007's blind judge (`bench/voice-safety/judge.py`).
2. **The #34 and #33 regressions:** `examples/clinic-confirmation` and
   `examples/clinic-context` EvalSuites, live, with `dialogue: full`.
3. **Pairwise naturalness, seed 44:** `tessera-context-full` C0, 3 repeats,
   judged against market-bench-004's competitor transcripts. Also C1–C3,
   for the fault guarantees.
4. **Replication, fresh seed 45:**
   - the five LLM competitors, `tessera-context` (marker) and
     `tessera-context-full`, C0, 3 repeats;
   - each Tessera variant judged against the seed-45 competitor transcripts.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| V1 | `full` is as safe as `marker` on promises | `full`: ≤ 1 unsupported claim in 108 observations (dev + replication), and not more than `marker` + 1 |
| V2 | The regressions hold | Every scenario of both EvalSuites passes live with `full` |
| V3 | It recovers naturalness on seed 44 | `full`'s natural share of decided pairs ≥ 45% (market-bench-004's D3 was 53%, today's `marker` 32%) |
| V4 | The gain replicates on a fresh seed | On seed 45, `full` − `marker` ≥ 10 points of natural share |
| V5 | The guarantees hold | `full` on seed 44: C0–C3 ≥ 95%, 0 duplicates, 0 unconfirmed moves |

**If V1–V5 hold,** the recommendation is to make `full` the default and
remove `marker`. Otherwise `marker` stays, and this is reported.

**Budget:** $10: safety runs and judge about $1.5, runs about $1.5, three
naturalness judges about $6.6.
