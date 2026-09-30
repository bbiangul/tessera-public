# Free mode 005: mixed new-service requests (#35)

2026-09-24, base 1d07796. Measure before changing production.
The seed-37 transcript repeats confirmation instead of answering a new
cleaning request. First measure the actual interruption classifier; do
not assume this alone is the cause of every unanswered request.

## Alternatives and frozen selection

A: current Jev interruption request and 0.6 winning-class threshold.
B: same request plus explicit priority for an additional service request,
even with yes, while preserving same-process corrections as answers.
C: unchanged request; combine question/request probabilities because both
already run the identical free-answer/reask handler. If their sum is at
least 0.6, choose question; otherwise retain A's behavior. This must not
combine permission with interruption probabilities.

24 prewritten synthetic dev cases in clinic, hotel, shipping and payments;
12 prewritten unrun replication cases. Labels describe handler behavior:
answer, side (question OR request), cancel, goodbye. Three repeats per
case. Record full requests, probabilities, errors, latency and cost. A/C
share exact model responses, so only A and B call Jev. Shuffle jobs with a
fixed seed and alternate A/B order. Each partition has a $0.10 hard ledger
budget; use pinned jev-1.13.0. No new text model.

Eligibility: 100% on side requests; zero side requests delivered as answers;
at least 97% overall handler accuracy, 100% on same-process corrections,
plain yes/no and cancellation controls; no model errors. Select eligible
highest accuracy, tie within 1 percentage point by fewer extra calls,
then smaller prompt, then simpler implementation. If baseline does not
reproduce a side-handling failure, do not attribute the transcript to
classification; investigate the free-answer guard separately. Only the
winner advances unchanged to replication, with the same gates. No tuning
on replication. Failed candidates stay in the report.

## Full-flow promotion gates

Only implement a replicated winner. Script a new-service question before
confirmation and a yes-plus-service question; require clinic phone on each,
no effects until a subsequent unqualified yes and exactly one reschedule.
Retain #33 corrections/refusals and #34 wording tests. Verify live/replay,
unit tests with race, make ci and make lint-extra. Refresh affected cassettes.
Then used market seed 24 C0-C3 (240 runs): C0 >=97%, zero duplicate,
unconfirmed or foreign effects. Fresh off-script seed 38: >=95% judged
success, zero false claims, <=5% dead ends; manually retain/reject flags
with reasons, preserve raw judgments. No tuning on 38; unrelated findings
receive issues. No blanket conversation or naturalness superiority claim.

Overall cap $3: classifier selection/replication $0.10 each, targeted live
and refresh $0.30, market $0.40, off-script $0.40, judge $0.60, reserve $1.10.
Results under bench/results/freemode-005; datasets bench/interruption.
Freeze inputs and experimental code before calls; preserve base+patch.

## Registered development revision D, after initial selection

A 63/72, B 71/72, C 66/72. None eligible. The baseline reproduces the
new-service question with probabilities split between question/request;
it also classifies mixed yes-plus-service messages as answer. B removes
those answer errors but one side message remains split (0.59 request,
0.11 question). C only fixes splitting, not incorrect answer preference.
Preserve the failed first round and original harness. D combines B's
unchanged instruction with C's unchanged same-handler probability sum.
Rerun development with A/B/C/D (A/C paired, B/D paired); no prompt, label,
threshold or dataset changes. D must meet the original gates, then
replicate A/D on the still-unrun original replication set. Development
is reused openly, not called a fresh test. Allocate $0.10 from reserve
for the added round. No production changes before replication.
