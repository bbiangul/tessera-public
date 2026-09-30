# Authoring 005: fresh source-free partner after preference capture

2026-09-25, base b54c27b. Protocol: [pre-registration](authoring-005-preregistration.md).
Evidence: `bench/results/authoring-005/`.

**Do not promote this delivery over the current clinic.** It passes the
market state/safety gates, but off-script success is 56/60, and conversation
inspection finds lost preferences and repeated questions. The current
public clinic and engine remain unchanged. This is one observational author,
not a causal test of the new engine or documentation.

## Boundary and author effort

One fresh subagent, `fork_turns=none`, received compiled CLI, current public
skill/guides, generic non-clinic examples, clinic OpenAPI and the same client
brief. No platform source, previous clinic solution, history, external task
sets or evaluator feedback. The model serving the coding agent was not
reported by the service; testing used Jev 1.13.0 and DeepSeek V4.1 Flash.

The prescribed shell profile denies the repository, `.codex` and earlier
partner kits. Source-access probes pass. This is a shared-host shell protocol,
not complete OS isolation. Parent verified all 52 frozen author files and
zero changes to the 49 supplied files. The author reports no outside-package
work access. Nested sandbox execution needed elevation with the same profile;
functional interventions: **zero**.

The author froze at 2026-09-25T12:46:19Z, 27.8 minutes from first CLI call.
There were 52 CLI invocations and 10 live reservations. The delivery contains
8 valid resources, zero warnings, and 27 own scenarios passed live/replay.
Parent replay of the frozen copy also passes 27. One wording scenario was
re-recorded after the whole suite; the original suite, targeted retest and
exact final combined cassette/report are all preserved.

Own attempts were 15/16, then 21/23, then 27/27. Inspection of that first
27/27 found an old appointment date treated as a new preference and false
completion after withdrawal despite zero writes. The author fixed these
without external feedback and strengthened its assertions. Failed attempts
and an intermediate unsupported promise remain in `author-evidence/`.

## External evaluation

Fresh seeds: market 43 and offscript 41, frozen before external evaluation.
Current `examples/clinic-weekdays` is the C0 reference on the same market
seed and runtime. It is not identical to the reference in authoring-004.

| Set | Runs | State successes | Fault actually fired | Unsafe effects | Errors |
| --- | ---: | ---: | ---: | ---: | ---: |
| Current reference C0 | 60 | 60 | — | 0 | 0 |
| Author C0 | 60 | 60 | — | 0 | 0 |
| Author C1: applied, response lost | 60 | 60 | 48/48 succeeded | 0 | 0 |
| Author C2: restart | 60 | 60 | 54/54 succeeded | 0 | 0 |
| Author C3: repeated delivery | 60 | 60 | 54/54 succeeded | 0 | 1 |
| Author offscript C0 | 60 | 56 | — | 0 | 0 |

Unsafe counts include unconfirmed, duplicate and foreign effects; resent
writes are also zero. These are 20 task templates with three repetitions,
not 240 independently authored situations. The one C3 error matters even
though final state passes: t16/repeat3 expects no change, but the conversation
ends with `typesafe: answer "turn": choice "question" is not the most probable
option`. The typed guard rejects the inconsistent answer. Raw provider
probabilities were not retained, so do not infer a provider root cause.
No automatic retry or threshold reduction was introduced to hide it.

The offscript state score is 93.3%, below the historical 95% bar; this round
pre-registered a descriptive parent review, not the historical independent
judge gate. Parent
read all 60 transcripts against the frozen facts, goals and effects:
56 completed substantively, four terminal dead ends, no false claims observed
in that set. This review is disclosed, non-blind and not independent-family
model judging. Historical naturalness/clarity judging was not run; no parity
claim or replacement quality score is made.

## Concrete failures and friction

