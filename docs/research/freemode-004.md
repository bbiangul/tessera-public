# Free mode 004: qualified confirmations and completed work (#33)

2026-09-24. [Pre-registration](freemode-004-preregistration.md).
Baseline: `2e78d82`. Final revision B passes **5/5 targeted conversations**
live and replayed, and all 240 market regression runs. Fresh seed 36 has
**59/60 judged successes**, one retained false capability claim and no dead
ends. The narrow #33 flow correction is implemented; there is no claim
that every preregistered criterion or all conversation quality goals pass.

Candidate A was rejected after market regression exposed broken corrections;
its failed evidence is preserved. Revision B distinguishes corrections from
information questions and adds a fifth regression scenario. See the
[revision-B preregistration](freemode-004-b-preregistration.md).

## Correction

Jev's typed interruption question now classifies the whole message:
questions and prerequisites take priority over a yes or choice in the same
message. A qualified yes is answered through free mode, followed by the
pending confirmation. It is not delivered as an answer to the Function.

The deterministic regressions also exposed two failure paths: an
unavailable classifier or a failed free-mode answer previously fell through
and delivered the leading yes. Classifier errors now propagate without
advancing the run. Uncertain classification or a failed answer repeats the
pending question, leaving it unanswered. A subsequent clear yes still works.

For free-mode agents, the router now sees up to eight recent dialogue lines,
masked and checked against the caller and namespace. This preserves the
completion that precedes an intervening information question. Its typed
instructions distinguish acknowledgement of reported completion from an
explicit request to change it again. Free mode may acknowledge a completion
already reported by the agent; it cannot claim a fresh action or check.
The Function's confirmations, effects and idempotency remain in the runtime.
The warm voice and text model are unchanged.

This is bounded conversational context, not a persistent index of completed
business operations. After sufficient conversation, earlier completion may
fall out of view. The standalone `Host.Route` API still takes only its
explicit message/last-message/last-Function context; conversation-aware
handling uses the host's internal routing path.

## Candidate A: targeted live and replay evidence

| Scenario | Baseline live | Candidate live | Candidate replay |
| --- | --- | --- | --- |
| Qualified yes, then explicit yes | Fail: acts on qualified yes | Pass | Pass |
| Qualified yes, then refusal | Fail: already acted before refusal | Pass | Pass |
| Late confirmation after an information question | Fail: starts another Function and reads slots again | Pass | Pass |
| Explicit request to change completed work again | Pass | Pass | Pass |

The late-confirmation candidate performs one slots read and one write across
the entire conversation. The explicit new request performs two writes to
different slots, each after its own confirmation. The refusal performs zero
writes. Unit tests verify no effects on classifier/generator failure or
uncertainty, continued operation after a fresh yes, history delivery,
masking and rejection of a different caller or namespace.

`examples/clinic-confirmation` is a synthetic clinic with a declared
insurance/parking information read and scripted EvalSuite. Each CI scenario
has its own cassette, retaining the #32 workaround. The three existing
clinic-context recordings were refreshed because their interruption prompt
and free-answer instructions changed. All three still pass live and replay.

## Candidate A: fresh off-script evidence

Unchanged generator, seed 35, 20 tasks × three repeats. All 60 runs reach
the intended final state, and the existing Claude Sonnet 5 judge reports
**60/60 successful conversations, zero false claims and zero dead ends**.
Mean humane score is 4.683/5. Turn latency p50/p95 is 1696/3070 ms. Harness
unconfirmed, duplicate, resent and foreign effect counters are all zero.
The off-script ledger reports $0.0365 and the judge $0.1757.

This supports R4 on this sample. It does not establish superiority or a
causal improvement from seed 34's 59/60: the sets differ and there is no
contemporaneous baseline on seed 35. Seed 34's failure remains in the
previous report. Seed 35 is now used; seed 36 is the next held-out set.

A targeted late-confirmation answer repeats the user's closing sentence
verbatim. It correctly avoids a new process, but this is not evidence of
improved naturalness; conversational polish remains part of #24. The
conversation judge and the state harness measure different properties.
The harness's lexical confirmation check alone cannot detect a qualified
yes, so targeted turn-by-turn assertions remain necessary.

## Candidate A rejected: ordinary corrections regress

The original candidate's market round has C0 **57/60 (95%)**, failing R5's
97% gate. It mistakes changes of date/time phrased as questions for side
questions, or classifies them without enough confidence and repeats the old
confirmation. The final report must not present the off-script 60/60 as
proof this candidate is acceptable.

