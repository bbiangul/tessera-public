# Free mode 001 (RFC-0004 part B): results

- **Date:** 2026-09-24
- **Pre-registration:** [freemode-001-preregistration](freemode-001-preregistration.md).
- **Code measured:** commit `fbca890`. Results are in
  `bench/results/freemode-001`:
  - `offscript/` holds the runs and the off-script judge's verdicts;
  - `market/` holds the runs;
  - `naturalness/` holds the judged pairs.
- **Spend:** $3.65 of the $7 budget:
  - off-script runs $0.34, their judge $1.13;
  - market runs $0.12;
  - naturalness judge $2.06.

## Results

### Off-script, held out (seed 32, 360 conversations)

| Competitor | Success (95% CI) | State right | Answered | False claims | Dead ends | Humane (1–5) |
| --- | --- | --- | --- | --- | --- | --- |
| **tessera** | **95%** (86–98) | 60 | 57 | 3 | 2 | 4.18 |
| plain | 98% (91–100) | 60 | 60 | 1 | 0 | 4.70 |
| langgraph | 98% (91–100) | 60 | 60 | 1 | 0 | 4.68 |
| langgraph-sqlite | 98% (91–100) | 60 | 60 | 1 | 0 | 4.73 |
| agents-sdk | 100% (94–100) | 60 | 60 | 0 | 0 | 4.63 |
| agents-sdk-sqlite | 100% (94–100) | 60 | 60 | 0 | 0 | 4.72 |

The baseline, on the seed-31 set, was: Tessera 22% success, 47 dead
ends in 60 runs, humane 1.8.

Tessera by kind, successes out of runs:

| Kind | Tessera |
| --- | --- |
| side-question | 18/18 |
| info-only | 12/12 |
| uncovered-action | 12/12 |
| small-talk | 6/6 |
| over-reach | 6/6 |
| new-request-midway | **3/6** |

- The literal "não entendi" appeared 0 times, down from 47 runs.
- In 21 runs a side question was answered and the pending question was
  asked again.
- The other stacks' 3 misses are judge flags: an invented original date
  (the date was real), a cancellation "already forwarded", and a "hoje"
  without basis.

### Market (seed 24, Tessera, 240 conversations)

| Condition | Fired | Success | Duplicates | Unconfirmed moves | p50 / p95 turn |
| --- | --- | --- | --- | --- | --- |
| C0 | 60 | 100% (94–100) | 0 | 0 | 1.89 / 9.78 s |
| C1 | 48 | 100% (93–100) | 0 | 0 | 3.18 / 21.8 s |
| C2 | 48 | 100% (93–100) | 0 | 0 | 1.87 / 3.45 s |
| C3 | 48 | 100% (93–100) | 0 | 0 | 1.67 / 3.39 s |

**The C0 p95 is a cold start.** The 12 slowest turns are exactly the
first turns of the first batch of 12 parallel conversations (8.9–10.3 s).
Without that batch, C0 has p50 1.95 s and p95 3.65 s. The C1 p95 is the
fault's own hold.

### Naturalness (300 pairs, both orders)

Tessera's wins / losses / ties against the competitors' market-003
transcripts:

| | Baseline | voice-001 | **freemode-001** |
| --- | --- | --- | --- |
| Natural | 0 / 281 / 19 | 38 / 154 / 108 | **26 / 179 / 95** |
| Clear | 0 / 240 / 60 | 0 / 159 / 141 | **0 / 161 / 139** |
| Efficient | 21 / 42 / 237 | 18 / 59 / 223 | **18 / 32 / 250** |
| Natural, win share of decided | 0% | 20% (15–26) | **13% (9–18)** |

The judge flagged 19 distinct Tessera transcripts for a factual error.
On review they are the same kinds as voice-001:
- the period relevance defect: all of a day's times, the morning
  included, offered for an afternoon request (13);
- the judge's own date errors: 2026-10-02 is a Friday (4);
- the t13 loop described below (1);
- an "inconsistency" with no wrong fact (1).

None is a wrong date, time, appointment or action.

## Hypotheses

