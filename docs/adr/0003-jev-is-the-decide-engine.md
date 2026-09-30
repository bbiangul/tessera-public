# ADR-0003: Jev is the Decide engine; LLM decision work is paused

- **Status:** accepted
- **Date:** 2026-09-22
- **Supersedes:** the "two Decide backends, LLM default in open source" rule of
  RFC-0001 v0.2.

## Context

Benchmarks 001–004 (on the previous platform's decision points; not published) compared Jev
with DeepSeek V4.1 Flash under identical questions:

- Accuracy ties (~97% on 69 cases).
- Jev's confidence separates right from wrong answers (AUROC 0.978; every
  error below 0.7). The small LLM's does not: logprobs are saturated at ~1.0,
  voting agrees on its errors, verbal probabilities reach AUROC 0.708.
- Jev is 2.6–4.7× faster at p95 and cheaper.

Confidence routing (ask, escalate, proceed) is the reason the Decide engine
exists, so the backend that gives usable confidence wins.

## Decision

1. **Jev (TypeSafe) is the Decide engine.** Routing thresholds are tuned for
   it, and the model version is pinned per namespace.
2. **The `llm-constrained` backend stays in the code** as a measurement tool
   and an emergency fallback, always with `Calibrated = false` and
   conservative routing. It is not a supported default.
3. **Open-source Decide backend: backlog.** Evaluate Laya (an open-source
   System One-style model) as the self-hosted alternative, with the same
   benchmark harness, before promising a Jev-free open-source path.
4. **LLM evaluations resume with the Generate engine.** When text generation
   work starts, the small-model-first rule (ADR-0002) applies again, starting
   with DeepSeek V4.1 Flash.

## Consequences

- Self-hosted users need a TypeSafe key until an open-source backend passes
  the benchmark. This is stated plainly in the docs.
- Vendor risk (early access, rate limits, US-only serving) is carried on
  purpose; mitigations are the pinned version, the fallback backend, and the
  Laya evaluation.
- The real-message calibration study is still needed, now to set Jev's
  default thresholds rather than to pick a backend.
