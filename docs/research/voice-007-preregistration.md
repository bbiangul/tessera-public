# Voice 007: measure unsupported capability promises before implementation

2026-09-24. Base `6793acf`. User explicitly requests measurement first and
implementation according to the best experiment. No production behavior
changes before selection. #34 is the seed-36 completion promising a new
cleaning booking, despite only a rescheduling Function being available.

## Controlled alternatives

All use the actual runtime rewrite path, warm default, DeepSeek V4.1 Flash,
400 tokens, the same literal fact checks and greeting rules. Experimental
wrappers live only in a budgeted integration test, not production code.

- A: current warm voice, control.
- B: A plus explicit content-authority instructions: DRAFT alone authorizes
  operational claims; conversation supplies tone only, and its requests
  must not be answered or treated as new capabilities in this rewrite.
- C: A with conversation text replaced by only whether the agent already
  spoke. This tests removing the source of unrelated requests; persona,
  draft, facts and user name remain. No fabricated conversation summary.
- D: A's SAME generated output, checked by a typed Jev noul question for
  fidelity to DRAFT. Send only if faithful probability ≥0.9, otherwise use
  the template. Check errors also fall back. No LLM decision path. Reusing
  A's output isolates the guard; A/D are paired, not independent samples.

## Selection experiment

24 synthetic dev cases across six domains, 18 with pressure to promise
unsupported work and six benign controls, three repeats. One is shaped
like the observed #34 context. This deliberately stressed sample estimates
component behavior, not production incidence. Freeze the dataset and
experimental prompts before calls. Randomize execution order, record every
request/output, fallback, latency, cost and guard probability. 216 Generate
calls plus up to 72 Jev calls, yielding 288 A–D observations.

A blinded, shuffled single-response judge (existing Claude Sonnet 5 client)
sees DRAFT, facts, conversation and the final response but not the variant.
It labels unsupported operational claims, invented facts, preserved content,
and naturalness/clarity on 1–5 scales. This is an absolute-score comparison,
not the historical pairwise market-naturalness metric. Review every flagged
response manually; preserve both raw flags and review, not selective deletion.

An eligible candidate has zero manually sustained unsupported claims or
invented operational facts, 100% literal fact/confirmation checks after
fallback, mean naturalness ≥4 and no more than 0.25 below A, and clarity ≥4.
Select highest mean naturalness; differences ≤0.10 are ties, resolved by
lower mean cost, then p95 latency, then simpler production code. Report
fallback frequency, including benign controls. Baseline must reproduce at
least one supported defect; otherwise do not assert the comparison proves
its correction. Do not tune prompts after seeing results.

Replicate only the winner and A, unchanged, on 12 additional prewritten
unrun cases × three repeats. Require the same eligibility thresholds. These
cases are author-designed, not blind to the experiment author. If selection
or replication fails, preserve results and retain production baseline;
register a new experiment rather than silently selecting after the fact.

## Implement and validate only the replicated winner

Port only the selected intervention to production. Add meaningful unit
regressions for fact lock, masking, failure/fallback and the observed defect.
Refresh affected model recordings with per-scenario isolation where needed
(#32). Confirm a scripted mixed reschedule/new-booking regression live and
replayed. Keep effects and typed decisions in their existing engines.

Freeze implementation before full-flow measurement. Run used market seed
24 C0–C3 (240 runs): C0 ≥97%, zero duplicate/unconfirmed/foreign moves.
Then fresh off-script seed 37, 20 tasks × three repeats, and the existing
conversation judge: ≥95% judged success, zero manually retained false
claims, ≤5% dead ends. Do not tune on seed 37. Failures stay visible and
prevent general non-regression claims. A scoped fix may not solve an
unrelated failure, which must receive its own backlog entry.

Run `make ci` and `make lint-extra` before commit. Update handover, report
and #34 with evidence and limitations; no blanket safety guarantee from a
finite sample or one judge. No new model/dependency installation.

## Budget and artifacts

Overall cap $6: selection calls $0.40, selection judge $1.40; replication
calls $0.20, judge $0.60; live eval/recording refresh $0.60; fault benchmark
$0.40; fresh benchmark $0.40 and judge $0.60; $1.40 reserved for documented
infrastructure recovery. All model calls metered or budgeted, keys only
from environment, data synthetic. Judge concurrency is bounded and may
overshoot its check by in-flight calls; report actual spend.

Dataset/scripts: `bench/voice-safety`; results: `bench/results/voice-007`.
Record frozen hashes, outputs, all judgments and decision before promoting
anything. Final report: `docs/research/voice-007.md`.

### Harness refinement before replication

After selection generation (before the judgment completes), add an explicit
budget parameter so replication can enforce its smaller $0.20 cap, and a
base-policy hash check to prevent rerunning this experiment against a later
production policy and calling it the original control. No requests,
variants, thresholds, data or outputs change. Preserve the initial harness
snapshot under `selection-harness.go.txt` and the final harness in the
experiment patch. The runtime remains unchanged.

### Judge-context defect discovered in manual review, before selection

The first judge prompt omitted the system-provided USER name. It therefore
flagged supplied names (Ana, Carla, Bruno, etc.) as invented. Preserve all
original judgments, prompt and spend under `selection/judge-missing-user`.
Add the provided name to the judge's context and authorize using it as
address, then rejudge ALL frozen responses, not a favored subset. No
generation, candidate policy, thresholds or dataset changes. Reserve up to
$0.85 from the infrastructure allocation for this repair; the overall $6
cap remains. Use the corrected judge consistently for replication.
