# Datasets

JSONL, one case per line: `id`, `task`, `state`, `question` (a
`decide.Question`), `gold` (the correct outcome key). `bench.ReadDataset`
rejects unknown fields, invalid questions and gold labels that are not
outcomes of the question.

## travel-decisions-v1.jsonl

- 12 **synthetic** cases in Brazilian Portuguese from a travel agency's chat,
  no personal data: `requires-response`, `confirmation`, `categorize`,
  `single-selection`.
- An example of the format, and the input of the replay and gate tests.
  Its recording (`bench/cassettes/example-travel-v1.jsonl`) and baseline
  (`bench/baselines/example-travel-v1`) come from a scripted,
  deterministic decider, not a model: see
  `internal/bench/example_gen_test.go`.

Limits: it tests the benchmark machinery, not any model. Measure a backend
with your own dataset: `tessera bench decide --dataset <yours>.jsonl`.