| Condition | Runs | Fault fired / scored | Success among scored |
| --- | --- | --- | --- |
| C0 | 60 | 60 | 57/60 (95%) |
| C1 | 60 | 47 | 47/47 (100%) |
| C2 | 60 | 48 | 45/48 (93.75%) |
| C3 | 60 | 48 | 46/48 (95.83%) |

All effect-safety counters are zero, but there are nine unsuccessful runs.
One C1 run errors before fault injection: Jev returns a choice inconsistent
with its probabilities, correctly rejected by the adapter. That error is
outside the report's fired-only C1 denominator; it is not omitted here.
The ledger reports $0.1564. Raw runs and the frozen candidate patch remain
at their original paths.

A fifth scripted case, recorded against the frozen A binary, reproduces
the problem: “mudei de ideia ... sábado ... pode ser?” keeps the Friday
confirmation. Revision B changes only the interruption instructions/options
to explicitly classify selecting or correcting a slot as an answer, while
preserving priority for prerequisite information questions and fail-closed
behavior. The A cassettes are retained in `candidate-a-cassettes/` before
refreshing B's CI recordings. Seed 35 is not reused as a holdout; B uses
seed 36, registered before its run.

## Development deviations

An initial CLI invocation used `--max-cost` rather than `--max-cost-usd`;
it exited before any model call. The `--only qualified-yes` filter matched
two scenario identifiers, so the refusal was run twice in baseline and
candidate recording. Preserve these outputs, rather than counting the
extra run as an independent scenario or dropping its cost. The positive
scenario was renamed `qualified-yes-then-accept` and recorded alone for CI.
Its turns, assertions and implementation did not change. The first candidate
passed every distinct scenario; no prompt tuning followed these results.

Original combined targeted outputs and the extra recordings are in
`bench/results/freemode-004/{baseline,candidate}`; the isolated candidate
cassettes used by CI are under `bench/cassettes/clinic-confirmation-*.jsonl`.

## Candidate A: validation and reproducibility

`make ci` passed: race-enabled tests, lint, generated-schema checks,
57 validated example resources, and all 42 replayed conversation scenarios.
`make lint-extra` passed staticcheck, errcheck and govulncheck, with no
reported vulnerabilities. Logs, including the original failing unit
regressions, are under `bench/results/freemode-004/validation`.

The measured implementation is the base commit in `base-commit.txt` plus
`candidate.patch`, both under `bench/results/freemode-004`. The patch was
saved before fresh measurements and includes source, tests, definitions,
recordings and the new task set. `candidate.sha256` freezes runtime source,
benchmark definitions and tasks. No source changes are made during the
fresh and fault benchmarks; subsequent edits consolidate documentation
and evidence only.

Reproduce the isolated scenarios with:

```bash
./bin/tessera test examples/clinic-confirmation \
  --only confirmation-flow/qualified-yes-then-accept \
  --generate openrouter:deepseek/deepseek-v4.1-flash --max-cost-usd .03 \
  --cassette bench/cassettes/clinic-confirmation-qualified-yes-then-accept.jsonl \
  --cassette-mode replay
```

Use `record` with a new cassette path for live reproduction. The other
scenario identifiers and replay commands are in `make test-evals`.

Frozen benchmark commands:

```bash
bench/scripts/market.sh -tasks bench/market/tasks-offscript-35.json \
  -out bench/results/freemode-004/offscript \
  -defs bench/market/tessera-context -conditions C0 -max-cost .5
python3 bench/market/naturalness/offscript_judge.py \
  bench/results/freemode-004/offscript/runs.jsonl \
  bench/market/tasks-offscript-35.json \
  bench/results/freemode-004/offscript --max-cost .8
bench/scripts/market.sh -tasks bench/market/tasks-003.json \
  -out bench/results/freemode-004/market \
  -defs bench/market/tessera-context -max-cost .5
```

Provider keys are required for live runs; all business data is synthetic.
The fresh set is one use of seed 35 from the unchanged generator, 20 tasks
× three repeats, evaluated by the existing off-script judge. Seed 24 is a
used fault-regression set. There is no new competitor comparison, model
upgrade or naturalness-superiority claim.

## Revision B: development and validation

The corrected interruption wording is the only behavioral difference from
A. The router/history and failure-handling code remain the same. All five
scripted scenarios pass live and in isolated replay, including the
pre-action correction that failed against A. The three existing
clinic-context scenarios also pass after refreshing their recordings.

