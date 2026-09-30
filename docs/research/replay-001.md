# Replay 001: scenario-scoped model recordings (#32)

2026-09-24, base `7a16d69`. [Preregistration](replay-001-preregistration.md).
This is a recording/replay correction, with no model-prompt or production
conversation change. No model-quality experiment is needed for this boundary.

## Reproduction and implementation

A deterministic EvalSuite records two scenarios with the same request and
different generated responses. Running the new regression against the old
runner fails: scenario B replays “resposta 1” instead of its recorded
“resposta 2”. The failing log is preserved. Resetting each scenario's
position cannot distinguish their shared request key.

New recordings include an explicit namespace/suite/scenario scope in their
metadata and key. The canonical request and provider payload do not change.
Both Decide and Generate adapters select that scope before each scenario;
the runner clears it afterward. Repeated requests within a scenario still
consume the recorded responses in order, and an extra call still misses.
No implicit state is added to context.Context.

A target with only unscoped legacy entries keeps its previous key/reset
semantics. This preserves existing benchmark and example cassettes but
cannot retroactively infer their missing scenario identity. Re-record a
whole suite to migrate. Once a target contains scoped entries, a scoped
miss never falls back to an unscoped entry or another scenario. Partial
re-recording replaces the sequence for encountered keys while retaining
other scenarios. Unencountered keys remain, as before; start from a new
file for an exact fresh snapshot.

Scope selection occurs between scenarios, not during in-flight calls.
EvalSuite execution is sequential. Sharing one adapter across concurrent
scenario executions is not supported; this change does not introduce
parallel scenario execution.

## Deterministic gates

- Full suite, reversed order, and each scenario selected alone return the
  exact original per-scenario transcripts, using fake Decide/Generate
  providers and a real recording file.
- Namespace, suite and scenario partitions cannot read one another.
- A repeated request consumes its first and second responses, then misses.
- Missing scopes, changed requests and extra occurrences miss.
- Re-recording B preserves A and replaces B's old sequence.
- Wholly legacy targets replay, while mixed targets cannot use legacy
  answers for missing scoped scenarios.

These tests run with the race detector. Existing engine/core tests also
verify that saving one target preserves other targets in the same file.

## Real recordings and offline reproduction

Using unchanged clinic definitions and assertions, recorded each complete
suite live into a single file with a $0.10 ledger cap per suite:

| Suite | Live | Complete replay | Individual replay | Transcripts |
| --- | --- | --- | --- | --- |
| clinic-context | 3/3 | 3/3 | 3/3 | identical to live |
| clinic-confirmation | 7/7 | 7/7 | 7/7 | identical to live |

Actual live spend **$0.004113 of $0.20**; replay makes no provider calls.
`bench/results/replay-001/summary.json` records counts and spend. All live,
full replay and individual outputs are committed. No market benchmark was
run: routing, engine requests and production behavior are unchanged.

CI now uses `bench/cassettes/clinic-context-evals.jsonl` and
`bench/cassettes/clinic-confirmation-evals.jsonl`. Historical per-scenario
files remain for prior research; CI no longer requires them. Other suites
exercise backward compatibility with their unchanged legacy recordings.

```bash
./bin/tessera test examples/clinic-context \
  --generate openrouter:deepseek/deepseek-v4.1-flash \
  --cassette bench/cassettes/clinic-context-evals.jsonl --cassette-mode replay
./bin/tessera test examples/clinic-context --only read-context/timeout-after-side-question \
  --generate openrouter:deepseek/deepseek-v4.1-flash \
  --cassette bench/cassettes/clinic-context-evals.jsonl --cassette-mode replay
```

The same pattern applies to the confirmation suite. Missing or changed
requests remain errors; no silent live fallback is introduced.

`make ci` passes: race-tested packages, build/lint, 57 valid resources and
all 45 replay scenarios, including unchanged legacy suites. `make lint-extra`
passes staticcheck, errcheck and govulncheck with no reported vulnerabilities.
Validation logs include the original failing regression. All registered
gates pass; close #32. This round uses no new off-script set, so seed 39
remains fresh. The next product-level measurement is authoring-003 with
the current conversation policy; RFC-0005 still awaits owner decisions.
