# backoffice-002: approving on WhatsApp

- **Date:** 2026-09-26.
- **Pre-registration:** `backoffice-002-preregistration.md`.
- **RFC:** RFC-0006.
- **Issue:** #13.
- **Data:** `bench/results/backoffice-002/`.
- **Harness:** `bench/backoffice/bobench`, now 20 scenarios. It runs the
  real WhatsApp adapter behind Meta-signed webhooks, with a fake channel
  plugin that records what would be sent.

## Built

- **`HumanQueue.spec.channels: [whatsapp]`** and `Team` members'
  `whatsapp` numbers.
- **The adapter's `DeliverTasks`:** each approval task goes to the
  WhatsApp of every member who may answer it, once per member.
  - It uses the channel plugin's `send-buttons` with Aprovar / Recusar
    buttons, whose ids carry the task's hash.
  - An escalated or forwarded task reaches the new queue's members.
  - `tessera serve --whatsapp` runs it with the outbox.
- **A button tap answers** only when:
  - the sender's number, attested by Meta's signed webhook, is a
    member's;
  - the member holds a role of the queues the task reached;
  - the hash is still the pending task's.

  Otherwise the sender gets the reason, and nothing is answered.
- **Free text from a member never answers a task.**
- **Admission:**
  - a WhatsApp queue needs roles and a `send-buttons` action;
  - a `whatsapp` number has 8 to 15 digits.
- **The documented `send-buttons` binding** for the Graph API is checked
  by `TestWhatsAppButtonsBody`: the exact JSON of Meta's interactive
  button message.

## Result

| # | Hypothesis | Result |
| --- | --- | --- |
| N1 | A member with the role taps Aprovar: one effect, recorded as them | **Supported** (the buttons went to Rita only) |
| N2 | A stranger's tap: refused | **Supported** |
| N3 | A member without the role: refused | **Supported** |
| N4 | A second tap after the answer: refused, one effect | **Supported** |
| N5 | "pode seguir" as text: no answer | **Supported** |
| N6 | A 60% discount: only the owner gets the buttons; the reception's tap refused; the owner's tap answers | **Supported** |
| N7 | backoffice-001's measures stay 0; examples pass | **Supported**: all 0 in 20 scenarios; `make ci` green |

**Unit tests also cover:**
- a forged webhook signature (401);
- a forward reaching the manager's WhatsApp, where the manager's tap
  answers.

## Not validated yet

- **A real WhatsApp number.** Meta's API is stood in for by the fake
  plugin and locally signed webhooks. The payloads follow Meta's
  documented formats, and the request body is checked exactly, but no
  message has reached a phone. That needs a WhatsApp Cloud API test
  number configured by the owner.
- **Outside the 24h window,** Meta only accepts templates. This version
  sends interactive messages; the template fallback is not built. Until
  then the signed-link notification is the way in.
