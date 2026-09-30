# Authoring 002: results

- **Date:** 2026-09-24
- **Pre-registration:** [authoring-002-preregistration](authoring-002-preregistration.md).
- **Code measured:** main at `0200f80`, with the harness fix below.
  Results are in `bench/results/authoring-002`:
  - `defs/`: the authored agent;
  - `market/`: its C0–C3 runs;
  - `naturalness/`: the judged pairs.
- **Spend:** runs about $0.12, judge $2.08, the author's own tests $0.015.

## Deviation: a harness bug the author found

- The first run failed 48 of 60 C0 conversations: "I found no upcoming
  appointment". The authored agent offers only **future** appointments,
  which is sensible.
- The harness started Tessera's clock at 9h30 on the tasks' day, and
  every task's appointment is at 9h that day, so they had already begun.
  The LLM stacks get the date without a time, and the hand-written agents
  do not filter by time, so nobody had noticed.
- The clock now starts at 8h (commit `0200f80`) and the run was repeated.
- Earlier Tessera results are unaffected: none of those agents compared
  times.

## Effort

| | authoring-000 (docs only) | **authoring-002 (skill)** |
| --- | --- | --- |
| Wall time | 5.7 min | 12.9 min |
| Tool calls | 35 | 42 |
| validate / test runs | 6 / 4 | 7 / 8 |
| Interventions | 0 | 0 |
| EvalSuite scenarios | 9 | **22**: faults (409, 404, timeouts with and without the effect), mid-process questions, someone else's appointment, a double "sim" |
| authoring-000's four doc gaps hit again | — | **none** |

**New gaps it reported**, now documented (commit `c122097`) or opened:
- how HTTP answers become failures;
- `path` is a template;
- `idempotencyKey` is bare CEL;
- an `ask` text must list its options;
- sandbox keyword arguments;
- `expect.handoff` needs `freeMode.functions`;
- `onError` cannot tell 409 from 404 (#30).

It also reported that the docs point to `examples/clinica`, which the
experiment had hidden from it: an artefact of the setup, not a gap.

## The agent

| | Hand-written D (voice-002) | authoring-000 | **authoring-002** |
| --- | --- | --- | --- |
| C0 / C1 / C2 / C3 | 100 / — / — / — | 98 / 100 / 98 / 100% | **100 / 100 / 100 / 100%** |
| Duplicates, unconfirmed moves | 0, 0 | 0, 0 | **0, 0** |
| Natural, share of decided | 16% (12–22) | (templates, not judged) | **4%** (2–8) |
| Clear, share of decided | 9% (5–17) | — | 1% (0–5) |
| Judge's factual flags on Tessera | 5 | — | 8 |

- **The voice ran:** 98% of messages were rewritten, and 3 fell back to
  the template.
- **But the messages are short and plain.** For example: "Oi, Ana! Posso
  remarcar sua consulta de sexta 02/10 às 9h para sexta 09/10 às 15h?
  Responda sim para confirmar." Then "Pronto, Ana! Sua consulta … foi
  remarcada …".
- The judge's reasons in the 217 lost "natural" pairs: warmth 74%,
  formatting 50%, detail 50%.

## Hypotheses

| # | Verdict |
| --- | --- |
| B1: one brief, green, 0 interventions, ≤ 60 min | **Supported:** 12.9 minutes. |
| B2: safe and successful | **Supported:** 100% in C0–C3, 0 duplicates, 0 unconfirmed moves. |
| B3: talks as well as the hand-written agent (within 5 points) | **Not supported:** natural 4% against 16%, clear 1% against 9%. |
| B4: authoring-000's doc gaps are gone | **Supported:** none of the four came up. |

## What this says

- **Safety and success now come from the brief alone.** A coding agent
  with the skill builds an agent with the market harness's guarantees
  (no duplicate under timeout, retry or crash) in about 13 minutes, with
  a test suite that covers the faults.
- **Conversation quality does not.** Our rules say what the content must
  contain (who, from and to; narrowed offers). They do not produce the
  warmth and structure the judge rewards. Even the best hand-written
  variant is at 16%.
- **The owner's first goal, "not a 2015 chatbot", is still open** after
  the voice, the free mode, the content rules and the ablations.

The measured candidates left:
1. **A larger model for the voice.** The RFC started with the small one
   "and moves up only if measurement shows need". The four naturalness
   rounds (voice-001, freemode-001/002, voice-002) are that measurement.
   The judge is Claude, so the candidate should be another family
   (e.g. DeepSeek V4.1, not Flash) to keep the judge blind to style
   affinity.
2. Letting the voice restructure a message (lines, a closing offer)
   within the fact lock, which today only the persona hints at.
