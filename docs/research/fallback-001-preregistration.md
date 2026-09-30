# fallback-001: after two rejected drafts, quote the document. Pre-registration

- **Date:** 2026-09-28, before any change.
- **Issue:** #68.

## The problem

**In knowledge-003,** the stricter grounding check rejected a correct
answer twice ("gestante pode clarear em casa?"), and the user heard
"Desculpe, não entendi". The document had the answer.

## The candidate (F), one, fixed now

When the check rejects the second draft and FACTS has documents, one
more generation asks for only two things:
1. the sentence of the documents most relevant to the message, copied
   word for word between quotes;
2. that a person from the team confirms the specific case.

**Code checks the quote:** the text between the first pair of quotes
must appear in a passage, comparing whitespace and case loosely. If it
does, the reply goes out. If not, the behaviour is today's, with the
fixed message. The claims guard still runs after it.

## Data

`bench/fallback/messages.json`, written now, test only:
- **12 controls:** a rule covers the case, though not word for word;
- **12 traps:** most are not covered. Three are disguised controls
  (t01, t03, t05), where the rule does settle it.

**The agent:** knowledge-001's clinic, as shipped after knowledge-003.

**Variants:**
- **current:** the shipped binary;
- **F.**

**Runs:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- the judge is Claude Sonnet 5, given every document and the truth.

**Per reply, the judge scores:**
- `correct`;
- `overreach`: it concludes for a case the documents do not settle;
- `not_understood`: a "não entendi"-type reply.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| F1 | Fewer dead ends | "não entendi" on all 72 replies: F ≤ 2, and ≤ current's − 2 (if current has ≥ 3) |
| F2 | Controls no worse | F's controls correct ≥ current's (of 36) |
| F3 | No new overreach | F's overreach on the 9 real traps ≤ current's + 1 (of 27) |
| F4 | Quotes are real | every fallback reply sent carries a quote found in a passage (unit test; and counted from the journal, where the fallback marks the turn) |
| F5 | Nothing else changes | knowledge-001's unit tests and `make ci` pass |

**If F1, F2, F3 and F5 hold,** F ships.

**Also reported:** how often the fallback ran.

**Budget:** $1.5.
