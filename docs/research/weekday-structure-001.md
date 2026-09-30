# Weekday structure 001 — deterministic composition did not improve interpretation

2026-09-25, base a6933a9. **Reject G and H.** Both structured candidates performed
worse than the E benchmark reference on development. Following the registered
stop rule, no replication or integration calls were made. Public helpers and
the engine remain unchanged; the set-composition implementation is benchmark-only.

## Experiment

E asks its existing ambiguity guard plus eight weekday-mask questions. G/H keep
E's exact guard, ask a base set (all/weekdays/weekend/explicit names), and extract
inclusions/exclusions. Code computes `(base union inclusions) minus exclusions`.
G uses seven three-way role choices (include/exclude/none), nine questions total.
H uses fourteen yes/no operands, sixteen questions total. All questions run in
one Jev 1.13.0 request, with .7 confidence required before accepting a mask.
Missing/uncertain operands clarify; conflicting H operands also clarify.

Development combines the two consumed ambiguity-003 sets with four additional
union/range examples, each with two wrappers: 60 distinct messages, twice each,
120 trials per arm. It is explicitly development data. The new 20-message
replication set was frozen before calls and remains unexecuted. No adaptive
prompt repair, oracle relabeling or reruns occurred.

## Results

| Arm | Correct | Clear cases correct | Unneeded clarification | Unsafe accepted masks | Median ms | Cost USD |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| E reference | 114/120 | 90/96 | 6 | 0 | 265 | 0.010839024 |
| G role choices | 94/120 | 70/96 | 26 | 0 | 397.5 | 0.017133984 |
| H separate booleans | 84/120 | 60/96 | 35 | 1 | 275.5 | 0.023050944 |

All three clarified the 24 policy-ambiguous trials and had zero provider errors.
E also fails the strict all-cases gate; it is not promoted by beating the new
candidates. Accuracy, abstention and safety are separate metrics. No calendar
writes or real-user effects were executed.

The all-operand confidence requirement exposes more uncertain intermediate
answers. Among G's failures, 15 involve base confidence, 13 operand confidence
and 3 scope confidence; H has 13, 28 and 5 respectively. These categories overlap
and do not sum to failure counts. `G-failures.json` and `H-failures.json` identify
the exact uncertain fields; they describe observed decision paths, not a causal
attribution of model internals.

H's unsafe acceptance is `dev-old-faq-0`, repeat0. The desired new day is Thursday
(the message also identifies the old Monday appointment and asks about Saturday
opening hours). It confidently classifies Sunday as included (.79), combines it
with Thursday and returns `[0,4]` instead of `[4]`. The deterministic arithmetic
is correct for those operands, but an operand is wrong. Computation cannot make
incorrect semantic input safe by itself.

All arms ran in sequential blocks, four concurrent messages per block. Latency
and cost differences are descriptive for this workload, not broad speed claims.
G/H are more expensive here; splitting the representation is not demonstrated
as an improvement. Nor does this one implementation disprove all structured
representations.

## Engineering and limits

New unit tests check weekend minus Sunday, all days minus Saturday, weekdays
plus Saturday, explicit inclusions/exclusions and corrected-preference operands.
Every missing or uncertain operand must abstain; contradictory H inclusion and
exclusion abstains. These tests prove the set operations and guard behavior,
not natural-language understanding. Actual live errors remain in the evidence.

The harness stores assembled masks alongside raw questions' answers. Existing
E/default benchmark paths retain their behavior. No runtime/API/schema or public
example changes were made. `make ci` and `make lint-extra` pass. All frozen source,
binary, protocol and dataset hashes verify unchanged.

Provider spend **$0.051023952 of $0.50**, three completed runs, zero replication,
integration or interrupted-run reserves. No Generate or judge calls.

## What to measure next

The useful next product experiment is end-to-end recovery from uncertainty:
can a short clarification resolve the six E reference abstentions, preserve
restrictions, and finish the request with an acceptable number of turns? Compare
against the current public helper and retain safety gates; do not lower confidence
or silently treat unknown as unrestricted. That is a proposed study, not evidence
that the recovery already works. Avoid promoting more parsing layers merely
because deterministic computation sounds safer.

## Reproduction

`python3 bench/results/weekday-structure-001/summarize.py` recomputes metrics.
Live reproduction uses `go run ./bench/weekdaybench -dataset <dev.json>
-ambiguity-policy E|G|H -mode batch -repeats 2 -max-cost .10 -out <new.jsonl>`
and `TYPESAFE_API_KEY` from the environment. Never overwrite frozen outputs.
`selection.json` records rejection and the unexecuted replication set.
