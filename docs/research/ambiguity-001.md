# Ambiguity 001 — a same-batch guard passed classification, failed conversation

2026-09-25, base 814dd34. **Do not promote candidate A.** It passed development
and separate frozen replication, but a conversation wrapper caused a confident
misclassification. Frozen candidate definitions and cassettes remain under
`bench/results/ambiguity-001/candidate`; the public helpers remain unchanged.

## Policy and comparison

The test policy requires clarification for vague weekday scope or contradictory
availability. Clear ranges, explicit weekdays, numeric dates, old appointment
days and FAQ mentions should not cause unnecessary clarification. This is an
author policy, not a universal linguistic truth about the word “semana”.

All arms use the same eight existing Jev questions. A adds a yes/no question:
“É necessário esclarecer o escopo dos dias?” B instead chooses clear/ambiguous.
Both guards are in the same request; only confidently clear (.7) allows using
the original mask. No new LLM decision path or serial request is introduced.

Each set has 12 distinct messages, repeated twice, with 4 ambiguous and 8 clear.
The replication messages and both candidate prompts were frozen before calls;
A was selected after development. B's failures remain in the score.

| Arm/set | Ambiguous clarified | Clear exact masks | Unsafe acceptances | Median ms |
| --- | ---: | ---: | ---: | ---: |
| Baseline/development | 4/8 | 16/16 | 4 | 264.5 |
| A/development | 8/8 | 16/16 | 0 | 276 |
| B/development | 6/8 | 16/16 | 2 | 276.5 |
| Baseline/replication | 4/8 | 16/16 | 4 | 264.5 |
| A/replication | 8/8 | 16/16 | 0 | 256 |

No provider errors or unnecessary clarification of clear messages in these
runs. Classifier spend $0.008949696. Blocks ran sequentially with four concurrent
samples, not randomized/interleaved; do not claim a latency advantage for A.
The baseline's replication failures were contradictions r03/r04; development
failures were d01/d04. B missed d01 twice. These are policy failures, not actual
calendar writes.

## Integration failure, retained and reproduced

After A passed replication, a preregistered extension inserted the unchanged
question and conservative known=false handling into the two weekday helpers.
Four original helper conversations and four new integration conversations were
recorded. Result: **7/8 live and 7/8 replay**, with the same failed scenario.

Input: “Filtre o calendário: durante a semana posso, menos terça e sexta.”
A returned ambiguity probability **.24**, meaning clear at .76, above the .7
threshold. The helper therefore offered Saturday and Sunday. Expected behavior
was clarification, with no proposed dates. This differs slightly from d01
(prefix, casing and omitted articles); the run does not isolate which change
caused the failure. Do not attribute it solely to the prefix or to randomness.

The contradictory request correctly asked for clarification; explicit range
and numeric-date cases passed. No threshold lowering, regex exception, prompt
repair or rerun was used to hide the integration failure. Candidate artifacts
include the changed helper, eight-scenario suite and runtime regression patch.
The candidate test patch covered ambiguous/uncertain scope overriding even a
confident absent-restriction answer. It is retained as evidence, not applied to
the unchanged public helper.

## Reproduction and next experiment

Classifier: `go run ./bench/weekdaybench -dataset <dev-or-replication.json>
-ambiguity-policy baseline|A|B -mode batch -repeats 2 -max-cost .10 -out <new.jsonl>`.
No flag keeps the historical weekdaybench behavior unchanged.

Failed conversation, offline:

```
./bin/tessera test bench/results/ambiguity-001/candidate/decide-batch \
  --cassette bench/results/ambiguity-001/helper.jsonl --cassette-mode replay
```

This command intentionally returns nonzero: the preserved failure is evidence.
`summary.json` and `summarize.py` reproduce classifier selection without model
calls. `frozen-hashes.json` identifies datasets/harness; `selection.json` records
selection before replication. The hash manifest was written after development
started, but dataset/prompt files were created before calls and unchanged during
all classifier runs. Do not present that manifest as a pre-call timestamp proof.

Next: develop a family of semantically equivalent phrasings and conversation
wrappers, measure consistency plus false clarification rates, and reserve new
families for replication. Retain this failure as development data. No automatic
batching or universal ambiguity rule belongs in the engine on this evidence.

## Supplemental clinic-run deviation

The 53-scenario supplemental clinic run started on the candidate. Public helper
files were restored while it was still running, before discovering that the
EvalSuite runner reloads definitions for each scenario. That run is therefore
invalid as candidate regression evidence and must not support a promotion claim.
Its output and spend are retained and labeled invalid; no repeat is needed to
reject a candidate whose separate frozen helper conversation already failed.
Future live evaluations must run directly from the frozen candidate directory.

The invalid supplemental run was interrupted (SIGINT, exit 130) to avoid more
unusable calls. The CLI had not flushed its cassette/results, so its actual
billed spend is unavailable; reserve the entire $0.30 cap. Classifier plus helper
spend is about $0.0097; with that conservative reserve, total stays below $0.31
of $0.50. `budget.json` records exact components. No clinic pass-count claim.

Final `make ci` and `make lint-extra` pass on the retained public definitions
and new benchmark/scorer tests. An earlier unit command encountered sandbox
cache restrictions; the full authorized checks passed afterwards. The intentionally
failing frozen candidate suite is outside `examples/` and not promoted to CI.
