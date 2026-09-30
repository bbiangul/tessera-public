# Ambiguity 001 — clarify instead of guessing weekday scope

2026-09-25, base 814dd34. Compare unchanged eight-question baseline against two
same-request guard candidates: A yes/no ambiguity question, B choice between
clear/ambiguous weekday scope. No LLM decisions, Jev 1.13.0, .7 confidence.
A guard permits the existing mask only when confidently clear; otherwise
returns clarification. Independent questions stay in one batch.

Freeze development and separate replication messages before calls. Each set:
12 messages, 4 intentionally ambiguous, 8 clear (including absent restrictions,
numeric dates, old appointment and FAQ mentions). Run each development arm
2 repeats. Gate: 100% ambiguous clarification, >=95% clear exact masks, zero
unsafe accepted masks, zero errors. Pick passing arm by fewer unneeded
clarifications, then lower median latency. Evaluate selected arm and baseline
on replication, two repeats, same gates. If none pass, stop; no adaptive variant
or rewriting the oracle. Full messages/distributions and failures retained.

Ambiguity is an explicit product policy here: bare colloquial 'durante a semana'
without named days or 'dias úteis' does not authorize a guessed weekend scope;
ask to clarify. Contradictory availability also needs clarification. This is
not a claim of linguistic ground truth. Explicit weekday ranges and exclusions
have exact masks. No production behavior changes unless replication passes;
then any integration needs live/replayed conversation evidence separately.

Budget $0.10 per arm/set, at most five runs = $0.50; metered, no text/judge calls.
No conversation-quality or universal engine-equivalence claim.

## Integration extension, after successful replication, before integration calls

A passed development and untouched replication (24/24 each). Freeze A unchanged.
Integrate it in both public weekday helpers, mapping anything except confidently
clear to known=false. Add four helper conversation scenarios (vague, contradictory,
explicit range and numeric date) plus keep all four old helper and 53 clinic cases.
Record live/replay separately; promotion requires all these cases passing, no
unsafe effects. Integration caps: helper $0.10, clinic $0.30; measured classifier
spend $0.008949696 plus reserved integration $0.40 stays below total $0.50.
This extends the original five classifier runs; no classifier prompt/oracle tuning.
