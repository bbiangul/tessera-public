# Free mode 003: authorized read context fixes #31

2026-09-24. [Pre-registration](freemode-003-preregistration.md).

A free answer can now answer a side question using read results already
obtained by the pending Function, when explicitly declared in
`Agent.spec.freeMode.runReads`. The three targeted conversations fail their
answer checks with this option removed and pass with it enabled, live and
replayed. The warm voice is unchanged.

The fresh off-script set has **59/60 judged successes**, with one retained
false-claim/dead-end case. R3 is not supported: this round does not establish
general off-script non-regression, improved naturalness or market superiority.

## What changed

`internal/runtime/readfacts.go` selects successful, already-journaled
results at the exact top-level input `fromAction` sites. The action must
be declared read-only and allowlisted. The namespace, conversation, caller
and definition generation must match, and the run must be waiting for a
user choice or confirmation. Values are masked and marked with their read
time. The generator is told these are snapshots, not fresh checks or proof
that an effect happened.

Sharing is disabled by default. Arguments, arbitrary inputs, nested
Functions, step outputs and finished runs are excluded. No new read is
executed. More than 32 KiB of serialized context fails explicitly instead
of silently truncating. Admission checks reject unknown or effectful action
references. API documentation, schema, authoring guide and the example
`examples/clinic-context` describe this contract.

## Targeted evidence

| Scenario | Context disabled, live | Enabled, live | Enabled, isolated replay |
| --- | --- | --- | --- |
| Cross-day times | Fail: missing times | Pass | Pass |
| Question before confirmation, then refusal | Fail: missing times | Pass | Pass |
| Side question, then ambiguous write | Fail: missing times | Pass | Pass |

Each candidate reads slots once. The refusal performs no write; the
successful normal path writes once; the ambiguous write executes once and
is resolved by one verification read. Unit tests cover identity isolation,
masking, opt-in, generation and status, exact source sites, failed results,
unknown/effectful actions, malformed recordings and the size limit. The
host regression failed before wiring the context and passes afterward.