| # | Hypothesis | Verdict |
| --- | --- | --- |
| B1 | Off-script success ≥ 80% | **Supported:** 95% (86–98), up from 22%. |
| B2 | Dead ends ≤ 5% | **Supported:** 2 of 60 (3%), both in t18, down from 47 of 60. |
| B3 | 0 effects outside Functions; 0 false claims of an action | **Not supported.** 0 effects outside Functions: state right in 60/60, and the free mode cannot act by admission. But 3 runs (all t18) carry a false-claim flag. The free mode did not cause them: the router and the voice did (below). |
| B4 | Market C0 ≥ 97%; 0 duplicates in C1 and C3; 0 unconfirmed moves | **Supported:** 100% in every condition, 0 and 0. |
| B5 | Natural ≥ 30% and clear ≥ 20% of decided pairs | **Not supported:** natural 13%, lower than voice-001's 20%; clear 0%. Only efficiency improved: losses fell from 59 to 32. |
| B6 | p50 turn ≤ 2.0 s in C0 | **Supported, narrowly:** 1.89 s (competitors 2.1–2.3 s in market-003). |

## What the failures show

**1. t18, "book a new cleaning" after a reschedule (3 of 3 runs).**
- The reschedule finishes. The patient then asks "consegue agendar a
  limpeza nova?".
- **Router:** Jev sends the new request to the Reschedule Function, not
  to the free mode. Nothing in the agent says the Function only moves
  existing appointments. It starts a second reschedule of the
  appointment that was just moved. The patient says no, so nothing moves
  and the state stays right.
- **The voice changed the action:** in one run its day question said
  "Para qual dia você quer **agendar**?", while the template says
  "remarcar". The patient then believed a new booking was happening.
  The voice locks facts (values) but not the action's verb.
- In another run the Function's day question loops, then escalates
  ("algo deu errado").

**2. Re-greeting in the middle of a conversation (21 of 60 off-script
runs, 2 of 60 market runs).**
- A later agent message opens with "Oi, Diego!":
  - the free answer greets despite the rule;
  - repeating the pending question word for word also repeats the
    greeting the voice had put in it ("Oi, Marina! Claro, te ajudo a
    remarcar…").
- The judge names this as repetition and a loop.

**3. t13, a question the Function could answer.**
- At the day question the patient asks "quais horários específicos tem
  nesses dias?".
- The classifier calls it a question, and the free mode answers "não
  consigo ver por aqui": it has no slot read, while the Function has the
  slots.
- The pending question then repeats with its greeting.

**4. Naturalness fell from 20% to 13%.** Keyword counts over the judge's
reasons in lost "natural" pairs:

| Reason | voice-001 (154 lost) | freemode-001 (179 lost) |
| --- | --- | --- |
| Less detail (dentist, full date, current appointment) | 48% | **63%** |
| Warmth (name, greeting, closing) | 42% | **55%** |
| Formatting | 30% | 29% |
| Repetition or a loop | 43% | **21%** |
| Relevance (period) | 23% | 17% |
| An extra step | 23% | 12% |

- The flow fixes worked. Loops and extra steps fell by about half, and
  efficiency losses fell from 59 to 32.
- What the judge now penalises most is **content**:
  - the other stacks name the dentist, the full date and the current
    appointment, and structure the confirmation in lines;
  - Tessera's templates do not carry those facts, and the voice may not
    add facts, by design.
- Warmth also counts more. The voice now greets only in the first
  message, and a direct confirmation that skips the one-slot question
  leaves fewer messages to be warm in.

This is an observation from keyword counts over one judge, not a
measured cause.

## Conclusion

Part B does what off-script handling needed:
- success rises from 22% to 95%;
- dead ends fall from 47 to 2;
- no effect happens outside a Function;
- the fault guarantees and C0 success hold, still under 2 s p50.

It does not make the conversation more natural in the judge's eyes. The
remaining gap is:
1. **Content:** templates without the detail the others give. This is an
   authoring rule (the confirmation and the offers name the dentist, the
   full date and the current appointment), plus formatting where the
   channel allows it.
2. **Re-greeting:** a platform defect, to fix before the next run:
   - repeat the pending question in the voice as a continuation rather
     than word for word;
   - drop a greeting after the first agent message deterministically,
     rather than by instruction.
3. **Routing a new request to a Function that only resembles it:**
   - an authoring rule: Functions say what they do not do;
   - a voice check that keeps the action's verb.
4. **Process questions at a pending question** (t13): the free mode should
   be able to read what the pending step offers.

Seed 32 is now spent. The next measured run of these fixes needs a new
held-out seed.
