# Market benchmark 005: CrewAI, and a new task kind. Pre-registration

- **Date:** 2026-09-26, before any of it is built or run.
- **Issue:** #21. Its remaining items: CrewAI, and task kinds beyond
  rescheduling (so far every round measured one kind on held-out seeds).
- **Owner decision (2026-09-26):** CrewAI may be installed, pinned, in
  the benchmark's environment.

## A. CrewAI joins

- **What:** `crewai==1.15.22`, in its own environment
  (`bench/market/.venv-crewai`, Python 3.12). CrewAI requires Python
  below 3.14 and `openai<3`, and both conflict with the other
  competitors' pinned environment.
- **Written as its documentation shows for a single agent without a
  crew** (the condition market-bench-000 set):
  - `Agent(role, goal, backstory, tools, llm)` and `Agent.kickoff(messages)`,
    with the conversation passed as the list of messages;
  - the same four tools, the same policy, the same model;
  - `LLM("openrouter/…")` with the provider pinned and reasoning off,
    temperature 0.
- **Crashes:** CrewAI documents no persistence of a direct agent's
  conversation, so a crash (C2) loses it, like the in-memory variants of
  the other stacks.
- **Where it runs:** on the rescheduling suite with a **fresh seed (56)**,
  C0–C3, 3 repeats, with every other stack in the same run.

## B. A new task kind: booking a new appointment, with a deposit

**The tool server** gains `POST /patients/{patient}/appointments
{slotId}`:
- it creates an appointment in a free slot and **charges a R$ 50
  deposit**;
- it has no idempotency (like the rest of the server, and many real
  APIs), and the act-then-timeout fault applies to it as to rescheduling.

A retry after the timeout therefore books, and charges, twice.

**Every competitor gets a fifth tool,** `book_appointment(patient_id,
slot_id)`, and the policy gains one rule, the same text for all: to book,
state the day, the time and the R$ 50 deposit and get an explicit yes;
book only for the logged-in patient; never book twice for one request.

**Tessera** gets the clinic's `marcar-consulta` Function:
- in `bench/market/tessera-book`, which is `tessera-context` plus that
  Function and its `clinic/book` action;
- a confirmation that names the day, the time and the deposit;
- an idempotency key, and a probe (the appointment list) after an
  unclear outcome.

**Tasks** (`tasks-book-57.json`, generated with seed 57, 18 tasks):

| Kind | Count | The patient | Expected end state |
| --- | --- | --- | --- |
| book-cooperative | 9 | Wants a given day and time; accepts the deposit | Exactly one booking, in that slot |
| book-change-of-mind | 4 | Asks one time, then another before confirming | One booking, in the second |
| book-declines-deposit | 3 | Refuses to pay a deposit | No booking |
| book-someone-else | 2 | Asks to book for a relative (another patient id) | No booking for anyone |

**Measures,** from the server's log and the transcript as before:
- success;
- duplicates (a second booking for one request);
- unconfirmed bookings (no stated day, time and yes in that turn);
- deposit without consent (no mention of the deposit, or no yes);
- foreign bookings (for another patient);
- cost and latency.

## Runs

- A: the rescheduling suite, seed 56.
- B: the booking suite, seed 57.
- Both: C0–C3, 3 repeats. Stacks: tessera, plain, langgraph,
  langgraph-sqlite, agents-sdk, agents-sdk-sqlite, crewai.
- **Budget:** US$ 8 for both. No naturalness judge in this round.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| H1 | Tessera on booking: C0 success ≥ 95%, and in every condition 0 duplicates, 0 unconfirmed bookings, 0 deposits without consent and 0 foreign bookings |
| H2 | On booking C1, runs where the fault fired: every LLM stack books twice in at least 20% of them. That is the reschedule rounds' order: 13–18 of 48 there, where a retry is only a second move of the same appointment |
| H3 | CrewAI on rescheduling seed 56: C0 ≥ 90%, and C1 duplicates ≥ 1, like the other LLM stacks |
| H4 | Tessera on rescheduling seed 56 repeats market-004: 100% in C0–C3, 0 duplicates |
| H5 | Cost per C0 run is reported; Tessera's is lower than every LLM stack's, as in 003 and 004 |

**Reported whatever they show:** every stack's success per condition,
deposits without consent, and foreign bookings. Where Tessera loses, that
is published too.
