# Autobatch 001 — fast batching, no unconditional fusion

2026-09-25, base b0744f3. Verdict: **the preregistered gate failed**. Batch was
87.8% faster at median full-mask latency and 54.4% cheaper, but both modes
failed the same weekday interpretation in both repeats. No production change.

## Live measurements

Frozen 12 new synthetic Portuguese messages, twice per arm; unchanged eight
questions and scorer from weekdaybench, Jev 1.13.0. ABBA block order, four
concurrent samples within each block. These are classification latencies, not
conversation latencies. Repetition count is 24, distinct messages only 12.

| Metric | Serial | Batch |
| --- | ---: | ---: |
| Correct full masks | 22/24 | 22/24 |
| Confident forbidden-day allowances | 2/24 | 2/24 |
| Provider errors | 0 | 0 |
| Requests | 192 | 24 |
| Median latency | 2322.5 ms | 283 ms |
| p95 latency (nearest rank) | 2643 ms | 948 ms |
| Reported cost USD | 0.003620736 | 0.001652112 |

Total reported spend $0.005272848 against the $0.20 cap ($0.05 per metered
block). No generation/judge calls. Raw answers, costs and latencies are retained.
The harness records billed dollar cost, not token counts; no token-count claim.

Failure a02: “Durante a semana posso, menos na terça e na sexta.” The frozen
oracle allows Monday/Wednesday/Thursday. Both modes also confidently permit
Sunday (~.95) and Saturday (.94). The oracle interprets “durante a semana” as
weekdays, a colloquial phrase that can be ambiguous; retain the registered
failure, do not silently relabel it. No actual calendar effects were executed.
A follow-up classifier study should test explicit weekdays and ambiguous
phrasing separately, allowing clarification for ambiguity.

Across 192 paired question results, 191 have equal thresholded values. The one
difference is a09, question Tuesday, first repeat: serial .30 versus batch .34.
The explicit-restriction meta-answer is false, so the existing scorer ignores
that weekday answer; final operational masks agree on all 24 pairs. This does
not prove universal model equivalence. Maximum probability difference .15,
mean absolute difference .00536. No adjustment or rerun after seeing the results.

## Actual runtime boundary experiment

`TestBatchChangesQuestionsIssuedBeforeFailure` runs independent same-state
questions through the real runtime, with scripted low confidence and provider
failure. In each case:

| Arm | Requests | Questions issued | Final status |
| --- | ---: | ---: | --- |
| Sequential | 1 | 1 | Failed |
| Explicit batch | 1 | 2 | Failed |

Identical final status does not imply identical work or failure semantics.
The serial second question never executes. Both questions are submitted in the
batch. Existing tests also exercise valid human uncertainty policy and durable
batch replay, but this experiment does not implement or prove crash-safe fusion.

Conclusion: keep explicit batching for author-declared independent groups.
Do not automatically rewrite consecutive steps based only on equal input state.
Future scheduling design must declare whether speculative questions/cost are
permitted, preserve per-step recovery and journal compatibility, and never cross
waits/effects/control flow without a stronger contract. The classifier failure
is shared by both arms and is not evidence batching caused it.

## Reproduction and checks

Protocol: `docs/research/autobatch-001-preregistration.md`.
Data and all outputs: `bench/results/autobatch-001/`.
Recompute summaries with `python3 bench/results/autobatch-001/summarize.py`.
Live runs use `go run ./bench/weekdaybench -dataset <cases.json> -out <new.jsonl>
-mode serial|batch -repeats 1 -max-cost .05`, with `TYPESAFE_API_KEY` from the
environment. Run arms serial, batch, batch, serial into new output files.
The evaluation set is now used; do not call it held-out in a follow-up study.

Four scripted boundary cases pass with the race detector. Full CI and static
analysis results are recorded alongside the experiment. No API, prompt or
production engine behavior was changed in this round.
