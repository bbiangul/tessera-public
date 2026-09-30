# backoffice-001: identity, roles and authenticated approval

- **Date:** 2026-09-26.
- **Pre-registration:** `backoffice-001-preregistration.md`.
- **RFC:** RFC-0006.
- **Issue:** #13.
- **Data:** `bench/results/backoffice-001/`.
- **Harness:** the same deterministic `bench/backoffice/bobench`, 14
  scenarios (backoffice-000's 10 plus the M2 ones).

## Built

| What | How |
| --- | --- |
| `kind: Team` | Members with roles and a contact; `verify`, the action that sends a one-time code |
| `HumanQueue.spec.roles` | Who answers. Also the members of the queues a task reached, by an SLA escalation or a forward |
| Authenticated task pages | Pick a member; a 6-digit code (10 min, 5 tries, single use, bound to the link and the member) goes to their contact; only the code answers, recorded as the member's id |
| The API | `Reply.By` must be such a member (403 otherwise). New `POST /v1/runs/{id}/forward {queue, by}` |
| `escalate.route` | `[{when, queue}]`: a declared rule picks the queue |
| `selector.who` | The router offers a Function only to callers it holds for |

## Result

**`agent-001`** (Team, roles, route, `who`):

| Measure | backoffice-000 | backoffice-001 |
| --- | --- | --- |
| Unauthorized effects | 1 | **0** |
| Approvals from someone without the role | 2 | **0** |
| Stale approvals | 0 | **0** |
| Exposures | 1 | **0** |
| Wrong level | 1 | **0** |
| Unsupported | 2 | **0** |

**What is refused:**
- a forwarded link with a guessed code;
- an API answer by a non-member;
- the manager answering a task not escalated or forwarded to them;
- a wrong code;
- a code used on another task;
- the reception approving the owner's task;
- a forward by someone outside the queue;
- a team Function offered to a patient.

**What succeeds, once each:**
- the reception's approval by code;
- the owner's approval of a 60% discount, routed by rule;
- the manager's approval after the SLA escalation;
- the manager's approval after a forward;
- the dentist's own block.

**The same agent without roles** (`agent-without-roles`), on the new
code: every hole is still there, as the queues are open. Roles are what
close them, and a queue without roles behaves as before.

| # | Hypothesis | Result |
| --- | --- | --- |
| M1 | backoffice-000's scenarios all 0 | **Supported** |
| M2 | The new refusals refuse and the new successes succeed | **Supported** |
| M3 | Every example and EvalSuite still passes | **Supported** (`make ci`: all replay suites green) |

## Limits

- **One-time codes live in the server's memory.** A restart means asking
  for a new code. A multi-replica deployment needs them in the journal
  (the RFC-0007 cluster phase).
- **The API still trusts its token holder** to say which member is
  writing: that is the channel adapter's job (a WhatsApp number, a
  Slack user id).
  - The platform now checks that member's role.
  - Binding it to the channel's verified sender is the next step: the
    approval surfaces (WhatsApp, Slack, Teams, e-mail) in RFC-0006's
    order.
- **Not built yet:**
  - audiences with their own voice and free mode (`who` only filters the
    menu);
  - the conversational back office;
  - the RFC's LLM-stack baseline.

## Addendum: a security fix and a live check (same day)

**A hole found on review, and fixed.** Codes had 5 tries each, but a
link holder could ask for new codes without end: a slow brute force, and
a flood of messages to the member.
- Codes are now rationed per sliding hour: 3 per member per link, and
  10 per link.
- The 4th request gets 429.
- Covered by `TestCodeRequestsAreRationed`, and seen live.

**The live check** (`tessera serve bench/backoffice/agent-live`):
- Jev routed; DeepSeek extracted.
- The clinic's effects, the queue notifications and the codes went to
  SQLite tables that stand in for the real systems (the "channel").
- The task page was driven in a real browser.

| Check | Result |
| --- | --- |
| Patient asks a 20% discount | Routed; task in `recepcao`; the notification carries the signed link |
| Patient asks to block a dentist's day | Not offered (`who`): "Não entendi" |
| Dentist D1 asks D2's schedule | Refused by the precondition |
| Dentist D1 asks own schedule | Answered |
| The link opened | Lists only Rita (the queue's role) |
| "Send me a code" | A code goes to Rita's contact only |
| A guessed code | "Wrong or expired code" |
| Rita with her code | "Recorded"; discount applied **once**; resume `"by":"rita"`; run Succeeded |
| The used link again | 410 |
| 4 code requests on a new task | 200, 200, 200, **429** |
| API answer by Gil (manager) on a reception task | 403 with the reason |
| A 60% discount | Task in `dono` (the rule) |
| Forward by Dora (not in `recepcao`) | 403 |
| Forward by Rita to `gerencia` | Accepted; the task's queues are [recepcao, gerencia] |
| Dentist blocks own Friday | Confirmed, blocked once |

**Still not validated:** a real channel delivering the code (WhatsApp,
e-mail). The SQLite table stands in for it.
