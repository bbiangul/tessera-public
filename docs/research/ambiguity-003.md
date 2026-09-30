# Ambiguity 003 — clear exclusions expose two separate failure paths

2026-09-25, base 2ce368b. **No promotion.** Selected E passes development
52/52 but replication is 49/52, failing the registered all-clear-cases gate.
No integration calls or edits to public examples were made.

## Design

The development set includes all eight inherited helper conversation inputs,
prior vague-scope and clear-exclusion failures, named corrections, group
corrections, old-appointment/FAQ context and bounded-range exclusions.
13 base requests x2 phrasings x2 repeats =52 trials, only 26 distinct messages.
Replication freezes 13 different requests/wrappers with the same categories.
Three families per set require clarification by author policy; ten have exact
masks. Neither set represents a new business domain or linguistic ground truth.

C is unchanged. E prepends explicit semantics: “qualquer dia” means all seven
days; excluding named days from that set is clear; exclusion is not contradiction;
explicit corrections replace earlier preferences. F asks separate vague/conflict
questions with the same semantics. All arms keep the original eight mask
questions; C/E have 9 questions and F 10, in one Jev 1.13.0 request per trial.
Confidence .7 is unchanged. No LLM generation, regex exception or adaptive repair.

## Measurements

| Arm/set | Correct | Ambiguous clarified | Clear masks correct | Unneeded clarification | Median ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| C/development | 50/52 | 12/12 | 38/40 | 2 | 267.5 |
| E/development | 52/52 | 12/12 | 40/40 | 0 | 267 |
| F/development | 52/52 | 12/12 | 40/40 | 0 | 273.5 |
| C/replication | 47/52 | 12/12 | 35/40 | 5 | 264.5 |
| E/replication | 49/52 | 12/12 | 37/40 | 3 | 256 |

E wins the preregistered fewer-questions tie-break over F in development.
F was not evaluated on replication or integration. C repeats the inherited
clear-exclusion failure twice in development. All arms had zero provider errors
and zero unsafe *accepted* masks. Individual questions can still confidently
permit a forbidden day; another uncertain answer stops the aggregate mask.
Thus zero unsafe accepted masks is not a claim that every question was correct.

Blocks were sequential, four concurrent samples each. Latencies are descriptive;
no speed-improvement or universal superiority claim. Consistent-family counts
are in `summary.json` and are separate from correctness: consistently wrong
clarification can still be consistent. No run was repeated after seeing failures.

## E's three replication failures

1. `excluded-group-0`, both repeats: “Você perguntou os dias. Fim de semana,
   mas domingo não.” Scope guard says clear (.19/.18 probability of ambiguity),
   yet weekday questions are uncertain: Monday .61/.64, Tuesday .69/.62,
   Thursday .67/.68, Friday .43/.32. Wednesday is even confidently permitted
   (.75/.71), although only Saturday is allowed. The per-answer confidence
   rule correctly stops the aggregate. This failure is downstream of the guard.
2. `old-info-0`, second repeat: “Você perguntou os dias. A antiga está na terça.
   Tem estacionamento domingo? Para remarcar, sexta apenas.” Day mask is clear,
   but ambiguity .31 means only .69 confidence that scope is clear, below .7.
   This failure is in the guard. Do not lower the threshold to hide it.

`failure-attribution.json` records the exact answers and identifies which
scoring branch required clarification. This attributes the output logic, not
the model's internal reasoning or the causal effect of an individual word.
No actual appointments or external effects were involved.

## Conclusion and next experiment

Adding more guard wording is insufficient: two failures come from the eight
weekday questions, not ambiguity detection. Keep the current public helper.
The next development study should evaluate a structured representation of the
requested base set, inclusions, exclusions and corrections, with deterministic
set operations after typed Jev decisions. It must preserve ambiguous-group
clarification and all inherited cases. This is a proposed experiment, not an
implemented or proven architecture; it need not require more serial calls.

## Evidence, checks and reproduction

Total provider spend **$0.023792748/$0.50**, five completed classifier runs,
no integration calls or interrupted-run reserves. Frozen executable/data/source/
protocol hashes were verified unchanged. Original failing rounds remain intact.

`make ci` and `make lint-extra` pass on the measurement-harness extension and
unchanged public examples. E/F are benchmark policies only. Existing scorer
unit tests now also cover their guarded handling of uncertainty/missing answers.

Recompute with `python3 bench/results/ambiguity-003/summarize.py`.
Live reproduction: `go run ./bench/weekdaybench -dataset <dev-or-replication.json>
-ambiguity-policy C|E|F -mode batch -repeats 2 -max-cost .10 -out <new.jsonl>`
with `TYPESAFE_API_KEY` from the environment. These sets are now consumed.
`selection.json` records the choice of E before replication.
