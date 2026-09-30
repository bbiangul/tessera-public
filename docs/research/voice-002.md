# Voice 002: naturalness ablation, results

- **Date:** 2026-09-24
- **Pre-registration:** [voice-002-preregistration](voice-002-preregistration.md).
- **Code measured:** branch `voice-ablation`, commit `4e3c7ad`. Results
  are in `bench/results/voice-002/{A,B,D}`: the runs and the judged pairs.
- **Spend:** $6.35, the three judges plus about $0.09 of runs.

## Results (seed 24, C0, 60 conversations per variant; 300 judged pairs each)

| Variant | Natural W / L / T | Natural share of decided | Clear W / L / T | Clear share | Efficient share | Tessera transcripts flagged for a fact | C0 success | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A, control | 17 / 191 / 92 | 8% (5–13) | 2 / 178 / 120 | 1% | 43% | 57 | 100% | 1.87 s |
| B, greetings free | 17 / 195 / 88 | 8% (5–12) | 0 / 168 / 132 | 0% | 45% | 55 | 100% | 2.00 s |
| **D, richer content** | **32 / 168 / 100** | **16%** (12–22) | **8 / 77 / 215** | **9%** (5–17) | 33% | **5** | 100% | 2.02 s |

The variants did what they said:
- B greeted in 10% of later messages; A and D in none.
- D's messages are longer (133 characters on average, against 110), and
  offer only the times of the period asked for.

## Hypotheses

| # | Verdict |
| --- | --- |
| W1: the greeting rule cost naturalness (B ≥ A + 10 points) | **Not supported.** B equals A (8% and 8%). Greeting freely, as in voice-001, does not bring back voice-001's 20%. |
| W2: content costs naturalness or clarity (D ≥ A + 10 points on either) | **Not supported at the pre-registered bar:** +8 points on natural (8% → 16%) and +8 on clear (1% → 9%). Clarity **losses** fell from 178 to 77 and ties rose from 120 to 215. The factual flags fell from 57 to 5: they were the period relevance defect, which D removes. |
| W3: no variant breaks the process | **Supported:** 100% C0 success in all three, 0 unconfirmed moves. |

## What this says

- **Content is the lever.** The judge's clarity losses and nearly all its
  factual flags came from what the Function says and offers:
  - which appointment, with whom, from when to when;
  - times outside the period asked for.

  An authoring change fixed them, with no platform change.
- **The greeting rule is not what cost naturalness.** The fall from
  voice-001's 20% is still unexplained. Other changes since voice-001:
  - the direct confirmation when one time is left;
  - the "Depois eu confirmo com você" line;
  - the re-ask;
  - the free mode's presence.

  None was isolated here. Given the judge's stability, the remaining gap
  is worth one more ablation, not a guess.
- **Still far from the LLM stacks on "natural":** 16% of decided pairs.
  The judge's remaining reasons are about formatting and warmth.

## What changes

1. **Authoring rules** (guide and the Part C skill):
   - confirmations and results name who, from when and to when;
   - offers are narrowed to what the user asked (a period, a day).
2. **The benchmark agent** adopts D's content as its baseline for the next
   rounds. Future measurements say so.
3. **`voice.greeting`** stays, with `first` as the default. Nothing here
   favours `any`.
4. **Next ablation (voice-003), from D:**
   - the one-time direct confirmation;
   - the "confirmo com você" line;
   - a persona that allows formatted lists;

   each on its own.
