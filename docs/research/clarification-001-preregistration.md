# Clarification 001 — durable recovery, not perfect first-turn classification

2026-09-25, base 5393b6d. Read-only weekday preference conversation, no bookings,
slots availability, dates/period extraction, external effects or text generation.
Compare P public eight-question helper, B E guard without durable recovery,
R same E guard plus one durable reply wait and one reclassification of a full
restatement of desired weekdays. Shared renderer reports actual allowed weekdays.
R asks: 'Quais dias da semana você pode? Liste todos os dias desejados.'
The complete restatement replaces weekday preferences only; no general claim
about preserving partial corrections or unrelated scheduling constraints.

Freeze 10 synthetic development dialogues before calls: four previously
abstaining phrasings, two ambiguous, two clear controls, a still-vague reply and
silence/timeout. Harness sends the scripted clarification only when the agent
actually asks, whether the arm is waiting or ended. At most one extra user turn.
For silence advance time, never model a reply. Record run ids, statuses,
transcript, actual permitted days, number of additional turns, model spend.
A valid direct result needs no reply. An unresolved case must show no allowed
set. Dead-end clarification is not a completion. Every clear result must match
its oracle exactly, with no forbidden-day offer at any turn.

Two repeats per arm, sequential arms; gates R: all resolvable dialogues complete
in <=1 extra turn, no unsafe offered set, unresolved/silence safe and bounded;
no extra turns on clear controls; same run across recovery. Replay must reproduce
semantics. If dev passes, compare R/P on a separate frozen six-dialogue set,
two repeats, same gates. Otherwise stop; no adaptive repair or promotion.
Model Jev1.13.0, .7 helper confidence; one batch per helper classification.
Budget .10 per arm/set, up to five runs=.50 total. Actual host/runtime and
scenario-scoped cassettes, metered calls, fixed clock, frozen resources/binary.
No public example or engine changes unless separate integration is justified.

Before calls: the recovery reclassification batch also asks whether the reply
actually supplies a weekday preference. 'Ainda não sei' must not become
unrestricted merely because it contains no restriction. This extra independent
yes/no question is in the same request (10 questions on recovery); only a
confident yes permits a final mask. Silence terminates after a 1h simulated wait.

## Packaging extension after successful replication, before packaging calls

R passes dev20/20 and replication12/12. Publish only a standalone read-only
weekday-recovery example (separate namespace), not a clinic/engine replacement.
Frozen packaging: six CLI EvalSuite scenarios for clear control, clear exclusion,
ambiguous-to-clear, contradiction-to-correction, still-unknown reply and silence.
Require all six live/replay, then add their cassette to CI. Cap .05 within total
.50 using settled prior spend. No prompt or threshold changes. Existing examples
stay intact. Routines demonstrate a full restatement, not partial additions.
