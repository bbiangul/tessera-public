# Preference 001: preserve preferences across answered side questions

Base 88252a0. Synthetic clinic data only. User explicitly authorized Jev and
OpenRouter/DeepSeek calls under $3. All writes use EvalSuite sandboxes.
Artifacts and frozen datasets: `bench/results/preference-001/`. Decisions
remain Jev1.13.0; text generation is DeepSeek V4.1 Flash.

## Diagnosis

Parent baseline: 7/8 live/replay. “Qualquer sábado serve, mas tem
estacionamento?” followed by “Pode buscar então” loses the Saturday
preference. Repeating just this case with consistent Mon-Sat hours still
fails; fixture contradiction is not the cause of preference loss. Original
fixtures claim Mon-Fri hours while supplying Saturday slots: retain this
confound in historical evidence. The accepted raw wait reply previously
contained only the acknowledgment; the answered mixed message was absent.

A fresh partner with no conversation history received only public CLI,
skills/docs and client resources. Its baseline8 diagnostics: 6/8 mechanical,
7/8 semantic. Mixed preference+parking advanced without answering parking;
withdrawal was semantically correct but missed an exact substring. The
partner authored and froze its own scenarios and never accessed source.
The local shell sandbox denied source/history directories; this is an
access protocol on a shared host, not whole-agent VM isolation. Initial
approval denials and empty-cassette failures remain in partner-blocked;
completed baseline and maintenance evidence is in partner-final.

## Classifier selection

Three repeats per message, shuffled seed5005 and concurrency6. Each new
replication set was frozen before calls. Failed replication becomes
**development**, never reused as a fresh confirmation. Gate: >=95%, no
current factual question accepted as an answer, no pure correction sent to
side handling. Raw requests and probabilities are retained.

| Variant | Development | Fresh replication | Interpretation |
|---|---:|---:|---|
| A existing |30/36|—|Baseline|
| E mixed instruction |34/36|—|Ambiguous acknowledgments|
| F independent information Noul |36/36|33/36|Referential continuation fails|
| G earlier answered side context |72/72|31/36|Old FAQ contaminates classification|
| H extra continuation Noul |102/108|26/36|Fails; replication mistakenly run before gate review|
| I current-message information only |141/144|34/36|Withdrawal confidence splits|
| J I plus continuation question |141/144|—|Same quality, higher cost|
| K I plus stop-current Noul |173/180|36/36|Selected classifier|

K asks reply type, current factual information, and current-process
withdrawal **in one Jev request**. Stop>=.8 overrides to cancel; otherwise
information>=.8 goes to side handling; otherwise existing .6 choice policy.
No extra serial classifier, no threshold reduction, no LLM decision path.
The three questions have no dependencies on one another. Unit tests compare
the production question map exactly to a frozen K request.

K development p50 270ms/p95 381ms; fresh replication p50 286ms/p95 796ms,
cost $0.001530018 for36. This is observed batch latency, **not** a paired
speedup measurement against serial execution. The previous weekday-001
serial/batch measurement remains separate evidence. The 36 trials are
12 distinct messages repeated3 times, not36 independently authored cases.
K's seven development misses remain: one ambiguous acknowledgment, three
other-person appointment cancellations classified as ending this process,
and three goodbyes classified as cancellation (same handler but wrong gold).
These are limits, not evidence of universal conversation correctness.

Deviations: H replication ran despite development missing its gate; it is
unpromoted diagnostic evidence. The first K harness constructed an invalid
question kind; all180 rows failed locally, with zero provider tokens and
$0.099 conservative ledger reservations. They are not model accuracy data.
The harness now validates before metering. An interrupted setup run has no
completed outcome; its possible spend is conservatively reserved separately.

## Integrated candidates

The opt-in wait journals side user messages after a side response is
produced, without resuming, extending the deadline or granting consent.
Context is chronological, per wait/caller/generation, bounded at8 messages
and32KiB, explicitly consumed by the client Function. Only opted-in reply
waits use K. Confirmations and ordinary waits keep existing behavior.

