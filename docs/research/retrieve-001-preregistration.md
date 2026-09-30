# retrieve-001: documents inside a Function. Pre-registration

- **Date:** 2026-09-27, before any of this is built.
- **RFC:** RFC-0008, part A: "Dentro de uma Function, um passo
  `retrieve` dá os trechos como dado tipado, com origem."

## What will be built

**A step `retrieve: {bases: [...], query: <string expression>, k: 1–10
(default 4)}`.** Its output is the list of passages found:
`{source, section, text}`.

It uses what the free mode uses (the host serves it to the runtime):
- the index, including plugin sources;
- the caller's audience filter;
- the lesson lifecycle (expired, suspended and retired lessons are not
  returned);
- the relevance filter, with the query as the message.

The result is recorded with `once`, so a resumed run replays the same
passages even if the documents changed.

**Admission:**
- bases exist in the namespace;
- the query compiles as a string expression;
- k is between 1 and 10;
- a step still has exactly one kind.

## Measure

**The agent** (`bench/retrieve/`): knowledge-001's clinic with a Function
`orcamento`, routed for price questions:
1. `retrieve` over the public documents, with the patient's message as
   the query;
2. `generate`: answer the price question from the passages only, and say
   there is no price here when they do not have it;
3. `respond` with the answer.

**Variants:**
- **with**, as above;
- **without**, the same Function with no retrieve step (passages
  empty): the baseline.

**Messages:** `bench/retrieve/messages.json`, 10 written now: 6 prices on
the list, 4 not on it.

**Runs:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- the judge is Claude Sonnet 5, given the price list and the truth, not
  the variant.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| T1 | The Function answers from the documents | with: prices on the list correct in ≥ 16 of 18 |
| T2 | It does not invent prices | with: a price given for an item not on the list in ≤ 1 of 12; without: reported |
| T3 | The audience holds inside Functions | unit: a patient's retrieve never returns a staff-only base's passage; the staff's does |
| T4 | Replays are stable | unit: a run resumed after the documents changed replays the passages it first retrieved |
| T5 | Lessons' lifecycle holds | unit: an expired lesson is not retrieved |
| T6 | Nothing else changes | `make ci` and `make lint-extra` pass |

**Budget:** $1.
