# Ambiguity 002 — wording consistency improves, clear exclusion regresses

2026-09-25, base 925e5a7. **Do not promote C.** Both new candidates pass the
registered development-family gate; selected C passes separate replication,
but its frozen helper conversation adds an unnecessary clarification to a
previously supported clear exclusion. The public helper and clinic stay unchanged.

## Design and frozen boundary

Six base-request families x4 phrasings x2 repeats per set =48 trials, but only
24 distinct messages. Three families are ambiguous by the declared policy;
three are clear. Replication uses different base requests and wrappers, within
the same task categories, not a new domain. This tests a product policy about
ambiguous weekday-group boundaries, not universal linguistic ground truth.

A is the unchanged rejected yes/no guard from ambiguity-001. C explains that
excluding a named day from a vague group does not establish the group's bounds.
D separates vague boundaries and contradictions into two independent questions.
A/C have nine questions per request, D ten. All use Jev 1.13.0, .7 confidence,
one request per trial and the same eight weekday-mask questions. No new LLM path.

Binary, harness sources, data and initial protocol hashes were written before
calls. The integration appendix was subsequently registered before integration
calls. Initial protocol prefix and all frozen data/code/definitions were verified
unchanged afterwards. No interrupted or mixed-definition runs in this round.

## Results

| Arm/set | Correct | Ambiguous clarified | Clear masks correct | Consistent families | Median ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| A/development | 45/48 | 21/24 | 24/24 | 5/6 | 265 |
| C/development | 48/48 | 24/24 | 24/24 | 6/6 | 267 |
| D/development | 48/48 | 24/24 | 24/24 | 6/6 | 264.5 |
| A/replication | 48/48 | 24/24 | 24/24 | 6/6 | 257.5 |
| C/replication | 48/48 | 24/24 | 24/24 | 6/6 | 270.5 |

A's three development failures were the vague weekday exclusion with a greeting
(two repeats) or calendar-filter prefix (one repeat). C/D fix these observed
cases. C wins the registered tie-break by using fewer questions, not by superior
accuracy to D. D was not run on replication or integration. A and C tie on
replication: no universal superiority or speed improvement claim for C.
No provider errors in these classifier runs. Blocks were sequential, with four
concurrent samples per block; latency differences are descriptive, not a causal
performance result. Full per-family decisions are in `summary.json`.

## Integration rejected the selected guard

Copied the previous round's frozen eight-scenario helper, replacing only A's
guard with the exact C question. Output handling remains conservative: only a
confidently clear scope allows the mask; everything else returns known=false.
Public definitions were not edited during the evaluation.

Result: **7/8 live, 7/8 replay**, same failure:

- Input: “Filtre o calendário: qualquer dia menos sábado.”
- Expected: Friday and Sunday in the fixed demonstration calendar.
- Actual: “Não entendi a preferência de dias. Pode esclarecer?”
- Ambiguity probability .35 (clear .65), below the required .7 for proceeding.

C fixes the previous vague-scope failure (ambiguity .90, asks to clarify) but
now unnecessarily blocks a clear exclusion. Both are part of the acceptance
contract. Do not lower the threshold or delete the clear case to manufacture
a pass. The contradictory request passes through conservative uncertainty
(ambiguity .64), not confident detection; that distinction is retained in raw
answers. All four newly added integration cases pass; one inherited case fails.
No actual appointments were moved by this fixed-calendar helper.

The family classifier did not include the unrestricted-set-minus-one-day case,
although the inherited helper suite did. That coverage gap explains why its
integration gate is necessary; it does not identify the model's internal cause.
No adaptive candidate repair or additional selection occurred after this failure.

## Checks, cost and next work

Classifier spend $0.019999476; helper live $0.000771372; **total $0.020770848**
of $0.50. Replay costs zero. The integration appendix's preliminary classifier
subtotal was $0.000000060 too high; `budget.json` records the actual sum.
No interrupted-run reservations in this study.

Final `make ci` and `make lint-extra` pass on unchanged public examples and the
extended measurement harness. New unit tests require both D guards (or C's one
guard) confidently clear; uncertain, positive or missing guards cannot authorize
an unrestricted mask. Historical A/B and default benchmark behavior remain.

Next development set must contain all existing clear-exclusion regressions,
explicit corrections, broad groups with exclusions and vague groups with
exclusions, each with multiple wrappers. Reserve new request/wording families
for evaluation. The failed C helper remains a candidate, not recommended policy;
no automatic grouping or engine business-policy change was made.

## Reproduce

`python3 bench/results/ambiguity-002/summarize.py` recomputes classifier metrics.
`-ambiguity-policy A|C|D` on `go run ./bench/weekdaybench` runs the relevant
batch arm with `-dataset`, `-out`, `-repeats 2`, `-mode batch`, `-max-cost .10`.
Use new output paths for any later calls. Sets here are now consumed.

The retained integration failure reproduces offline (expected exit status 1):

```
./bin/tessera test bench/results/ambiguity-002/candidate-helper \
  --cassette bench/results/ambiguity-002/helper.jsonl --cassette-mode replay
```
