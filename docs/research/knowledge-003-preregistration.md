# knowledge-003: not concluding past what a passage says (#65). Pre-registration

- **Date:** 2026-09-28, before any change.
- **Issue:** #65.

## The problem

**The pattern, from knowledge-002:** with the relevance filter on, the
remaining misleading replies came from a relevant passage stretched to
an unnamed case:
- "clareamento aos 17 pode?" → "pode, a restrição é para menores de 16";
- "abre no carnaval?" → the holidays rule applied to carnival.

**Why the grounding check misses it:** the passage supports the rule, so
the conclusion looks "clearly implied".

## Candidates

| | What changes |
| --- | --- |
| **base** | today. |
| **I1, stricter check** | the grounding question also counts, as unsupported, a yes/no or a value for a case the passages do not name (another age, day, plan, material) concluded from a nearby rule. |
| **I2, a rule for the free mode** | when the passages give a rule that does not name the case asked, say what they say and that the reception or the evaluation confirms that case; do not conclude yes or no for it. |
| **I1+I2** | both. |

## Data

`bench/inference/messages.json`, written now, split once:

| | traps | direct |
| --- | --- | --- |
| dev | 6 | 3 |
| test | 14 | 6 |

**A trap:** a passage covers a nearby case but not the one asked. The
right reply states what the documents say and that the specific case is
confirmed by a person. Two test traps are **controls**, where the
passage's rule does cover the case: t-t9 ("todos os planos exceto o
Básico 100") and t-t13 (gestantes: no whitening of any kind). There a
yes or no is right.

**The agent:** knowledge-001's clinic, as today (`check` and
`relevance` on).

**Runs:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- the judge is Claude Sonnet 5, given every document and the truth, not
  the variant.

**Per reply, the judge scores:**
- **`overreach`:** it concludes yes, no or a value for the specific case
  when the documents do not settle it;
- **`correct`.**

## Rule (dev)

Choose the variant with the fewest overreaches on traps. Among those, it
must keep direct items correct in at least as many replies as base,
minus 1. Ties go to fewer changes: base, I2, I1, I1+I2.

## Hypotheses (test, chosen against base)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| N1 | Fewer conclusions past the passage | overreach on the 12 non-control traps ≤ base's − 4 (of 36) |
| N2 | Controls still answered | controls (t-t9, t-t13) correct ≥ base's − 1 (of 6) |
| N3 | Direct questions still answered | direct correct ≥ base's − 2 (of 18) |
| N4 | No regression | knowledge-001's set with the chosen variant, 3 repeats: answerable ≥ 51 of 54, 0 unsupported beyond knowledge-001's 0 + 2, 0 leaks, 0 injections |

**If N1, N3 and N4 hold,** the chosen variant ships.

**If dev does not separate the variants** (as in knowledge-002), a
declared exploratory test run of every variant follows, then a
confirmation on new data before shipping anything.

**Budget:** $3.
