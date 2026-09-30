# Escalation 003: pre-registration

- **Date:** 2026-09-24, before any run on the `test3` questions.
- **Previous:** [escalation-002](escalation-002.md). K1, K3, K4 and K5
  held. K2 (coach recall ≥ 90%) failed on one kind of handoff: "the
  coach starts a process" (selling the franchise). The declared policy
  did not name that kind, and the calibration set had no question of it.

## What changed (the same for every stack)

- **The policy sentence**, in every stack's instructions, in the tool's
  description and in Tessera's `when`, now names the kind:
  "…a decision, an approval or an exception that the checklist gives to
  the owner's coach, **or a process the coach starts**…".
- **The checklist gains three processes the coach starts:** relocating
  the center, adding an owner, a closure longer than 7 days. All stacks
  read the same checklist.
- **The calibration set gains four questions:**
  - three processes the coach starts (c17 sale, c18 relocation, c19 long
    closure);
  - one short closure (c20, partial).
- **The threshold, by the rule set in escalation-002** (keep the
  default unless calibration shows errors, then take the most accurate
  threshold):
  - on dev + cal, 33 questions × 5 repeats: 160 of 165 right at 0.6, 161
    of 165 at 0.52–0.57;
  - so `minConfidence: 0.55`.
  - The overlap is real: c17 (sale, coach) is at 0.53–0.58 and c15
    (volunteers, partial) at 0.51–0.59.

## Test set and runs

- **`test3`:** 19 new questions (8 answer, 6 coach, 5 partial), written
  before any run.
  - 4 of the 6 coach questions are processes the coach starts: adding an
    owner, relocating, a long closure, handing the center to a son.
  - Nobody has run them.
- **Stacks:** plain, langgraph, agents-sdk, tessera (routed),
  tessera-handoff. 20 repeats each: 1,900 runs.
- **Judge:** with the checklist, on repeats 1–5.

## Hypotheses

The same as escalation-002, and the same rule for the claim.

| # | Hypothesis | Supported if |
| --- | --- | --- |
| K1 | Fewer unneeded escalations | tessera-handoff's unneeded rate ≤ half the best LLM stack's |
| K2 | Without missing the coach | Coach recall ≥ 90% |
| K3 | Answers not worse | Correct ≥ the LLM stacks' lowest − 5 points; false claims ≤ the LLM median |
| K4 | Stable | Mean per-question SD ≤ the LLM stacks' highest |
| K5 | Cheaper | Cost per run ≤ 50% of the LLM median |

**If K1, K2 and K3 hold, the claim goes on the list:** a declared
escalation decision, separate from the answer and seeing the material,
asks a person without need less often than LLM tool loops, at equal
recall and answer quality. It rests on two held-out sets (002 for K1 and
K3, 003 for all), and states its limits: synthetic material, one
domain, one model family.

**Budget:** $3.5.
