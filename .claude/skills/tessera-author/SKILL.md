---
name: tessera-author
description: Build or change a Tessera agent (plugins, Functions, Agent voice and free mode, EvalSuites) from a plain-language request, safely, and prove it works. Use when someone asks for a new agent, a new capability, or a change to how an agent talks, escalates or acts.
---

# Authoring a Tessera agent

You turn a request ("an agent that reschedules Agenda appointments, talks
like the front desk, and tells the team when it can't") into definitions
that validate, pass their EvalSuite, and hold under failures. The person
should not need to read YAML. Every rule below was measured in
`docs/research/`; the file is named where it matters.

Read first: `AGENTS.md`, `docs/guides/authoring.md`,
`docs/guides/testing.md`, `docs/guides/validation.md`. Use
`./bin/tessera explain <Kind.spec.field>` for any field you are unsure of.
Do not guess field names.

## The loop

1. **Understand the request.** Write down, in the person's words:
   - what users may ask;
   - what the agent may do to the world;
   - what needs a person;
   - what it must never do.

   Ask the person only what you cannot settle yourself. **Always ask for
   the effect class of each write** (next step): never guess it.
2. **Map the systems.** For each API: read-only actions, writes, and how to
   check that a write happened (a read that shows it). Declare the plugin
   with `http` bindings (`docs/guides/authoring.md`, section 1).
   - If the API's error texts are the business's own words (a rule, a
     phone, a fee), set `connection.publicErrors: true`, so a refusal
     reaches the user as what the system said (drift-002).
   - If an API may change what it requires (a new field, a fee), offer
     `improvise.repair` for its writes. Ask the person first: a person on
     their team approves each repair (drift-007).
   - **Effect classes:** `read`, `idempotent-write`, `irreversible`,
     `external-message`. A wrong class removes confirmation and
     verification from an irreversible action. Propose one, and have the
     person confirm each irreversible or external-message action.
   - Every irreversible action gets `verify` (a probe read) so a timeout
     after it applied is not repeated. This is the market benchmark's C1
     result (`docs/research/market-bench-001.md`).
3. **Write the Functions.** One per thing the user asks for, with a
   `selector.description` in the users' language, the way they ask.
4. **Write the Agent:**
   - `messages` in the users' language;
   - `voice` (persona, examples, `keep` with the action's verbs);
   - `freeMode` (instructions, public reads of the business's facts,
     `functions` for handing off to a person);
   - `humanInTheLoop`;
   - `audiences`, when the team also talks to the agent (below).
5. **Write the EvalSuite** (below) before you run anything.
6. **Run the gates** (below). Read failures in the run's timeline, not in
   the final message: `./bin/tessera runs show <id> <dir> --db ...`.
7. **Fix and repeat** until every gate passes. Then report to the person:
   - what the agent does;
   - what needs a person;
   - which effect classes they confirmed;
   - what you measured.

## Rules that were learned the hard way

**Confirmations and effects**
- The `confirm` names **everything** the action acts on: which appointment
  *and* the new time (`CONFIRM_INCOMPLETE`). A confirmation naming only
  the time moved the wrong appointment (market-bench-001).
- The `confirm` is a plain question: "Posso remarcar … para …? Responda
  sim para confirmar." Not "Confirmo …?", which simulated users read as
  already done (market-bench-003).
- `idempotencyKey` from the inputs that define the effect
  (`inputs.appointment.id + ':' + inputs.newSlot.id`), never a timestamp.
- A "no, X instead" at a confirmation restarts the Function from that
  reply: inputs must resolve from a single message.
- Give distinct answers to distinct failures with `when` on `onError`
  strategies (`sources.error.code == 'CONFLICT'` for a taken slot, `'NOT_FOUND'`
  for a vanished appointment). The safety rules still hold (#30).

**Asking**
- Narrow long lists: ask the day, then that day's times. Never two weeks
  of slots.
- Narrow to what the user already said, such as a period (manhã/tarde):
  - an extracted input, then a computed input (`value:`) with the matching
    options;
  - in voice-002 this halved the judge's clarity losses and removed its
    factual flags.
- Content is the lever for natural conversation: templates say who, from
  when and to when. The voice may not add facts, so what the template
  lacks the conversation lacks (voice-002).
- A request about someone else ("a consulta da minha mãe") must be
  detected, not silently applied to the caller (market-bench-001, t15).

**Routing and the free mode**
- `selector.description` also says what the Function does **not** do when
  a neighbouring request looks similar ("not for booking a new
  appointment"). Without it, a new-booking request reached the reschedule
  Function (freemode-001).
- Phrase a search's selector as a request to look for options, and list
  the opinion questions it is not for ("pousada ou hotel?", "quanto tempo
  dura o voo?"). That took general questions sent to a search from 30%
  to 0% with no search lost (router-004).
- Handing off to a person from the free mode is `freeMode.functions`, not
  a routed Function: the router cannot see the free mode's reads. A routed
  "ask the coach" sent 30–48% of answerable questions to the coach
  (escalation-000, 002).
- `when` names **every kind** of thing that goes to the person: decisions,
  approvals, exceptions, processes they start. It is followed literally.
  Missing "processes the coach starts" cost 17 points of recall
  (escalation-002).
- The handoff Function's message to the user still says what the material
  says (the rule, the step): generate it with the reads in view, or
  template the rule. A bare "I've passed this to your coach" was judged
  incorrect on every coach question, 57% correct against 95%
  (authoring-001).
- Calibrate `minConfidence` on labelled questions covering each kind, never
  on your test questions. Rule: keep 0.6 unless the calibration set shows
  errors, then take the most accurate threshold (escalation-003).
- Every agent gets `freeMode.instructions`, even without reads: what it
  does, what it does not, and what to offer instead. On the traveler this
  took off-script dead ends from 100% to 29%, with 0 false claims
  (traveler-freemode-001). The rest were general questions routed to a
  search: say in `selector.description` what the search is not for.
- When users ask for current public information the business does not
  hold (rules of airlines or countries, seasons, requirements), offer web
  search: `tessera add web` and `freeMode.search`. Ask the person first:
  it sends queries to an outside provider. Queries never carry personal
  data, and the results are cited, never obeyed (websearch-001).
- Mark the business's own facts `public: true` on free-mode reads, or a
  clinic phone becomes a masked placeholder the free mode refuses to send.

**Audiences and the team** (audiences-001)
- **When the team also talks to the agent,** give each kind of caller an
  audience: its `functions` (the menu), `voice`, `freeMode` and
  `messages`. The last audience has no `who` and catches the patients.
- **Roles come from the `Team`, never from text.**
  - Put each member's WhatsApp number and `attributes` (such as
    `dentistId`) in the Team.
  - Write `who` over `caller.roles`, and read ids from
    `caller.attributes`.
  - "Sou o dentista" from another number is a patient.
- **Keep `authorize` on team Functions** as the second barrier. A
  team free mode's reads use the member's attributes, so they cannot
  read for anyone else.
- **Let the team talk to the agent on Slack and Teams:** add `post-text`
  to those plugins. Give team audiences `freeMode: { tasks: true }`, so
  "tenho algo para aprovar?" is answered (backoffice-006).
- **Policies, FAQ and prices go in a `KnowledgeBase`** (Markdown files,
  one topic per heading) named in `freeMode.knowledge`, not pasted into
  `instructions`. Staff-only documents get their own base with
  `audiences` (knowledge-001).
- **Leave `freeMode.self` on (the default)**, so the agent says truthfully
  what it does for the person asking, what it needs and who approves.
  It is built from the definitions and never goes stale (self-001). Do
  not list capabilities in `instructions`. Write Function descriptions
  as what the agent does ("Shows the week's discounts").
- **Never put a phone number (such as `caller.waId`) in a task's
  message:** name the patient by an id. The default masker misses phones
  written as bare digits (#55).
- **What the team sees of a conversation:** `sees` on the team audience,
  masked unless the person asks for full.
- **When a person must approve every action,** or every message, use
  `autonomy: approve-actions` or `draft`. A draft audience gets no free
  mode, and `messages.withAPerson` must be in the users' language.
- **Approvals on the team's channels:** `HumanQueue.channels`
  (whatsapp, slack, teams, email) and the members' addresses; see
  `docs/guides/authoring.md`. After an `escalate` with `task: approve`,
  a denial ends the Function: write `onDeny` in the users' language.

**Voice**
- `keep` lists the action's verbs ("remarc"), so the rewrite cannot turn
  "remarcar" into "agendar" (freemode-001).
- Leave `greeting` at `first`. Greeting freely did not change the judge's
  verdict (voice-002).
- Leave `latitude` at `warm`, the default: it took the blind judge's
  "natural" share from 13–16% to 55% on two seeds (voice-004/005).
  `strict` is only for texts that must go out exactly as written.
- Leave `dialogue` at `full`, the default (voice-009). `marker` hides the
  dialogue from the voice and measured about 10 points less natural.
- The persona and examples should sound like a warm person (light emoji,
  a closing offer). authoring-002's persona asked for short sentences,
  with the strict voice, and scored 4%. The two causes were not
  separated, so do not ask for brevity unless the person wants it.

## The EvalSuite (write it first)

The suite needs at least:
- the happy path;
- a refusal: not allowed, or not the caller's;
- an unknown or ambiguous input;
- a change of mind at the confirmation;
- a question in the middle of a process (the free mode answers it, and the
  pending question comes back);
- a request no Function covers, and a request that needs a person;
- for **every irreversible action**, the plugin failing definitely
  (`definite("NOT_FOUND", ...)`) and ambiguously (`ambiguous("timeout")`
  after applying). There must be no duplicate effect.

Use `simulator: scripted` turns for exact paths. Scenario syntax is in
`docs/guides/testing.md`.

## Gates (all must pass before you say it is done)

```bash
./bin/tessera validate <dir>                              # 0 errors; read every warning
./bin/tessera test <dir> --decide typesafe:jev-1.13.0 \
  --generate openrouter:deepseek/deepseek-v4.1-flash      # live, budgeted
./bin/tessera test <dir> --cassette c.jsonl --cassette-mode record
./bin/tessera test <dir> --cassette c.jsonl --cassette-mode replay   # what CI runs
```

- **Keys:**
  - `TYPESAFE_API_KEY` and `OPENROUTER_API_KEY`, from the environment or
    as the person provides them.

  Never print, echo or commit a key.
- If a gate cannot run (no key, no network), say which, and do not claim
  it passed.

## Reporting

End with a short report for the person:
- the capabilities;
- the actions and their effect classes (marking the ones they confirmed);
- what goes to a person;
- the gates' results, with numbers;
- what you could not settle.

Do not paste YAML unless asked.
