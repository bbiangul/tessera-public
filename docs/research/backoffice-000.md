# backoffice-000: the back office today (RFC-0006 baseline)

- **Date:** 2026-09-26.
- **RFC:** RFC-0006.
- **Issue:** #13.
- **Harness:** `bench/backoffice/bobench`.
- **Agent:** `bench/backoffice/agent`.
- **Data:** `bench/results/backoffice-000/baseline.json`.

**What the harness is:**
- One clinic agent that patients and the team both use, written with
  what Tessera has today (`caller.role`, `authorize`, queues without
  members).
- Each scenario runs through the real host, server and signed task
  links.
- Routing and extraction are deterministic fakes. The properties
  measured are the platform's, so a run is reproducible, free, and a
  snapshot of the code rather than a sample.
- The RFC's LLM-stack baseline (a loop and LangGraph with `interrupt()`)
  is not built yet.
- **Order, declared:** this baseline was run before the pre-registration
  was written, and backoffice-001's pre-registration came after it. The
  harness is deterministic (no model, fixed fakes), so the result could
  not change with the order. The order is recorded here all the same.

## Result

| Scenario | What happened | Finding |
| --- | --- | --- |
| approve-by-the-reception | The reception approves by link; the discount happens once | ok |
| **forwarded-link-approves** | A patient holding the reception's link approves their own discount, and it happens | **1 unauthorized effect, 1 approval from someone without the role**: the approver's name is typed, not verified |
| **api-holder-approves** | An API client answers as "anyone" | **1 approval from someone without the role**: no role on queues |
| replayed-approval | A used link is refused (the task hash) | ok |
| **big-discount-goes-to-the-owner** | A 60% discount goes to the reception | **wrong level; unsupported**: no rule routes a task by its content |
| sla-escalates | After the SLA, the manager is notified | ok, but the task stays the reception's: both can answer it |
| **manual-forward-to-manager** | — | **unsupported**: no way to hand a task to another queue |
| **patient-asks-a-team-function** | The router sends a patient to `bloquear-agenda`; `authorize` refuses | **1 exposure** (no effect) |
| dentist-reads-another | The precondition refuses; nothing read | ok (written into the Function) |
| dentist-blocks-own-day | Confirmed, blocked once | ok |

**Totals:**

| | Count |
| --- | --- |
| Unauthorized effects | **1** |
| Approvals from someone without the role | **2** |
| Stale approvals | 0 |
| Exposures | **1** |
| Wrong level | **1** |
| Unsupported | **2** |

## What this means

The RFC's rule, "who may do what comes from verified identity and
declared rules", does not hold today for approvals:
- the task link is a bearer token;
- the approver's name is free text;
- queues have no members.

Everything the next milestone must change shows up here. backoffice-001
pre-registers it.
