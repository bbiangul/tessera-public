# ADR-0002: Measure before believing; start with the smallest model

- **Status:** accepted
- **Date:** 2026-09-22

## Context

The design (RFC-0001) rests on claims about models: that a System One model
gives better decisions than an LLM, that probabilities are calibrated, that
latency and cost are lower. The first benchmark behind those claims was biased:
the model under test received hand-tuned prompts the others did not. Once the
prompts were equalized, the accuracy gap disappeared (that benchmark used
data from the previous platform and is not published). Unmeasured claims had already shaped
the design.

## Decision

1. **Every design claim about a model, a latency, a cost or a behavior is
   backed by a measurement in this repository** — a benchmark under
   `bench/results/` or a test — or it is written as an open hypothesis.
2. **Benchmarks are fair by construction.** Every target receives the same
   question text; `internal/bench` sends the same `decide.Question` to every
   backend. Reports include distinct-case counts and confidence intervals.
3. **LLM work starts with the smallest reasonable model**, currently
   `deepseek/deepseek-v4.1-flash`. A larger model is tried only after a
   measurement shows where the small one fails, and that failure is recorded.
4. **Hidden variables are pinned and reported.** Examples found so far: the
   upstream provider behind a router (it changes logprobs and error rates) and
   reasoning mode (reasoning models otherwise spend the output budget and
   return nothing).
5. **Calibration is never assumed.** `decide.Answer.Calibrated` stays false
   until a measured profile says otherwise.

## Consequences

- Slower to claim things, faster to find out we were wrong.
- Every model-facing feature ships with a dataset and a benchmark command.
- Results are committed, so anyone can check the numbers and rerun them.
