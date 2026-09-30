# Market benchmark 005: CrewAI, and booking with a deposit

- **Date:** 2026-09-26.
- **Pre-registration:** [market-bench-005-preregistration.md](market-bench-005-preregistration.md),
  committed before anything was built or run.
- **Issue:** #21.
- **Results:** `bench/results/market-005/`:
  - `reschedule-56`: 1,680 runs;
  - `book-57`: 1,512 runs.
- **Spend:** about US$ 4.
- **The two runs:** 7 stacks, C0–C3, 3 repeats each.
  - A: rescheduling, fresh seed 56, Tessera with `tessera-context`.
  - B: the new task kind, booking with a R$ 50 deposit (seed 57), Tessera
    with `tessera-book`.

C1–C3 count only the runs where the fault fired.

## Summary

- **Rescheduling (A): the result of 003 and 004 holds on a fresh seed.**
  - Tessera: 100% in C0–C3, with 0 duplicates.
  - The LangGraph, Agents SDK and plain stacks: 98–100% in C0, 88–94% in
    C1, with 8–14 duplicates in 48–49 fired runs.
- **Booking (B), the new task kind:**
  - Tessera: 100% in C0, C1 and C3, and 97% in C2. The one failure is
    explained below.
  - It made no duplicate, no booking without a stated yes, no deposit
    without consent and no booking for someone else, in any condition.
  - The LLM stacks: 5–13% duplicates in C1. **That is fewer than the 20%
    I predicted.** They re-sent after the timeout almost every time (38–39
    of 39), but a re-send of the same slot hits "slot not available",
    because their own first booking holds it. A duplicate needs the agent
    to book a *second* slot, as it does when it tells the patient the
    slot "was just taken".
- **CrewAI:**
  - Success is on par (96–100%).
  - **It made 0 duplicates in C1 in both runs, unlike the other LLM
    stacks,** because after the timeout it rarely re-sent.
  - **It made 2 duplicates and 2 unconfirmed bookings in C3** (the same
    message delivered twice), where no other stack made any.
  - It costs about 10× per run: 40–62 model calls per conversation,
    against 4–5.
- **Cost:** Tessera is the cheapest per C0 run in both runs, but on
  booking only narrowly: US$ 0.00028 against 0.00029–0.00031.

## Rescheduling, seed 56 (A)

| Stack | C0 | C1 success | C1 duplicates | C1 re-sent after applied | C2 | C3 | Cost / C0 run | Model calls / C0 run |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **tessera** | **100%** | **100%** | **0 / 48** | **0** | 100% | 100% | **$0.000267** | 9.7 |
| plain | 98% | 94% | 8 / 48 | 44 | 94% | 100% | $0.000385 | 5.5 |
| langgraph | 100% | 88% | 14 / 49 | 48 | 100% | 100% | $0.000364 | 5.4 |
| langgraph-sqlite | 100% | 94% | 9 / 49 | 44 | 100% | 100% | $0.000360 | 5.3 |
| agents-sdk | 98% | 94% | 8 / 48 | 45 | 94% | 100% | $0.000373 | 5.4 |
| agents-sdk-sqlite | 100% | 90% | 14 / 48 | 45 | 100% | 100% | $0.000372 | 5.3 |
| crewai | 98% | 100% | **0 / 48** | 2 | 98% | 100% | $0.003928 | 62.4 |

In C3 every stack made 0 duplicates in rescheduling.

## Booking with a deposit, seed 57 (B)

