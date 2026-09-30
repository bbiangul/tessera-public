# backoffice-003: approving on Slack, and denials that did not stop

- **Date:** 2026-09-26.
- **Pre-registration:** [backoffice-003-preregistration.md](backoffice-003-preregistration.md),
  including its addendum on denials, written before their scenarios.
- **Issues:** #13 (approval surfaces), #53 (a denial did not stop the
  Function).
- **Results:** `bench/results/backoffice-003/`.

## Summary

- **Slack approvals work as registered.** S1–S8 hold: 27 scenarios on
  agent-003, all measures 0. Two deliberate breakages were caught by the
  harness.
- **The live check found a worse bug, older than Slack** (#53): a denied
  approval still ran the Function's action. Dora's "Recusar" applied a
  60% discount, and the patient was told "aprovado". No scenario before
  this one had ever denied a task.
- **Fixed:** a denied approval now ends the Function. On the three deny
  scenarios, effects after a denial went from 1 each to 0; it was checked
  live as well.

## What was built

- **Slack as a task surface** (`internal/channels/slack`, `tessera serve
  --slack`):
  - approval tasks go to the Slack DMs of the members who may answer;
  - a click is answered only when:
    - Slack's signature is valid;
    - the request is at most 5 minutes old;
    - it comes from the configured workspace;
    - the user ID is a member's, with the role;
    - the hash is still the task's;
  - the message then shows the outcome.
- **The shared task surface** (`runtime.ChannelTasks`,
  `runtime.AnswerTask`): which tasks go to whom, and whether an answer
  counts. WhatsApp was moved onto it; its scenarios are unchanged.
- **#53:**
  - `escalate.onDeny` and `messages.denied`;
  - a denied or unclear approval ends the Function.

## Results against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| S1 | A member with the role approves: one effect, as that member, outcome shown | **Held** |
| S2 | A Slack user who is no member: refused | **Held** (and told why) |
| S3 | A member without the role: refused | **Held** |
| S4 | The right user ID from another workspace: refused | **Held** (not even answered) |
| S5 | Forged signature; request replayed after 6 min: 401 | **Held** |
| S6 | Clicked twice: one effect | **Held** |
| S7 | 60% goes only to the owner's DM | **Held** |
| S8 | backoffice-001/-002 measures stay 0 on agent-003 | **Held**: 20 earlier scenarios, all 0 |
| D1–D3 | A denial on the link, WhatsApp, Slack: 0 effects, 0 "aprovado" | **Held after the fix** |
| D4 | Baseline: each denial shows 1 effect **and** 1 "aprovado" | **Half held.** 1 effect each (the bug). But 0 "aprovado": agent-003's Function returns `'aprovado'` as a value and sends it to no one. The live clinic, which sends it as a message, did tell the patient |
| D5 | After the fix, every other scenario stays 0; examples pass | **Held.** agent-001 and -002 reruns match their committed results exactly; `make ci` passes |

**The harness can fail.** Each breakage was caught by the harness:

| Breakage | Scenario | Measured |
| --- | --- | --- |
| The workspace check removed | sl-other-workspace | unauthorized 1, nonRole 1 |
| The replay window widened to 1h | sl-forged-and-replayed | forged 1, unauthorized 1 |
| The #53 fix disabled | TestADeniedApprovalStopsTheFunction | 3 of 4 cases fail |

## The live check

**Setup:**
- the real binary, `tessera serve --slack`, run in the project's own
  `tessera:dev` image, so the TLS roots can include a local certificate;
- Jev routed and DeepSeek extracted;
- the real http plugin with the documented `slack` bindings
  (`bench/backoffice/agent-live-slack`), talking over verified TLS to a
  local stand-in of Slack's Web API;
- clicks signed like Slack's.

| Check | Result |
| --- | --- |
| Patient asks 20% | Routed; only Rita's DM gets the task, with the bot token, with the task-id buttons |
| Forged signature; a correctly signed click 400 s old | 401, 401 |
| Gil (no role) clicks | Refused; the message says "Você não pode responder esta tarefa." |
| Rita's ID from another workspace | Nothing happens |
| A stranger clicks | "Você não está na equipe: nada foi respondido." |
| Rita clicks Aprovar | "✅ Aprovado por Rita."; resume `"by":"rita"`; discount applied once; run Succeeded |
| Rita clicks again | "Esta tarefa já foi respondida ou mudou." |
| 60% | Only Dora's DM; Rita's click refused |
| **Dora clicks Recusar (before the fix)** | "❌ Recusado por Dora.", **then `clinic/discount` ran (`affected: 1`) and the patient heard "Desconto de 60% aprovado."** |
| Dora clicks Recusar (after the fix) | No action intent; the patient hears the clinic's `denied` message; run Succeeded |
| Rita approves 20% (after the fix) | Discount applied once, as before |

One oddity, noted and not chased: after the first run, reading the
container's SQLite from the host showed only P1's discount. The journal
recorded P2's as `affected: 1`. SQLite's shared-memory WAL index is
likely not coherent across Docker's file sharing. The journal is taken
as the record. **Update (backoffice-005):** my own host reads were very likely the
cause. A host `sqlite3` checkpoints and removes the container's WAL, and
the container's later writes diverge. The later live checks keep the
databases in a named volume and read only through the API.

## A semantics change to know

- **Before:** a timeout's safe answer (`onTimeout: { answer: false }`) on
  an approval let the Function go on with `false`.
- **Now:** it is a denial, and the Function stops with `onDeny` or
  `messages.denied`.

`TestSLAStages` was updated to say so. No example relied on it.

## Not validated

- **A real Slack workspace:** it needs the owner's Slack app (signing
  secret, bot token with `chat:write`) and a public URL for
  `/channels/slack/interactions`. The bindings are checked against the
  body shapes Slack documents, not against Slack itself.
- **A real WhatsApp number** (backoffice-002, still pending).
- **Deny with a reason** (RFC-0006, "negar com motivo"): the buttons
  deny without one. A reason can be given on the task page.
