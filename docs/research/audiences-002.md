# audiences-002: context visibility and autonomy levels

- **Date:** 2026-09-26.
- **Pre-registration:** [audiences-002-preregistration.md](audiences-002-preregistration.md),
  committed before any code.
- **RFC:** RFC-0006, section 1, the last two parts. With this, every part
  of RFC-0006 is built and measured.
- **Results:** `bench/results/audiences-002/`.

## Summary

- **Who sees what of a patient's conversation is declared** per audience:
  masked, in full, or nothing (the default). Every view is journaled in
  the task's run.
- **How much the agent does alone is declared** per Agent and per
  audience:
  - `approve-actions`: a person approves every irreversible action and
    message-sending effect, with its arguments;
  - `draft`: that, and every message a run sends the user.
- **A1–A4 and V1–V5 hold.** The harness runs 65 scenarios on agent-008.
  Earlier agents are identical. Seven deliberate breakages were caught,
  four in the unit tests and three in the harness.
- **One prediction was incomplete** (A4): two earlier scenarios changed,
  not one. Both change for the reason A1 intends.
- **The live check passed.** It found that the platform's fixed messages
  go out alone under `draft`, as registered. In the live clinic that
  meant English, because the clinic did not set `withAPerson`.

## What was built

- **`audiences[].sees: [{audience, conversation: masked|full}]`**, in
  `runtime.Visibility`, `TaskContext` and `TaskView`:
  - the viewer's audience comes from their Team identity, and the seen
    audience from the task's run;
  - the conversation's last 6 lines are masked with the masker, or not;
  - the WhatsApp, Slack, Teams and e-mail task messages use `TaskView`;
  - `freeMode.tasks` adds the lines to the facts, which are always masked
    for the model;
  - each view writes `<site>/viewed/<member>/<channel>` in the run once
    (member, channel, visibility, line count). A view is shown only if
    its audit was written.
- **`autonomy: autonomous | approve-actions | draft`**, on the Agent and
  on audiences (`runtime.AutonomyFor`):
  - under `approve-actions`, `effect()` asks the `humanInTheLoop` queue,
    "Aprovar <act> com <args>?", as an `approve` task (hash-bound, roles,
    channels). A denial ends the Function, with `messages.denied`;
  - under `draft`, a message to the user (`send`, and the questions of
    `waitFor`) waits, after the voice, for a person to approve that exact
    text. A denial sends nothing and ends the run.
- **Admission:**
  - `approve-actions` and `draft` need `humanInTheLoop`;
  - a draft audience, or an Agent without audiences, may not have a free
    mode, its own or inherited;
  - `sees` names audiences that exist.

## Results against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| V1 | Rita's (equipe) task messages show the patient's lines masked | **Held.** On Slack and WhatsApp the e-mail and phone are replaced, and the request is kept |
| V2 | Dora (dono, full) sees them unmasked | **Held** |
| V3 | A dentist, and an audience without `sees`, get the text only | **Held.** Only the text, and no audit record |
| V4 | Every view is journaled; none for views without context | **Held.** `rita/slack`, `rita/whatsapp` masked and `dora/whatsapp` full; nothing for Bruno |
| V5 | `freeMode.tasks` gives the model the conversation masked, even for a `full` viewer | **Held.** Dora's free mode got `[EMAIL]` and `[PHONE]` |
| A1 | The dentist's block waits for the reception: approved, one effect; denied, none, with the dentist audience's denied message | **Held** |
| A2 | Autonomous audiences behave as on agent-007 | **Held.** Every other scenario is unchanged |
| A3 | `draft` (unit tests): the exact text waits; approved, sent once; denied, nothing; a confirmation waits too; admission refuses a free mode | **Held** |
| A4 | Earlier scenarios unchanged except dentist-blocks-own-day; agents 001–007 identical; examples pass | **Held, with one miss in the prediction.** bo-slack-confirmation (the same block, on Slack) also waits now, for the same reason as A1, and I did not list it. Agents 001–007 are identical; `make ci` passes |

**Breakages the checks caught:**

| Breakage | Caught by |
| --- | --- |
| `draft` switched off | unit tests (draft, visibility) |
| `approve-actions` switched off | unit test; harness au2-dentist-approve-actions (unauthorized 1) and au2-dentist-denied |
| No masking | unit test; harness vis-reception-masked (leaks 1) |
| No audit record | unit test (0 records) |
| Every viewer sees in full | harness vis-reception-masked and vis-none (leaks, and an audit where none belongs) |

**How the last breakage was checked:** my first attempt did not compile.
The harness command hid the error, so it looked like "nothing caught".
The second attempt compiled and was caught. The lesson: check that a
breakage builds before reading its silence.

**Baseline on the earlier agents (all autonomous):** in
au2-dentist-approve-actions the block happens right after the dentist's
"sim", with no one approving. That scenario counts it as unauthorized.
The harness reports it for agents 001–007 as the baseline, not as a
regression.

## The live check

**Setup:** `bench/backoffice/agent-live-autonomy` through
`tessera serve --whatsapp --slack --teams`, with Jev and DeepSeek, the
Slack, Bot Framework and Meta stand-in, and state read through the API.

| Step | Result |
| --- | --- |
| A patient on WhatsApp asks 20% and writes an e-mail and a phone | Rita's Slack card: the task, then "Conversa: user: quero um desconto de 20%, meu email é [EMAIL] e meu fone [PHONE] / agent: …". The run's journal has `viewed/rita/slack`, masked, 2 lines |
| Dr. Bruno on Slack: "bloqueia minha sexta", "sim" | Rita's card: `Aprovar clinic/block com {"day":"sexta","dentistId":"D1"}?`. She approves, and he gets "Agenda de sexta bloqueada." |
| A partner (a Slack user outside the Team) asks the hours | Rita's card: "Enviar ao usuário esta mensagem? … Atendemos parceiros de segunda a sexta, das 8h às 18h…". She approves, and the partner gets exactly that text |
| The partner asks again, and Rita denies the draft | Nothing reaches the partner |
| The partner writes while their first draft waits | The platform's fixed `withAPerson`, at once and unreviewed, as registered. In **English**, because the clinic sets no `messages.withAPerson` |

**Findings, for the author:**
- **The task's own text is the Function's.** The live clinic's discount
  task names the patient by `waId`, so the reception sees the number in
  the text, even with a masked conversation below it. The authoring
  guide now says to keep personal data out of task texts.
- **Under `draft`, set `messages.withAPerson` and `notUnderstood`:** they
  go out unreviewed.

## Not built

- **Editing a draft before sending it** (RFC-0001: "aprova, edita ou
  descarta"): a draft is approved or denied.
- **Context on the task page behind a link:** its identity is only
  verified at the answer.
