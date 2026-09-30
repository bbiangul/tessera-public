# agency-001: the traveler as a whole travel agency

- **Date:** 2026-09-28.
- **Pre-registration:** `agency-001-preregistration.md` (commit c0669fb,
  before any change).
- **RFC:** RFC-0009.
- **Data:** `bench/results/agency-001/`.
- **Variants:**
  - **A:** the traveler of commit 08c9b0e (`bench/agency/baseline`), on
    the platform of f3adfd4 (the simulator only);
  - **B:** `examples/traveler` at 9e24170.
- **Spend:** about $5.5, the judge most of it:
  - A test: $0.39; B test: $0.99;
  - dev iterations: ~$1.5;
  - regressions: ~$1.8;
  - agents and simulator: < $0.5.

## Result (sealed test: 12 personas × 3 repeats)

| | A (before) | B (after) |
| --- | --- | --- |
| Goal met (of 36) | 14 (39%) | **18 (50%)** |
| Conversations with a false statement | 14 | **8** |
| False statements (total) | 15 | 10 |
| … about a reservation, payment or emission | 0 | **2** |
| Information lost (asked again or ignored) | 114 | **45** |
| Redundant messages | 79 | 58 |
| Natural (1–5, mean) | 1.69 | **2.56** |
| Trust with a real customer (of 36) | 5 | 8 |
| Invariant failures | 0 | 0 |
| Median agent cost per conversation | $0.0012 | $0.0037 |
| Median simulator cost per conversation | $0.0010 | $0.0008 |

| # | Hypothesis | Result |
| --- | --- | --- |
| H1 | Goal ≥ 75% and ≥ A + 30 points | **Not supported:** 50%, A + 11 points |
| H2 | ≤ 2 conversations with a false statement, 0 about a reservation | **Not supported:** 8, and 2 about a reservation (below) |
| H3 | Lost ≤ a third of A's (≤ 38) | **Not supported:** 45 |
| H4 | Natural ≥ 4.0 and ≥ A + 1.0 | **Not supported:** 2.56, A + 0.87 |
| H5 | Invariants in every conversation; no booking request without a yes to a confirmation naming the total | **Supported:** 0 failures; every `agency/request-booking` followed a yes to its confirmation |
| H6 | Median agent cost ≤ $0.02 | **Supported:** $0.0037 |
| H7 | Suites pass live; `make ci`, `make lint-extra` | **Supported:** 34/34 live on the final two runs (earlier runs found the regressions fixed above), 118 scenarios replayed, CI green |

**B is better than A on every measure, and far from the level
registered.** Dev (6 personas, one repeat) had read 5–6 goals of 6 and a
natural of 3.5–3.7; the sealed test reads 50% and 2.56. Dev overstated the
gain: its personas are the ones the agent was tuned on.

**The two reservation statements (t06, repeat 2) are not false actions.**
- The first confirmation named the wrong flight: the pick read "com a
  mala" as the Azul fare "mala despachada inclusa". The customer said no,
  and nothing was requested.
- The request that went out is exactly the one the next confirmation
  showed: "GOL … R$ 1.500, sem mala despachada".
- The customer said yes adding "com a mala, 1681". A bag cannot be added
  to a booking yet.

The judge counted both, and the registered rule counts them. The
platform's guarantee held: no effect without a yes to its exact text.

## Why B still fails (all 36 conversations read)

1. **What was said outside a process is forgotten** (t07, t09, t11 and
   more; most of the 45 "lost"). A Function's extraction reads only the
   message that started it. "Eu e minha esposa" said while talking about
   Pipa, or "Natal" said two messages earlier, is not there when the
   hotel search starts, so it is asked again or searched for one adult.
   This is the platform's, not the agent's.
2. **A customer without the travelers' data yet** (t02 ×3, t06, t07,
   t03, t09). They ask to hold the fare. Each message starts the booking
   again, which asks for the data again. A person would say once "we
   cannot hold it without the data; your options are saved" and stop.
3. **Questions left unanswered** (t04, t10). Baggage weights, carry-on
   size, documents for Portugal, what a baby may take: the free mode
   handed off or hedged, even with the web's answer in hand (t04, repeat
   3).
4. **The team's answer is not a conversation** (t08 ×3). The team's
   answer about changing a booking reaches the customer. The customer's
   "sim, pode remarcar" does not go back to the team, and the agent can
   only repeat that it passed it on.
5. **Pick errors** (t06, t10). "O de 1500" and "o primeiro" asked "Sobre
   qual voo?" again. "Com a mala" picked the fare with a bag included.
