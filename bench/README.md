# Benchmarks and evals

How model behaviour is measured and guarded (RFC-0002).

| Path | What |
| --- | --- |
| `datasets/` | Labelled cases (JSONL). Synthetic, no personal data. |
| `results/` | The few recorded runs the tests read; the raw data of the studies in `docs/research/` is not published. |
| `baselines/` | The run that CI gates against, per pinned model version. |
| `cassettes/` | Recorded model calls for free, offline replay in unit tests. |

## Levels

- **Replay ($0, every commit):** `internal/bench/replay_test.go` replays
  `cassettes/example-travel-v1.jsonl` through the real runner and gate, and
  must reproduce `baselines/example-travel-v1`. If a dataset question
  changes, the cassette misses and the test fails, naming the question:
  prompt changes are visible in review. The example's recording comes from a
  scripted decider (`internal/bench/example_gen_test.go`), so it tests the
  machinery, not a model.
- **Measuring a backend:** `tessera bench decide` on your own labelled
  dataset, recorded with `--cassette ... --cassette-mode record`, and
  `tessera bench gate` against your previous run.

## Recording a baseline for your dataset

```bash
make build
export TYPESAFE_API_KEY=...   # or --target llm:<model> with OPENROUTER_API_KEY
./bin/tessera bench decide --dataset bench/datasets/<yours>.jsonl \
  --target typesafe:jev-1.13.0 --repeats 3 --max-cost-usd 0.05 \
  --cassette bench/cassettes/<yours>.jsonl --cassette-mode record \
  --out bench/baselines/<yours>
```

Recording replaces this target's entries for every request it makes and
keeps other targets' entries in the same file. Commit the cassette and the
baseline together, and say in the PR why the behaviour changed.

## Upgrading the model

A new Jev version gets its own cassette and baseline. Run the live eval for
both versions and gate the new one against the old baseline before switching
the pinned version.
