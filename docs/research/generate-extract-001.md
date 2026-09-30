# Generate benchmark 001: can the small model extract a flight search?

- **Date:** 2026-09-23
- **Question:** before building the `extract` step, is DeepSeek V4.1 Flash
  (the small-model-first choice, ADR-0002) good enough to turn a pt-BR travel
  request into a typed search: places, dates (including relative ones),
  passengers, cabin?
- **Dataset:** `bench/datasets/travel-extract-v1` — 30 synthetic requests
  written from the traveler's test cases. Gold dates are computed by
  `gen_travel_extract.py` from a fixed "today" (Wednesday 2026-09-23), not
  typed by hand; ambiguous phrasing accepts every reasonable answer.
- **Raw data:** `bench/results/005-extract-travel-v1.md`.

## Results

| Configuration | Records fully right (95% CI) | Invalid | $ per case | p50 / p95 |
| --- | --- | --- | --- | --- |
| Structured outputs on | 89/90 = 98.9% (94.0–99.8) | 0 | $0.000095 | 1.08 s / 3.42 s |
| Structured outputs off (prompt + local validation) | 90/90 = 100% (95.9–100) | 0 | $0.000046 | 0.90 s / 3.48 s |

The one miss: "Florianópolis no feriado de 12 de outubro, fico uma semana"
came back with no dates, once, from one provider.

Latency by upstream provider (structured run): Together p50 0.51 s,
DeepInfra 1.20 s, Wafer 1.07 s, OpenInference 4.02 s.

## Findings

1. **The small model is enough for this extraction.** Relative dates
   ("depois de amanhã", "terça que vem", "daqui a 15 dias, volto 4 dias
   depois"), passengers ("eu, meu marido e nossos filhos de 5 e 10 anos",
   "bebê de colo" → 0) and cabins were all right. There is no reason, on
   this evidence, to try a larger model for extraction.
2. **The dataset is too easy to rank configurations.** One case separates
   the two runs; that is noise. Structured outputs are not needed for
   validity (local validation plus one repair caught nothing to repair), and
   they doubled the cost, probably by routing to pricier providers.
3. **Latency is a provider choice, not a model choice.** The p95 comes from
   the slowest provider; pinning one (for example Together) matters more
   than the model for user-facing turns.

## Decision for now

- Build the `extract` step on the Generate engine with DeepSeek V4.1 Flash,
  local schema validation and one repair, structured outputs off by default,
  provider pinned per namespace.
- Dates are extracted as ISO dates against `STATE.today`; the runtime
  still checks them (future, return after departure) in CEL, never trusting
  the model's arithmetic alone.

## v2: a harder set (same day)

`travel-extract-v2`: 30 requests with typos ("qro ir pra lisbooa"),
corrections ("dia 10, não, dia 12"), holidays by name, numbers in words,
English ("flight to NYC on Oct 5th for 2"), a negated cabin, nicknames
("Floripa", "BH"), mixed families and noise. Two runs of 90 trials:
179/180 records fully right. The one miss dropped the date of "levo meu
cachorro, voo pra Aracaju dia 9 de outubro" once and did not recur. The
first run also exposed a gold value that was too strict ("quero voltar de
Lisboa dia 30" is a return date or a one-way flight home; both are now
accepted). Raw data: `bench/results/006-extract-travel-v2.md`.

**Still no reason to try a larger model for extraction.** The measurement
that could change that is real traffic: anonymized conversations of a
deployed traveler, once a privacy review allows it.

## Next measurement

`travel-extract-v2` must be harder before any stronger claim: multi-turn
state (the destination in one message, the dates in the next), slang and
typos, contradictions and corrections ("na verdade dia 15"), holidays by
name, and requests that must stay null. Re-run with a pinned provider and
3 repeats; add a larger model only if the small one drops below the bar
the traveler's EvalSuite needs.
