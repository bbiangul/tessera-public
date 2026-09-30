# History 001 (issues #27, #28): results

- **Date:** 2026-09-24
- **Pre-registration:** [history-001-preregistration](history-001-preregistration.md).
- **Results:** `bench/results/history-001`: `before.jsonl` (main at
  `4a805c6`) and `after.jsonl` (the fix), each with its judged turns.
- **Spend:** $0.72.

## Results (8 conversations × 3 turns × 3 repeats)

| | Before | After |
| --- | --- | --- |
| Turn 1 correct | 24/24 | 24/24 |
| Turn 2 correct (a follow-up) | **12/24** | **24/24** |
| Turn 3 correct (a follow-up) | 18/24 | 21/24 |
| Follow-ups together | 30/48 (62.5%) | **45/48 (94%)** |
| Replies "a person from the team is looking at this" | 3 | **0** |

**Before:**
- "what about someone halfway through one?" (a degree, asked just before)
  had no context. The handoff sent it to the coach, with an unrelated
  checklist quote.
- The next message then got "a person from the team is looking at this".

**After:** the three remaining misses are one question in one
conversation (m07): "and our center director?" after "can my business
partner come too?" about owner training.
- Two replies read it as a hiring question.
- One said the director cannot attend at all, which is false: an
  additional staff member attends for $500.

## Hypotheses

| # | Verdict |
| --- | --- |
| H1: follow-ups ≥ 90% | **Supported:** 94% (45/48), from 62.5%. |
| H2: first turns ≥ 23/24 | **Supported:** 24/24. |
| H3: no blocked conversation | **Supported:** 0, from 3. |

## Limits

- 8 conversations, written by us, one domain.
- The baseline was run to confirm the problem before the hypotheses were
  written down. The fix is structural (the history is journaled and read);
  nothing was tuned to these conversations.
