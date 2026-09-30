# Market benchmark 001: Tessera against LangGraph, the OpenAI Agents SDK and a plain loop

- **Date:** 2026-09-23
- **Pre-registration:** [market-bench-000](market-bench-000-preregistration.md),
  with its deviations (all made before the full run).
- **Code measured:** commit `f9b9b85`. The raw results, every
  transcript included, are in `bench/results/market-001/runs.jsonl`.
- **Size:** 1,440 conversations: 6 competitors × 4 conditions × 20 tasks
  × 3 repeats. Total spend $1.72, simulator included. Rates are given with
  95% Wilson intervals.

## Setup in one paragraph

The task is rescheduling a dental appointment over chat, in Portuguese.
Twenty synthetic patients each play a goal: 12 cooperative, 4 where the
right outcome is no change, and 4 who change their mind. The patient
simulator is DeepSeek V4.1 Flash at temperature 0. Every model call of
every competitor uses the same model and provider (DeepSeek, reasoning
off), except Tessera's decisions, which go to Jev. The tool server is the
same for all. The competitors:

- Tessera: the product.
- A plain tool-calling loop.
- LangGraph 1.2: `create_agent` with the in-memory checkpointer (`langgraph`)
  or SQLite (`langgraph-sqlite`).
- OpenAI Agents SDK 0.22: SQLiteSession in memory (`agents-sdk`) or on a
  file (`agents-sdk-sqlite`).

An independent reviewer read every implementation before the full run.
Its 11 findings were fixed, and the fixes are listed as deviations.

## Results

C1–C3 count only the runs where the fault fired. The cost column is in
thousandths of a dollar per successful C0 run: DeepSeek tokens are priced at the list
price for everyone, and Jev as Typesafe reports it.

| Competitor | C0 success | C1 success | C1 runs with a duplicate move | C1 runs re-sending an applied move | C2 success | C3 success | Unconfirmed moves (after review) | Cost per successful C0 run (m$) | p50 turn |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Tessera** | **92%** (82–96) | **94%** (84–98) | **0 / 50** | **0 / 50** | 90% (79–96) | 94% (84–98) | **0** | **0.13** | **0.32 s** |
| plain loop | 100% (94–100) | 79% (66–88) | 17 / 48 | 43 / 48 | 98% (89–100) | 100% | 0 | 1.08 | 2.3 s |
| langgraph | 100% (94–100) | 88% (75–94) | 14 / 48 | 43 / 48 | 96% (86–99) | 100% | 0 | 1.02 | 2.7 s |
| langgraph-sqlite | 100% (94–100) | 83% (70–91) | 15 / 48 | 46 / 48 | 100% | 100% | 0 | 1.02 | 2.8 s |
| agents-sdk | 100% (94–100) | 71% (57–82) | 23 / 48 | 48 / 48 | 98% (89–100) | 100% | 2 (C3) | 1.09 | 2.7 s |
| agents-sdk-sqlite | 100% (94–100) | 79% (66–88) | 15 / 48 | 47 / 48 | 100% | 100% | 2 (C3) | 1.06 | 2.2 s |

The fault fired in 48–51 runs out of 60 per competitor: the 4 no-change
tasks never reach a move. Those fired runs cover 16–17 distinct tasks.

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| H1 | LLM stacks sometimes move without an explicit yes (C0) | **Not supported.** In normal conditions no stack moved without a confirmation. |
| H2 | Under act-then-timeout Tessera does not duplicate; others do in ≥ 10% | **Supported.** Tessera: 0 duplicates, 0 re-sends. The others duplicated in 29–48% of fired runs and re-sent the applied move in 90–100%. |
| H3 | After a crash Tessera resumes; in-memory stacks lose the conversation | **Not supported as success.** In-memory stacks still reached 96–98%, because the patient repeats what they wanted. Tessera 90%, its failures coming from the flaws below, not from the crash. |
| H4 | Tessera is not worse at success (within 5 points in C0) | **Not supported.** 92% against 100%: 8 points, from two product flaws (below). |
| H5 | Tessera costs ≤ 50% of the median competitor | **Supported.** $0.00013 per successful C0 task against $0.00102–0.00109, about 8 times less. |
| H6 | Tessera is slower per turn (measured only) | **The opposite.** p50 0.32 s against 2.2–2.8 s. |
| H7 | Retried deliveries do not duplicate effects in Tessera | **Supported.** 0 in C3. The Agents SDK made 4 unconfirmed moves in C3 (next section). |

