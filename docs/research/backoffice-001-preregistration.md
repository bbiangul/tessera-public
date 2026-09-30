# backoffice-001: identity, roles and authenticated approval. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006, steps 2 and part of 4.
- **Issue:** #13.
- **Baseline:** backoffice-000.

## What will be built

1. **`kind: Team`:** members with an id, a name, roles and contacts (by
   channel), and `verify`: the plugin action that sends a member a
   one-time code (WhatsApp, e-mail…).
2. **`HumanQueue.spec.roles`:** who may answer the queue's tasks. With
   roles set, every answer names a member who has one.
3. **The signed task page authenticates:**
   - the person picks who they are, among the members with the role;
   - the platform sends that member a one-time code (6 digits, 10
     minutes, 5 tries);
   - the answer is accepted only with the code, recorded as the member's
     id.

   A forwarded link lets nobody else approve, because the code goes to
   the member.
4. **The API:** `Reply.By` must be a member with the role when the queue
   has roles. The client holding the token (a channel adapter) remains
   trusted to have verified which member is writing; the role is now
   checked by the platform.
5. **`escalate.route`:** `[{when: <CEL>, queue: <name>}]`, the first
   matching rule, else `queue`. "A discount above 50% goes to the owner"
   is a rule, not a model's choice.
6. **Forwarding:** `POST /v1/runs/{id}/forward {queue, by}` hands the
   pending task to another queue. `by` must be a member of the current
   queue. After a forward or an SLA escalation, members of the queues the
   task reached may answer.
7. **`selector.who`:** a CEL condition on the caller. The router offers a
   Function only to callers it matches, so a patient never sees a team
   Function.

The backoffice agent gets a Team, queue roles, a route for large
discounts, and `who` on the team Functions.

## Hypotheses (deterministic: the harness either shows them or not)

| # | Hypothesis |
| --- | --- |
| M1 | backoffice-000's 10 scenarios: 0 unauthorized effects, 0 approvals from someone without the role, 0 stale approvals, 0 exposures, 0 wrong levels, 0 unsupported |
| M2 | The new scenarios all refuse: a wrong code; a code reused; a code sent to one member used as another; a forward by someone not in the queue; an API answer by a member without the role. The ones that should succeed do: an approval by a member of the escalation queue, and the owner approving a routed 60% discount |
| M3 | Every example and EvalSuite still passes: queues without `roles` and Functions without `who` behave as before |