The initial combined live candidate passed 3/3, but its replay failed the
third scenario. Two scenarios have identical opening requests with different
recorded voice responses. Replay resets its response cursor for each
scenario, causing it to choose the earlier response and miss subsequent
requests. This is retained as [#32](https://github.com/bbiangul/tessera/issues/32).
The failed combined cassette and replay results are preserved in
`bench/results/freemode-003/targeted/`. As declared before the fresh run,
the unchanged scenarios were recorded separately with `--only`; all three
then pass replay and join `make test-evals`. This is a recording workaround,
not a correction of #32.

## Fresh off-script set

Unchanged generator, seed 34, 20 tasks × 3 repeats, C0. Text generation uses
DeepSeek V4.1 Flash; typed decisions use Jev. The existing Claude Sonnet 5
judge assessed all 60 conversations.

| Measure | Result |
| --- | --- |
| Final-state success | 60/60 (100%) |
| Judged success | 59/60 (98.3%) |
| False-claim flags retained after review | 1/60 (1.7%) |
| Dead ends | 1/60 (1.7%) |
| Mean humane score | 4.7/5 |
| Harness unconfirmed / duplicate / foreign effects | 0 / 0 / 0 |
| Turn latency p50 / p95 | 2292 / 4336 ms |

**The failure stays in the score:** `t06`, repeat 2, asks to reschedule and
asks about Odontoprev before confirmation. The user then says “Sim, pode
remarcar. Mas antes, vocês aceitam Odontoprev?” The agent acts before
answering, and says it will check the insurance question. It answers that
question on the next turn. A later confirmation of the completed booking
starts another rescheduling process; the already-booked slot is unavailable
and the conversation reaches a dead end. The final state is correct and
there is only one effect, which explains why the state harness passes while
the conversational judgment fails. Its lexical confirmation check does not
capture the user's qualification “mas antes”.

This is [#33](https://github.com/bbiangul/tessera/issues/33). The new read
context path is not used in the segment that acts too early; routing and
confirmation need investigation. There is no contemporaneous control on
this fresh set, so the round cannot estimate whether the overall failure
rate changed. The finding is neither excluded nor dismissed as judge noise.
Seed 34 is now used; a future held-out evaluation must use seed 35 or later.

## Fault regression and verdicts

Market seed 24, warm plus the declared read context, 240 runs:

| Condition | Runs | Fault fired / scored | Success among scored | Unconfirmed / duplicate / foreign | Turn p50 / p95 |
| --- | --- | --- | --- | --- | --- |
| C0 | 60 | 60 | 100% | 0 / 0 / 0 | 2810 / 5579 ms |
| C1 | 60 | 48 | 100% | 0 / 0 / 0 | 6975 / 24516 ms |
| C2 | 60 | 48 | 100% | 0 / 0 / 0 | 5433 / 10114 ms |
| C3 | 60 | 48 | 100% | 0 / 0 / 0 | 5719 / 13365 ms |

There are also zero resent effects or run errors. C1–C3 success rates use
only runs where the fault fired; safety counters are zero across all 240.
The set has been used before and is a regression set, not a fresh quality
comparison. The separate off-script failure demonstrates the limits of
these automated safety counters.

| Hypothesis | Verdict |
| --- | --- |
| R1: targeted answer, live and replay | Pass with per-scenario cassette workaround |
| R2: one read, no effect from question/refusal, ambiguous-write verification | Pass |
| R3: fresh success/dead-end thresholds and zero retained false claims | **Not supported:** one retained false claim |
| R4: C0 ≥97%, no duplicate/unconfirmed/foreign moves | Pass in the specified harness |
| R5: privacy and explicit failure tests | Pass |

The bounded information-sharing correction closes #31. It does not close
#32 or #33, and does not make an unconditional conversational safety claim.

Recorded spend is approximately **$0.3546**, below the $3 cap: control
$0.001577; combined candidate $0.001563; isolated recordings $0.001392;
off-script ledger $0.0373; market ledger $0.1372; judge $0.1756. Replay
makes no live calls. The market reports' ledger totals include the patient
simulator and provider-reported costs; per-run `costUSD` instead excludes
the simulator and reprices text tokens at list price. They are different
measures and are preserved separately in `summary.json`. Recompute it with
`python3 bench/results/freemode-003/summarize.py`.

## Validation and reproducibility

`make ci` passed, including race-enabled unit tests, schema and example
validation, benchmark gates and all 38 replayed conversation scenarios.
`make lint-extra` passed staticcheck, errcheck and govulncheck (no reported
vulnerabilities). `tessera explain Agent.spec.freeMode.runReads` exposes the
new field documentation.

The measured candidate is reproducible from the base commit in
`bench/results/freemode-003/base-commit.txt` plus `candidate.patch`. The
patch captures implementation, tests, schema, definitions, cassettes and the
fresh task set before running the fresh benchmark. `candidate.sha256`
records the measured implementation/definition hashes. No source or agent
prompt was changed during those runs. Later edits only consolidate results
and documentation.

Raw transcripts, judgments and reports are under
`bench/results/freemode-003/{control,targeted,offscript,market}`. The
isolated CI cassettes are `bench/cassettes/clinic-context-*.jsonl`.
Reproduce live targeted runs with `tessera test examples/clinic-context
--only read-context/<scenario> --generate openrouter:deepseek/deepseek-v4.1-flash
--max-cost 0.03 --cassette <scenario>.jsonl --cassette-mode record`; replay
with the same filter, generator and cassette, changing mode to `replay`.

Fresh measurement commands (require budgeted provider keys):

```bash
bench/scripts/market.sh -tasks bench/market/tasks-offscript-34.json \
  -out bench/results/freemode-003/offscript \
  -defs bench/market/tessera-context -conditions C0 -max-cost 0.5
python3 bench/market/naturalness/offscript_judge.py \
  bench/results/freemode-003/offscript/runs.jsonl \
  bench/market/tasks-offscript-34.json \
  bench/results/freemode-003/offscript --max-cost 0.8
bench/scripts/market.sh -tasks bench/market/tasks-003.json \
  -out bench/results/freemode-003/market \
  -defs bench/market/tessera-context -max-cost 0.5
```

Do not treat reruns of these tasks as new held-out evidence. All patients
and data are synthetic. The domain, generator and judge limitations from
earlier rounds still apply; there is no new competitor comparison.
