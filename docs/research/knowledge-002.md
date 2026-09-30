# knowledge-002: nearby passages are not answers (#64)

- **Date:** 2026-09-27.
- **Pre-registration:** `knowledge-002-preregistration.md`, with an
  addendum written after dev and test and before the confirmation.
- **Data:** `bench/results/knowledge-002/`.
- **Spend:** about $0.05 for the agents and $2.5 for the judge.

## What was built

**`freeMode.knowledge.relevance`** (on by default after this measure):
- After retrieval, one Jev request asks, for each of the passages found,
  whether it answers the message or directly settles part of it.
- The others are dropped before the handoff and the answer.
- If Jev errors, all passages are kept.

**Unit test:**
- only the relevant passage reaches the answer;
- an error keeps all of them;
- 3 mutations caught.

## Result

**Dev (12 messages) did not reproduce the problem.** Every variant
handed off all 24 near and none replies, with 0 misleading:
- base;
- R1, the relevance filter;
- R2, the handoff wording;
- R1+R2.

The registered rule therefore chose **base**, and Q1–Q3 compared base
with itself. The dev set was too easy.

**Test (24 new messages)**, base against R1 run as exploration:

| | base | R1 |
| --- | --- | --- |
| Misleading, near and none (of 48) | 7 | 2 |
| Handoffs, near and none (of 48) | 32 | 41 |
| Answer items correct without a handoff (of 24) | 20 | 21 |

Base's misleading replies were the problem the issue describes:
- "Sim, pode trazer seu filho" to a question about children, 3 of 3;
- whitening's coffee rule applied to a cleaning;
- implant authorization times applied to braces.

**Confirmation (20 new messages, R1 the only candidate, declared before
the run):**

| | base | R1 |
| --- | --- | --- |
| Misleading, near and none (of 42) | 14 | **7** |
| Handoffs, near and none (of 42) | 19 | **27** |
| Answer items correct without a handoff (of 18) | 18 | 18 |
| Median turn | 3.2 s | 2.0 s |

The median turn is over every run of both variants. A handoff answers
with the waiting message, without generating text, so R1 is faster
because it hands off more.

**Hypotheses (addendum)**

| # | Result |
| --- | --- |
| C1 | **Supported:** 7 ≤ 14 − 3. |
| C2 | **Supported:** 27 ≥ 19 + 3. |
| C3 | **Supported:** 18 ≥ 18 − 2. |
| Q4 | **Supported.** knowledge-001's set with relevance on: answerable 54 of 54, 0 leaks, 0 injections obeyed, 0 unsupported; `make ci` passes. |

**Relevance ships on by default.**

## What remains (R1's 7 of 42)

Two questions, every repeat:
- **"Clareamento em adolescente de 17 anos pode?"** "Pode, sim: a
  restrição é para menores de 16." The passage is relevant, but the
  inference goes beyond it.
- **"Vocês abrem no feriado de carnaval?"** The closed-on-holidays rule
  was applied to carnival.

Both are inferences from a relevant passage: the agent reasons one step
past what the document says. That is a grounding question, not a
relevance one. A stricter check ("does the reply conclude something the
passage does not state?") is the next candidate, on new data.

## Limits

- One clinic, synthetic documents, one model family, one judge.
- Retrieval stays BM25 plus the rewrite. A larger corpus to test
  vectors is still open.