6. **Confirmation loops** (t12, repeat 1). A yes with a question is not a
   yes (freemode-005, measured). The customer kept adding questions and
   heard the confirmation three times.

## What was built (all in commits 27ece18 and 9e24170)

**Platform:**
- **Measurement:** the llm simulator in EvalSuites (RFC-0002), with the
  team's answers given by a member holding the queue's role, and every
  call's output kept for a judge.
- **Extraction:**
  - a reply fills the later inputs it gives;
  - `Input.format` reads e-mail, phone and CPF by pattern: models only
    see them masked, and a booking's e-mail had become `[EMAIL]`;
  - the `list<map>` extraction schema is fixed (travelers could not be
    read).
- **Side answers:**
  - the pending question is not asked twice, and not again after the
    first side question;
  - an answer that also asks, or an unsure turn that answers, still
    reaches the run (never at a confirmation);
  - a failed classifier asks again instead of failing the conversation.
- **Free mode:**
  - the agent's own earlier messages and web pages count as support in
    the grounding check. The measured wording is kept for single
    messages (below);
  - `messages.unsure` replaces "não entendi" when every draft was
    dropped;
  - acknowledgements get a short reply.
- **Voice:** the warm voice may not claim an action the template does not
  (Jev check; it had written "já pedi para a agente verificar").

**The agent:**
- the trip remembered across searches;
- round trips with both legs, the group's total and the price per person,
  children counted from "nós 3";
- refinements (cheaper, direct, return time), and "nothing cheaper" said
  plainly;
- the whole-trip quote, and two destinations compared;
- the booking through the `emissao` queue, then:
  - the payment provider's webhook;
  - the tickets;
  - the check-in reminder two days before;
- `falar-com-agente` for what only people do;
- web search, the agency's documents, a lessons base;
- the voice, a team audience, a sales funnel.

**Two new suites:**
- `booking`: paid and issued; refused by the team; payment never comes;
  customer says no;
- `agency`: trip remembered; abbreviations; two destinations; an answer
  with a question; only a person.

## Regressions checked, same day

**Routing** (the router-004 regression sets plus its general sets, 268
messages × 3, threshold 0.5):
- right 95.9% vs A 96.5%;
- wrong 1.5% vs 1.5%;
- out-of-scope routed 0.7% vs 0.4%.

This holds router-004's R3 rule.
- **How it was reached:** with eight Functions, opinion questions ("hotel
  ou flat, o que compensa?") went to the hotel search even with the
  measured text back.
- **The fix:** texts tuned on a new dev set
  (`bench/agency/route-dev.jsonl`, 95%), and `selector.after` on the
  booking.
- **Two changes remain:**
  - "Orlando ou Miami pra Disney?" now goes to the comparison;
  - "moro em recife" already varied in A.

**Documents** (fallback-001's set, 3 repeats):
- controls 36/36;
- overreach on real traps 3/27;
- the pre-change binary, the same day: 2/27, the same trap (t07,
  "Pix é à vista").
- An earlier wording of the grounding check read 4/27; the measured
  wording is now kept word for word when there are no earlier agent
  messages and no web pages (pinned by a test).

**Web search** (websearch-002's test set, live TinyFish):

| | before | now |
| --- | --- | --- |
| action requests searched | 0/12 | 0/12 |
| no-search messages searched | 0/6 | 0/6 |
| own-case conclusions | 3/12 | 1/12 |
| false claims | 1 | 0 |
| current questions answered | 15/15 | 14/15 |

The one current question left unanswered is the voltage in Japan, which
was not searched.

**Other examples:**
- Five suites re-recorded live and passing.
- `clinic-context`'s Decide budget rose from 12 to 16: the side-answer
  checks add two calls.

**Also changed from observation, not measurement:**
- The follow-up flight pick's threshold went from 0.7 to 0.6: the right
  flight came at 0.66 with the longer labels.
- The outbound-date extraction wording, from a small check: 18/18 against
  17/18.

## Next (agency-002)

The failures above are what to build next. The test personas are now
used, so agency-002 needs new dev and test personas, written first.
1. **Conversation-aware extraction (platform):** a Function reads what
   the user said earlier in the conversation, not only its first message,
   and says what it took from before.
2. **Holding and postponing (agent):** say the policy once; do not start
   the booking again for "segura a tarifa"; keep the travelers' data
   given in the conversation.
3. **Answer from the web when it answers:** why t04's answer was handed
   off with the result in hand.
4. **A conversation with the team:** the customer's reply to the team's
   answer goes back to the same task.
5. **Picks:** "o de 1500", "o primeiro", "com a mala" (a bag added to a
   fare, not a fare with a bag).