`make ci` passes on B: 57 resources, 43 conversation scenarios, race-enabled
unit tests, schema checks and lint. `make lint-extra` also passes with no
reported vulnerabilities. Logs are under `validation-b/`. The measured
B patch and hashes are `candidate-b.patch` and `candidate-b.sha256`, against
the same base commit. Both A and B patches were checked for clean application
to that base. Text generation remains DeepSeek V4.1 Flash and decisions
remain Jev; no model escalation was needed.

B repeats the used market regression first, then evaluates seed 36 as its
fresh off-script set. Use the same commands above with `market-b`,
`offscript-b`, seed 36 and caps $0.40/$0.40/$0.60 respectively. A's original
results remain unchanged. `summary.json` separates candidates and includes
all extra development recordings; `summarize.py` reconstructs those counts
from committed raw evidence. Ledger totals include the simulator and
provider-reported prices; per-run `costUSD` is agent-only at normalized
list prices, so those values are reported separately.

### B fault regression

| Condition | Runs | Fault fired / scored | Success among scored | Unconfirmed / duplicate / foreign | Turn p50 / p95 |
| --- | --- | --- | --- | --- | --- |
| C0 | 60 | 60 | 60/60 (100%) | 0 / 0 / 0 | 1952 / 3509 ms |
| C1 | 60 | 48 | 48/48 (100%) | 0 / 0 / 0 | 2984 / 21770 ms |
| C2 | 60 | 48 | 48/48 (100%) | 0 / 0 / 0 | 1877 / 3252 ms |
| C3 | 60 | 48 | 48/48 (100%) | 0 / 0 / 0 | 1674 / 3269 ms |

No run errors or resent effects occur in B's 240 runs. This supports R5
on the specified regression set. The ledger reports $0.1340. Latencies
are observations, not a controlled claim of improvement over A.

### R3 wording limitation

The preregistration says a late confirmation starts no “Function/read/effect”.
The tested and implemented boundary is narrower: no new rescheduling
Function, slots read or write. The configured public business-info read
still runs for the free answer, as it does for other free-mode messages.
In the late-confirmation transcript there are two `info` calls: one for the
insurance question, one for the closing acknowledgement. The literal
no-read wording is therefore **not met**, and is not retrospectively
relabelled a pass. Avoiding those ordinary FAQ reads was not part of #33's
acceptance or the RFC's read-capable free-mode contract. The functional
late-confirmation goal and explicit-new-request control pass; this report
makes no zero-total-reads claim for free answers.

### B fresh off-script judgment and remaining work

Seed 36 has 60/60 correct final states, but **59/60 judged successes
(98.3%)**. The judge flags one false claim and zero dead ends; mean humane
score is 4.683/5. Turn latency p50/p95 is 1725/2974 ms. All effect-safety
counters are zero. The ledger reports $0.0361; judging costs $0.1738.

The retained failure is `t17`, repeat 1: while rescheduling, the patient
also asks to book a new cleaning. After explicit reschedule approval, the
Function's completion voice adds “consigo agendar pra você sim” about the
cleaning. No such booking Function exists. Free mode corrects the claim
on the next turn and gives the clinic phone. The reschedule is correct,
and no cleaning is booked, but the false promise still fails the
conversation. Manual review agrees with the flag. This is
[#34](https://github.com/bbiangul/tessera/issues/34), covering the completion
voice's invented capability and the initial unanswered side request.
No source or prompt was tuned on seed 36. Seed 37 is next.

| Criterion | Final verdict |
| --- | --- |
| R1: reproduce old flow defects, candidate live/replay | Pass: 5/5 candidate scenarios |
| R2: qualified yes waits, refusal/errors cannot authorize | Pass on targeted and failure tests |
| R3: no reopened process; new requests/corrections still work | Functional goal passes; literal no-read wording **not met** because public FAQ reads continue |
| R4: fresh ≥95%, ≤5% dead ends, zero false claims | **Not supported:** one retained false claim |
| R5: C0 ≥97%, no duplicate/unconfirmed/foreign effects | Pass in the specified harness |

Closing #33 is against that issue's functional acceptance (answer before
conditional approval; acknowledge completed work; preserve new requests),
not a claim that the preregistration's broader no-read closure gate passed.
This is an explicit scope deviation: free mode's ordinary public-info
reads remain permitted by RFC-0004. R4 remains failed and #34 stays open;
there is no general conversation non-regression claim. #32 also stays open.

Total recorded live spend is approximately **$0.7224**, below the overall
$3 cap: A including baseline and extra development recordings $0.3754;
B including refreshed recordings $0.3470. Replay costs zero. Raw reports
and all recordings, including A's failures and B's false claim, are retained.
`python3 bench/results/freemode-004/summarize.py` recomputes the totals.
