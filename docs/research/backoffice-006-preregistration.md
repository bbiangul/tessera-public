# backoffice-006: the conversational back office. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006, step 6, section 5: "the dentist asks 'my schedule
  tomorrow' and 'block Friday afternoon'; the reception sees its pending
  tasks in its own conversation and answers right there; the owner asks
  for a report. Effects stay only in Functions."

## What exists today

- **WhatsApp:** the team talks to the agent. Audiences (audiences-001)
  give each role its menu, voice and free mode, and the member's identity
  comes from the Team.
- **Slack and Teams:** they only carry approval clicks. A message a
  member writes there reaches nothing.
- **Tasks in the conversation:** a member cannot ask "do I have anything
  to approve?". The free mode knows nothing of tasks.

## What will be built

1. **Conversation on Slack** (Events API, direct messages):
   - Events arrive at `/channels/slack/events`, with the same v0
     signature, 5-minute window and workspace check as clicks, plus the
     Events API's URL handshake.
   - A retried event (same `event_id`) is handled once. Bot messages and
     edits are ignored.
   - The sender's Slack user ID is a Team member's, or the sender is a
     caller without roles.
   - Replies go to the user's DM through the `slack` plugin's `post-text`
     action.
2. **Conversation on Teams** (personal chat):
   - A message activity without a task button, with the same JWT and
     tenant checks as clicks, is a message to the agent.
   - The sender's Entra object ID is a member's, or the sender is a caller
     without roles.
   - Replies go through the `teams` plugin's `post-text` action.
3. **One delivery for all channels** (`internal/channels/outbox`): the
   outbox algorithm, extracted from the WhatsApp adapter and used by
   Slack and Teams:
   - a message is marked before it is sent;
   - a refusal is final;
   - other failures are retried with backoff;
   - a conversation's messages keep their order.

   A run's later messages (a confirmation question, a result after an
   approval) reach the member in the channel they wrote from.
4. **Tasks in the conversation:** `freeMode.tasks: true` gives the free
   mode the caller's open tasks, as `FACTS.myTasks`, masked like any
   private read.
   - A task is listed if the caller is a member who may answer it, by the
     same rule as approvals (`Answerers`).
   - The free mode may say how many and which. It says they are answered
     with the buttons or the link sent to the member, **never by a message
     in the conversation** ("pode aprovar" approves nothing, as before).
5. **The owner's report** is an ordinary read Function on the owner
   audience's menu. Nothing new in the platform.

## Measure

**The backoffice harness gains conversation scenarios on
`bench/backoffice/agent-007`:**
- **The agent:** agent-006 plus:
  - `post-text` on the `slack` and `teams` plugins;
  - Bruno's Slack and Teams IDs;
  - `freeMode.tasks` on the team audiences;
  - an owner audience with a discount report.
- **Messages arrive** as signed Slack events, Teams activities with tokens
  from the key stand-in (`teamstest`), and signed Meta webhooks.
- **Instruments:**
  - the fake plugins record every text sent;
  - the fake Decide records every menu;
  - the fake Generate records the free mode's facts.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| B1 | Dr. Bruno on Slack, "mostra minha agenda": his schedule (D1 from the Team via his Slack ID), in his DM; only the dentist menu is offered |
| B2 | The same on Teams, in his personal chat |
| B3 | A Slack or Teams user who is no member: no team Function offered, no team read |
| B4 | Refused before anything (no run, no reply): a forged or stale Slack event, one from another workspace, and a Teams message from another tenant |
| B5 | Slack sends the same event twice (`event_id`): one run, one reply |
| B6 | Rita asks on Slack "tenho algo para aprovar?" with 2 reception tasks and 1 owner task open: the free mode's facts list exactly her 2, and the reply reaches her DM |
| B7 | Rita writes "pode aprovar o desconto" in the conversation: 0 effects |
| B8 | Dr. Bruno on Slack, "bloqueia minha sexta" then "sim": one effect; the confirmation question and the result reach his DM through the outbox |
| B9 | Dora (owner) on WhatsApp asks for the week's discounts: she gets the report. A patient asking the same is not offered it |
| B10 | Every earlier scenario stays 0 on agent-007; agents 001 to 006 rerun identical; every example passes; the WhatsApp adapter's own tests pass unchanged |

**After:** a live check through `tessera serve --slack --teams --whatsapp`,
with Jev and DeepSeek, against local stand-ins of Slack, the Bot Framework
and Meta. State is read through the API only, with the databases in a
Docker volume (the lesson of backoffice-005).
