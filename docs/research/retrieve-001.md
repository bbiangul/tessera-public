# retrieve-001: documents inside a Function

- **Date:** 2026-09-27.
- **Pre-registration:** `retrieve-001-preregistration.md` (commit e5d44ce,
  before the code).
- **Code:** 31736e6.
- **Data:** `bench/results/retrieve-001/`.
- **Spend:** under $0.01 for the agents and $0.10 for the judge.

## What was built

**A step `retrieve: {bases, query, k}`.** Its output is the list of
passages, `{source, section, text}`, found the way the free mode finds
them:
- only the bases the caller's audience may read;
- the query rewrite;
- no expired, suspended or retired lesson;
- the relevance filter, with the query as the message.

The host serves it to the runtime (`Runtime.Retrieve`). The result is
recorded with `once`, so a resumed run replays the same passages.

**Admission:**
- bases exist;
- the query is a string expression;
- a step has exactly one kind.

**Unit tests:**
- a patient never gets the staff-only base, and the staff do;
- a run waiting at an `ask` when the price changed replays the old
  passage, while a new run gets the new one;
- an expired lesson is not retrieved;
- the admission rules;
- 6 mutations caught.

## Result

**`orcamento`:** retrieve, then generate ("answer only from PASSAGES,
never a price that is not in them"), then respond. 10 price questions, 3
repeats.

| | with retrieve | without (no passages) |
| --- | --- | --- |
| Prices on the list, correct (of 18) | **18** | 0 |
| A price given for an item not on the list (of 12) | **0** | 0 |

The questions included a sum: evaluation plus cleaning, R$ 300.

| # | Result |
| --- | --- |
| T1 | **Supported:** 18 of 18. |
| T2 | **Supported:** 0 of 12. Without passages the Function invented nothing either: it said it had no price, 18 times, for prices the clinic publishes. |
| T3, T4, T5 | **Supported** (unit tests). |
| T6 | **Supported:** `make ci`, `make lint-extra`. |

**A harness mistake, fixed before judging:**
- The first run of both variants ended in "Something went wrong": my
  output template read `steps.resposta.output.value`, but a generate
  step's output is the text itself.
- The journal showed the retrieve and the generate had worked. A
  temporary debug print, not committed, showed the template error.
- Both variants were rebuilt and rerun, and only the rerun was judged.

**Also found:** a failed run does not journal its reason. With no
failure queue it is lost, and finding this took a debug build. Opened as
an issue.

## What it gives the author

A process can now read the business's documents at the step where it
needs them, with the same audience, lifecycle and relevance rules, and
with the passages recorded in the run as its audit. Examples:
- a quote;
- the rescheduling policy before offering a slot;
- the preparation for an exam.
