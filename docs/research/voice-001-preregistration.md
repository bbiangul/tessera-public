# Voice 001 (RFC-0004 part A): pre-registration

- **Date:** 2026-09-24, before the measured run.
- **Baseline:** [rfc0004-baseline](rfc0004-baseline.md). Naturalness was 0
  wins out of 281 decided pairs and clarity 0 out of 240.

## What changed

- **`Agent.spec.voice`** (persona, examples). Every message to a user is
  rewritten by the Generate backend, DeepSeek V4.1 Flash, the same small
  model the competitors use.
- **Locked facts:** every value the template interpolated must appear
  word for word; a joined list is checked item by item.
- **Falls back to the template when:**
  - a fact is missing;
  - a confirmation is no longer a question;
  - a masked placeholder appears;
  - the text is empty or too long;
  - the call fails.
- **Recorded once:** replays send the same words.
- **The model sees masked input.** The masker now also masks `[]string`
  and `map[string]string`, which used to pass unmasked. The voice's tests
  found that gap.
- **The harness passes the patient's name** in Tessera's caller, as every
  competitor's prompt always had it.

**Development set:** seed 23, the persona only. **Test set:** seed 24.
There Tessera runs again (C0–C3, 3 repeats) and its new C0 transcripts are
paired with the competitors' **unchanged** market-003 transcripts. They
are judged by the same judge (Claude Sonnet 5, reasoning off, both orders).

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| V1 | The voice closes most of the naturalness gap | Tessera wins ≥ 40% of decided "natural" pairs |
| V2 | …and the clarity gap | Tessera wins ≥ 40% of decided "clear" pairs |
| V3 | The voice introduces no false fact | No judge-flagged Tessera error is, on review, a wrong date, time, appointment or action |
| V4 | Safety and success hold | C0 success ≥ 97%; C1 and C3 0 duplicates; 0 unconfirmed moves |
| V5 | The latency stays below the competitors' | Tessera p50 turn ≤ 2.0 s (competitors 2.1–2.3 s) |
| V6 | The cost advantage stays | Cost per successful C0 task ≤ 50% of the competitors' median |

Also reported: the share of messages that fell back to the template, and
why.

**Known not to be fixed by a voice:** listing a whole day when a period
was asked, and repeating a question after a goodbye. Both are part B.
They may keep clarity low.

**Budget:** $6 (runs and judge).
