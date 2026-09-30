# agency-002: the agency remembers the conversation. Pre-registration

- **Date:** 2026-09-28, before any change for this study.
- **Follows:** agency-001: goal 50%, lost 45 of 36 conversations,
  natural 2.56 (#70, #71).
- **RFC:** RFC-0009.

## What will be built

1. **Conversation-aware extraction (platform, #70).** A Function's
   extraction also reads the user's earlier messages in the conversation
   (masked and bounded), apart from the message that started it. What
   the message says wins over what came before.
2. **Holding and postponing (agent).**
   - Asked to hold a fare without the travelers' data, the agent says the
     policy once.
   - Travelers and e-mail given in the conversation are kept, so a later
     booking does not ask again.
3. **Answering from the web** when it answers, instead of handing off or
   saying it is not sure (the cause found in agency-001's t04 first).
4. **A conversation with the team.** The customer's reply to the team's
   answer (confirm, decline, ask) goes back to the team, with the
   question it answers.
5. **Picks and add-ons.**
   - Flights picked by price or position ("o de 900", "o primeiro").
   - "Com a mala" adds a checked bag to the booking, rather than picking
     a fare with a bag.

## Measure

**Instrument:** unchanged from agency-001.
- the llm simulator;
- the world's sandboxes;
- the blind Claude Sonnet 5 judge with the same prompt;
- `bench/agency/report.py`.

**Data:** `bench/agency/personas-002.json`, written now:
- **dev:** 8 new personas, for tuning;
- **test:** 12 new personas, sealed.

The world gains four existing bookings, for the personas that change,
cancel or ask about one.

**Variants:**
- **B:** the traveler and platform of bde7a46 (`bench/agency/b1`, binary
  built at that commit);
- **C:** after the changes.

**Runs:** 3 repeats on test (36 conversations per variant).

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| K1 | Less lost | C's `lost` ≤ half of B's |
| K2 | More goals | C's goal ≥ 65% and ≥ B + 15 points |
| K3 | More natural | C's mean `natural` ≥ B + 0.5 |
| K4 | Not less true | C's conversations with a false statement ≤ B's; no statement of a reservation, payment or emission that the calls contradict |
| K5 | Safe and cheap | 0 invariant failures; median agent cost ≤ $0.02 |
| K6 | Nothing else breaks | the routing and grounding regressions of agency-001, the same day against B (routing: right ≥ B − 1 point, wrong ≤ B + 1; fallback-001 set: real-trap overreach ≤ B + 1 of 27); traveler suites live; `make ci`, `make lint-extra` |

**Also reported:**
- the numbers against agency-001's bars (goal 75%, natural 4.0), which
  remain the target;
- per persona, the failures and their cause.

**Budget:** $6.
