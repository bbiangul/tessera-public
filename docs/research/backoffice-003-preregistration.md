# backoffice-003: approving on Slack. Pre-registration

- **Date:** 2026-09-26.
- **RFC:** RFC-0006, the approval surfaces. Slack is the next channel after
  WhatsApp, in the RFC's order.
- **Issue:** #13.
- **Honest timing:** written after the adapter was built and unit-tested
  (`internal/channels/slack`), and before the harness scenarios below
  were written or run. The unit tests cover the same properties on a
  smaller fixture, so the hypotheses are not blind to the design. They
  are blind to the harness results.

## What was built

1. **`HumanQueue.spec.channels: [slack]`,** plus the `Team` members'
   `slack` field (their Slack user ID).
2. **Presenting:** `tessera serve --slack` posts each approval task of
   such a queue to the DM of every member who may answer it.
   - It uses the `slack` plugin's `post-task` action (chat.postMessage;
     admission requires it and `update-task`).
   - Button `action_id`s carry the task's hash
     (`task:<hash>:approve|deny`).
   - After an escalation or a forward, the new queue's members get it.
3. **Receiving:** a click in an interactivity request is an answer when:
   - Slack's signature (`v0`, signing secret) is valid;
   - the request is at most 5 minutes old;
   - it comes from the configured workspace (`TESSERA_SLACK_TEAM_ID`);
   - the user ID is a member's, and that member holds a role of the queues
     the task reached;
   - the hash is still the pending task's.

   The message's buttons are then replaced with the outcome
   (`update-task`, chat.update).
4. **Shared with WhatsApp:** which tasks go to whom, and whether an answer
   counts (`runtime.ChannelTasks`, `runtime.AnswerTask`). WhatsApp was
   refactored onto them, so its backoffice-002 scenarios must still pass.

## Measure

**The backoffice harness gains Slack scenarios, run against a new agent
(`bench/backoffice/agent-003`).** That agent is agent-002 with `slack`
added to every queue's channels and a Slack ID on every member, so the
WhatsApp and Slack surfaces run side by side. The scenarios use:
- Slack interactivity requests signed with the signing secret;
- a fake `slack` plugin that records `post-task` and `update-task`;
- the real adapter.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| S1 | A member with the role clicks Aprovar in their DM: the effect happens once, recorded as that member, and the message shows the outcome |
| S2 | A Slack user who is no member clicks the same button: refused, no effect |
| S3 | A member without the role (the manager on a reception task not escalated to them): refused |
| S4 | The right user ID from another workspace: refused, no effect |
| S5 | A forged signature, and a correctly signed request replayed after 6 minutes: HTTP 401, no effect |
| S6 | The button clicked twice: one effect |
| S7 | A 60% discount routed to the owner: only the owner's DM gets the buttons |
| S8 | Every backoffice-001 and -002 measure stays 0 on agent-003, and every example passes |

**Not measured here:** a real Slack workspace. It needs the owner's Slack
app, its signing secret and bot token, and a public URL for the
interactivity endpoint. Until then this is validated against local
stand-ins of Slack's API, and the report says so.

## Addendum: denials (#53), before any code or scenario for them

The live check found that a **denied** `escalate … task: approve` still
runs the Function's action: Dora's "Recusar" applied a 60% discount, and
the patient heard "aprovado". No backoffice scenario so far denied a
task. The earlier zeros cover the wrong person, stale answers and forged
requests, not "denied, done anyway".

**The fix to build:**
- a denied approval ends the Function; no later step or action runs;
- the user hears `escalate.onDeny`, else the agent's `messages.denied`;
- the run ends, recorded as denied by that member.

**New measures, per scenario:**
- `deniedButDone`: effects after a denial;
- `toldApproved`: messages to the user saying "aprovado" after a denial.

| # | Hypothesis |
| --- | --- |
| D1 | Rita denies on the link: 0 effects, 0 told approved, the run ends |
| D2 | Rita taps Recusar on WhatsApp: the same |
| D3 | Rita clicks Recusar on Slack: the same |
| D4 | **Baseline** (the code before the fix, agent-003): D1–D3 each show 1 effect and 1 "aprovado". This is recorded as the bug's measurement, not as a failure of the harness |
| D5 | After the fix: every other scenario stays 0, and every example passes |
