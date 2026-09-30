# Provider 001: does the OpenRouter provider matter? Results (#5)

- **Date:** 2026-09-25
- **Pre-registration:** [provider-001-preregistration](provider-001-preregistration.md).
- **Results:** `bench/results/provider-001` (the run), and
  `bench/results/provider-001-structured-attempt` (the first attempt,
  below).
- **Spend:** $0.23 for the run. The first attempt's ledger shows $0.50,
  but providers billed $0.007: failed calls are charged at the ledger's
  estimate (#7's partial fix).

## Deviation: structured outputs

The first attempt used `bench extract`'s default, structured outputs.
Every pinned target failed in about 30 ms. OpenRouter's reason:
- "Filter by Parameters removed … deepseek";
- the DeepSeek provider does not support `json_schema`;
- a pinned request has no fallback.

The product's Generate does not use structured outputs (it asks for JSON
in the instructions), so the run was repeated with
`--structured-outputs=false`. The attempt is kept.

## Results (travel-extract-v2, 30 cases × 3 repeats)

| Target | Records right (95% CI) | Invalid replies | p50 / p95 | Served by | $ per case |
| --- | --- | --- | --- | --- | --- |
| unpinned (the default until now) | 100% (95.9–100) | 0 / 90 | 1.31 / 5.05 s | **6 providers**: DeepInfra 69, AtlasCloud 12, Alibaba 3, OpenInference 3, Sail Research 1, Modal 1 | 0.000044 |
| `@DeepSeek` | 98.9% (94.0–99.8) | 0 / 90 | 1.29 / **1.41 s** | DeepSeek, 90 | 0.000068 |
| `@DekaLLM` (the cheapest) | **76.7%** (66.9–84.2) | **21 / 90** | 5.75 / 16.4 s | DekaLLM 69, errors 21 | 0.000028 |
| `@OpenInference` (fp4) | 100% (95.9–100) | 0 / 90 | 3.29 / 4.16 s | OpenInference, 90 | 0.000030 |

## Hypotheses

| # | Verdict |
| --- | --- |
| P1: providers differ by ≥ 5 points | **Supported:** 100% and 98.9% against 76.7% (DekaLLM, with 21 invalid replies and a 16 s p95). |
| P2: unpinned is not reproducible | **Supported:** one run went through 6 providers, **never DeepSeek**, the provider every benchmark in this repository measured. |

## Decision

A spec that names no provider is now pinned to the model's measured
providers (`generate.DefaultProviders`; `deepseek/deepseek-v4.1-flash` →
`DeepSeek`) in `serve`, `chat`, `test` and the benchmarks. `@any` gives
the choice back to OpenRouter, and `@X,Y` still pins explicitly. The ledger
already records the provider of every call.

**One exception:** `bench extract` with structured outputs (its default)
does not apply the default pin, because DeepSeek does not support them. It
says so on stderr.

## Limits

- One task (extraction), one day, 90 calls per target.
- Unpinned scored 100% here because OpenRouter mostly chose DeepInfra;
  the point is that it chose, among providers that differ.
- The cheaper providers were not measured on conversation (voice, free
  mode). The drift alert in the nightly evals (#5's other half) needs the
  owner's GitHub secrets (#15).