| Stack | C0 | C1 success | C1 duplicates | C1 re-sent | C2 | C3 | C3 duplicates | Deposit without consent | Cost / C0 run |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **tessera** | **100%** | **100%** | **0 / 39** | **0** | 97% | 100% | 0 | 0 | **$0.000282** |
| plain | 100% | 87% | 5 / 39 | 39 | 100% | 100% | 0 | 0 | $0.000308 |
| langgraph | 100% | 92% | 3 / 39 | 38 | 100% | 100% | 0 | 0 | $0.000303 |
| langgraph-sqlite | 100% | 90% | 4 / 39 | 39 | 100% | 100% | 0 | 0 | $0.000308 |
| agents-sdk | 100% | 90% | 4 / 39 | 39 | 100% | 100% | 0 | **1** | $0.000299 |
| agents-sdk-sqlite | 100% | 95% | 2 / 39 | 38 | 100% | 100% | 0 | 0 | $0.000293 |
| crewai | 96% | 100% | **0 / 37** | 12 | 100% | **90%** | **2** (+2 unconfirmed) | 0 | $0.002760 |

## Against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| H1 | Tessera booking: C0 ≥ 95%; 0 duplicates, unconfirmed, deposits without consent or foreign bookings in every condition | **Held.** C0 100%. One C2 run failed on success (below), without any of those violations |
| H2 | Every LLM stack duplicates in ≥ 20% of fired C1 booking runs | **Did not hold.** 5–13% for LangGraph, Agents SDK and plain, and 0% for CrewAI. The premise missed that a re-send of the same slot is refused as "not available" |
| H3 | CrewAI rescheduling: C0 ≥ 90%, and C1 duplicates ≥ 1 | **Half held.** C0 98%, but **0 duplicates in C1**; see CrewAI below |
| H4 | Tessera rescheduling seed 56 repeats 004: 100% C0–C3, 0 duplicates | **Held** |
| H5 | Tessera's cost per C0 run below every LLM stack's | **Held**, narrowly on booking (4% below the cheapest) |

## What the transcripts show

**How an LLM stack books or moves twice** (LangGraph, rescheduling C1,
the same pattern in booking):
1. The call applies, and the answer arrives after the client gave up.
2. The re-send gets "slot not available", because its own first call
   holds the slot.
3. The agent tells the patient the time "was just taken by someone else",
   offers another, and moves the appointment again.

The patient ends up with a second change they never needed, and a false
explanation.

**CrewAI:**
- After the timeout it rarely called the tool again (2 of 48 runs in
  rescheduling).
- In the transcripts read, it told the patient the change was done. That
  was true here, because the call had in fact applied.
- This benchmark does not measure what it would say when a timed-out call
  had not applied, so its 0 duplicates are not evidence that it handles
  an unknown outcome. That is a hypothesis for a future condition: a
  timeout *before* the effect.
- In C3 (the patient's message delivered twice) its conversation carries
  the message twice. In 2 of 39 runs it booked twice, and in 2 it booked
  without a yes in that turn.

**Tessera's one booking failure** (C2, b12, a change of mind):
1. The patient asked for 14/10, then said "mudei de ideia, prefiro quarta
   07/10 às 15h".
2. The confirmation was asked again for 14/10: the correction was not
   taken.
3. After the crash, the free mode answered that it could not confirm the
   booking.

In C0 the same change-of-mind tasks passed (12 of 12), so this is not
every time. It is a real weakness of correcting a pending confirmation,
and an issue was opened for it.

## Cost and calls

- Tessera makes about twice the model calls of LangGraph, the Agents SDK
  and plain: 9.7–10.6 against 4–5. Most are Jev decisions, which cost
  little, and it stays the cheapest per run.
- CrewAI's direct agent makes 40–62 calls per conversation, at about 10×
  the cost, with the same model.

## Setup notes

- **CrewAI 1.15.22** (the owner approved the install), in its own
  environment: it needs Python below 3.14 and `openai<3`.
- **Telemetry:** CrewAI sends telemetry by default. The benchmark's server
  turns it off (`OTEL_SDK_DISABLED`, `CREWAI_DISABLE_TELEMETRY`,
  `CREWAI_DISABLE_TRACKING`, checked in its source).
  - One smoke call made while installing, before telemetry was off, may
    have sent CrewAI's anonymous usage telemetry.
  - Its content was synthetic and trivial ("Responda só: ok").
- **The fifth tool and policy rule** (booking) were given to every
  competitor in both runs, so the rescheduling run is not identical to
  004's setup.
