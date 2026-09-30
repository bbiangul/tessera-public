# Authoring 004: two source-free partner authors

2026-09-24. Protocol: [pre-registration](authoring-004-preregistration.md).
Base 4927065. Exact kits, deliveries, transcripts and raw judgments are in
`bench/results/authoring-004/`. This is a two-author descriptive pilot, not
a causal estimate: one independently generated agent per guidance arm.

## Delivery boundary and method

Both new subagents started with `fork_turns=none`, with the session default
model (actual model not exposed). They received only a compiled CLI, skill,
product guides, generic non-clinic examples, a client brief and OpenAPI.
The candidate additionally received a conversation checklist: business-facing
identification, readable messages, preference/range discovery and transcript
review. No reference solution, previous author, internal source or held-out
feedback was supplied. No functional intervention in either delivery.

Shell profiles denied repository/.codex reads and the other/previous partner
kits. Probes verified denial of source reads; neither author reported access
outside the kit, and neither altered supplied files. This is a prescribed
shell workflow on a shared filesystem, NOT a VM or a comprehensive agent
permission boundary. Both initially needed elevated execution for nested
sandbox-exec, retained the same profile, and reported that infrastructure
friction. No implementation coaching was given.

The distributed binary reports f1e5f56; evaluators build 4927065. Those
commits have identical internal/, api/ and cmd/ trees; intervening changes
are documentation/evidence only. Both kits have the same binary hash.
Deliveries were frozen before external evaluation and were never repaired.

## Author effort

| Metric | Current guidance | Additional conversation guidance |
| --- | --- | --- |
| Functional interventions | 0 | 0 |
| Minutes from first CLI to freeze (upper bound) | 7.4 | 20.5 |
| CLI invocations | 33 | 43 |
| Live reservations | 3 | 9 (one rejected before execution) |
| Own final live/replay cases | 19/19, 19/19 | 21/21, 21/21 |
| Actual author test-provider spend | $0.010469 | $0.045400 |

Both validate without errors/warnings and replay from the frozen copy.
The candidate independently caught and fixed more issues in its own suite,
including lost preferences across messages and unrequested alternative hours.
This extra work cannot be attributed causally to guidance with one author.
Cost excludes coding-agent inference. Candidate reused `live-final.json` for
three invocations and preserved prior copies as live-03/live-04; effort.py
uses those copies, avoiding double-counting the final report. The rejected
Starlark admission has a log and no model execution/report.

## Documentation findings

Common technical notes supplied both authors cover calendar helper signatures,
reports/cassettes outside resource trees, successful-call effect counts and
scenario-scoped recordings. Neither reported the previous formatDate/output
placement/effect-count confusion. Both still needed validation to discover
that verify.with is local to the plugin and the allowed human task kinds.
Both mention the unavailable inline clinic example in testing.md.

A reusable allowlisted packager now lives in `bench/authoring/build_kit.py`.
It inventories hashes, converts unavailable Markdown guide links into clearly
unavailable background citations, and supports explicit denial of other kits.
It is not a general distribution/security product or complete dependency
closure checker. The original experiment kits and hashes remain unchanged.

## External results

Market seed42, 240 runs per authored agent; 60 C0 reference runs. Offscript
seed40, 60 runs per authored agent. Pairwise judge sees 60 matched task/repeat
pairs per arm in both orders against the same current reference. No source
or authored definition was changed during evaluation.

| Gate/metric | Current guidance | Additional guidance |
| --- | --- | --- |
| Market C0 | 60/60 | 60/60 |
| Fired C1 / C2 / C3 | 48/48 each | 48/48 each |
| All market runs | 240/240 | 240/240 |
| Duplicate / unconfirmed / foreign effects | 0 / 0 / 0 | 0 / 0 / 0 |
| Offscript state + judged success | 60/60 | 58/60 |
| Raw offscript false claims / dead ends | 0 / 0 | 0 / 1 |
| Mean humane rating (1–5) | 4.433 | 4.417 |

