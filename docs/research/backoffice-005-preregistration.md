# backoffice-005: approving by e-mail (Outlook and Gmail). Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006, the approval surfaces: "e-mail, Outlook (Microsoft
  Graph) and Gmail (Gmail API): a single-use link plus a code. A reply to
  the e-mail only counts through the link, because an e-mail's sender is
  forged."
- **Issue:** #13.

## What will be built

1. **`HumanQueue.spec.channels: [email]`,** plus the `Team` members'
   `email`.
   - Admission requires the queue's roles and a plugin named `email` with
     a `send-task` action.
   - It also requires the Team's `verify`, which sends the code.
   - `tessera serve --email` needs `--public-url`.
2. **Presenting:** each approval task is sent to the e-mail of every
   member who may answer it.
   - The e-mail carries a signed link **bound to that member**.
   - The adapter passes `send-task` both `{to, subject, text, link}` and
     `raw`, a complete RFC 822 message encoded as base64url. The adapter
     builds `raw` (UTF-8 subject as an encoded word, base64 body), so no
     text of the task can add a header.
   - The Outlook binding uses Graph `sendMail`, with the existing
     `connection.oauth2`.
   - The Gmail binding uses `users.messages.send` with `raw`. Its OAuth2
     uses the **refresh-token grant**, new in `connection.oauth2` as
     `refreshToken`.
3. **Answering:**
   - Only through the link; the e-mail is never read back.
   - The link's page offers only its member: "send me a code".
   - The code goes through the Team's `verify`, never to whoever holds
     the link.
   - Only the code answers, with the existing rationing and tries.
   - Once the task is answered, the link stops (410).

## Measure

**The backoffice harness gains e-mail scenarios on
`bench/backoffice/agent-005`:** agent-004 with `email` added to every
queue and an e-mail on every member. The scenarios use:
- a fake `email` plugin that records the e-mails and their links;
- the real server's link pages;
- the one-time codes, captured from the fake verify action.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| E1 | A member with the role opens their link, gets a code, approves: one effect, recorded as that member |
| E2 | The e-mail forwarded to an outsider: the code still goes only to the member; a guessed code is refused; 0 effects |
| E3 | Rita's link cannot be used for another member: asking a code for Gil on it is refused, and no code is sent to Gil |
| E4 | After the answer the same link gives 410; a second answer changes nothing (one effect) |
| E5 | A 60% discount: only the owner gets the e-mail |
| E6 | A denial through the link: 0 effects (#53) |
| E7 | The raw message parses as RFC 822 with the member as To, the UTF-8 subject decoded exactly, and the link in the body. A task text with CR/LF and "Bcc:" adds no header |
| E8 | The documented Outlook and Gmail bindings send what Graph and the Gmail API document. The refresh-token grant gets and reuses a token |
| E9 | Every earlier scenario stays 0 on agent-005, and every example passes |

**After:** a live check through `tessera serve --email --public-url` in
the `tessera:dev` image, against local stand-ins of Graph and Google's
APIs, with a real browser on the link page. Real mailboxes need the
owner's Microsoft 365 app (Mail.Send) or Google OAuth client; until then,
the report says so.
