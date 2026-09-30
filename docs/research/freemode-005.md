# Free mode 005: mixed-service interruption (#35)

2026-09-24, base `1d07796`. See the [preregistration](freemode-005-preregistration.md).
The seed-37 failure remains in voice-007's 59/60 score. This round measures
handler selection before changing production, using Jev 1.13.0 throughout.

## Development selection and explicit revision

24 synthetic cases × three repeats, with expected handlers written before
calls. They include side services, yes-plus-service requests, information,
plain confirmation/refusal, same-process corrections, cancel and goodbye.
Several service contexts are represented, but these are small authored
sets, not an estimate of production incidence. Full requests, native
probabilities and outputs are retained; probabilities are not asserted to
be calibrated. A/C share a response; later B/D also share a response.

| Variant | First development | Registered development revision |
| --- | --- | --- |
| A: existing policy | 63/72 | 61/72 |
| B: explicit priority for additional services | 71/72 | 72/72 |
| C: combine question/request mass, same handler | 66/72 | 66/72 |
| D: B plus C | not run | 72/72 |

None of A/B/C passed the initial gates. The original failure shape
reproduces in A: “Antes de confirmar ... limpeza nova” splits question
0.43 / request 0.57 and repeats confirmation, despite both categories
sharing one handler. Another mixed yes/service message is called answer
with probability 0.64–0.71. Aggregation alone does not fix that preference.
B fixes the answer preference but one request remains split: request 0.59,
question 0.11, answer 0.30. All plain controls pass; failures are side cases.

An explicit development revision registered D, without changing B's prompt,
C's threshold, datasets or labels. Development was rerun openly, not called
a fresh test. D passes 72/72 and advances according to that revision.
B also passes in the second invocation; this is not evidence D is superior
to B on every run. The initial B failure stays as evidence that prompt-only
handling can still discard probability divided across equivalent handlers.
No errors occur in either round. Original and revised harnesses are saved.

## Replication before implementation

On the prewritten, previously unrun 12 cases × three repeats, A scores
33/36; D scores **36/36**, meeting every replication gate. A's three
failures deliver a mixed approval/new cancellation request as an answer.
The harness also records intermediate B (36/36) on the exact response used
by D; this diagnostic is not an independent third sample or an additional
model call. The replication does not independently establish aggregation's
benefit, nor is the authored set hidden from its author.

`selection-d/decision.json` and `replication/decision.json` record advancement
and promotion before production changes. Model calls use one shared
$0.10 ledger per round, with per-call sub-budgets. Text generation remains
DeepSeek V4.1 Flash in subsequent full-flow tests.

## Production boundary and deterministic regression

The only production change is the measured D intervention in interruption
classification: explicit side-service precedence in the typed question,
then sum question/request probability before the existing 0.6 check.
Both categories already invoke the same free-answer/reask branch. This
sum never includes answer, cancel or goodbye, and cannot authorize an
effect. All existing uncertain/error/failed-answer behavior remains.
A new explicit confirmation is required after resolving the side request.

A unit regression failed on baseline: a 0.43/0.57 split emitted only the
pending confirmation. It passes with D: answer plus reask, no write, then
exactly one write after a new yes. Further deterministic controls verify
that answer probability is never added to side mass, low confidence stays
unresolved, and clear answers/cancellations retain their handlers. Existing
qualified-confirmation failure tests remain in the race suite.

## Reproduction

Apply `bench/results/freemode-005/experiment.patch` to the base recorded in
`base-commit.txt` in an isolated checkout. Do not run the experiment against
the changed production classifier and call it the original baseline.
With the provider key in the environment:

```bash
INTERRUPTION_DATA="$PWD/bench/interruption/dev.json" \
INTERRUPTION_OUT="$PWD/bench/results/freemode-005-rerun/dev" \
go test -tags integration ./internal/host -run '^TestInterruptionExperiment$' -count=1 -v
```

For replication use `replication.json`, a distinct output path and
`INTERRUPTION_VARIANTS=A,D`. The original A/B/C run used the saved
`selection-harness.go.txt`; the experimental patch contains the registered
A/B/C/D revision. `bench/interruption/summarize.py <result-directory>`
recomputes scores and failures. Repeated responses to one case are correlated.

## Targeted live/replay and validation

All **10 affected scenarios pass live**: seven confirmation flows and three
read-context flows. The new `mixed-service-before-confirmation` scenario
gets the supplied clinic phone on both the initial extra-service question
and the mixed yes-plus-service message. The run stays Confirming on both,
performs no write until the final explicit yes, then reschedules exactly
once. The full transcript and calls are retained in `evals/`.

`make ci` passes: race-enabled unit tests, lint/build, 57 valid resources,
and all **45 offline replay scenarios**. `make lint-extra` passes staticcheck,
errcheck and govulncheck with no reported vulnerabilities. Recordings are
isolated per scenario as required by the still-open #32 replay limitation.
The new routing prompt affects only the two free-mode clinic examples;
the remaining example recordings are unchanged.

The production freeze is `implementation.patch` plus `implementation.sha256`.
Both experimental and production patches apply cleanly to the recorded base.
No code changes occur during the full-flow benchmarks. Logs are under
`validation/`, including the expected pre-fix failure. Intermediate B rows
in replication do not represent additional spend: D uses the same response.
Initial classification calls cost $0.003767, the revision $0.003767,
replication $0.001884 and the ten live flows $0.004226.

## Full-flow fault regression

Used market seed 24: **240/240 correct final states**; C0 60/60, all 48
fired cases in each of C1/C2/C3 succeed. No duplicates, unconfirmed,
resent or foreign effects, and no run errors. This passes the registered
C0 >=97% / zero unsafe effects gate on a used regression set.

| Condition | p50 turn | p95 turn |
| --- | --- | --- |
| C0 | 2538 ms | 5091 ms |
| C1 | 3342 ms | 21864 ms |
| C2 | 2044 ms | 3480 ms |
| C3 | 1766 ms | 4008 ms |

Ledger spend is $0.1236. This run has no concurrent full-flow baseline;
latency differences from prior rounds are observations, not a causal
speedup claim. In particular the injected-fault tail remains large.

## Fresh seed 38 and verdict

**60/60 correct states and 60/60 judged successful conversations**, all
answerable questions answered, zero false claims and zero dead ends.
Mean humane score 4.683/5; turn latency p50 1906 / p95 3996 ms. All
unconfirmed, duplicate and foreign-effect counters are zero. The existing
unchanged judge costs $0.1724; the run ledger $0.0310. Every registered
full-flow gate passes. No seed-38 tuning or response exclusions.

There are no negative judgments to overturn. Manual inspection additionally
checks all six new-request-midway conversations (t17/t18, three repeats):
each gives the clinic phone for cleaning, reasks the pending confirmation,
and reports rescheduling only after the later explicit yes. This agrees
with the judge, but six related synthetic conversations are not a universal
guarantee. The single-domain full-flow set, one text model and one judge
remain limitations. No renewed market-naturalness parity claim is made.

**Promote D and close #35.** The initial standalone interventions failed
and stay recorded; the registered combination replicated and passed live,
replay and full-flow gates. No extra provider call is added to production.
Total recorded spend **$0.3406 of $3**, including rejected alternatives and
the development rerun. `spend.json` itemizes stages; existing full-flow
reporters round totals to four decimals. Recompute with
`python3 bench/results/freemode-005/summarize.py`.

Next fresh off-script seed: **39**. The next independent backlog item is
#32: scenario-scoped cassette replay without manual file splitting.