Reference C0 is 60/60. Naturalness shares below count only consistent wins
and losses, not ties or order disagreements:

| Metric | Control wins / losses / ties / disagreements | Share | Candidate wins / losses / ties / disagreements | Share |
| --- | --- | --- | --- | --- |
| Natural | 2 / 56 / 0 / 2 | 3.45% | 0 / 34 / 0 / 26 | 0% |
| Clear | 0 / 7 / 29 / 24 | 0% | 2 / 10 / 15 / 33 | 16.67% |
| Efficient (descriptive) | 0 / 4 / 41 / 15 | 0% | 3 / 2 / 40 / 15 | 60% |

Neither arm reaches the precommitted 45% naturalness/clarity floors. The
candidate does not improve the naturalness share over control. Its apparent
efficiency gain rests on only five decided pairs; its clarity share on twelve.
The 26 naturalness and 33 clarity order disagreements further limit inference.
This is not evidence of equivalence, nor a causal claim that all additional
guidance worsens authoring. It is sufficient to reject this candidate under
the precommitted promotion rule.

## What the transcripts show

Control still exposes opaque appointment/professional IDs, offers dates
outside a requested week (t13), and repeats weekday options to a Saturday-only
request (t14). Candidate removes opaque IDs in ordinary confirmations and
correctly restricts next-week dates in t13, but repeats its question after a
polite withdrawal; “any Saturday” causes repeated requests for a date/range.
All these can pass state-only scoring when the expected outcome is no write.
The additional guide was not sufficient to eliminate conversation dead ends.

Candidate offscript failures are t04/repeats 1 and 3: it claims no requested
availability for Thursday October 8 at 17h although slot D1-1008-17 exists.
No write occurs. Both negative success judgments are retained. Raw judge
false_claim is false; the offscript judge sees FAQ facts/effects, not the slot
inventory. Manual review records two availability contradictions separately.
The underlying extraction/filter failure is not established from the retained
transcripts; do not label this a platform runtime bug without reproducing it.

One raw naturalness factual flag per arm concerns ignored preferences/
withdrawal rather than invented dates (control t14/r2/TA, candidate t13/r1/TB).
Manual review confirms the listed slots exist. Raw flags and all preferences
remain unchanged; see each arm's review.md and flag files. No post-hoc score
repair or treatment tuning occurred.

## Decision and implementation

**Do not promote the candidate checklist or authored clinic.** Retain the
current skill and runtime. #36 remains open: safe execution and a green
self-authored suite still do not establish reference-level conversation.
Next, reproduce the false-unavailability case with journal evidence and
measure date/range extraction and actual conversational progress, including
weekday-only preferences, withdrawal while choosing and alternative requests.
Use new authors and fresh sets (market43+, offscript41+) for any later
held-out promotion decision; these sets are now development evidence.

Retain the independently verified technical kit work for #37: reusable
allowlist builder, public calendar signatures, output separation and effect
semantics, explicit unavailable citations and CLI/budget contract. After all
measurements, the guide's missing clinic example was replaced with the
included travel EvalSuite, Expr delimiters were clarified, and verify.with's
API comment/schema/explain output now specifies a local action name. These
small documentation fixes were NOT present in the frozen author kits and do
not retroactively change their results. Required public material and remaining
Markdown links were checked; inline internal references are background only.
A complete adversarial/portable distribution remains outside this experiment.

Total test-provider spend: **$1.563669 of $8**, including author test calls;
coding-agent inference excluded. Spend/effort scripts recompute from preserved
reports (evaluator components rounded to four decimals). Both frozen deliveries
validate and replay; make ci and make lint-extra are the final repository gates.
No push. Historical evidence and unrelated local databases/binaries untouched.

Final validation passed: `make schema`, `make ci`, `make lint-extra`, frozen
validate/replay for both deliveries, and the final kit smoke/link/hash checks.
Backlog: #37 closed; #36 and parent #24 updated and remain open.
