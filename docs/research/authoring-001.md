# Authoring 001 (RFC-0004 part C): results

- **Date:** 2026-09-24
- **Pre-registration:** [authoring-001-preregistration](authoring-001-preregistration.md).
- **What was measured:**
  - the two authors' definitions, copied to
    `bench/results/authoring-001/{skill,docs}-defs`;
  - their runs on `test4` and the judge's verdicts in
    `bench/results/authoring-001`.
- **Spend:** runs $0.14, judge $1.08, and the authors' own tests
  ($0.003 and $0.001).

## Effort

| | With the skill | Docs only |
| --- | --- | --- |
| Wall time | 3.4 min | 2.2 min |
| Tool calls | 26 | 24 |
| validate / test runs | 4 / 2 | 3 / 2 |
| Human interventions | 0 | 0 |
| Gates at the end | validate 0 errors, 0 warnings; 9/9 live; 9/9 replayed | validate 0/0; 6/6 live; 6/6 replayed |
| Questions it would have asked | 5 | 4 |
| Platform gaps it reported | 3 (#27, #28, and the EvalSuite limit in #29) | 1 (the EvalSuite limit, #29) |

Both built the same design:
- the free mode reads the checklist, marked public;
- a non-routed `ask-coach` is started through `freeMode.functions`;
- its `when` names the four kinds.

Neither calibrated the threshold. Both said so, and why: no labelled
set, and tests cannot see the probabilities.

## The agents on `test4`

| Agent | Unneeded escalation | Recall | SD | Correct | False claims |
| --- | --- | --- | --- | --- | --- |
| hand-written (escalation-004) | 14% | 100% | 0.01 | 95% | 0 |
| **authored, with the skill** | 13% | 100% | 0.00 | **57%** | 0 |
| **authored, docs only** | 13% | 100% | 0.01 | **56%** | 1 |

**Every judged-incorrect reply of the authored agents is on a coach or
partial question:**
- 30 coach replies each: the handoff message is a fixed template ("This
  needs your coach, so I've sent your question…"). It does not say what
  the checklist states, while the rubric, the same for every stack, asks
  for the rule.
- 15–16 partial replies each: the escalated partials (v14, v20) get the
  same bare message.

The hand-written `ask-coach` generates its message with the checklist in
view: "I've asked your coach — for reference, the checklist says…".

## Hypotheses

| # | Verdict |
| --- | --- |
| A1: one brief is enough (green gates, 0 interventions, ≤ 60 min) | **Supported** for both authors: 3.4 and 2.2 minutes. |
| A2: as good as the hand-written agent (each within 5 points) | **Not supported.** Escalation matched (13% against 14%, recall 100%), but correct fell to 57% against 95%, from the bare handoff message. |
| A3: the skill matters (≥ 10 points on a metric) | **Not supported:** the two agents are equivalent. The guide already carries the lessons that decide escalation. The with-skill author found more platform gaps (3 against 1), which is not a metric of A3. |

## What changes

1. **The skill and the guide:**
   - when a Function hands off to a person, its message to the user still
     gives what the material says: generate it with the reads in view, or
     template the rule;
   - handing off is not a reason to say less.

   This is the one lesson missing, and the 38-point gap came from it
   alone.
2. **The platform gaps the authors reported** are issues:
   - #27: free-mode turns are not in the history, so free answers greet
     every turn;
   - #28: a handoff blocks the conversation until the person answers;
   - #29: an EvalSuite cannot assert the handoff or see its probability,
     so authors cannot calibrate the threshold themselves.
3. **A2 again (authoring-002)** after the skill change, with a new author
   and a new domain, so the skill is not tuned to this one.
