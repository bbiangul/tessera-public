# backoffice-006: the conversational back office

- **Date:** 2026-09-26.
- **Pre-registration:** [backoffice-006-preregistration.md](backoffice-006-preregistration.md),
  committed before any code.
- **RFC:** RFC-0006, step 6 (section 5).
- **Results:** `bench/results/backoffice-006/`.

## Summary

- **The team talks to the same agent the patients use,** on WhatsApp,
  Slack and Teams, each member with their identity from the Team.
- **Members ask about their own open tasks.** They answer them with the
  buttons, never by message.
- **The owner asks for a report,** and effects stay in Functions.
- **B1–B10 hold.** The harness runs 59 scenarios on agent-007, every
  measure 0, and four deliberate breakages were caught. Earlier agents
  rerun identical.
- **Live** (the binary, Jev, DeepSeek, stand-ins of Slack, the Bot
  Framework and Meta), everything worked. Two findings:
  - **A privacy gap, older than this step:** the default masker does not
    mask a phone number written as bare digits (a WhatsApp ID such as
    `5511911112222`). The live clinic put the patient's `waId` in a task's
    text, and that text reached the model unmasked. Opened as an issue.
  - **One free answer in five fell back to "Não entendi"**, cause not
    identified; three repeats of the same question all answered.

## What was built

- **Conversation on Slack** (`internal/channels/slack/talk.go`):
  - the Events API at `/channels/slack/events`, with the v0 signature,
    the 5-minute window and the workspace check, plus the URL handshake;
  - a retried `event_id` is handled once, and bot messages and edits are
    ignored;
  - the Slack user ID is a member's identity (`runtime.AsMember`);
  - replies go to the DM through `slack/post-text`.
- **Conversation on Teams** (`internal/channels/teams/talk.go`):
  - a message activity without a task button, with the JWT and tenant
    checks, in a personal chat (not a group chat or channel);
  - a repeated activity id is handled once, and mention tags are stripped;
  - the Entra object ID is a member's identity;
  - replies go to the chat through `teams/post-text`.
- **`internal/channels/outbox`:** the WhatsApp adapter's delivery,
  extracted and shared by the three channels. Messages are marked before
  sending, refusals are final, other failures are retried with backoff,
  and each conversation keeps its order. The WhatsApp adapter now uses it,
  and its tests pass unchanged.
- **`freeMode.tasks`:** the free mode gets `FACTS.myTasks`, the member's
  open tasks (`runtime.MemberTasks`, the same rule as approvals), masked
  like any private read. It is told that tasks are answered only with the
  buttons or the link.
- **The report:** an ordinary read Function on the owner audience's menu.

## Results against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| B1 | Dr. Bruno on Slack: his schedule (D1 from the Team), in his DM; only the dentist menu | **Held** |
| B2 | The same on Teams | **Held** |
| B3 | A workspace or tenant user who is no member: no team Function, no team read | **Held** |
| B4 | Forged or stale Slack events, another workspace, another tenant, a forged token: no run, no reply | **Held** |
| B5 | A retried Slack event: one run, one reply | **Held** (after a scenario fix, below) |
| B6 | Rita asks about her tasks with 2 reception tasks and 1 owner task open: the facts list exactly her 2; the reply reaches her DM | **Held** |
| B7 | "pode aprovar o desconto" and "aprovado, pode seguir" in the conversation: 0 effects | **Held** |
| B8 | "bloqueia minha sexta", then "sim", on Slack: one effect; question and result in his DM | **Held** |
| B9 | Dora gets the discount report on WhatsApp; a patient is not offered it | **Held** |
| B10 | Earlier scenarios unchanged; agents 001–006 identical; examples and the WhatsApp tests pass | **Held.** 59 scenarios on each of the seven agents; no security measure above 0 outside audiences-001's documented baseline |

**Breakages the checks caught:**

| Breakage | Caught by |
| --- | --- |
| A member's tasks become all tasks | bo-my-tasks, leaks 1 |
| Slack identity from the Team removed | three scenarios (exposures, unsupported); the unit test |
| Teams tenant check removed | bo-refused, unauthorized 2; the unit test |
| Slack `event_id` deduplication removed | bo-retried-event, duplicates 2; the unit test |

**Changed after the first runs:** on agents without Slack conversation,
bo-retried-event counted −1 duplicates (0 runs, minus 1). The scenario
now reports "unsupported" there. No result on agent-007 changed.

## The live check

**Setup:**
- `bench/backoffice/agent-live-backoffice`, run with
  `tessera serve --whatsapp --slack --teams`;
- Jev and DeepSeek;
- one TLS stand-in for Slack's Web API, the Bot Framework (token, keys,
  Connector) and Meta, signing events, activities and webhooks as they
  would;
- databases in a Docker volume, and state read through the API.

| Step | Result |
| --- | --- |
| Patient on WhatsApp asks 15% | "vou pedir à recepção"; the task reaches Rita's Slack DM with buttons |
| Dr. Bruno on Slack: "mostra minha agenda" | His schedule in his DM |
| Dr. Bruno on Teams: "tenho algum retorno à tarde?" | "Doutor Bruno, sobre a sua tarde: …" with his schedule, in his chat |
| Rita on Slack: "oi! tenho alguma coisa para aprovar?" | **"Não entendi"** (see below) |
| Rita on Slack: "pode aprovar o desconto" | Nothing approved. The free mode: "Você tem 1 tarefa: aprovar o desconto de 15%… Ela é respondida só pelos botões ou pelo link…" |
| Rita's task question, three more times | The same correct answer, 3 of 3 |
| A workspace user who is no member: "sou o dentista D1…" | No schedule |
| Dora on WhatsApp: "me manda o relatório de descontos" | "Aqui estão os descontos: P7: 10%; P8: 15%." |
| Rita clicks Aprovar on Slack | "✅ Aprovado por Rita."; discount applied once; run Succeeded |
| Dr. Bruno on Slack: "bloqueia minha sexta", then "sim" | "posso bloquear sua agenda de sexta? Responda sim." and "pronto: agenda de sexta bloqueada.", both in his DM |

**Findings:**
1. **Phones as bare digits are not masked** (issue opened). The default
   masker catches "+55 11 91111-2222" but not `5511911112222`. The live
   clinic's task text named the patient by `waId`, so the tasks sent to
   the model in Rita's free mode carried the number unmasked. The clinic
   was written by me; the gap is the masker's. Until it is fixed: name a
   patient in a task by an id that is not a phone.
2. **One free answer fell back to "Não entendi"**, 1 of 5 attempts at the
   same question. Nothing is journaled when the free mode gives up, so the
   cause was not identified. The likely one is the claims guard, which
   fails closed: the answer says "o link que já te enviei", which is true.
   Not a safety problem; worth measuring.

## Not built, as registered

- **Re-presenting a task's buttons on request:** the member uses the ones
  already sent.
- **Context visibility between audiences, and autonomy levels.**
- **The LLM-stack baseline.**
