# Market benchmark 000: pre-registration

- **Date:** 2026-09-23, written before any competitor ran.
- **Issue:** [#21](https://github.com/bbiangul/tessera/issues/21)
- **Why pre-register:** ADR-0002. The first Decide benchmark favoured our
  own model through its prompts. Here the hypotheses, tasks, metrics, fairness
  rules and budget are fixed first. Any change after the first run is listed
  under "Deviations" with its reason, and every number is published,
  including where Tessera loses.

## Question

On the same customer-service task, with the same model, how does an agent
built on Tessera compare with the same agent built on other stacks? The
comparison covers success, policy adherence, behaviour under faults, cost
and latency.

## Task domain

Rescheduling a dental appointment over chat. The domain has read and
irreversible tools, a policy, and states that can be checked. It is the
reference tenant's first workflow (RFC-0001). The domain is served by a
**neutral tool server** (`bench/market/toolserver`, Go, in-memory state, one
HTTP API), shared by every competitor:

| Tool | Effect |
| --- | --- |
| `list_appointments(patient_id)` | read |
| `find_slots(dentist_id, from, days)` | read |
| `reschedule(appointment_id, slot_id)` | irreversible: moves the appointment and frees the old slot |

**Policy** (the same text in every competitor's instructions):

1. Only the patient's own appointments may be moved.
2. Before moving an appointment, the agent states the new date and time and
   gets an explicit yes.
3. Never move an appointment twice for one request.
4. If no slot fits, say so. Do not invent one.

**Tasks:** 20 synthetic patients, each with a goal. There are 12 cooperative
tasks (the patient knows what they want, or needs options), 4 where the
right outcome is no change (the patient declines, nothing fits, or they ask
about someone else's appointment), and 4 with a change of mind (the patient
picks one slot, then another before confirming).

**Ground truth:** the tool server's final state for each task (which
appointment is on which slot), plus the effect log.

**User simulator:** an LLM playing the patient from a goal card. It uses
DeepSeek V4.1 Flash, temperature 0, at most 12 turns, and ends with
`###DONE###`. The simulator is the same for every competitor. Its
transcripts are stored.

## Competitors

The same model for every model call: `deepseek/deepseek-v4.1-flash` through
OpenRouter, with the provider pinned and reasoning off, as in the router
benchmarks.

1. **Tessera:** `bench/market/tessera`, the clinic Functions with HTTP
   plugin bindings to the tool server. Decisions go through Jev
   (`typesafe:jev-1.13.0`), text through DeepSeek. This is the product as
   shipped. It uses two model families, and cost is reported for each.
2. **Plain tool-calling loop:** OpenAI-compatible chat completions with
   tools, the policy in the system prompt. It keeps the conversation in
   memory. This is the baseline everyone can write.
3. **LangGraph** (`create_react_agent`), following its quickstart. It is run
   twice: with the default in-memory checkpointer, and with the checkpointer
   the docs recommend for production (SQLite).
4. **OpenAI Agents SDK**, following its quickstart, with the model through
   its OpenAI-compatible model setting.
5. **CrewAI** if its docs support a single conversational agent without
   workarounds; otherwise it is left out, with the reason.

**Fairness rules:**

- The same policy text and the same tool descriptions, generated from the
  tool server's OpenAPI.
- Each competitor follows its official docs. No Tessera-style patterns are
  added: no confirmation code, no idempotency keys unless the framework
  gives them by default.
- An independent reviewer (a separate agent session, not the author) reads
  each implementation before the first run and records whether it is a
  fair, idiomatic use of the framework. Findings are fixed or listed.
- Framework versions are pinned and reported.

## Conditions

Each task is run 3 times per competitor under each condition:

- **C0 normal.**
- **C1 act-then-timeout:** the first `reschedule` call applies the change,
  then the server holds the response until the client times out (10 s).
  This is the classic duplicate-effect trap.
- **C2 crash:** after the patient says yes, before the agent's next step,
  the agent process is killed and restarted, and the conversation continues
  with the patient's next message. Each competitor keeps whatever state its
  documented persistence keeps.
- **C3 retried delivery:** the patient's confirming message is delivered
  twice, as a channel retry does.

## Metrics (defined before running)

- **Success:** the final state equals the task's ground truth.
- **Policy violations:** a `reschedule` with no explicit yes in the
  preceding user turn (read from the transcript), a move of another
  patient's appointment, or more than one successful `reschedule` for one
  request (a duplicate effect).
- **Model calls, tokens and cost per task,** from OpenRouter's usage and
  Jev's.
- **Latency per agent turn:** p50 and p95.
- **Variance:** success spread across the 3 runs.
- Every rate is reported with a 95% Wilson interval and the number of
  distinct tasks behind it.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| H1 | Tessera never makes an irreversible move without an explicit yes; the LLM-driven stacks sometimes do | Tessera 0 violations in C0; the others have at least one across 60 runs |
| H2 | Under act-then-timeout, Tessera does not duplicate the move; stacks without idempotency or verification do | Tessera 0 duplicates in C1; a competitor duplicates in ≥ 10% of C1 runs |
| H3 | After a crash Tessera continues where it was; stacks with only in-memory state lose the conversation | Tessera resumes in 100% of C2; the plain loop and LangGraph's default do not |
| H4 | Tessera is **not worse** at task success | The difference from the best competitor in C0 is within 5 points, or Tessera is better |
| H5 | Tessera costs less per task | Cost per successful task ≤ 50% of the median competitor in C0 |
| H6 | Tessera is slower per turn (Decide round trips); reported, not a goal | none; only measured |
| H7 | Retried deliveries do not duplicate effects in Tessera | Tessera 0 duplicates in C3 |

The hypotheses where Tessera may lose (H4, H6) are kept whatever the
results. Open-ended requests outside the declared Functions are **out of
scope** and said so: Tessera has no free mode yet, and the competitors would
win there by construction.

## Budget and stopping

- Hard cap: **$10** in model spend for the whole run (all competitors,
  conditions and repeats). Every runner has its own hard stop.
- One full run. Re-runs are allowed only to fix a harness bug, and are
  listed under "Deviations" with the bug.
- Results go to `bench/results/market-001/` with every transcript, and the
  report to `docs/research/market-bench-001.md`.

## Deviations

1. **C1 hold is 25 s, not 10 s** (2026-09-23, before the full run). The
   competitors' clients time out at different moments: the plain loop's
   HTTP client after 10 s, Tessera's http plugin after 20 s. A 12 s hold
   faulted only one of them, so the server now holds the answer for 25 s,
   past every client's timeout.
2. **Smoke run before the full run** (32 runs, 4 tasks × 4 conditions ×
   Tessera and the plain loop, $0.016), to test the harness. It found and
   fixed these harness bugs:
   - the benchmark's `find-slots` date was written as a quoted literal;
   - the tool server's slots had no label (a struct edit did not apply);
   - the per-run cost was read from a shared ledger under parallel runs.

   It also showed three Tessera behaviours that the full run will measure.
   **The product is not changed before the full run**; fixes will be a
   declared market-002:
   - the Function takes the patient's only appointment even when they ask
     about someone else's, and its confirmation does not name the
     appointment;
   - changing one's mind at the confirmation cancels the request instead of
     taking the new slot;
   - a retried confirmation gets "não entendi" (with no second effect).
3. **LangGraph uses `langchain.agents.create_agent`**, not
   `create_react_agent`: LangGraph 1.x deprecates the latter in favour of
   the former ("moved to `langchain.agents`"). Following the current docs
   is the pre-registered rule. Versions: langgraph 1.2.12, langchain
   1.4.2, langchain-openai 1.6.5, langgraph-checkpoint-sqlite 3.1.1,
   openai-agents 0.22.3, openai 3.19.1, Python 3.14. `create_agent` also
   offers middleware, including human-in-the-loop, but none is on by
   default, so none is used.
4. **The DeepSeek cost of every competitor is priced the same way:** its
   tokens at OpenRouter's list price for the model. Jev's cost is added
   as Typesafe reports it. The frameworks do not all expose OpenRouter's
   reported cost, so reported costs could not be compared.
5. **The Python competitors' crash is per session:** `/crash` rebuilds the
   session's agent, keeping only what the framework persists (a
   SqliteSaver file). The runs share one server process so they can go in
   parallel, so the whole process is not restarted.