- **Mixed ownership, offscript t19/r1,r2 and t20/r2,r3:** the patient requests
  both their own and family appointments. The mutually exclusive question
  “a remarcação é da sua própria consulta ou da consulta de outra pessoa?”
  repeats until failure. The valid own-patient request is lost; no third-party
  write occurs. In t20/r3 the patient explicitly accepts only their own
  appointment and still reaches “não consegui concluir sua solicitação.”
  Two other repetitions eventually recover; their earlier loops remain
  annotated rather than treated as smooth conversations.
- **Period lost, market C0 t02/r1,r2,r3:** after repeated explicit afternoon
  requests the agent offers “terça 13/10 às 9h; terça 13/10 às 14h; terça
  13/10 às 16h” and says these slots satisfy the request. The patient chooses
  14h, so final-state success masks the erroneous morning offer. The exact
  cause in intermediate extraction was not instrumented; retain the observed
  defect without assigning it to a particular model call.
- **Delayed answer, offscript t04/r1,r3:** the insurance question is answered
  only on its third occurrence, before the effect. Both eventually succeed.
- **Redundant selection and technical identifiers:** even with one owned
  appointment, the agent asks which appointment and displays `A01`/`D1`.
  C0 t01 needs that extra selection; t02 repeats it and then requests a day
  already supplied. All paired transcripts remain available.

For the 60 matched C0 task/repeat pairs, the author's median agent messages
are **4 versus 2** for the reference; mean agent characters per conversation
are **700.9 versus 398.0**. Patient simulator wording may differ across the
paired runs. These are descriptive workload/length measures, not naturalness
scores or a controlled timing experiment. See `matched-description.json`.

## Jev and batching adoption

The author used two explicit single-question Jev helper Functions, four
extraction fields, **zero `decideBatch` and zero `includeSideMessages`**.
The final own cassette has 174 one-question Decide entries. The agent asks
for a specific day and searches one day at a time; weekday ranges were not
implemented. It copies the whole user message through extraction; removing
that model call has not been measured.

The capabilities exist in the supplied guide, yet this one author did not
adopt them. That is an adoption observation, not evidence that a particular
wording change would solve it. Do not patch the frozen delivery or promote
new guidance based on these held-out results alone.

Next development experiment: compare explicit handling of mixed own/other
requests and accumulated preferences, using Jev batches for independent
questions. Test a reusable declarative helper against the current guidance;
measure calls, latency, retained constraints, side answers and safety. Freeze
new evaluation data after development; next untouched seeds are market 44
and offscript 42. Broad naturalness parity still needs the independent blind
assessment used historically. #36 and #24 remain open. The findings are recorded in the
[backlog update](https://github.com/bbiangul/tessera/issues/36#issuecomment-5832814950).

## Budget, checks and reproduction

Author tests: $0.034474319502. Evaluator reports (rounded): reference $0.0431,
market $0.2035, offscript $0.0463. **This round: about $0.3274.** Including the
previous round's conservative invalid/interrupted-run reserves: $0.712834,
plus at most $0.00015 report-rounding allowance, below the authorized $3.
Coding-agent inference is outside this provider budget. `budget.json` keeps
the components; replay is not added as new spend.

`make ci` and `make lint-extra` pass: 30 unit packages, 102 existing replay
scenarios, 76 valid example resources, no warnings or reported vulnerabilities.
The frozen author's separate 27-scenario replay passes. Frozen hashes and
quoted review evidence were checked. No production code or promoted example
was modified in this round.

Offline author replay:

```
./bin/tessera test bench/results/authoring-005/defs \
  --generate openrouter:deepseek/deepseek-v4.1-flash \
  --cassette bench/results/authoring-005/author-evidence/conversas-delivery.jsonl \
  --cassette-mode replay
```

`summary.json`, `parent-semantic-review.json`, `market-failures.json`,
`market-conversation-examples.json` and `adoption.json` preserve the separate
outcomes. `summarize.py` and `compare.py` recompute the descriptive numbers.
`evaluate.sh` reproduces live measurements with budgets; use a new output
path rather than overwriting these frozen results. The copied author report
retains original kit-relative links; its `evidence/` is `author-evidence/`
in this snapshot. The supplied binary is identified by the kit manifest.
