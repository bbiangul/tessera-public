# ADR-0007: agent mode, an LLM plans the turn (experimental)

- **Status:** experimental, 2026-09-28. The owner approved trying it
  ("vai", after agency-003 recommended it).
- **Amends:** ADR-0003, for one opt-in mode, until agency-004 measures it.

## Context

- **Three studies, a long tail.** agency-001..003 moved the traveler from
  39% to 58% of goals and from 1.7 to 3.25 natural. Each new set of
  customers brought cases no Function or free-mode rule foresaw.
- **Where the router and the free mode fall short** is the dialogue
  between processes:
  - two requests in one message;
  - "the same a week later";
  - a follow-up that needs a read and then an answer.
- **An LLM planning tool calls** handles that kind of dialogue well. It is
  also an LLM decision path, which ADR-0003 paused.

## Decision

1. **An opt-in agent mode** (`Agent.spec.agentMode`, per audience).
   - The Generate engine plans each turn: which declared tool to call,
     with which arguments, and when to answer.
   - Its tools:
     - the audience's Functions (started as runs, with their own inputs,
       confirmations, approvals and idempotency);
     - read-only actions;
     - document search and web search.
2. **Jev keeps every decision that needs calibrated confidence:**
   - whether a message answers a pending question or confirmation (the
     measured interruption classifier);
   - the `decide` steps inside Functions;
   - every gate before an effect (confirmation, approval, the claims
     guard, the voice's claims check).
3. **The planner never acts.** Effects happen only in Functions, so every
   guarantee of an effect is the Function's, as today.
4. **Its replies pass the free mode's checks:**
   - grounding against what the tools returned and what the agent said;
   - the claims guard.
5. **Small model first** (ADR-0002): DeepSeek V4.1 Flash plans first. A
   larger model is tried only if it fails, and where it fails is recorded.
6. **It stays experimental until agency-004 compares it with the router
   on new personas.** Adopted only if it wins there with no safety cost.

## Consequences

- One more path to test. The default stays the router and the free mode.
- If it wins, a later ADR states where each engine decides.
