# Decide: which flight does a follow-up mean? (001)

- **Date:** 2026-09-23
- **Question:** RFC-0001's traveler criterion: "questions about earlier
  flights hit the right flight in at least 95% of the suite". Can the
  Decide engine resolve "e a bagagem do segundo?" to the flight the agent
  showed?
- **Dataset:** `bench/datasets/traveler-followups-v1` — 32 references
  (ordinals, airlines, prices, times, "direto"/"com escala", English, and
  two that pick no single flight, whose right answer is "none" so the agent
  asks back). The question is exactly what the runtime sends for a choice
  `ask`: the flights as the agent listed them, plus none.
- **Raw data:** `bench/results/007-traveler-followups.md`.

| Target | Case accuracy (95% CI) | Consistency | AUROC | ≥0.9 coverage / accuracy |
| --- | --- | --- | --- | --- |
| Jev jev-1.13.0 | 96.9% (84.3–99.4) | 100% | 0.892 | 75% / 100% |
| DeepSeek V4.1 Flash, logprobs | 84.4% (68.2–93.1) | 84% | 0.753 | 73% / 94.3% |

## Findings

1. **Jev meets the criterion on this set.** Its one miss, "o quarto",
   went to *none* with 0.8 (in Portuguese "quarto" is also a hotel room):
   the agent would ask "Sobre qual voo?" instead of guessing.
2. **The small LLM fails on ordinals** ("o segundo", "o primeiro",
   "o último"), sometimes with high confidence (o1 at 1.0 for "o último"),
   the same pattern as the earlier Decide benchmarks.
3. **n = 32 is small**: the lower bound of Jev's interval is 84%. The
   criterion should be re-checked on anonymized real follow-ups.

## Decision

`flight-details` resolves the flight with a choice `ask` (Jev), threshold
0.7, asking back below it.

## Addendum (2026-09-23, later): the DeepSeek backend drifted

The traveler scenario `follow-up-baggage` passed 6 live runs on
`llm:deepseek/deepseek-v4.1-flash` in the morning and failed 8 of 8 in the
afternoon, also on the commit that had passed (a worktree of f0a9198) and
also pinned to the DeepSeek provider (`@DeepSeek`, 3 of 3 failed). Nothing
in the request changed: "e a bagagem do segundo?" now gets P(o2) ≈ 0.37
and P(none) ≈ 0.62 on some providers, so the agent asks "Sobre qual
voo?" (correct behaviour for an unsure answer, but not the expected one).

Re-running this dataset (`bench/results/010-followups-provider-drift/`):
Jev 96.9% case accuracy, as in the morning; DeepSeek 81.2% (84.4% in the
morning). OpenRouter spread the 96 DeepSeek trials over 10 providers,
with per-provider accuracy from 33% to 100%.

Lessons: an OpenRouter model name is not one model behaviour; pin the
provider for anything measured, and expect drift anyway. The nightly live
evals (both backends) exist to catch this. The phase 1 criterion "passes
on both backends" held in the morning and holds for Jev now; for the
fallback backend it is 21/22 while this drift lasts.
