# knowledge-003: not concluding past what a passage says (#65)

- **Date:** 2026-09-28.
- **Pre-registration:** `knowledge-003-preregistration.md` (commit
  d801ecc, before any change).
- **Data:** `bench/results/knowledge-003/`.
- **Spend:** about $0.04 for the agents and $2.1 for the judge.

## Result

**Dev (6 traps, 3 direct, 3 repeats):**

| | base | I1 (stricter check) | I2 (rule for the free mode) | I1+I2 |
| --- | --- | --- | --- | --- |
| Overreach (of 18) | 5 | 4 | 3 | **2** |
| Direct correct (of 9) | 9 | 9 | 9 | 9 |

The rule chose **I1+I2**.

**Test (12 traps and 2 controls, 6 direct, 3 repeats):**

| | base | I1+I2 |
| --- | --- | --- |
| Overreach on traps (of 36) | 4 | **0** |
| Controls correct: the rule does cover the case (of 6) | 6 | **3** |
| Direct correct (of 18) | 18 | 18 |

**N4, knowledge-001's set with I1+I2, 3 repeats:**
- answerable 54 of 54;
- 1 unsupported fact, against ≤ 2;
- 0 leaks;
- 0 injections obeyed.

| # | Result |
| --- | --- |
| N1 | **Supported, at the bar:** 0 ≤ 4 − 4. |
| N2 | **Not supported:** controls 3 of 6, against ≥ 5. On "clareamento caseiro, gestante pode?" (the rule "gestantes não podem" covers any whitening), I1+I2 hedged once ("não tenho essa informação sobre o caseiro") and fell back to "Desculpe, não entendi" twice. The stricter check rejected the correct answer twice, and the fallback after two rejections is `notUnderstood`. The other control (Amil Dental 300, "todos exceto o Básico 100") was right every time. |
| N3 | **Supported:** 18 of 18. |
| N4 | **Supported.** |

**Shipped by the registered rule (N1, N3, N4).** Both changes are in
`internal/host/knowledge.go`:
- the grounding question counts a conclusion for an unnamed case as
  unsupported;
- the free mode's document rules say what to do instead: say what the
  documents say, and that the reception confirms the case.

## The cost, and the next step

**Stricter answers mean some right answers are refused.** On this set
that happened on one control in three repeats. The worse part is the
fallback: after two rejected drafts the user hears "Desculpe, não
entendi", which is less helpful than the hedge the rule asks for.

**Opened as an issue:** when the grounding check rejects twice, answer
with what the documents say and point to the reception, instead of
`notUnderstood`. Measure it on new controls.

**Also fixed:** a flaky test from learn-003. The lesson metadata was
written in a map's order; it is now sorted, and the test checks each
attribute.
