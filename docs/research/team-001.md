# team-001: a simulated team that answers only from the world's facts

- **Date:** 2026-09-29.
- **Pre-registration:** `team-001-preregistration.md` (e5c5c43).
- **Data:** `bench/results/team-001/`.
- **Model:** DeepSeek V4.1 Flash, the simulator's model.
- **Judge:** Claude Sonnet 5, with the facts sheet.
- **Spend:** about $1.05.

## Result (46 tasks × 3 = 138 answers)

| # | Hypothesis | Result |
| --- | --- | --- |
| T1 | Grounded ≥ 97% | **Supported:** 137/138 (99.3%) |
| T2 | Correct ≥ 90% of in-sheet answers | **Supported:** 69/72 (95.8%) |
| T3 | Declines ≥ 95% of out-of-sheet answers | **Supported:** 46/48 (95.8%) |
| T4 | Valid booking values 18/18 | **Supported** |

**What it got wrong:**
- **One ungrounded answer:** "não trabalhamos com pacotes para Buenos
  Aires". The sheet does not say that.
- **Three incomplete answers:** the same in-sheet answer three times, a
  free cancellation without its "até 48 horas antes" condition.
- **Two out-of-sheet answers that did not decline:** Cristo Redentor
  tickets and a boat tour in Salvador. Both answered from the sheet's
  own rule that tours are sold only in Natal and Maceió. They are
  grounded, but they do not decline.

**Decision (the pre-registered rule):**
- T1, T3 and T4 hold.
- The simulated team is used in agency studies from agency-007 on
  (EvalSuite `spec.team.facts`).
- `bench/agency/run.py` adds it when the world has
  `team-facts.md`.
