# Provider 001: does the OpenRouter provider matter? Pre-registration (#5)

- **Date:** 2026-09-25, before any run.
- **Why:**
  - `--generate openrouter:deepseek/deepseek-v4.1-flash`, the default of
    `serve`, `chat` and `test`, does not pin a provider. On 2026-09-25
    OpenRouter lists **27 providers** for this model, with quantizations
    fp4, fp8 and unspecified, and input prices from $0.04 to $0.375 per
    million tokens.
  - decide-followups-001 found 33–100% accuracy by provider on a follow-up
    set.
  - Every benchmark since then pinned DeepSeek by hand (`@DeepSeek`); the
    product default does not.
- **Set:** `bench/datasets/travel-extract-v2.jsonl` (30 cases) with `travel-extract-v1.task.json`, the same fields.
  Deterministic field scoring against gold values (the comparator fixed in
  #38), no judge. 3 repeats.
- **Targets:**
  - `openrouter:deepseek/deepseek-v4.1-flash`, unpinned (today's default);
  - `@DeepSeek`, the first-party provider every benchmark used;
  - `@DekaLLM` and `@OpenInference`, the two cheapest, one of them fp4.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| P1 | Providers differ | The best and worst pinned targets differ by ≥ 5 points of field accuracy, or by ≥ 5 points of schema-valid replies |
| P2 | Unpinned is not reproducible | The unpinned runs are served by ≥ 2 providers (from the providers the responses report) |

**If P1 and P2 hold,** the default pins the measured provider (DeepSeek for
this model), with an explicit opt-out, and records the provider (the
ledger already does). **If neither holds,** document it and keep today's
behaviour.

**Budget:** $0.5.
