# Engine guarantees 001 — enforce validity, preserve author policy

2026-09-25. Implemented one engine invariant: a failed or malformed individual
`decide` cannot masquerade as a valid low-confidence answer. Explicit batches
already enforced this. No automatic sequential batching was introduced.

## Measurement

Fault injection, no real model calls. The corrected pre-change harness tested
four faults under each of continue/fail/human. Of 12 cases, 11 failed the expected
safe behavior: three nil-response panics, six missing/inconsistent yes/no
answers proceeding to confirmation, one provider error continuing and one
provider error waiting for human classification. Provider error with policy
`fail` already stopped correctly. No booking occurred: the subsequent action
still required confirmation. These are synthetic contract tests, not observed
production incidents or a model error-rate estimate.

An initial harness setup accidentally supplied YAML as Starlark; it was corrected
before recording the baseline. A first assertion also rejected the standard
failure notification; the recorded baseline permits that notification and checks
that booking/continuation messages do not occur.

`bench/results/engine-guarantees-001/baseline.log` records the failing baseline.
The final code passes all 12 fault cases and three valid uncertainty cases,
the full `make ci` suite (including 102 replay scenarios), and `make lint-extra`.
All tests use scripted answers or existing replay: $0 new provider spend.
Initial full-check attempts encountered sandbox cache/network restrictions;
both checks were rerun successfully with the required access.

## What changed

Single decisions now check nil responses, answer presence and answer consistency
before constructing a step result. Single and batch decisions share the same
validator (kind, finite probabilities, mass, allowed labels, and consistency
of the selected choice/yes-no result). The runtime records a sanitized failure
and applies `onError` before checking any confidence threshold. Provider details
are not exposed. An existing journaled error also follows the failure path when
replayed; successful journaled outputs are unchanged.

For valid uncertain answers, the author retains `continue` (null value), `fail`,
or `human`. No API/schema changes, new dependencies, model calls or prompt changes.
This patch covers Function `decide` steps; it is not a new validation wrapper
for every host/router/provider path. Batch score output uses its validated
probability distribution, as before.

## Engine boundary and batching

| Responsibility | Current decision |
| --- | --- |
| Invalid provider response | Engine rejects; author may declare `onError` recovery |
| Valid uncertainty | Author chooses clarification/escalation policy |
| Independent questions declared together | `decideBatch` masks shared state, sends one Jev request and journals one durable result |
| Consecutive decisions | Remain sequential; input independence alone is insufficient |
| Scope, preferences and conversation order | Author declares business semantics; no universal clinic policy in the engine |

A concrete counterexample to automatic merging is an independent second
question following a first decision with `onLowConfidence: human`: serial
execution waits before issuing the second question, while a batch sends it
immediately. On first-question failure, a batch also evaluates work that serial
execution would never reach. It changes costs, cancellation and journal/replay
boundaries even if final successful answers happen to match. Previous weekday
measurements establish a speed benefit for explicitly grouped questions, not
universal semantic equivalence of rewriting a workflow.

Next optimization study must define admissible groups and failure/journal
compatibility before measuring scheduling alternatives. Do not infer permission
to cross effects, waits or control flow from equal state expressions. The
source-free authoring-005 evidence remains frozen; no new partner/parity claim.
