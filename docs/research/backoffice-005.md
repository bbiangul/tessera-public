# backoffice-005: approving by e-mail (Outlook and Gmail)

- **Date:** 2026-09-26.
- **Pre-registration:** [backoffice-005-preregistration.md](backoffice-005-preregistration.md),
  committed before any code.
- **Issue:** #13. This completes the RFC-0006 channel list: WhatsApp,
  Slack, Teams and e-mail.
- **Results:** `bench/results/backoffice-005/`.

## Summary

- **E-mail approvals work as registered.** E1–E9 hold. The harness runs
  44 scenarios on agent-005, with four channels side by side, and every
  measure is 0.
- **The live check passed with Outlook (Graph) and with Gmail,** through
  the real binary. The link page was driven in a real browser.
- **One harness scenario was too weak, and was fixed after the fact.**
  E3 ("Rita's link cannot be used for another member") held for the
  wrong reason: in agent-005 the other members lack the reception role,
  so they were refused anyway. A mutation that removed the link's member
  binding went unnoticed by the harness; the unit test caught it. The
  scenario now forwards the task to management first, so Gil may answer,
  and it catches the mutation. The change was made after the first run
  and is reported as such.
- **The live check also exposed a flaw in my own method.** Reading the
  container's SQLite from the host corrupts what the container sees.
  Details are below. It is not a Tessera bug.

## What was built

- **`channels: [email]`** and the members' `email`. Admission requires:
  - the queue's roles;
  - the Team's `verify`;
  - a plugin named `email` with `send-task`.
- **Member-bound links** (`tasklink.Claims.Member`,
  `Server.MemberTaskLink`):
  - the page offers only that member;
  - a code is asked only for them;
  - when the member can no longer answer (the task went to another
    queue), the page says so.
- **`internal/channels/email`:** each task is e-mailed once per member,
  with the link. `send-task` gets both the parts and `raw`.
- **`internal/mailmsg`:** builds the RFC 822 message.
  - Headers come from one-line values, with display names encoded.
  - The subject is a UTF-8 encoded word, and the body is base64.
  - `verify` also gets `inputs.raw` (the code as a message) when the
    contact is an e-mail, so Gmail can send codes.
- **`connection.oauth2.refreshToken`:** the refresh-token grant, for
  Gmail.
- **The documented bindings:** Outlook through Graph `sendMail` (client
  credentials, `Mail.Send`), and Gmail through `users.messages.send`
  with `raw` (refresh token, `gmail.send`).

## Results against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| E1 | A member approves through their link and code: one effect, as that member | **Held** |
| E2 | Forwarded to an outsider: the code goes only to the member; a guess fails; 0 effects | **Held** |
| E3 | Rita's link cannot be used for another member | **Held**, with the scenario strengthened as described above; the unit test isolates it too |
| E4 | After the answer the link gives 410; one effect | **Held** |
| E5 | 60%: only the owner gets the e-mail | **Held** |
| E6 | A denial through the link: 0 effects | **Held** |
| E7 | `raw` parses as RFC 822, with To, the UTF-8 subject exact and the link in the body; CR/LF plus "Bcc:" in the task adds no header | **Held.** The injected text stays in the subject's text and the body, and the message has exactly its 6 headers |
| E8 | The Outlook and Gmail bindings send what Graph and the Gmail API document; the refresh-token grant is reused | **Held.** http plugin tests show the exact bodies, and one token serves two sends |
| E9 | Earlier scenarios unchanged; examples pass | **Held.** agent-001 to -004 rerun identical to backoffice-004's results; `make ci` passes |

## The live check

**Setup:**
- `tessera serve --email --public-url http://127.0.0.1:18995` in the
  `tessera:dev` image;
- Jev routed and DeepSeek extracted;
- local stand-ins of Graph, the Gmail API and both token endpoints, over
  verified TLS.

**Outlook** (`bench/backoffice/agent-live-email`; codes by e-mail
through Graph as well):

| Check | Result |
| --- | --- |
| Patient asks 20% | One token (client credentials); one e-mail through Graph, to Rita only, from the clinic's mailbox. The e-mail has the task, the instructions ("Responder a este e-mail não tem efeito") and her link |
| The link in a real browser | Offers only Rita; "Send me a code" e-mails a code to Rita; entering it and Approve gives "Recorded" |
| Journal | resume `"by":"rita"`; discount applied once; run Succeeded |
| The link again, in the browser and by POST | "This task is no longer open"; 410 |
| 60% | E-mailed only to Dora |
| Dora's link: a code for Rita, a code for Gil | 403, 403; no code sent to them |
| Dora's link: a guessed code | 403 |
| Dora denies with her code | "Recorded"; no action; the patient hears the clinic's `denied` message |

**Gmail** (the same clinic with the Gmail binding):

| Check | Result |
| --- | --- |
| Patient asks 20% | One refresh-token grant. The stand-in decoded both `raw` messages (the task's and the code's) as valid RFC 822: To is Rita, the subjects are "Aprovação pendente: …" and "Seu código: …" exactly, and the body has the link |
| Rita answers with the code | Recorded; discount applied once; run Succeeded |
| A non-ASCII display name in From | Found here: it went into the header unencoded. Fixed with `net/mail` formatting, and covered by the unit test |

**A flaw in my method, not in Tessera.**
- **What happened:** in the Outlook run I read the container's
  `runs.db` with `sqlite3` from the host between steps. After that, the
  server said "run not found" for a task that had just been e-mailed and
  answered on its page.
- **Why:** SQLite's shared-memory locking does not work across Docker's
  macOS file share. On closing, a host reader believes it is alone,
  checkpoints the container's WAL and removes it, and the container's
  later writes diverge.
- **The rerun:** the databases went into a Docker named volume the host
  cannot touch, and state was read only through the API
  (`/v1/tasks`, `/v1/runs/{id}`). Everything above comes from that rerun.
- **Earlier reports:**
  - the backoffice-003 note ("only P1's discount visible from the host")
    was very likely the same effect;
  - backoffice-003 and -004 also read from the host between steps. Their
    tables agree with the journal and the stand-ins' logs, but those
    reads were not a sound method.

## Not validated

- **Real mailboxes** need the owner's setup:
  - a Microsoft 365 app with `Mail.Send` and a sender mailbox, or a
    Google OAuth client with a refresh token for `gmail.send`;
  - a public URL for the links.
- **Deliverability** (spam filtering, SPF/DKIM of the sending domain) is
  the mail provider's side, and was not measured.
- **Cosmetic, left open:** a run's timeline says "escalate to recepcao"
  even when the rule routed the task to `dono`.
