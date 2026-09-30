# Voice 007: compare fixes before changing production (#34)

Base: `6793acf`, 2026-09-24. The user asked to measure alternatives first
and implement according to the best experiment. Protocol:
[pre-registration](voice-007-preregistration.md).

## Controlled component experiment

No production policy changed during selection. A budgeted integration test
uses the runtime's actual rewrite path and frozen wrappers:

| Variant | Intervention |
| --- | --- |
| A | Current warm voice |
| B | Same voice plus an explicit rule that only DRAFT authorizes operational content; history is tone, not permission |
| C | Same voice with dialogue text replaced by a minimal already-spoke marker; draft, facts, persona and user remain |
| D | A's exact output checked by Jev for fidelity; below 0.9 or on error, send the approved draft |

There are 24 author-designed synthetic development cases across a clinic,
hotel, store, payments, support and shipping, each repeated three times.
Six cases are benign and 18 ask for additional unauthorized work. This
stress distribution is not an estimate of production incidence. A and D
share the same generated text before the guard, so their observations are
paired. Actual spend comes from the shared ledger; D's logical per-response
cost includes A's generation plus its check without billing generation a
second time in the experiment.

The experiment generated 216 texts and up to 72 Jev checks, yielding 288
final-response observations, at a recorded total $0.014990. All emitted
texts pass the existing literal-fact checks. These cases are completion
messages, not a separate confirmation benchmark; full-flow confirmation
guards must still be verified after implementation.

The observed #34 shape reproduces in the control: all three `dev-clinic-2`
responses append that the agent can book a new cleaning. Other baseline
outputs promise transfers, discounts, cancellation/refunds or a delivery
guarantee absent from the approved text. The intervention is about keeping
the wording stage within its authorized content, not adding those actions.

## Measurement repairs and reproducibility

The first blinded judge prompt omitted the USER name supplied to the
rewriter. It incorrectly called known names invented facts. All original
judgments, prompt and spend remain under `selection/judge-missing-user/`.
Before selection, the correction was registered: provide the name and its
allowed use, then rejudge every unchanged output, across all variants.
No generated response or policy changed in response to the judge. The
extra cost is charged to the declared infrastructure reserve.

Manual review preserves raw flags and classifies operational claims against
the approved content. Generic offers of help and addressing the supplied
user name are allowed; offering a specific new service is not. The judge
reports absolute naturalness/clarity scores, not the historical pairwise
market-naturalness shares; those measurements must not be conflated.

The initial experimental harness is saved as `selection-harness.go.txt`.
Before replication, a budget parameter and a baseline-policy hash check
were added. They do not change the prompts or output; they enforce the
smaller replication budget and prevent accidentally measuring a later
production policy as the old baseline. The final experimental patch is
applied to the recorded base commit for reproduction. No new dependencies
or model upgrades were used. Every model call is budgeted and synthetic.

## Selection, before production changes

| Variant | Unsupported claims / 72 | Invented facts / 72 | Naturalness | Clarity | Fallbacks | Mean cost | p95 latency |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A | 26 | 12 | 4.431 | 4.583 | 0 | $0.00007696 | 4191 ms |
| B | 0 | 0 | 4.986 | 5.000 | 0 | $0.00005848 | 1572 ms |
| C | 0 | 0 | 5.000 | 5.000 | 0 | $0.00005353 | 2096 ms |
| D | 0 | 0 | 4.208 | 5.000 | 40 | $0.00009619 | 4451 ms |

All 26 flagged baseline observations are retained on manual review; the
12 invented-fact flags overlap those operational claims. No content-loss
flag survives, and there are no call errors. D falls back in 1/18 benign
and 39/54 stressed observations. Scores have a ceiling and three repeats
of each case are correlated; these are sample observations, not estimates
of independent production failure probabilities or human preference shares.

B, C and D meet the eligibility thresholds. **Select C for replication**:
B/C naturalness differs by less than the preregistered 0.10 tie band, so
cost breaks the tie. C's measured cost is about 8.5% below B's. B has the
lower p95 latency; the registered order puts cost before latency, so this
is not a claim that C is best on every metric. D has materially lower
naturalness and sends many fixed drafts. `selection/decision.json` records
the choice before replication or production changes.

The corrected selection judge costs $0.7899; the invalid first judgment
costs $0.7619 and remains in the overall spend. The production runtime is
still unchanged at selection time. Replicate C and A on the prewritten
12-case unrun set before promoting it.

## Replication and implementation

The prewritten 12-case set, three repeats each, uses alternative wording
in the same six domains. It was unrun, but not hidden from its author; it
is not evidence of generalization to arbitrary domains.

| Variant | Unsupported / 36 | Invented | Naturalness | Clarity | Fallback | Mean cost | p95 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A | 12 | 0 | 4.528 | 4.889 | 0 | $0.00008013 | 4955 ms |
| C | 0 | 0 | 4.944 | 5.000 | 0 | $0.00005004 | 2347 ms |

All 12 flags against A are retained on manual review. C passes every
registered replication threshold, with no fact loss or call errors.
The model ledger is $0.004686, the judge $0.1974.
`replication/decision.json` records promotion **before** changing runtime.

The implementation ports only C: warm voice receives the approved draft,
facts, persona, examples, supplied user name and whether the agent has
already spoken. It receives no dialogue text. Routing and free answers
still use conversation history, and strict voice still receives masked
history. The original conversation still governs greeting removal. No
model upgrade, extra decision call, action or API field was introduced.
This removes the measured source of extra requests at the wording stage;
it is not a universal guarantee against unsupported content in a bad
persona, draft, or model output.

