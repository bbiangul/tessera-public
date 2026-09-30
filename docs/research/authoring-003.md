# Authoring 003: a software-house partner without platform sources

2026-09-24, runtime base `f1e5f56`.
[Preregistration](authoring-003-preregistration.md).
The owner explicitly required an external-partner context, not an agent
with repository access or our previous conversation.

## What the author received

A fresh subagent was started with `fork_turns=none`, no model override,
and a single partner assignment. It received a separate kit directory:
compiled CLI, existing authoring skill, three product guides, generic
travel/franchise/outreach examples, synthetic client brief and public
clinic API contract. No engine source, .git, research results, hand-written
clinic, previous authored clinic, test tasks or evaluation scores were
included. Generic resource examples are documentation, not platform source.
The synthetic client's effect classes were specified in the initial brief.

`kit/AGENTS.md`, `kit/BRIEF.md`, `kit/manifest.json` and the launcher/profile
preserve the exact delivery contract and initial file hashes. The platform
binary is identified by hash and version, not committed as a large binary;
it can be built from the recorded source revision by the evaluator.
Some documentation links refer to research/source outside the kit. The
author is told these are unavailable citations, not permission to browse
the repository; reported gaps remain part of the experiment.

## Isolation, precisely

The collaboration service shares a filesystem: this is not a separate VM
and not a global technical permission boundary around all possible tools.
The assignment forbids outside-kit access and tools other than sandboxed
terminal commands. Its sandbox-exec profile denies repository source reads
and writes, and .codex reads. A source-file probe gets Operation not
permitted while the distributed CLI works. This protects the prescribed
shell workflow, not against an agent deliberately bypassing that workflow.
The launcher supplies provider credentials without exposing them, logs CLI
commands and reserves at most twenty $0.05 live test calls. It is not a
security boundary against a malicious agent; bypass is prohibited and
would invalidate the intended partner simulation.

The parent prepares evaluation independently and does not provide authoring
suggestions or evaluation feedback. The author must freeze delivery before
seeing evaluator results. Record actual compliance and any interventions;
never upgrade this protocol into a claim of OS-level isolation of every
tool. The parent's initial launcher `version` probe is setup, not author effort.

## Evaluation design

Fresh market seed 41 remains hidden from the author. The current
hand-written reference runs C0 on the same tasks/runtime as the eventual
frozen authored candidate. Paired judgment is blind, both orders, using the
existing judge. It is a direct comparison to the current reference, not a
reuse of historic 55% preference against different competitor transcripts.
Fresh off-script seed 39 separately tests answer coverage and false claims.
The author sees neither dataset, reference definitions nor scores.

## Frozen author delivery

The subagent delivered four resources and **19/19 passing live and replay
scenarios**, with zero validation errors/warnings. Its report states no
outside-kit access, binary inspection or launcher/profile edits; all
initial supplied-file hashes match. This compliance statement is not a
complete OS audit of every tool. No functional question or implementation
intervention was sent to the author, and no evaluator feedback was provided.
An initial nested-sandbox error was resolved by the author using the already
specified escalated sandbox-exec invocation, not by dropping the profile.

There are 33 logged author CLI invocations: 23 explain, four validate and
six test (including help). Four test invocations reserved live budget;
one failed admission before model calls. The author CLI window spans 9.8
minutes; initial reading/report writing fall outside that window. A
prelaunch probe and post-delivery snapshot bound total time (see effort.json),
so do not label the CLI window exact total authoring time. Actual author
API spend is $0.00879147; $0.20 was reserved, below the $1 author cap.
These costs exclude the coding agent's own inference/account usage.

Failures were preserved: an incorrect probe action reference/date expression,
a result JSON saved inside the resource tree, and an ambiguous-effect test
whose count assumed successful responses rather than raw attempts. The
author moved results out of definitions and supplemented the timeout case
with a checker that verifies exactly one attempted write and a later probe.
It expanded 13 scenarios (12 initially passing) to 19 final passing cases.
No original failed evidence was relabeled a success.

The delivery is copied unchanged into `defs/`; `delivery.sha256.json`
freezes it. `author-report.md`, `author-evidence/`, `author-usage/` preserve
the author's account and attempts. The evaluator validates those exact
resources; it does not repair them to improve external scores.

## External fault benchmark: execution gate passes

New seed 41: **240/240 authored runs reach the expected state**, including
60/60 C0 and all 48 fired cases in each of C1/C2/C3. No duplicate,
unconfirmed or foreign effects; no reported run errors. The reference also
passes 60/60 C0 on these tasks. Both authored-state thresholds pass.
Authored ledger spend $0.1209; p50/p95 C0 turns 2224/3316 ms, C1
2893/21930 ms, C2 2016/3062 ms, C3 1742/2969 ms. Reference C0 is
2306/3734 ms. These timings are observations, not a speed superiority test.

