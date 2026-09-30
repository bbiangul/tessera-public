# team-001: a simulated team that answers only from the world's facts. Pre-registration

- **Date:** 2026-09-29, before the team is built.
- **Why:** agency-006. In both variants, 10 of 48 conversations ended
  because the customer asked the team something the scenario had not
  scripted. A real agency's team answers. Without that, comparisons of
  agent mode are mostly about who reaches the team first.

## What is built

- EvalSuite `spec.team.facts`: a Markdown sheet of what the team knows
  (`bench/agency/world/team-facts.md`, written now).
- When a task has no scripted answer, a model plays the team member:
  - DeepSeek V4.1 Flash, the simulator's model;
  - it sees the task, the conversation and the sheet;
  - it answers only from the sheet, and otherwise says it does not have
    the information;
  - a task asking for a value (a booking) gets the value, in the format
    the task and the sheet give.

## Measure

- **Data:** `bench/agency/team-001/tasks.json`, written now. It holds 46
  tasks:
  - **24 questions the sheet answers,** each with the key fact expected;
  - **16 questions it does not answer:** wifi on board, a parrot, a
    hotel's gym and similar;
  - **6 booking tasks,** in the booking Function's own words.
- **Runs:** each task 3 times, 138 answers.
- **Judge:** Claude Sonnet 5, with the sheet, per answer:
  - **grounded:** every fact is in the sheet;
  - **correct:** for a question the sheet answers, it gives the expected
    fact;
  - **declines:** for one it does not, it says it does not have that,
    without inventing.
- **Booking values are checked by code:**
  - `reserved` is true;
  - the locator matches the sheet's format;
  - the payment link is the sheet's, with that locator.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| T1 | It invents nothing | grounded ≥ 97% of the 138 answers |
| T2 | It answers what it knows | correct ≥ 90% of the 72 in-sheet answers |
| T3 | It says when it does not know | declines ≥ 95% of the 48 out-of-sheet answers |
| T4 | Bookings are well-formed | 18/18 valid values |

**Decision rule:** the simulated team is used in agency studies from
agency-007 on if T1, T3 and T4 hold. If T2 fails, it is still used, and
the report says how often it under-answers.

**Budget:** $1.
