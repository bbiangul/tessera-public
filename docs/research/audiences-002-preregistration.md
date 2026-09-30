# audiences-002: context visibility and autonomy levels. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **RFC:** RFC-0006, section 1:
  - "o que cada audiência vê do contexto de outra é declarado (a recepção
    vê a conversa do paciente mascarada…). Tudo fica no journal, como
    auditoria";
  - "o nível de autonomia (draft, approve-actions, autonomous, do
    RFC-0001) passa a ser por audiência".

## What exists today

- **What a member sees of the conversation behind a task:** nothing. The
  channels (WhatsApp, Slack, Teams, e-mail) and `freeMode.tasks` show the
  task's text only.
- **Autonomy levels:** none exist. Every agent acts as `autonomous`; a
  person approves only where a Function declares an `escalate`.

## What will be built

1. **`audiences[].sees: [{audience, conversation: masked | full}]`.**
   - A member's audience is the one their Team identity (roles,
     attributes) matches.
   - The conversation behind a task is that of its run.
   - A member sees that conversation (its last lines) only when their
     audience declares the run's audience. `masked` passes it through the
     masker (e-mails, phones, documents); `full` does not. Without a
     declaration they see nothing, as today.
   - It is shown where the member's identity is attested: the task
     messages on WhatsApp, Slack, Teams and e-mail, and `freeMode.tasks`.
     What goes to a model is masked regardless.
   - **Every view is journaled** in the task's run (who, which channel,
     which visibility): the audit.
   - The task page behind a link shows the task's text only, as today.
2. **`autonomy: autonomous | approve-actions | draft`**, on the Agent and
   per audience (the audience's wins). The run's audience decides, as for
   voice and messages.
   - **`autonomous`**, the default: as today.
   - **`approve-actions`:** before every effect that is `irreversible` or
     `external-message`, the run asks the `humanInTheLoop` queue for
     approval.
     - It is an `approve` task, with the action and its arguments in the
       text, so the task's hash binds the approval to them.
     - The usual roles, channels and answers apply.
     - A denial ends the Function (#53): no effect, and the user hears
       `messages.denied`.
   - **`draft`:** `approve-actions`, and every message a run sends the
     user (answers, questions, confirmations) waits for a person's
     approval of that exact text.
     - Approved, it goes out.
     - Denied, nothing is sent and the run ends.
     - The platform's own fixed messages (not understood, not allowed) and
       free-mode answers are not runs. Admission refuses a free mode on a
       `draft` audience or Agent, so nothing in draft reaches a user
       unreviewed.
   - Admission requires `humanInTheLoop` for `approve-actions` and
     `draft`.

## Measure

**The backoffice harness gains scenarios on `bench/backoffice/agent-008`:**
agent-007 plus these declarations:
- `equipe` sees `paciente` conversations masked;
- `dono` sees them in full;
- `dentista` sees none;
- `dentista` has `autonomy: approve-actions`.

`draft` is measured by host unit tests (a small agent) and in the live
check. The bench clinic's patients use the free mode, which `draft`
excludes.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| V1 | Rita's (equipe) Slack card for a patient's discount task shows the patient's last messages, masked: an e-mail and a phone the patient wrote do not appear, the rest of the text does |
| V2 | Dora (dono, `full`) sees the same lines unmasked |
| V3 | A dentist, and an audience without `sees`, get the task's text only |
| V4 | Every view is journaled: one audit record per member, channel and task shown with context, none for views without it |
| V5 | `freeMode.tasks` gives the model the conversation masked, even for a `full` viewer |
| A1 | The dentist's block (irreversible, `approve-actions`) waits for the reception's approval: approved, one effect; denied, none, and he hears the dentist audience's `messages.denied` |
| A2 | Audiences that stay `autonomous` behave exactly as on agent-007: the patient's discount flow and the owner's report |
| A3 | `draft` (unit tests): a run's answer waits for approval of its exact text. Approved, it is sent once; denied, nothing is sent. A confirmation question in draft waits too. Admission refuses a free mode on a draft audience |
| A4 | Every earlier scenario stays 0 on agent-008, except that dentist-blocks-own-day now waits for approval, as A1 intends (declared here); agents 001–007 rerun identical; every example passes |

**After:** a live check with Jev and DeepSeek on WhatsApp and Slack
stand-ins.
