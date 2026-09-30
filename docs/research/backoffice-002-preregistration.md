# backoffice-002: approving on WhatsApp. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006 (the approval surfaces; WhatsApp first, by the
  owner's decision).
- **Issue:** #13.

## What will be built

1. **`HumanQueue.spec.channels: [whatsapp]`,** plus `Team` members'
   `whatsapp` (their number, E.164 digits).
2. **Presenting:** the WhatsApp adapter sends each task of such a queue
   to the WhatsApp of every member who may answer it, as an interactive
   message with buttons.
   - It uses the channel plugin's `send-buttons` action (admission
     requires it).
   - Button ids carry the task's hash (`task:<hash>:approve|deny`).
   - After an escalation or a forward, the new queue's members get it.
3. **Receiving:** a button reply is an answer when:
   - its sender's number, attested by Meta's signed webhook, is a
     member's;
   - that member holds a role of the queues the task reached;
   - the hash is still the pending task's.

   Otherwise the sender gets a short reason, and nothing is answered. No
   code is needed: the channel verified the person.
4. **Free text from a member never answers a task** (RFC-0006: "pode
   seguir" is not an approval). It goes to the agent as any message does.

**Known limit:** outside WhatsApp's 24h window Meta accepts only
templates. This version sends interactive messages; a template fallback
comes later, and the signed-link notification still works.

## Measure

**The backoffice harness gains WhatsApp scenarios:**
- signed Meta webhooks;
- a fake channel plugin that records `send-buttons`;
- the real adapter.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| N1 | A member with the role taps Aprovar from their number: the effect happens once, recorded as that member |
| N2 | A number that is no member's sends the same button reply: refused, no effect |
| N3 | A member without the role (the manager on a reception task not escalated to them): refused |
| N4 | The button tapped again after the task was answered: refused, one effect |
| N5 | A member writes "pode seguir" as text: no answer, no effect |
| N6 | A 60% discount routed to the owner: only the owner's WhatsApp gets the buttons |
| N7 | Every backoffice-001 measure stays 0, and every example passes |

**After:** a live check against a real WhatsApp test number, once the
owner configures its credentials. Until then this is validated against a
local stand-in of Meta's API, and the report says so.
