# Ambiguity 002 — wording families before promotion

2026-09-25, base 925e5a7. Compare rejected A unchanged against C (reworded
single ambiguity guard) and D (two separate independent vague-scope/conflict
guards). All finite decisions use Jev 1.13.0 in one request: 9 questions A/C,
10 D. Same .7 threshold and eight existing mask questions. No extraction/LLM.

Six development base requests x4 phrasings, two repeats =48 trials/arm.
Three ambiguous families and three clear families. Replication: six different
base requests and four different wrappers, same categories and proportions,
also frozen before calls. This tests new request families and wrappers, not
new business domains. Distinct messages =24/set; repeats are not new examples.

Policy: vague weekday group boundaries remain ambiguous even when excluding
named days; exclusions alone do not identify the original group's bounds.
Unresolved contradictory restrictions also clarify. Explicit groups/ranges,
absence of weekday restrictions, calendar dates, old appointments and FAQ
mentions should not cause unneeded clarification. This is a declared author
policy, not a universal meaning of colloquial Portuguese.

Gate: all 24 ambiguous trials clarify; all 24 clear trials correct (>=95% rounds
to 23/24 but require 24/24 explicitly here); zero errors/unsafe acceptance;
every family's four phrasings correct in both repeats. Select passing C/D by
fewer questions, then p50 latency. If neither passes, stop without replication.
If one passes, compare it and A on replication. No adaptive repair or reruns.
Report per-family decisions, exact masks, contradictions and raw probabilities.
No production promotion solely from classifier success; frozen integration
conversation gates must be registered separately before calls.

Budget .10 per arm/set, at most five runs, .50 total. Metered live calls,
no Generate/judge. Freeze datasets, questions, executable hashes before calls.
Use only frozen binaries/data, leave public examples unchanged. Record failed
variants and consumed sets. Run CI/static checks before local commit.

## Frozen helper integration, after replication, before calls

C and A both pass replication 48/48; C fixes the development-family failure,
not a claimed universal superiority over A. Validate the reusable helper only:
copy ambiguity-001's frozen eight-scenario helper, replace only guard A with
exact guard C. Keep output known=false policy unchanged. Run all eight live
and replay, including the retained vague-scope failure. Cap .10; prior measured
classifier spend .019999536 plus .10 is below the total .50. If all pass, promote
only examples/decide-batch helper/evals and its cassette plus runtime guard tests.
The clinic's separate helper stays unchanged; its broader integration is outside
this bounded promotion and still needs a frozen clinical conversation study.