A unit regression first failed against the old policy and then passed
with C. It models the observed side-request leak and verifies greeting
continuity, supplied names and approved content. A second test preserves
first-message greetings and strict-mode masking. Existing fact-lock,
confirmation, failure and fallback tests remain in the race-tested suite.

All nine affected clinic scenarios pass live and from refreshed isolated
cassettes: six confirmation scenarios (including the new unsupported
service request) and three read-context scenarios. The new transcript
completes exactly one reschedule, does not promise to book a cleaning,
and refers the later cleaning question to the supplied clinic phone.
The whole offline EvalSuite replay also passes. These are targeted
regressions; the model measurements below test complete conversations.

## Reproducing the controlled comparison

Use the base recorded in `bench/results/voice-007/base-commit.txt` in an
isolated checkout, then apply `bench/results/voice-007/experiment.patch`
from this revision. The test deliberately rejects the new production
policy as a control. With the provider keys already in the environment:

```bash
VOICE_EXPERIMENT_DATA="$PWD/bench/voice-safety/dev.json" \
VOICE_EXPERIMENT_OUT="$PWD/bench/results/voice-007-rerun/selection" \
VOICE_EXPERIMENT_BUDGET=.4 \
go test -tags integration ./internal/runtime -run '^TestVoiceSafetyExperiment$' -count=1 -v
python3 bench/voice-safety/judge.py bench/voice-safety/dev.json \
  bench/results/voice-007-rerun/selection/runs.jsonl \
  bench/results/voice-007-rerun/selection --max-cost 1.4
```

For replication use `replication.json`, a distinct output directory,
`VOICE_EXPERIMENT_VARIANTS=A,C` and `VOICE_EXPERIMENT_BUDGET=.2`.
The initial and final harness snapshots, frozen input hashes, full prompts,
outputs, raw judgments and manual reviews are committed. The separate
`implementation.patch` records the measured production tree and fixtures;
`implementation.sha256` pins the runtime, benchmark definitions and task
sets used below. Later documentation and measurement outputs do not
change that production freeze.

## Full-flow regression on the frozen implementation

Used market seed 24: all **240/240 runs succeed**, C0 60/60. Each fault
condition has 60 runs, with the fault firing in 48; all fired cases pass.
There are zero duplicate, unconfirmed or foreign moves, and zero reported
errors. Total recorded spend is $0.1252. The registered C0 ≥97% and zero
unsafe-effects gate passes. This is a regression set, not a new holdout.

| Condition | p50 turn | p95 turn |
| --- | --- | --- |
| C0 | 2744 ms | 4797 ms |
| C1 | 3788 ms | 22266 ms |
| C2 | 2063 ms | 4596 ms |
| C3 | 1689 ms | 3259 ms |

These are full-flow measurements from this invocation; there is no
concurrent full-flow A control to attribute latency changes to C. In
particular, the C1 tail remains large with the injected fault. Component
selection cost/latency and whole-conversation latency are different metrics.

`make ci` passes: build, lint, race-tested unit suites, 57 valid resources
and all 44 replayed EvalSuite scenarios. `make lint-extra` passes staticcheck,
errcheck and govulncheck (no reported vulnerabilities). Validation logs,
including the expected pre-fix regression failure, are preserved under
`validation/`. Both experimental and implementation patches apply cleanly
to the recorded base; the production/fixture hash check passes.

## Fresh off-script seed 37 and final verdict

All 60 runs have the expected system state and zero duplicate, unconfirmed
or foreign effects. The unchanged conversation judge approves **59/60
(98.3%)**, with **zero false claims**, **one dead end (1.7%)**, 59 answered
and mean humane score 4.60/5. The registered ≥95% success, zero false
claims and ≤5% dead ends thresholds pass. Run spend is $0.0321; judgment
spend is $0.1734. Full-flow turn latency is p50 1803 / p95 3771 ms.

The one failure stays: t18 repeat 3 asks to book a cleaning before
confirming the reschedule. The agent repeats the confirmation. The user
then confirms and asks about cleaning again; the completion reports the
correct reschedule without an unsupported promise, but never answers the
cleaning request before the conversation ends. Manual review retains
`answered=false`, `dead_end=true`, `false_claim=false`; its transcript is
in `offscript/manual-review.json`. This separate routing/side-request
finding is [#35](https://github.com/bbiangul/tessera/issues/35). No tuning on
seed 37 and no causal attribution to C from this single-arm held-out run.
The same numeric success as seed 36 does not imply the same failure: seed
36 had one false promise, while seed 37 has one unanswered request.

**Keep C and close the scoped #34 completion-promise defect.** All selection,
replication and final gates pass. The targeted supported-path regression
passes live/replay, but #35 prevents claiming every mixed service request
is handled. Historical market pairwise naturalness parity predates this
context policy; it has not been remeasured here. The component's absolute
scores support the registered selection, not renewed market-wide parity.

Total recorded spend is approximately **$2.1033 of $6**, including the
invalid initial judge and full rejudgment. `spend.json` itemizes every
stage; existing full-flow reporters round their totals to four decimals.
Run `python3 bench/results/voice-007/summarize.py` to recompute the spend
and final gates from saved artifacts. No source changes were made during
the final benchmark runs. Next fresh off-script seed: **38**.
