# fallback-001: after two rejected drafts, quote the document (#68)

- **Date:** 2026-09-28.
- **Pre-registration:** `fallback-001-preregistration.md` (commit
  4642ebc, before the code).
- **Data:** `bench/results/fallback-001/`.
- **Spend:** about $0.02 for the agents and $0.70 for the judge.

## What was built

**When the grounding check rejects the second draft and there are
documents,** one more generation asks for the documents' most relevant
sentence, word for word between quotes, and a person to confirm the case.
- Code checks the quote appears in a passage, ignoring case, spacing,
  list marks and final punctuation.
- If it does, the reply goes out, and the turn journals
  `"fallback": "quote"`.
- If not, the behaviour is as before: the fixed message.

**Unit tests:**
- a real quote is sent and marked;
- an invented quote falls back to the fixed message;
- 3 mutations caught.

## Result

**The registered test** (12 controls, 12 traps, 3 repeats) **never
triggered the fallback.** No reply of either variant had two rejected
drafts.

| | current | F |
| --- | --- | --- |
| "Não entendi" (of 72) | 0 | 0 |
| Controls correct (of 36) | 36 | 36 |
| Disguised controls correct (of 9) | 8 | 7 |
| Overreach on real traps (of 27) | 0 | 1 |
| Fallbacks sent | — | 0 |

| # | Result |
| --- | --- |
| F1 | **Supported, vacuously:** 0 and 0. |
| F2 | **Supported:** 36 and 36. |
| F3 | **Supported:** 1 ≤ 0 + 1. The one overreach is not a fallback, since none ran. |
| F4 | **Supported by the unit test only:** there was nothing to count live. |
| F5 | **Supported:** `make ci`, `make lint-extra`. |

**Shipped by the registered rule.** My data did not reach the case the
issue is about, so the registered test says nothing about the benefit.

**Exploration, declared as such:** the exact question that failed in
knowledge-003 ("clareamento caseiro, gestante pode?"), 10 runs per
variant:

| | "Não entendi" | A verified quote of the rule |
| --- | --- | --- |
| current | **10 of 10** | 0 |
| F | 0 | **10 of 10** |

F's reply was: *Pela nossa política, "Quem não pode fazer? Gestantes,
lactantes e menores de 16 anos." Alguém da equipe confirma o seu caso
específico.*

The quote keeps the document's Markdown (`**…**`). On WhatsApp that
shows as literal asterisks; a cosmetic fix for later.

## Limits

- **How often double rejections happen:** 3 of 60 replies in
  knowledge-003's test, 0 of 144 here. The fallback is a safety net for
  a rare path, measured on one question.
- **The quote is not checked for relevance:** it is the model's choice of
  sentence, verified only as the documents' own words.