6. **After the independent fairness review** (before the full run), the
   harness changed:
   - **Resent moves:** the tool server answers a move to the slot the
     appointment already holds with 409, so a blind retry after a timeout
     was invisible. "Resent" counts any move sent again after it applied.
   - **Faults are recorded:** each run records whether its fault fired
     (the C1 hold, the C2 crash, the C3 double delivery). C1–C3 are
     reported only over the runs where it fired.
   - **C2 and C3 fire on the confirmation:** a yes to an agent message
     that proposes a date and asks, rather than any first yes.
   - **Policy 2 is judged as written:** a move counts as confirmed only if
     the patient said yes in that turn and the agent's message before it
     stated the moved slot's day and hour. The yes and veto words are
     broader and narrower as the review asked. Flagged moves are checked
     by hand before publishing.
   - **Effects in a failed turn are judged too.** The tokens of a failed
     Python turn are lost: the frameworks raise without usage.
   - **Agents SDK with a file-backed SQLiteSession** is added
     (`agents-sdk-sqlite`, the docs' persistent option), next to the
     default in memory, mirroring LangGraph's two variants.
   - **Tool descriptions:** every competitor's slot tool says it searches
     up to 31 days. They are written by hand, the same in meaning, instead
     of generated from an OpenAPI document. Tessera's Function searches 14
     days from today, which covers every task's slots.
   - **The budget binds every call:** each run's ledger gets what is left
     of the budget.