A:7/8 development, mixed fixed but repeated FAQ made Sunday uncertain in
the weekday extractor. B explicitly explains chronological preferences,
latest corrections and already-answered facts to the existing eight-question
weekday batch:8/8 development,3/4 reserved conversation replication. The
new twice-undecided case ends the single-wait resolver prematurely. A
separate baseline replication also fails this case (3/4); B did not create
that underlying single-wait limitation. B inherited37/37 live.

Same source-free partner maintenance validation of B:9/10 mechanical,
10/10 manually assessed for the intended preference/withdrawal behaviors.
It proves Saturday survives an acknowledgment without repetition, and a
later Tuesday correction replaces it. The unchanged withdrawal substring
still fails. Contradictory hours still produce one conflicting spoken
Saturday claim in that fixture; do not call the entire interaction flawless.
This is maintenance by the same partner, not another blind author.

C retains K and B weekday questions and allows up to three preference
replies in the declarative Function. It reasks when extraction has neither
a confident weekday restriction nor an explicit date; accumulated messages
remain chronological. Twelve known conversations pass live. Inherited37 pass both with unchanged
fixtures and with corrected Mon-Sat hours. Fresh4 are2/4 mechanically: two
oracle mistakes expected WaitingForUser/ISO dates despite one available
Friday slot correctly entering Confirming with localized16/10 às14h. Original
test and verdict remain immutable. A corrected copy asserts Confirming,
16/10,14h and no effect; it passes4/4 on the exact original replay and4/4
on a new live regression. This is transparent oracle correction, not four
untouched automated passes. No behavior was changed after those four calls.

Promote C as the scoped example:53 scenarios (37+12+4) with consistent hours
and corrected explicit expectations. The combined cassette is part of CI.
The source-free partner validated B's retention/correction; the additional
C retry behavior was validated by the parent, not a new blind partner.
Adaptive multi-round selection remains exploratory; these small sets do not
establish a population success rate or naturalness parity.

## Budget and checks

Recorded ledgers/results plus conservative reservations total **$0.385460**,
well below $3. This includes a $0.099 invalid-harness reservation with zero
provider tokens and the entire $0.15 cap for an interrupted setup whose
actual spend is unknown. Recorded successful/model-call spend excluding
these reserves is $0.136460; coding-agent inference is outside this budget.
Partner baseline+maintenance cost $0.012734701 is included exactly once.
`final-summary.json` and `summarize.py` reproduce the accounting; replay
metadata is not additional spend.

Thirty packages pass unit tests with race detection. The static analyzers
pass, including no reported vulnerabilities. `make ci` passes:76 resources validated with zero warnings,30 unit-test
packages and102 replay scenarios (53 in the updated clinic). `make lint-extra`
also passes; logs and checks.json are retained with the evidence. Runtime tests cover exact caller,
stale wait, unchanged deadline, reconstruction from the journal, boundaries,
and no inheritance into the next wait. Host tests cover one request/three
questions, masking, provider errors, and exact frozen-K question parity.
Admission rejects includeSideMessages without orReply. Remaining work is
broader-domain classifier measurement and fresh source-free authoring on
unused market43/offscript41; #36 and #24 remain open.

## Reproduction

Build with `make build`. Classifier harness (integration tag, skips without
keys and explicit dataset/output):

```
PREFERENCE_DATA=bench/results/preference-001/classifier-k-replication.json \
PREFERENCE_OUT=/tmp/preference-k PREFERENCE_VARIANTS=K \
go test -tags integration ./internal/host -run '^TestPreferenceExperiment$' -count=1
```

Conversation evidence can be replayed without providers, e.g.:

```
./bin/tessera test bench/results/preference-001/candidate-c \
  --generate openrouter:deepseek/deepseek-v4.1-flash \
  --cassette bench/results/preference-001/candidate-c-development.jsonl \
  --cassette-mode replay
```

For a new live measurement use a new cassette/result path, record mode and
an explicit budget. Never overwrite failed artifacts. No market43 or
offscript41 data was used in selection or promotion.

Backlog update: https://github.com/bbiangul/tessera/issues/36#issuecomment-5831975134
