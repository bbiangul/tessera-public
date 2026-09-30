# Free mode 004: qualified and late confirmations (#33)

2026-09-24, before live calls. Baseline: `2e78d82`.

## Intervention

Keep the warm voice and smallest text model (DeepSeek V4.1 Flash).
Jev's interruption classification gives questions and prerequisites
priority over a yes in the same message. An unavailable classifier,
uncertain classification or failed free answer cannot deliver that yes
to the pending Function. The router receives masked recent dialogue for
free-mode agents, checked against namespace and caller, and distinguishes
acknowledgements of reported completion from genuine requests for new work.
No keyword filter or LLM-based decision path is added.

## Development and tests

Unit regressions first reproduce actions after classifier/generator failure
and missing routing history. Test retained confirmation, a plain yes,
refusal, identity isolation, masked history and genuinely new requests.

Four scripted development EvalSuite scenarios in `examples/clinic-confirmation`:
qualified yes followed by explicit yes; qualified yes followed by no;
late confirmation after an intervening information question; an explicit
new request after completion. Run a baseline from the base commit and a
candidate, each scenario recorded separately (#32). Each scenario costs
at most $0.03. Allow up to three candidate passes, preserving failures and
naming any revisions. Refresh the three clinic-context cassettes because
the interruption question changes; replay every CI suite. Run `make ci`
and `make lint-extra` before committing.

## Frozen measurements

After development, freeze code/definitions and save the candidate diff
and hashes. Generate fresh off-script seed 35 with the unchanged generator:
20 tasks × 3 repeats, C0, `bench/market/tessera-context`. Judge all 60 with
the existing off-script judge, retain failures after manual review. Do not
tune on seed 35. Run market seed 24 C0–C3, 240 runs, as a used regression set.
There is no new competitor comparison or naturalness-improvement claim.

| Hypothesis | Acceptance |
| --- | --- |
| R1 | Baseline reproduces at least one original flow defect; candidate 4/4 live and replayed |
| R2 | Qualified yes cannot act until the question is answered and a subsequent confirmation arrives; no write after refusal or classifier/generator failure |
| R3 | Late confirmation starts no Function/read/effect; explicit new request still starts and requires its own confirmation |
| R4 | Fresh judged success ≥95%, dead ends ≤5%, zero manually retained false claims |
| R5 | C0 ≥97% success; zero duplicate/unconfirmed/foreign effects across all fault runs |

R1–R3 are required to close #33. R4 or R5 failure must stay visible and
prevents a general regression-free claim. Benchmark confirmation counters
are lexical and cannot establish qualified-confirmation safety: the
scripted tests and failure review are necessary.

## Budget and artifacts

Maximum $3: baseline + up to three candidate targeted passes $0.48;
clinic-context refresh $0.09; fresh off-script $0.50; fault regression $0.50;
judge $0.80; $0.63 for documented infrastructure recovery. No dependencies.
All tasks synthetic; provider keys only through the environment.

`bench/results/freemode-004/`: baseline/candidate transcripts and cassettes,
frozen patch/hashes, fresh runs/judgments, market report, validation logs.
Report all spend and deviations in `docs/research/freemode-004.md`.
Update handover and #33 in the same change; #32 remains a separate issue.

### Development recording deviations (before fresh measurements)

The first command used `--max-cost` instead of `--max-cost-usd`: rejected
before model calls. `--only` matches substrings, so the original
`qualified-yes` filter ran both qualified-yes scenarios; the refusal was
also recorded separately. Preserve all results and costs. Rename the
positive scenario to `qualified-yes-then-accept` and record it alone for
CI; only the scenario identifier changes, not turns, assertions or code.
The candidate passed all four distinct scenarios in its first live pass;
the baseline failed both qualified confirmations and the late confirmation,
while the new-request control passed.