A state-success metric can miss an unhelpful answer. For t13, the task's
expected final state is no write: the patient asks for options and decides
not to reschedule. The author requires a day before reading slots, so its
response cannot offer available dates for a broad next-week request and
asks for a preference again. Correct no-write state does not establish
that the patient's information need was met.

## Direct conversation comparison: quality gate fails

60 matched C0 task/repeat pairs, each judged in both orders, authored vs
current hand-written reference. Only opposite, consistent preferences
count as a decided pair. The existing judge's wording and scoring are
unchanged; no names of the authoring condition are supplied in its prompt.

| Metric | Author wins | Reference wins | Ties | Order disagreements | Author share of decided |
| --- | --- | --- | --- | --- | --- |
| Naturalness | 1 | 53 | 0 | 6 | **1.85%** |
| Clarity | 1 | 9 | 31 | 19 | **10%** |
| Efficiency (descriptive) | 2 | 10 | 33 | 15 | 16.67% |

Both preregistered >=45% quality thresholds **fail**. Clarity has only ten
decided pairs; its ties/disagreements and repeated tasks limit inference.
This is one author, one synthetic domain, one text model and one judge,
not a statistical equivalence study or a general rate for software houses.
Historical percentages against other LLM stacks are not comparable.

The qualitative reasons favor the reference's warmth and formatting and
criticize the author's technical identifiers. Exploratory overlapping
keyword counts among 53 naturalness losses: warmth 44, identifiers/technical
wording 27, formatting 16, emoji 4. This is descriptive, not a causal
ablation of persona, examples, content or fact-lock fallback behavior.
Both agents use the current warm runtime; the difference is their authored
definitions. The frozen candidate will not be edited to repair this score.

One raw factual flag concerns the reference, t13 repeat 2 in the reversed
order: the judge suspects invented availability because it sees final
write effects, not read results. All offered dates occur in that task's
slots; manual review rejects the suspected invention. The reference does
offer a broader range than the requested next week, which remains a
clarity limitation. Raw flags and every preference verdict stay unchanged
in `naturalness/`; no preference score is excluded. Judge spend $0.4118.

## Fresh off-script seed 39 and final gates

The frozen author delivery has **60/60 correct states and 60/60 judged
successful conversations**, zero false claims, zero dead ends, all questions
answered by the existing judge. Mean humane score 4.533/5; turn p50/p95
1800/3269 ms. No unsafe-effect counters or run errors. Run spend $0.0293;
judge $0.1746. There are no negative judgments to exclude or overturn.
This passes the off-script gate on that synthetic set; it does not undo
the failed matched conversation-quality comparison on seed 41.

| Registered gate | Verdict |
| --- | --- |
| One brief, own validate/live/replay green, zero functional interventions, <=60 min | Pass: 19/19, four valid resources, zero interventions; elapsed upper bound 12 min |
| Execution under normal/fault conditions | Pass: 240/240, all safety counters zero |
| Naturalness and clarity >=45% of decided pairs vs current reference | **Fail: 1.85% natural, 10% clear** |
| Fresh off-script >=95%, zero false claims, <=5% dead ends | Pass: 60/60, zero/zero |
| External-partner access protocol | No supplied-file edits or reported outside access; shell source denial verified; **not whole-agent OS isolation** |

**Do not claim that the current kit produces manual-level conversation
quality from one brief.** It enabled autonomous construction and the
measured execution safeguards; quality remains unresolved. The frozen
artifact and failed score are retained, not repaired by the evaluator.
Follow-up [#36](https://github.com/bbiangul/tessera/issues/36) covers measured
conversation quality and broad availability discovery;
[#37](https://github.com/bbiangul/tessera/issues/37) covers partner packaging
and documentation. Improve and compare public guidance with new authors,
not with evaluator feedback to this author. Next fresh market seed 42;
next off-script seed 40. Neither used seed is fresh again.

## Documentation findings and post-measurement change

The author reports missing expression-function signatures, ambiguous effect
count semantics and the result-JSON/resource-directory trap. Fourteen guide
links refer to excluded research, ADR or source files; some are optional
evidence citations, not essential authoring instructions. The exact kit is
preserved so packaging friction is visible, not retroactively removed.

After **all measurements finished**, the main testing guide's stale #32
workaround paragraph was corrected to distinguish legacy files from new
scoped recordings. The delivered guide snapshot, skill, candidate and scores
remain unchanged. No runtime or skill improvement was smuggled into this
round; further source-free documentation work is #37.

Independent replay of the delivered 19 scenarios and its raw-effect checker
passes. `make ci` passes (race tests, 57 example resources, existing 45 replay
scenarios) and `make lint-extra` passes with no reported vulnerabilities.
The authored experimental resources remain under results, not installed as
a production example or substituted for the reference. Logs are committed.

Total provider-call spend approximately **$0.7766 of $5**, including all
preserved author attempts, reference, fault/off-script runs and both judges.
This excludes coding-agent inference/account usage. Full-flow reporters
round to four decimals; author live costs retain ledger precision.
`python3 bench/results/authoring-003/summarize.py` recomputes scores and spend.
