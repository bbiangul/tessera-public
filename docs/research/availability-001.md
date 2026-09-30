# Availability 001: an inferred restriction hid a real slot

2026-09-24, base a2eab97. Protocol:
[availability-001-preregistration.md](availability-001-preregistration.md).
Evidence: `bench/results/availability-001/`. This is a development diagnosis
of the rejected authoring-004 candidate, not a new blind authoring score.
Historical definitions/results are unchanged; the corrected copy remains
experimental and is not promoted as a reference-quality clinic agent.

## Cause established by the journal

Repeat offscript40/t04 twelve times with the original definitions, clock,
API fixture and DeepSeek V4.1 Flash. Only 1/12 conversations succeeds; no
unsafe effects occur. All twelve FIRST extraction records contain both
`clockTime: "17:00"` and `period: "evening"`. The raw availability read
contains D1-1008-17. The authored filter requires evening hours >=18, so
it removes the real 17h slot and reports no availability. The one eventual
success follows later conversation; it does not make the initial extraction
correct. JSON journal exports preserve every record, without committing DBs.

This is a redundant inferred filter in the authored extraction contract,
not evidence that the calendar API omitted the slot or the runtime failed
to execute its declared expression. No runtime coercion or new decision
path was needed. Explicit conflicting user preferences must still be kept.

## Comparison before implementation

Keep the same runtime extraction batch instructions/schema and the same
small model/provider. Baseline versus A (period only when explicitly
requested; exact time alone means any) versus B (longer wording also
excludes greetings, old appointments and FAQ context, preserving conflicts).
Selection and replication messages were frozen before valid calls. Each
has 12 messages x3, including time boundaries, greetings, the old period,
explicit periods and conflicting period/time. All four fields and ownership
must match gold, not just a successful model response.

| Arm | Selection exact objects | Replication exact objects |
| --- | --- | --- |
| Original | 16/36 | 18/36 |
| A: explicit preference only | 36/36 | 36/36 |
| B: longer grounded instruction | 36/36 | Not advanced |

Both candidates pass selection; A wins the preregistered shorter-instruction
tie-break. This does not claim A is universally better or a statistical
estimate over all patient language. It fixes the identified redundant
restriction while preserving exact time and stated period in these cases.

The new diagnostic definition copy applies A to initial extraction and
follow-up answer descriptions. Twelve complete t04 conversations then pass
12/12, with first extraction period=any in all twelve and zero duplicate,
unconfirmed or foreign effects. This is same-case verification, not an
unseen market benchmark; market43/offscript41 remain unused.

## Two measurement corrections, with raw evidence retained

1. `bench extract` defaults to provider-constrained structured decoding;
   the runtime does not enable that option. Initial calls returned HTTP 404
   without model values. Rerun unchanged tasks with
   `--structured-outputs=false` to match runtime. The failed transport
   results stay under selection-*; valid runs are selection-runtime-*.
   Their $0.45 conservative ledger reservations are reported separately;
   there is no token-usage evidence for those HTTP failures.
2. CompareFields supported scalar values, arrays and `$any`, but ordinary
   nested gold objects fell through to false—even when exactly equal.
   Original trial/summary files remain untouched; rescore.py performs the
   preregistered exact JSON-object comparison on raw values. The production
   benchmark comparator now checks matching nested key sets recursively,
   preserving scalar normalization and `$any` alternatives. Regression tests
   fail before the fix and pass after it, including nested wrong values,
   missing/extra members, wrong types and nested alternatives. This scoring
   bug does not affect the state-based market or conversation judge results.

The full targeted + inherited suite initially passed 25/27 live. The two
new negative cases correctly refused unavailable/conflicting slots with
zero writes, but the evaluator expected Succeeded instead of this Function's
onUnresolved terminal state Failed. Initial tests/results are preserved.
Only those expected states were corrected; messages, effects, inputs and
gold data stayed. The same cassette then passed 27/27 offline. A final live
record/replay validates the corrected test contract separately.

## Independent external diagnostic partner

A fresh zero-history subagent received only public skill/docs/CLI, the
client-owned declarative definitions and a maintenance request to test
weekday preferences and withdrawal. It had no engine sources, previous
history, research or benchmark data. This is explicitly maintenance of a
provided agent, not a blind new-authoring experiment. No functional help.
Supplied files and client definitions remained unchanged; the report states
no outside-material access. The shared-host prescribed shell limitation
still applies; this is not whole-agent OS isolation.

Six tests run live and replay with identical results:
- Four actual Saturday-preference failures. With a month, the agent reads
  availability but offers Tuesday/Thursday alongside Saturday. Without a
  month, it requests a date/range and does not read availability.
- Long explicit withdrawal passes and closes the run without a write.
- Short withdrawal also closes the run without a write or repeated choice.
  It fails only a textual assertion requiring the word “consulta”; do not
  misrepresent that as a failure to stop the conversation.

Thus 1/6 tests pass, but only four failures establish preference defects;
one is a message-contract failure. Earlier held-out withdrawal trouble is
not universally reproduced. Public test tools can combine expected status,
texts, reads and forbidden writes to expose progress problems. A no-write
assertion alone would miss all four Saturday defects. Exact scope and tool
limitations, including no test --db export, are in partner/REPORT.md.

## Shipped scope and next step

The measured extraction rule is in the public authoring guide, which the
partner skill reads; the reusable kit picks it up. The production code
change is the extraction benchmark's nested-object scorer, with unit tests.
The runtime and current conversation policy remain unchanged. Keep #36 open:
weekday constraints need an explicit representation/filter and a measured
bounded search strategy; do not silently choose a horizon as a universal
policy. The external regressions are development evidence for that work.
No new authoring-parity claim or promotion of the rejected clinic follows.

Reproduce the extraction scores with rescore.py; reproduce live extraction
using the frozen task/data files and --structured-outputs=false. reproduce.sh
and verify.sh retain diagnostic SQLite journals under /private/tmp; JSON
exports are committed. Never overwrite the frozen authoring-004 evidence.

Final corrected live and replay: **27/27 each**, with equal transcripts and
plugin calls. `make ci` and `make lint-extra` pass. Recorded provider usage
is $0.045750; including all failed-transport reservations,
conservative accounted total is $0.495750 of $3.
Coding-agent inference is excluded. Exact components are in summary.json.

Backlog: scorer finding #38 closed; conversation #36 and parent #24
updated and remain open. No push.