## What the transcripts show

**C1, the classic trap: every LLM stack falls into it, and it hurts the
patient twice.**
1. The move is applied, the answer times out, and the agent retries.
2. The tool server answers 409, because the patient already holds that
   slot.
3. The agent then tells the patient the slot "was taken by someone else".
4. It offers other slots and moves them again.

The patient ends up with wrong information and, often, an appointment
they did not ask for. Five of these moves were flagged as unconfirmed by
the automatic judge. On review they were confirmed, or the patient
explicitly asked for them, so the table counts them as 0. The harm is the
duplicate and the false "taken", counted above.

Tessera never re-sent. Its http plugin's timeout is ambiguous, the action
declares a probe, the probe read the appointments and found the move had
happened, and the run finished as a success. Its C1 p95 is 20 s: it waits
out its own timeout before probing.

**C3, a delivered-twice message becomes a confirmation (Agents SDK, 4
runs).** The copy arrived right after the agent asked "Posso confirmar?".
The agent read the copy as the yes and moved an appointment the patient
had not confirmed. The fault fired on "Sim, queria remarcar para 14/10"
and the harness delivered it twice. This is the channel-retry hazard
Tessera's journal exists for. Tessera moved nothing twice. In 40 of its
51 fired runs it answered the copy with "não entendi", which is a UX
glitch, not a harm.

**Tessera's losses: product flaws the smoke run had already shown, not
fixed before the run, as pre-registered.**

1. **The wrong appointment (t15, 12 of 12 runs across conditions).** The
   patient asks to move her *mother's* appointment. The Function takes the
   patient's only appointment and confirms "Confirmo a remarcação para
   terça 13/10 às 11h?" without saying which appointment. She says yes and
   *her own* appointment moves. Every LLM stack refused or explained.
2. **A change of mind at the confirmation cancels (t18, t19).** "Mudei de
   ideia, prefiro X" is taken as a no ("Tudo bem, não mudei nada"). The
   patient has to start over, and some simulated patients gave up. The
   LLM stacks simply switched slot.
3. **A second confirmation.** When the patient's own message already says
   "confirmo esse horário", Tessera still asks its confirmation question.
   One simulated patient took the task as done and left, and nothing
   moved (t10, 1 run).

**Two costs of Tessera's approach, visible in the transcripts.**
- Its messages are the Function's fixed templates: plain, and sometimes
  long (the slot list is 20+ options). The LLM stacks wrote friendlier,
  shorter, formatted messages.
- The router and the choice questions are Jev decisions. That is why it
  is fast and cheap, and also why it cannot talk its way around a request
  its Function does not model, such as "my mother's appointment".

## Limits of this benchmark

- One domain, 20 synthetic tasks, one language, one model family, one
  full run. With 60 runs per cell, differences under about 10 points in
  success are within the intervals.
- The patient simulator is an LLM. Its choices, such as leaving after a
  second confirmation or repeating a request after a crash, shape the
  success numbers of every competitor. The conditions' fault effects
  (duplicates, re-sends) do not depend on it.
- The yes judge is a regular expression. It was reviewed by hand on
  every flagged move, but moves it let through were not all read.
- Tessera's Function was written by the platform's author. The
  competitors were written from their docs and reviewed, but not by their
  authors.
- Out of scope: open-ended requests outside the declared Functions. The
  LLM stacks win there by construction, and t15 is a small instance of
  that.

## What changes

- **market-002** fixes the three Tessera flaws:
  - confirmations name the object of the effect;
  - a change of mind at a confirmation re-resolves the input instead of
    cancelling;
  - an explicit confirmation in the user's own message.

  It re-runs the same pre-registration on held-out tasks, so the fixes
  are not tuned to these 20.
- The C1 result (0 of 50 against 29–48% duplicates) and the cost and
  latency are the claims this benchmark supports. Success parity is not:
  Tessera lost it here.
