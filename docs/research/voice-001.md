# Voice 001 (RFC-0004 part A): results

- **Date:** 2026-09-24
- **Pre-registration:** [voice-001-preregistration](voice-001-preregistration.md).
- **Code measured:** commit `da6b4fd`. Results are in
  `bench/results/voice-001`: the runs, and the judged pairs in
  `naturalness/`.
- **Test set:** seed 24. Tessera ran again (240 runs). Its C0 transcripts
  were paired with the competitors' unchanged market-003 transcripts, and
  the same judge was used (Claude Sonnet 5, reasoning off, both orders).
  Spend: about $1.8.

## Results

| | Before (baseline) | With voice |
| --- | --- | --- |
| Naturalness: wins / losses / ties (300 pairs) | 0 / 281 / 19 | **38 / 154 / 108** |
| Clarity: wins / losses / ties | 0 / 240 / 60 | 0 / 159 / 141 |
| Efficiency: wins / losses / ties | 21 / 42 / 237 | 18 / 59 / 223 |
| Success C0 / C1 / C2 / C3 | 100 / 100 / 100 / 100% | **100 / 100 / 100 / 100%** |
| Duplicates, unconfirmed moves | 0, 0 | **0, 0** |
| p50 turn | 0.58 s | 1.63 s (competitors 2.1–2.3 s) |
| Cost per successful C0 task | $0.00015 | $0.00040 (competitors $0.00097–0.00101) |
| Messages sent in the voice | — | 95.2% (159 of 167 in C0) |

**Fallbacks to the template (8 of 167):**
- a day written differently (4);
- the refusal sentence, which the template interpolates whole, so it
  cannot be paraphrased (3);
- one failed call (1).

All were safe.

## Hypotheses

| # | Verdict |
| --- | --- |
| V1 natural ≥ 40% of decided | **Not supported:** 20% (15–26%). The gap narrows a lot: losses fall from 281 to 154, ties rise from 19 to 108. |
| V2 clear ≥ 40% of decided | **Not supported:** still 0 wins. Losses fall from 240 to 159, ties rise from 60 to 141. |
| V3 no false fact from the voice | **Supported.** The judge flagged 52 transcripts. On review: 48 are the period relevance defect ("ofereceu 8h para quem pediu de tarde"); some claim "sexta 02/10" is wrong, but 2026-10-02 is a Friday; 4 misread the data (the "14h that did not exist" existed). None is a wrong date, time, appointment or action. |
| V4 safety and success hold | **Supported:** 100% in every condition, 0 duplicates, 0 unconfirmed moves. |
| V5 p50 ≤ 2.0 s | **Supported:** 1.63 s. |
| V6 cost ≤ 50% of the competitors' median | **Supported:** about 40%. |

## Why clarity still loses

The judge's reasons in the 159 lost clarity pairs, several per pair:

| Reason | Share of lost pairs |
| --- | --- |
| An extra step: Tessera asks the day, or lists the day's times, when the patient already gave the day and period | 52% |
| Less detail: the others name the dentist, the full date, the appointment | 47% |
| Repetition or a loop: the same question after a goodbye or a decline | 42% |
| Formatting: the others structure the confirmation in lines | 39% |
| Relevance: morning times offered for an afternoon request | 31% |
| Warmth: a name, a greeting | 30% |

Most of it is not the voice's job:
- **Flow** (the extra step, loops, relevance) is part B: questions that
  use what the user already said, a narrower offer, and an end that
  notices a goodbye.
- **Detail** is the Function's content. The voice may not add facts, by
  design, so the templates must carry them (the dentist, the full date).
  That is an authoring guideline.
- **Formatting** can be allowed in the voice when the channel supports
  it. It needs care on WhatsApp.

## Conclusion

The voice does what it was built for: warmer and personal messages, no
fact broken, no safety given up, still faster and cheaper than the
competitors. It does not by itself close the gap. The rest is flow,
which part B addresses, and content, a template matter for the authoring
guidelines.
