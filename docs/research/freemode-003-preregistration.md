# Free mode 003: answer from authorized in-process reads (#31)

- **Date:** 2026-09-24, before live model calls.
- **Baseline:** `ba72306`; voice-006 B/t13/repeat 2 asked for times
  across days and received “não consigo ver por aqui” even though the
  Function had already read those slots.

## Intervention and scope

Add opt-in `Agent.spec.freeMode.runReads`: allowlisted read-only action
names. During a side question, use only recorded successful top-level
input `fromAction` results of the pending Function. Check the same
namespace, conversation, caller and definition generation; mask every
shared result, bound total JSON to 32 KiB, and never execute a read to
construct context. Other inputs, arguments, steps, callees and finished
runs are not shared. State that these are snapshots, not fresh reads or
proof of an effect. Keep the voice-005 warm instructions unchanged.

The deterministic host regression fails before connecting this context:
it returns the missing-times answer. Unit tests cover opt-in/default
privacy, identity isolation, masking, limits and confirmation/replay.

## Development and measured tests

1. **Targeted development EvalSuite:** `examples/clinic-context`, three
   scripted conversations: cross-day times, a question at confirmation
   followed by refusal, and a question followed by an act-then-timeout.
   Run once with runReads removed as control, then with it enabled.
   Record separate cassettes and transcripts. Allow at most three live
   candidate runs for development, documenting any failed runs/changes.
   The final candidate must pass live and replayed and join `make ci`.
2. **Fresh regression set:** off-script seed 34, generated with the
   unchanged `gen_offscript.py`, 20 tasks × 3 repeats, Tessera C0 only.
   Judge with the existing off-script judge. This tests general handling,
   not the targeted slot-context issue; the EvalSuite tests that directly.
3. **Fault regression:** market seed 24, C0–C3, three repeats, using
   `bench/market/tessera-context` (warm + `runReads: [clinic/find-slots]`).
   Report success, duplicate/unconfirmed/foreign moves and latency.

Freeze code and definitions before the fresh set. Do not tune on seed
34 or rerun it as a new holdout. This is not a market-superiority claim;
no new competitor or naturalness comparison is planned.

## Hypotheses

| ID | Pass condition |
| --- | --- |
| R1 | Cross-day times fail their answer checks without context and pass with context; all 3 candidate scenarios pass live and replayed |
| R2 | Targeted scenarios read slots once, do not act on a side question or refusal, and recover the ambiguous write through verification |
| R3 | Fresh off-script success ≥90%, dead ends ≤5%, zero false-claim flags surviving manual review |
| R4 | C0 success ≥97%; zero duplicate, unconfirmed and foreign moves in every fired fault condition |
| R5 | Privacy and failure unit tests pass; no context when disabled or from another identity, generation, nested call or unsuccessful read |

If R1/R2/R4/R5 fail, do not call #31 fixed. If R3 fails, report the
failures, determine whether they are attributable to the intervention,
and do not claim general off-script non-regression. An unrelated finding
must stay visible rather than be excluded from the reported score.

## Budget and artifacts

Maximum $3: four targeted eval runs capped at $0.10 each (one control,
up to three candidate), market $0.50, off-script $0.50, judge $0.80;
$0.80 remains for documented recovery from infrastructure failures only.
No new dependency installations. The parallel judge's in-flight budget
limitation is unchanged and any overrun must be reported.

Results: `bench/results/freemode-003/{control,targeted,offscript,market}`;
candidate cassette: `bench/cassettes/clinic-context-evals.jsonl`.
Report: `docs/research/freemode-003.md`, including failures, exact code
snapshot, actual spend and any deviations. Per-run databases stay out
of Git. API field docs, authoring guide, handover and #31 update with
the implementation and evidence.

### Development deviation, before the fresh-set run

The control failed all three answer checks; the first live candidate
passed all three. Replay of the combined candidate cassette failed the
third: two scenarios share an identical opening request but received
different voice responses. ResetReplay restarts at the first response,
so the third scenario follows the second scenario's opening and misses
later recordings. Preserve that combined cassette and report the defect.
Record each unchanged scenario to a separate cassette with `--only`;
their aggregate budget is one additional candidate run ($0.10). CI uses
the same isolation. This changes recording organization, not the
Function, expectations, prompts or the fresh-set experiment.
