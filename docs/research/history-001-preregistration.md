# History 001 (issues #27, #28): pre-registration

- **Date:** 2026-09-24. Written after the baseline run and before the
  run of the fix.
- **Material:** `bench/escalation/conversations.json`: 8 synthetic
  conversations of 3 turns with the escalation benchmark's onboarding
  agent (`tessera-handoff`).
  - Turns 2 and 3 are follow-ups that need the earlier turns: "and how
    much is it?".
  - 3 repeats; the same judge, with the checklist and the conversation so
    far.
- **Baseline** (main at `4a805c6`, run before the fix):
  - turn 1: 24/24 correct;
  - turn 2: **12/24**;
  - turn 3: 18/24.
  - A context-less follow-up was handed to the coach, and the message
    after it got "a person from the team is looking at this" (#28).

## What changed

- **Free-mode exchanges are journaled** as finished runs of the
  conversation, with their messages acknowledged like any run's (#27):
  - the next free answer, the handoff decision and the voice see them;
  - follow-ups (`selector.after`) still route to the Function before.
- **The handoff decision sees the conversation.**
- **While a person handles a request,** the free mode answers other
  messages instead of "a person is looking at this" (#28).
- EvalSuites can assert the handoff (`expect.handoff`), and `tessera test`
  reports its probabilities (#29). No effect on this run.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| H1 | Follow-ups are understood | Turns 2 and 3 together ≥ 90% correct (baseline 30/48 = 62.5%) |
| H2 | Nothing regresses on first turns | Turn 1 ≥ 23/24 |
| H3 | No blocked conversation | 0 replies "a person from the team is looking at this" |

**Budget:** $0.6.
