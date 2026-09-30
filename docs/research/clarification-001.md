# Clarification 001 — complete the conversation instead of forcing a first answer

2026-09-25, base 5393b6d. **R passes development and separate replication.**
Publish the read-only `examples/weekday-recovery` demonstration with six live/
replayed CLI scenarios in CI. The engine and clinic example are not changed.

## What was compared

P uses the public eight-question weekday helper. B uses the measured E guard,
but ends after asking for clarification. R uses the same E guard and asks for
one **complete restatement of desired weekdays**, suspends with `wait.orReply`,
and interprets the reply in the same run. A tenth independent question in the
reply batch checks whether the user actually supplied a preference; “I still
do not know” must never mean unrestricted. Unknown reply or simulated 1h timeout
ends without selecting days. All helper decisions stay batched, confidence .7.

The real host/runtime processes every message and timer. The benchmark renders
the weekday set, not the old demo's three-date calendar. It therefore compares
helper behavior under a shared conversational wrapper, not complete clinic
agents. Scripted users supply a full restatement only if asked; a directly correct
result needs no follow-up. At most one extra user turn, with no real effects.

Development: ten dialogues repeated twice (eight resolvable, one unknown reply,
one silence). Replication: six separately frozen dialogues repeated twice (four
resolvable, one unknown reply, one silence). These are small synthetic sets;
repeats are not additional distinct cases. No human/naturalness judge was used.

## Results

| Arm/set | All cases | Resolvable completed | Unsafe offered sets | Extra user turns | Replies continuing same run |
| --- | ---: | ---: | ---: | ---: | ---: |
| P/development | 16/20 | 14/16 | 2 | 6 | 0/6 |
| B/development | 18/20 | 16/16 | 0 | 11 | 0/11 |
| R/development | 20/20 | 16/16 | 0 | 10 | 10/10 |
| P/replication | 10/12 | 8/8 | 0 | 6 | 0/6 |
| R/replication | 12/12 | 8/8 | 0 | 6 | 6/6 |

R adds no turns to clear controls, and finishes each resolvable case with at
most one clarification reply. All eight unresolved/silence trials across its
two sets stop safely. P/B can recover clear restatements by starting a new run;
R is not claimed to uniquely enable those completions. Their unknown replies
instead fall through routing and elicit another generic question, failing the
registered bounded-stop contract. R retains the pending interaction and ends
without choosing days. P's development vague request also permits forbidden
weekend days before clarification. Its replication has no unsafe offered sets.

The R-versus-P comparison changes guard policy and recovery. B isolates the
recovery difference in development only; no claim that all differences are
caused solely by waiting. R's model/runtime latency medians were 710ms development
and 734.5ms replication; P 606ms/713.5ms. These exclude user thinking time and the
simulated timer duration. Sequential blocks and small samples do not support a
broad speed claim. R's replication costs more than P ($0.001831 vs $0.001374).

## Harness correction and replay

The initial harness tried to acknowledge the synthetic unrouted outbox as if
it belonged to a run. That produced `run not found: ` on the two repeated unknown
replies in P and B, and incorrectly labeled their lack of a new run as continuity.
The host had behaved normally. Fix: acknowledge only nonempty RunID, and mark an
unrouted follow-up as no run continuity. A planted unit regression covers this.

Original live JSONL, original binary/source hashes and cassettes remain intact.
Corrected offline replay removed only those erroneous error/continuity fields;
completion, safety, masks and transcripts were identical. `replay-differences.json`
records every change. Development summaries use corrected replay semantics with
original live cost/latency; no paid reruns or changed prompts. The fixed harness
was frozen before replication. Replication's live/replay semantics match exactly
(excluding fresh run ids, elapsed time and replay's zero new spend).

All original resource/dataset hashes remain valid; only a separately recorded
packaging appendix was added to the protocol. No resource tree was changed while
an evaluation was using it.

## Published example and tests

After replication, copy R into a separate `weekday-recovery` namespace. Six CLI
EvalSuite scenarios pass live and replay: clear input, clear exclusion,
ambiguous-to-clear, contradiction-to-correction, still-unknown reply, silence.
The immutable packaging candidate matches the published resources. The cassette
is `bench/cassettes/weekday-recovery.jsonl`; `make ci` now runs its six cases.

A first packaging attempt named the expected wait state `WaitingForTime`;
admission rejected it before calls. Reply waits use `WaitingForUser`. The
corrected suite was frozen before live evaluation; the rejected log is retained.
No prompt/threshold changes were made during packaging.

Runtime behavior is composed from existing `respond`, `wait.orReply`, `call` and
`decideBatch`. Unit tests exercise same-run recovery, no run acknowledgment for
unrouted text, unknown reply and timeout, plus rejection of an earlier unsafe
offer even if a later result is correct. Final `make ci` and `make lint-extra`
pass. This does not establish process-crash recovery beyond the existing runtime's
durable-wait contract/tests; the new experiment itself does not inject crashes.

## Scope and cost

Full weekday restatement is the explicit contract. Partial additions (“e quarta?”),
side questions, date/time preferences and live appointment availability are not
covered. There are no writes or human handoffs; unresolved input simply stops.
The one-attempt limit and full-restatement policy belong to the author, not an
automatic engine optimization. Porting to the clinic requires its own frozen
regression study; no blanket conversation parity claim.

Comparison spend $0.011399766; packaging $0.000916062; **total $0.012315828/$0.50**.
All six live runs completed; replay has zero new spend. No Generate/judge calls.

Recompute with `python3 bench/results/clarification-001/summarize.py`.
Reproduce the public example using the commands in its README. The custom
benchmark runs with `go run ./bench/recoverybench -defs <frozen-dir> -dataset
<dataset.json> -out <new.jsonl> -cassette <new-cassette.jsonl> -max-cost .10`;
add `-mode replay` for offline verification. The sets are now consumed.
