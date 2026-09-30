# Writing an agent

From an empty folder to an agent that is validated, tested in whole
conversations, and safe to connect to a real system. Every file below is a
real file in [`examples/traveler`](../../examples/traveler); read them next
to this guide. Calendar helper signatures are in [expressions](expressions.md).
When working with a distributed CLI, read the [partner testing notes](partner-testing.md).

## 1. The plugin: what the agent can do to the world

A `PluginManifest` lists actions with their parameters, output type and
**effect class** (`read`, `idempotent-write`, `irreversible`,
`external-message`). The effect class decides what the runtime demands:
reads can be retried; irreversible actions need an idempotency key, are
never repeated when their outcome is unknown, and are confirmed or
approved.

A REST API needs no code: declare the request under `http`
([`plugins/booking.yaml`](../../examples/traveler/plugins/booking.yaml)).

```yaml
connection:
  baseUrl: https://booking-com15.p.rapidapi.com/api/v1
  auth: { header: x-rapidapi-key, secret: BOOKING_RAPIDAPI_KEY }
actions:
  search-flights:
    input: { fromId: string, toId: string, departDate: date, ... }
    effect: read
    http:
      method: GET
      path: /flights/searchFlights
      query: { fromId: "${ inputs.fromId }", ... }
      output: "sources.response.body.data.flightOffers"
```

**An API that takes an OAuth2 token** (the Bot Framework, Microsoft
Graph) uses `connection.oauth2` instead of `auth`:
- it uses the client-credentials grant: `tokenUrl`, `clientId` and
  `clientSecret` (names of secrets) and `scope`;
- the token is fetched through the plugin's egress, so the token URL's
  host goes in `egress.hosts`;
- it is kept until a minute before it expires, or until an API answers
  401;
- a refused token request fails the call before it is sent, with the
  endpoint's reason and never the secret.

**Syntax, where it differs:**
- `path` is a template: `/patients/{{ inputs.patientId }}/appointments`.
  Each value is escaped as one path segment.
- `query` and `body` values are literal unless the whole value is
  `"${ expr }"`.
- `idempotencyKey`, `when` and `value` sources are bare CEL, without
  `${ }`: `idempotencyKey: "inputs.appointment.id + ':' + inputs.newSlot.id"`.

**How HTTP answers become failures** (`internal/plugins/http/http.go`):

| The server's answer | Failure | Code |
| --- | --- | --- |
| Could not connect | definite, retryable | `UNREACHABLE` |
| No response, 408, 5xx | **ambiguous** (the effect may have happened) | — |
| 429 | definite, retryable | `RATE_LIMITED` |
| 401, 403 | definite | `FORBIDDEN` |
| 404 | definite | `NOT_FOUND` |
| 409 | definite | `CONFLICT` |
| other 4xx | definite | `INVALID` |
| 3xx, a body that is not JSON, or a `failWhen` that holds, for an action that changes something | ambiguous | — |

An ambiguous failure of an irreversible action is verified, never
repeated blind (see "What `onError` does").

**What the system said reaches the user** (drift-002). When a server
refuses with a 4xx, the plugin keeps its words: the `error`, `message`,
`detail` or `title` field, up to 300 bytes. After the failure's message
(`messages.failed`, an `onError` `respond`, an `escalate`'s
`whileWaiting`), the user gets one or two sentences of what the system
said they need to know or can do: a new rule, a phone to call, a fee, a
missing requirement.
- It never says the action was done; a Jev check drops any explanation
  that does.
- The system's words are masked like personal data unless the plugin
  declares `connection: { …, publicErrors: true }`. Declare it when the
  API's error texts are your business's own words, or a phone to call
  becomes a placeholder and the explanation is dropped.
- Ambiguous failures (timeouts) are unchanged: nobody knows what
  happened.
- It is **on by default**, together with the free mode's claims guard
  (it never says it registered or passed something on). Measured in
  drift-005/006: 0/96 false claims under API drift, every refusal
  explained. The plain LLM loop made 14–23 false claims.
  `Agent.spec.explainFailures: false` turns both off.

Secrets are names, never values (`TESSERA_SECRET_BOOKING_RAPIDAPI_KEY`
when self-hosted); `tessera validate` refuses a credential written into a
definition and a host outside `egress.hosts`.

**State between turns** (a user's profile, the last search) lives in a
plugin with its own database, the built-in `sql` plugin: migrations run
once each, statements are literal SQL with `:name` parameters (a value can
never change a statement), a SELECT must be a read, and
`:idempotencyKey` lets a write refuse to happen twice.

```yaml
spec:
  sql:
    migrations:
      - CREATE TABLE searches (conversation TEXT, key TEXT UNIQUE, results TEXT, at TEXT)
  actions:
    last-search:
      input: { conversation: string }
      effect: read
      sql:
        query: "SELECT results FROM searches WHERE conversation = :c ORDER BY at DESC LIMIT 1"
        params: { c: "${ inputs.conversation }" }
        result: row
        json: [results]
```

In an EvalSuite a `sql` plugin without a sandbox runs for real, on a fresh
database per scenario.

## 2. The Function: what the user can ask for

A `Function` declares what it needs (`inputs`), what must be true
(`preconditions`), what it computes (`steps`), the effect (`action`) and
what the user is told (`output`)
([`functions/search-flights.yaml`](../../examples/traveler/functions/search-flights.yaml)).

**Inputs** are resolved from their sources, in order, until one gives a
value:

```yaml
inputs:
  destination:
    type: string
    description: the city or airport the user wants to fly to
    resolve:
      - { id: said, extract: {} }                       # from the message
      - id: asked                                       # or ask
        ask: { text: "Para onde você quer voar?", minConfidence: 0.7, onLowConfidence: ask-user }
```

All `extract` sources of a Function share **one** model call; write a
`description` for each input, because that is what the model reads. Ask
only for what is required; give defaults with `value` sources. The answer
to a free-text question is read with the input's description and extract
instructions, and with the inputs already resolved, so "fico 2 noites"
answers a check-out question after the check-in is known: write inputs in
the order a person would ask them.

**A reply can answer more than was asked** (agency-001). The call that
reads the reply also reads the later inputs the first message left empty:
"15 de novembro, volto dia 22", answering "Qual o dia da ida?", gives the
return too, and it is not asked again. What the first message said is
never overwritten.

**What the user said before the request** (`extract: { conversation:
true }`, agency-002). By default an extract reads only the message that
started the Function. Marked, a value may also come from the user's
earlier messages in the conversation (the last 16 lines) when the message
leaves it out: "eu e minha esposa" said while asking about the weather is
there when the hotel search starts. The message always wins.
- Mark what describes the person or the trip: who travels, where from,
  where to.
- Leave unmarked what belongs to one request: "mais barato", a pick, a
  new trip's options.
- Say back what you assumed. The traveler's answers start with the route,
  dates and group they used.

A `format` input marked the same way also looks for its pattern in the
earlier messages, the latest first.

**Dates without a year** (`extract: { future: true }`). Models often read
"10 de julho", said in September, as this July. Marked, a date more than
a week before the event moves a year on; "ontem" stays past.

**Preconditions say what was understood.** `message` is a template:
"Entendi a ida como {{ … }}, que já passou" lets the user correct a
misreading instead of hearing it as a fact.

**Personal data is read by code** (`format: email | phone | cpf`, on a
`string` input). Models only get masked text, where an e-mail is
`[EMAIL]`; without `format`, asking for one could never work (in
agency-001 the booking's e-mail became the literal `[EMAIL]`). With it,
the extract source and the question take the first match in the user's
own words, and no model sees it:

```yaml
email:
  type: string
  format: email
  resolve:
    - { id: said, extract: {} }
    - { id: asked, ask: { text: "Qual e-mail para receber a reserva?", minConfidence: 0.7, onLowConfidence: ask-user } }
```

**Give every question a way out.** An input that is asked needs an
`onUnresolved`: without one, a user who says "te mando depois" twice ends
the run as failed ("algo deu errado") instead of hearing "sem problema,
me mande quando tiver".

**Extract preferences, not inferred restrictions.** A precise requested value
must not create an additional filter the user did not ask for. In
[availability-001](../research/availability-001.md), extraction returned
`clockTime: "17:00"` and `period: "evening"`; the agent's evening filter
started at 18h and discarded an available 17h slot. The measured instruction
keeps the period only when requested explicitly:

> period representa somente uma preferência de período explicitamente
> solicitada, nunca uma classificação inferida do horário numérico. Se o
> usuário informar apenas uma hora exata, use period=any e preserve clockTime.

Apply the same rule to the initial extraction and the description used for
follow-up answers. Preserve explicit constraints, even conflicting ones;
do not silently drop a stated preference to make an option fit. Test exact
times near period boundaries, greetings, the old appointment's time and
unavailable slots. Compare extracted fields and the actual filtered choices:
a final "no write" alone cannot prove the conversation answered the request.

**Routing.** `selector.description` is what the router reads. Write it in
your users' language, the way they ask: measured with Jev, this moved a
vague "quero viajar" from 0.09 to 0.91
([router-001](../research/router-001.md)). Measure a namespace's router
with `tessera bench route` on labelled messages, and keep a holdout set
you never tune on: on the traveler, two changes that looked principled
made the holdout worse ([router-002](../research/router-002.md)).
A follow-up Function ("e o da LATAM?") declares `selector.after: [the
Functions it follows]`: it is an option only right after them, which
stopped general questions from reaching it where wording could not. Set
the namespace's threshold from its benchmark with
`Agent.spec.routing.minConfidence` (default 0.6).

**Expressions and templates.** A whole value `"${ expr }"` is CEL; text
with `{{ expr }}` is a template, escaped for the channel. CEL has no
`now()`: use `event.time`, and the date helpers (`date('2026-10-12',
'America/Sao_Paulo')`, `formatDate`, `startOfDay`, `addBusinessDays`)
always take a time zone.

**Logic that is not an expression** goes in Starlark
([`scripts/`](../../examples/traveler/scripts)): deterministic, sandboxed,
no clock, no I/O. Test it like code; the traveler's scoring is proven
against the original bash on the same data
([`scoring_test.go`](../../examples/traveler/scoring_test.go)).

- **Limits per call:**
  - 10 million steps;
  - 100 000 values in the result;
  - the heap may grow by 256 MiB.
- **Isolation:** under `tessera serve`, each call runs in a child process
  with a hard 1 GiB data limit on Linux. A script that allocates too much
  in one instruction (`[0] * (200 << 20)`) fails its step with "the
  memory limit was reached" instead of taking the server down
  (starmem-001). This costs ~4 ms per call; `--in-process-scripts` turns
  it off.

**Failures** are handled where they happen: a step's `onError` (`respond`,
`escalate`, `fallback`, `retry`, `probe`, `fail`), an input's
`onUnresolved`. `onUnresolved` can read what the sources found, so it can
say *why* ("no appointments" is not "you did not pick one").

**What `onError` does, exactly** (`internal/runtime/action.go`). The
strategies run in order, until one ends the call. A failure is either
*definite* (the system said no; nothing happened) or *ambiguous* (a
timeout: the effect may have happened). Two rules hold whatever the chain
says:
1. **An ambiguous irreversible call is verified first.** If its plugin
   declares `verify` (probe or native), it is checked before the chain,
   even without a `probe` strategy.
2. **It is never repeated or re-routed blind.** While the outcome is
   unknown:
   - `retry` and `fallback` are skipped;
   - `respond` and `fail` become **Indeterminate**: the run goes to a
     person with the facts, never ends quietly.

Each strategy:
- `probe: {}` verifies again: happened → the call succeeds with the
  probe's output; did not happen → the failure becomes definite, so a
  later `retry` is allowed. It is skipped after a definite failure.
- `retry: { attempts, backoff }` repeats the call. It stops being allowed
  as soon as an attempt fails ambiguously.
- `fallback` takes another action, Function or value. Later strategies act
  on the fallback's call.
- `escalate` hands the call to a queue and waits for the person.
- `respond` sends a message and ends the run.
- `fail` ends the run as failed.
- If the chain ends without success: an unknown outcome → Indeterminate;
  otherwise the run fails.

**Telling failures apart (`when`).** Any strategy can carry a `when`, a
boolean expression that reads the failure as `sources.error`:
- `code`: `CONFLICT`, `NOT_FOUND` and the rest of the table above;
- `message`;
- `definite`.

It also reads `inputs`, `steps` and `caller`. Only when it is true does the
strategy run, and a `respond` template can quote the failure:

```yaml
onError:
  - { when: "sources.error.code == 'CONFLICT'", respond: { template: "Esse horário acabou de ser ocupado. Quer ver outros?" } }
  - { when: "sources.error.code == 'NOT_FOUND'", respond: { template: "Não encontrei mais essa consulta. Vou te mostrar as suas consultas." } }
  - { probe: {} }
  - { escalate: { queue: recepcao, task: input } }
```

`when` only narrows the chain:
- an irreversible action whose outcome is unknown is still verified first,
  and still reaches a person instead of a plain `respond`;
- a `probe` with a `when` does not count as "verified" for a later `retry`
  or `fallback` (`UNSAFE_RETRY`).

In an input's `onUnresolved`, `sources.error` is the last failed read, and
its `code` is `""` when the sources simply found nothing (#30).

A good chain for an irreversible action is `probe`, then `retry`, then
`escalate` with a `whileWaiting` message (the clinic example).

**How an `ask` with `choices` works** (`internal/runtime/inputs.go`):
1. The choices expression makes the options (at most 254). Each option's
   label is its `label` field when it has one.
2. **The question's text is sent as written; the options are not
   appended.** The text must list them itself, e.g. `{{ opts.map(o,
   o.label).join(', ') }}`.
3. **The triggering message is read first.** If it already picks an option
   above `minConfidence`, nothing is asked: "remarcar para terça" answers a
   day question. Write inputs so one message can resolve them, which is
   also what makes a correction at the confirmation work.
4. Otherwise, with `onLowConfidence: ask-user`, the text is sent and each
   reply is read, up to the runtime's ask limit. A reply that picks
   nothing asks again.
5. Other `onLowConfidence` values ask a person (the default queue) to pick.

"None of the options" is always an option, so a question about something
else is not forced into one.

**Types.** An input or action output type is one of:
- a basic type: `string`, `int`, `float`, `bool`, `date`, `timestamp`,
  `duration`, `map`, `any`;
- `list<T>`;
- a plugin-qualified name such as `clinic.Appointment`. The plugin must be
  one being validated. The name documents the shape: fields are not
  declared or checked, so CEL reads them as a map (`inputs.appointment.label`).

**Run statuses you will meet in tests:**
- `WaitingForUser`: a question is open.
- `Confirming`: the confirmation of an irreversible action is open; the
  next message is read as yes, no or a correction.
- `WaitingForHuman`: a queue task is open.
- `WaitingForTime`: a `wait` sleeps.
- `Indeterminate`: an effect's outcome is unknown and a person was asked.
- `Succeeded`, `Failed`.

**Waiting.** `- id: d1, wait: { until: "atTime(addBusinessDays(event.time,
1, 'America/Sao_Paulo'), '10:00', 'America/Sao_Paulo')" }` sleeps until the
next business day at 10h; `wait: { for: 48h }` counts from when the step is
reached. A sleeping run survives restarts, costs one row and does not block
the conversation: the user's messages start other runs meanwhile.
`wait: { for: 48h, orReply: true }` waits for the user's reply *or* the
time, whichever comes first: the reply goes to this run (not the router),
and `steps.<id>.output` is `{reply, timedOut}`, so a follow-up cadence
stops when the lead answers.

**Schedules.** A `Trigger` starts a Function without a message, on a cron
schedule read in the Agent's `spec.timezone` (never the server's):

```yaml
kind: Trigger
metadata: { name: morning, namespace: clinica }
spec:
  function: daily-report
  schedule: { cron: "0 9 * * 1-5", maxDelay: 2h }   # 9h on weekdays
```

Each scheduled time starts one run, however many schedulers run (the run's
ID is the trigger and the time); `event.time` is the scheduled time and
`event.trigger` the trigger's name. After downtime, a time later than
`maxDelay` (default 1h) is skipped, never replayed in a burst. `tessera chat`
runs the scheduler every second; a host calls `Host.Tick`.

**Webhooks.** A `Trigger` with `webhook: { secret: NAME }` starts its
Function when another system POSTs JSON to `/v1/hooks/<trigger>` on
`tessera serve`, signed: `X-Tessera-Timestamp` is the time in unix
seconds and `X-Tessera-Signature: sha256=<hex HMAC-SHA256, with the
secret, of timestamp "." key "." body>`, where key is the
`Idempotency-Key` header (empty without one). The key and the time are
signed so a captured delivery cannot start new runs under new keys, nor
be replayed more than 5 minutes later. The body is `event.payload`;
`caller.trigger` is the trigger's name (use it in `authorize`). An
`Idempotency-Key` (up to 128 letters, digits, `- _ . :`) makes a repeated
delivery start nothing: the run's ID is the trigger and the key. A
`webhook.key` expression (e.g. `event.payload.leadId`) takes the key from
the payload instead: one run per lead, however often the CRM sends it.

**Events.** A wait can also end when another system reports something:
`wait: { for: 48h, orReply: true, events: [{ name: reuniao-marcada, key:
"event.payload.leadId" }] }`. A webhook Trigger with `event: { name:
reuniao-marcada, key: "event.payload.leadId" }` delivers its payload to
the runs waiting for that event with the same key; with a `function` too,
it starts it when nobody waits. The wait's output says what ended it:
`{reply, timedOut, event, payload}`. With events, the reply path is
`steps.<wait>.output.reply != null` (an event is not a message from the
user, so it opens no channel window). There is no separate Workflow kind:
a Function with waits is one ([ADR-0006](../adr/0006-no-workflow-kind.md)).

**Milestone triggers.** `milestone: { subject: loja, name: loja-aberta }`
on a Trigger starts its Function once for every such milestone recorded,
whoever recorded it (the team in chat, the ERP by webhook), in the
conversation of the run that recorded it, with `event.payload`
{subject, id, name, at, attributes}. The scheduler checks every tick, with
a cursor kept in the database; milestones recorded before the trigger
existed do not start it. `testdata/lojas` tells the board when a
store opens.

```sh
ts=$(date +%s); key=evt-1; body='{"unidade": "12", "etapa": "obra"}'
sig=$(printf '%s.%s.%s' "$ts" "$key" "$body" | openssl dgst -sha256 -hmac "$SECRET" -r | cut -d' ' -f1)
curl -X POST localhost:8080/v1/hooks/erp-etapa -d "$body" -H "X-Tessera-Timestamp: $ts" \
  -H "Idempotency-Key: $key" -H "X-Tessera-Signature: sha256=$sig"
``` `testdata/lojas` takes the same steps from the team (chat)
and from the ERP (webhook) with one Function.

**Channels (WhatsApp).** A plugin with `spec.channel` declares a
channel's rules and the platform applies them: `window: 24h` (free text
reaches the user only within 24h of their last message; outside it a
`respond` fails with WINDOW_CLOSED, which its `onError` answers with a
template action) and `consent` (an expression, e.g. the CRM's opt-in in
`event.payload`, checked before every `external-message` action of the
Functions; without it nothing is sent, CONSENT_MISSING). The Agent's
`spec.channel` names the plugin its replies go through. Admission follows
every path (`WINDOW_RISK`): the window may be closed at the start of a
Function a Trigger or Funnel starts, after waits that reach the window
alone or added up, after an `escalate`, after a `call` that waits, and on
the timeout path of a `wait` with `orReply`. It is open again after an
`ask` the user answered and on the reply path, a branch case whose `when`
is exactly `!steps.<wait>.output.timedOut`. There, a `respond`, `ask` or
output template needs an `onError` fallback act that sends a template; a
`call` into a Function that sends free text is refused; a `freeText`
action must be replaced by a template (a fallback after it could send
twice). The runtime applies the same rule to every message, so what
admission cannot see still fails closed. A consent rule
authorizes outbound messages the way a confirmation authorizes other
irreversible effects. See `testdata/cadencia`: a cadence that stops when the
lead answers, tested on a simulated clock.

**Business numbers.** Declare who numbers are about (`kind: Subject`, a
pseudonymous `id` expression and closed attributes), put a `milestone` on
the step or action that makes a fact true, and follow subjects through a
`Funnel` (see `testdata/lojas`):

```yaml
  action:
    act: agenda/reschedule
    milestone: { name: remarcou, subject: paciente }   # only when confirmed
```

A milestone is recorded once, when the step succeeds (an effect: when
confirmed; a "no" records nothing). `expect.milestones` checks them in
EvalSuites. `tessera metrics funnel <dir> --namespace ns --funnel name --db
runs.db [--by attribute]` shows, per cohort, how many reached each stage
from the previous one, how many are still inside the window (pending, not
lost), the median and p90 time of each stage, and who waits longer than
1.5 times the stage's `expected`. With `onStuck: { function: ... }` the
scheduler starts that Function for each stuck subject, once per subject and
stage, with `event.payload` {funnel, stage, subject, since, days}: usually
an `escalate` to the team (`testdata/lojas`, `avisar-atraso`).

**Reports and alerts.** The built-in plugin `metrics` has read actions
`metrics/funnel` and `metrics/experiment`: a Function reads the numbers of
its namespace (durations in days, totals over cohorts) and a
`respond: { template: ..., to: "queue:<name>" }` step notes them to the
team without waiting. A schedule Trigger makes it a weekly report
(`testdata/lojas`, `relatorio-semanal`); a step with `when:` on the
numbers makes it an alert. Starlark's `%` has no precision (`%.1f`
fails): round by hand.

**A/B tests.** An `Experiment` names a Subject, variants with weights (the
first is the control) and the primary outcome, a stage of a Funnel with a
`within` window. A step `- { id: v, experiment: primeira-mensagem }` gives
the subject its variant (a hash: always the same one) and records that it
saw it; later steps branch on `steps.v.output`. `tessera metrics experiment
<dir> --namespace ns --experiment name --db runs.db` compares the variants
among the exposed with a sequential test, so reading it every day does not
produce false winners ([experiments-001](../research/experiments-001.md));
"inconclusive" until each variant has `minSamplePerVariant` decided
subjects. Promoting a winner is a change you make to the definitions.

`guardrails` are outcomes that must not rise: `{ name: opt-out, milestone:
opt-out, within: 7d, maxIncrease: 0.02 }` is the share of exposed subjects
with an `opt-out` milestone within 7 days. When a variant is significantly
worse than the control (the same sequential test) by more than 2 points,
the scheduler stops the experiment: every subject exposed from then on
gets the control and is left out of the comparison, and `onBreach` (a
Function, required with guardrails) starts once with `event.payload`
{experiment, guardrail, variant, controlRate, variantRate, difference}.
Measured in [experiments-002](../research/experiments-002.md): harm of 5
points over a 3% base is stopped every time at a median of 400 subjects
per arm; an A/A test read 200 times stops 1 to 4 times in 400. A stopped
experiment stays stopped; to try again, write a new Experiment.
`testdata/cadencia` guards its first-message test with the template's "Parar"
button.

**People and deadlines.** `escalate` hands a task to a queue and the run
waits. A `HumanQueue` gives the queue a service level: `remind` (the task
again), `escalate` + `escalateTo` (another queue), `timeout` +
`onTimeout` (`answer:` a safe default such as `false`, or `fail: true`),
counted in `businessHours` in the Agent's time zone when set. Each stage
happens once. The inbox is `GET /v1/tasks` (and the console); every answer
must send back the task's `taskHash`, so nobody approves something that
changed and a late or retried answer never answers the run's next task
(409 otherwise: read the task again), and `by` records who answered.

**A denied approval stops the Function** (#53). After an `escalate` with
`task: approve`, nothing more runs unless the answer is yes: no later step
and no action.
- A "no", an unclear answer and a timeout's `answer: false` all count as
  denials.
- The user hears the step's `onDeny` (`{ template: … }`), else the Agent's
  `messages.denied`.
- The run ends Succeeded, "the approval was denied".
- Before this, a denial returned `false` and the Function went on. In the
  backoffice-003 live check a denied 60% discount was applied, and the
  patient was told "aprovado".

**Where people hear about tasks** is pluggable: `notify` lists plugin
actions called for every task message (created, reminded, escalated),
once each, with the task as `inputs` (`text`, `kind`, `queue`, `stage`,
`runId`, `since`, `link`). Slack, WhatsApp, e-mail or your own webhook
are plugins like any other; the platform knows none of them. Only
`external-message` actions, and never those of the users' channel plugin
(the one with `spec.channel`): its consent and window rules protect users,
and a notification would skip them, so admission refuses it
(`NOTIFY_USER_CHANNEL`). The team's WhatsApp is its own plugin
(`testdata/cadencia`, `whatsapp-equipe`). The task text goes to that channel
as written: keep users' personal data out of `escalate` messages or out of
the notification's `with`. `link` is a
signed page to answer the task (`tessera serve --public-url
https://...`, 7 days, bound to the task's hash, signed with its own key:
`TESSERA_LINK_SECRET`, or one made on first start in the data directory; opening it changes
nothing, so link previews in chat apps answer nothing). Whoever holds
the link can answer: the name they type is recorded, not proven. A failed
notification is tried again after a minute, then with a doubling delay up
to an hour, without holding back other runs; one is recorded in the run's
journal only after it succeeded.

```yaml
kind: HumanQueue
metadata: { name: implantacao, namespace: rede }
spec:
  sla: { remind: 4h }
  notify:
    - act: slack/post            # testdata/lojas/slack.yaml
      with: { channel: "#implantacao", text: "${ inputs.text + ' ' + inputs.link }" }
```

An API that reports errors in a 2xx body (Slack answers
`{"ok": false}`) declares `http.failWhen: "!sources.response.body.ok"`;
without it a refused post would count as sent.

**The Agent.** One `Agent` per namespace holds what every Function
shares: the messages the platform sends on its own (not understood, a
person is handling it, not allowed, cancelled, failed), written in your
users' language, and the queue for escalations that name none. Without
it users get English defaults. `tessera explain Agent.spec`.

```yaml
kind: Agent
metadata: { name: traveler, namespace: traveler }
spec:
  messages:
    notUnderstood: "Desculpe, não entendi. Posso buscar passagens e hospedagem."
    failed: "Algo deu errado. Alguém da equipe vai te responder por aqui."
  humanInTheLoop: { queue: viagens }
```

**Declared types and read contracts** (drift-008). Declare what your
reads return, and every read is checked:

```yaml
spec:
  types:
    Appointment: { fields: { id: string, start: string, label: string, note: "string?" } }
  actions:
    list-appointments: { input: { patientId: string }, output: list<clinic.Appointment>, effect: read, http: … }
```

- A missing or mistyped required field is a definite `CONTRACT` failure
  that names the fields. Your `onUnresolved` or `onError` sees it, and
  the journal shows the API changed. Extra fields are always fine; `?`
  makes a field optional.
- With `improvise: { repairReads: true }`, a `CONTRACT` failure is first
  repaired by **renaming fields only** (`startsAt` → `start`). The model
  proposes the renames, and the platform refuses one that moves a
  declared field onto another or uses a source twice. The renamed answer
  must meet the type. The rename is journaled. Reads change nothing, so
  no approval is asked.
- Undeclared plugin types (`clinic.Visit` with no `types` entry) are
  still any object, so existing plugins are unchanged.

**Repairing a refused action** (`Agent.spec.improvise.repair`, off
unless set; RFC-0005 step 3, drift-007). APIs change: a new required
field, a fee that must be accepted. List the actions whose refused calls
may be repaired:

```yaml
spec:
  humanInTheLoop: { queue: recepcao }   # required: a person approves each repair
  improvise: { repair: [clinic/reschedule] }
```

When a listed action is refused definitely and the system says what it
needs:
1. **The model proposes only additions** to the request body. The
   platform rejects any change to a field already sent, any structure,
   more than three fields, and a question that drops an amount the
   system named.
2. **The user** is asked for a value only they know (a reason), and must
   agree with a clear yes to anything the change commits them to (the
   fee, by its amount).
3. **A person on the humanInTheLoop queue approves** the repair, seeing
   what the system said, the fields, and what the user agreed to.
4. **The action runs again** as an effect, with its own idempotency key
   (`<key>/repair`) and the action's verification.

Only http actions that send a body can be listed. The address, method,
headers and credentials never change.

**Who answers the team's tasks** (RFC-0006, backoffice-001):

```yaml
kind: Team
metadata: { name: equipe, namespace: clinica }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao], contact: "+5511900000001" }
    - { id: dora, name: Dora, roles: [dono], contact: "dora@clinica.com" }
  verify: { act: whatsapp/send-code, with: { to: "${ inputs.contact }", text: "${ 'Seu código: ' + inputs.code }" } }
---
kind: HumanQueue
metadata: { name: recepcao, namespace: clinica }
spec: { roles: [recepcao], sla: { escalate: 2h, escalateTo: gerencia } }
```

- **A queue with `roles`** is answered only by Team members holding one,
  and, after an escalation or a forward, by the members of the queues the
  task reached.
- **On the task's signed page** the person picks who they are, and a
  one-time code goes to that member's contact: a forwarded link approves
  nothing.
- **On the API,** `by` must be such a member.
  `POST /v1/runs/{id}/forward {queue, by}` hands a task on.
- **A queue without roles** works as before (whoever the channel
  trusts).
- **`escalate.route: [{when, queue}]`** picks the queue by a declared
  rule: `when: "inputs.percent > 50"` sends to `dono`.
- **`selector.who: "caller.role == 'dentist'"`** keeps a team Function
  out of a patient's menu. `authorize` still decides.

**Approving on WhatsApp** (backoffice-002). A queue with
`channels: [whatsapp]` sends each approval task to the WhatsApp of every
member who may answer it (`whatsapp: "5511900000001"` on the member),
with Aprovar / Recusar buttons.
- The tap comes back on Meta's signed webhook from the member's number,
  so the channel is the identity and no code is asked.
- The platform still checks the member's role and the task's hash.
- Free text ("pode seguir") never approves.
- Run `tessera serve --whatsapp`.
- The Agent's channel plugin needs a `send-buttons` action:

```yaml
    send-buttons:
      input: { to: string, text: string, buttons: list }
      effect: external-message
      freeText: true
      http:
        method: POST
        path: /messages
        body:
          messaging_product: whatsapp
          to: "${ inputs.to }"
          type: interactive
          interactive:
            type: button
            body: { text: "${ inputs.text }" }
            action: { buttons: "${ inputs.buttons.map(b, {'type': 'reply', 'reply': {'id': b.id, 'title': b.title}}) }" }
```

Outside WhatsApp's 24h window Meta refuses free-form messages (#131047).
When `send-buttons` is refused that way and the plugin also declares
`send-task-template`, the task goes out as an approved template instead,
with the same inputs. Register a template (utility category) with one body
parameter and two quick-reply buttons, Aprovar and Recusar. The buttons'
payloads carry the task ids, so a tap is answered exactly like the
interactive buttons:

```yaml
    send-task-template:
      input: { to: string, text: string, buttons: list }
      effect: external-message
      http:
        method: POST
        path: /messages
        body:
          messaging_product: whatsapp
          to: "${ inputs.to }"
          type: template
          template:
            name: tarefa_aprovacao
            language: { code: pt_BR }
            components: "${ [{'type': 'body', 'parameters': [{'type': 'text', 'text': inputs.text}]}, {'type': 'button', 'sub_type': 'quick_reply', 'index': '0', 'parameters': [{'type': 'payload', 'payload': inputs.buttons[0].id}]}, {'type': 'button', 'sub_type': 'quick_reply', 'index': '1', 'parameters': [{'type': 'payload', 'payload': inputs.buttons[1].id}]}] }"
```

Without `send-task-template`, a member outside the window gets nothing on
WhatsApp and the signed link stays the way in.

**What the user hears after the window closes.** A run's messages to a
WhatsApp user also need the 24h window. When a person answers a task
after it closed, the run's next message to the user (a result, a denial)
cannot be sent as free text, and the run ends `WINDOW_CLOSED` with nothing
delivered (#54). Keep such outputs as `output.value` (admission warns
`WINDOW_RISK`), and send what the user must know with an approved
template.

**Approving on Slack** (backoffice-003). A queue with `channels: [slack]`
posts each approval task to the Slack DM of every member who may answer
it (`slack: U0123ABCD` on the member, their Slack user ID).
- A click comes back in a request Slack signs. The platform checks:
  - the signature, and that the request is at most 5 minutes old;
  - that it comes from the workspace in `TESSERA_SLACK_TEAM_ID`;
  - the member's role and the task's hash.
- The message's buttons are then replaced with the outcome.
- Run `tessera serve --slack` with `TESSERA_SECRET_SLACK_SIGNING_SECRET`
  and `TESSERA_SLACK_TEAM_ID`.
- Set the Slack app's interactivity request URL to
  `<public URL>/channels/slack/interactions`.
- The bot needs `chat:write`.
- A plugin named `slack` declares both actions (Slack answers errors with
  200 and `ok: false`, hence `failWhen`):

```yaml
  connection:
    baseUrl: https://slack.com/api
    auth: { header: Authorization, prefix: "Bearer ", secret: SLACK_BOT_TOKEN }
  actions:
    post-task:
      input: { to: string, text: string, buttons: list }
      effect: external-message
      http:
        method: POST
        path: /chat.postMessage
        failWhen: "!sources.response.body.ok"
        body:
          channel: "${ inputs.to }"
          text: "${ inputs.text }"
          blocks: "${ [{'type': 'section', 'text': {'type': 'mrkdwn', 'text': inputs.text}}, {'type': 'actions', 'elements': inputs.buttons.map(b, {'type': 'button', 'action_id': b.id, 'style': b.style, 'text': {'type': 'plain_text', 'text': b.title}})}] }"
    update-task:
      input: { channel: string, ts: string, text: string }
      effect: external-message
      http:
        method: POST
        path: /chat.update
        failWhen: "!sources.response.body.ok"
        body: { channel: "${ inputs.channel }", ts: "${ inputs.ts }", text: "${ inputs.text }", blocks: "${ [] }" }
```

The whole clinic is in `bench/backoffice/agent-live-slack`.

**Approving on Microsoft Teams** (backoffice-004). A queue with
`channels: [teams]` sends each approval task to the personal chat of every
member who may answer it, with the bot, as an Adaptive Card
(`teams: <Entra object ID>` on the member).
- **Every click is checked.** The Bot Framework delivers a click with a
  JWT, and the platform checks:
  - the signature, against the Bot Framework's published keys;
  - that the key is endorsed for `msteams`;
  - the issuer, the audience (the bot's app ID) and the validity;
  - the `serviceUrl` claim;
  - the tenant (`TESSERA_TEAMS_TENANT_ID`);
  - then the member's role and the task's hash.
- **The card then shows the outcome.**
- **To run it:** `tessera serve --teams`, with `TESSERA_TEAMS_APP_ID` and
  `TESSERA_TEAMS_TENANT_ID`.
  - `TESSERA_TEAMS_OPENID_URL` is only for sovereign clouds.
  - The Azure Bot's messaging endpoint is
    `<public URL>/channels/teams/messages`.
- **Before a task can reach a member,** the app must be installed for
  them in the personal scope; otherwise Teams answers 403.
- **A limit of the Bot Framework:** its token proves only that the request
  comes from the Bot Framework for this bot and names the `serviceUrl`.
  - The member, the tenant and the button are in the activity's body,
    which is not signed.
  - Whoever captures a valid token can forge clicks and messages with it
    until it expires (about an hour), for instance from the logs of a
    TLS-terminating proxy.
  - Keep `Authorization` headers out of every proxy's and load
    balancer's logs.
  - Tessera still checks the tenant, the member's role and the task's
    hash, so a forged click can only do what a real member could (#54).
- **The `teams` plugin** declares `open-dm`, `post-task` and
  `update-task` (the full plugin is in `bench/backoffice/agent-live-teams`):

```yaml
  egress: { hosts: [smba.trafficmanager.net, login.microsoftonline.com] }
  connection:
    baseUrl: https://smba.trafficmanager.net/teams
    oauth2:
      tokenUrl: https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token
      clientId: TEAMS_APP_ID
      clientSecret: TEAMS_APP_PASSWORD
      scope: https://api.botframework.com/.default
  actions:
    open-dm:
      input: { user: string, tenant: string, bot: string }
      output: map
      effect: idempotent-write
      http:
        method: POST
        path: /v3/conversations
        body:
          bot: "${ {'id': inputs.bot} }"
          members: "${ [{'id': inputs.user}] }"
          isGroup: false
          channelData: "${ {'tenant': {'id': inputs.tenant}} }"
    post-task:     # an Adaptive Card; each button's data is {tessera: <task id>}
      input: { conversation: string, text: string, buttons: list }
      output: map
      effect: external-message
      http: { method: POST, path: "/v3/conversations/{{ inputs.conversation }}/activities", body: { … } }
    update-task:
      input: { conversation: string, activity: string, text: string }
      effect: external-message
      http:
        method: PUT
        path: "/v3/conversations/{{ inputs.conversation }}/activities/{{ inputs.activity }}"
        body: { type: message, text: "${ inputs.text }" }
```

**Approving by e-mail** (backoffice-005). A queue with `channels: [email]`
e-mails each approval task to every member who may answer it
(`email: …` on the member). The e-mail carries a link **bound to that
member**.
- **The e-mail is never read back,** because a sender is easily forged.
- **The link is the only way to answer,** and only with a one-time code:
  - the page offers only its member;
  - the code goes through the Team's `verify`, preferably on another
    channel (the member's phone);
  - the link stops once the task is answered.
- **A forwarded e-mail approves nothing.**
- **To run it:** `tessera serve --email --public-url …`
  (`TESSERA_EMAIL_FROM` sets the raw message's From). Admission requires
  the queue's roles, the Team's `verify`, and a plugin named `email` with
  `send-task`.
- **`send-task` gets:**
  - `to`, `subject`, `text` and `link`;
  - `raw`, the whole RFC 822 message encoded as base64url. The platform
    builds it, so no task text can add a header.
- **With Outlook** (Microsoft Graph, an app with the `Mail.Send`
  application permission):

```yaml
  egress: { hosts: [graph.microsoft.com, login.microsoftonline.com] }
  connection:
    baseUrl: https://graph.microsoft.com/v1.0
    oauth2:
      tokenUrl: https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token
      clientId: GRAPH_CLIENT_ID
      clientSecret: GRAPH_CLIENT_SECRET
      scope: https://graph.microsoft.com/.default
  actions:
    send-task:
      input: { to: string, subject: string, text: string, link: string, raw: string }
      effect: external-message
      http:
        method: POST
        path: /users/<the clinic's mailbox>/sendMail
        body:
          message: "${ {'subject': inputs.subject, 'body': {'contentType': 'Text', 'content': inputs.text}, 'toRecipients': [{'emailAddress': {'address': inputs.to}}]} }"
          saveToSentItems: false
```

- **With Gmail** (Gmail API, `gmail.send`, a refresh token from the
  mailbox's one-time consent):

```yaml
  egress: { hosts: [gmail.googleapis.com, oauth2.googleapis.com] }
  connection:
    baseUrl: https://gmail.googleapis.com/gmail/v1
    oauth2:
      tokenUrl: https://oauth2.googleapis.com/token
      clientId: GMAIL_CLIENT_ID
      clientSecret: GMAIL_CLIENT_SECRET
      refreshToken: GMAIL_REFRESH_TOKEN
  actions:
    send-task:
      input: { to: string, subject: string, text: string, link: string, raw: string }
      effect: external-message
      http: { method: POST, path: /users/me/messages/send, body: { raw: "${ inputs.raw }" } }
```

**Sending the code by e-mail too.** When a member's `contact` is an
e-mail, `verify` also receives `inputs.raw`: the code as an RFC 822
message, for Gmail. The clinic in `bench/backoffice/agent-live-email`
does this with Outlook.


**A failed run is handed to that queue** (#52). `messages.failed` tells
the user a person will follow up, so the run does not just end:
- it becomes a `takeover` task in the `humanInTheLoop` queue, with the
  reason (console, notifications, SLA);
- it ends `Failed` once someone takes it;
- meanwhile the free mode answers new messages as "a person is handling
  it".

Without `humanInTheLoop` a failure just ends the run. Write `failed` so it
does not promise a person. The same holds when the failure message could
not reach the user (outside a channel's window, or without consent):
nothing was promised, so nothing is handed over. In drift-004 the old
behaviour made "alguém da recepção vai te responder" false in 20 of 24
runs.

### How it talks: the voice and the free mode

**The voice** (`Agent.spec.voice`) rewrites every message to a user in a
persona ([voice-001](../research/voice-001.md)):
- The default warm voice receives the approved draft, facts, persona, user
  name and whether the agent already spoke. Dialogue text is kept out of
  this wording stage: it made the voice promise unrelated requested
  services (#34, voice-007). Routing and free-mode answers retain their
  conversation context; strict voice still receives masked dialogue.
- The Function's template stays the **content**: what to say and which
  facts.
- Every interpolated value is a locked fact. A rewrite that drops one,
  stops asking a confirmation, or leaks a masked placeholder is replaced
  by the template.
- Only the conversation's first message greets; later greetings are
  removed.
- `latitude: warm` (the default) lets the voice do what a friendly person
  does around the facts: acknowledge, add a transition, close with an
  offer of help, put facts on lines.
  - Before the voice-007 context change, on two seeds it moved the blind judge's "natural" share from 13–16%
    to 55%, and "clear" from 3–9% to 26–34%, with no wrong fact
    (voice-004, voice-005).
  - Use `latitude: strict` for texts that must go out as written: legal
    wording, approved WhatsApp templates.
- `dialogue: full` (the default) lets the warm voice see the masked
  dialogue, bounded by the rule that only the template authorizes actions
  and services. On three seeds it won 40.8% of the judge's decided
  "natural" pairs against 30.3% for `dialogue: marker`, which sees only
  whether the agent already spoke. Both made 0/108 unsupported promises
  (voice-008/009).
- A rewrite that says the agent did, is doing or will do something the
  template does not ("já pedi para a equipe verificar") is dropped, and
  the template goes out (agency-001).
- `keep` lists words a rewrite must keep whenever the template has them,
  matched as case-insensitive substrings. Put the action's verb there: in
  [freemode-001](../research/freemode-001.md) the voice turned "remarcar"
  into "agendar", and the patient thought a new booking was under way.

**The free mode** (`Agent.spec.freeMode`) answers what no Function
covers, and questions asked while a Function waits for an answer
([freemode-001](../research/freemode-001.md)):
- `instructions` say what it knows and what the agent cannot do here,
  and where to go instead (a phone, a person).
- `reads` are read-only actions whose results are its facts. Mark the
  business's own information `public: true`. Otherwise it is masked like
  personal data, and a clinic phone becomes a placeholder.
- During a pending question, an additional service request is answered or
  redirected before a new explicit confirmation, even when its message
  also says yes. Question/request probabilities share one side-answer
  handler and are combined; they are never added to action permission
  (freemode-005). Corrections within the current process remain answers.
- During a pending question, an information question or prerequisite takes
  priority over a yes in the same message ("sim, mas antes..."). The agent
  answers and asks for confirmation again; classifier failures, uncertain
  classification and failed answers cannot authorize the action. Include
  a qualified yes followed by refusal in your EvalSuite.
- With free mode enabled, routing sees the last eight dialogue lines,
  masked and checked against the caller and namespace. A late confirmation
  of work already reported completed should receive an acknowledgement;
  an explicit request to change it again starts a new confirmed process.
  Test both, including an intervening information question. History is
  bounded dialogue, not an authoritative or permanent completion index.
- `runReads`, e.g. `[clinic/find-slots]`, shares selected read results
  already obtained by the pending Function. Use this for a side question
  such as "which times are available on those days?" after the Function
  has read slots but only asked for a day. Without it, the free mode sees
  the conversation and its own `reads`, not the Function's read results.
  Only successful, top-level input `fromAction` sources are supported;
  arguments, other inputs, steps and called Functions stay private.
  The runtime checks the same caller, conversation, namespace and
  generation and always masks these snapshots. It does not call the
  plugin again. They are not a fresh availability check or proof that
  an action happened. Keep the combined JSON under 32 KiB: a larger
  context fails explicitly, so expose a smaller read result. See the
  live-and-replayed [contexto example](../../testdata/contexto).
- It never acts. Admission refuses reads that are not read-only.
- `functions` are Functions it may start instead of answering, such as
  asking a person. Before it answers a message no Function was routed
  to, one Decide question, which sees the reads' results, chooses
  between answering and each Function's `when`.
  - The choice is apart from writing the answer, so tuning one does not
    move the other.
  - Its probability is in the reply (`handoff`), and `minConfidence`
    (default 0.6) is the knob.
  - Give such a Function no `selector`: the router does not see the
    reads. In escalation-000 a routed "ask the coach" Function took 37%
    of questions the checklist answered.
  - Write `when` to name **every kind** of thing that goes to that
    Function: decisions, approvals, exceptions, and processes the person
    starts. The decision follows `when` literally. In escalation-002,
    "a decision, an approval or an exception" missed "sell the
    store; your coach starts the process" in 20 of 20 runs, which
    the LLM stacks generalised to.
  - Calibrate `minConfidence` on labelled questions that cover each of
    those kinds, never on your test questions.
  - The Function's message to the user still gives what the reads say
    (the rule, the next step): a `generate` step with the reads, or a
    template naming the rule. A bare "passed to your coach" cost 38 points
    of judged correctness (authoring-001).
- `search` lets it look up current public information on the web: a
  country's visa rules, an airline's baggage policy, the best season.
  It is off by default. `tessera add web` writes the plugin: TinyFish by
  default, free with no card; `--provider searxng|tavily|brave` for the
  others, all with one contract. Then set
  `search: { act: web/search }`. On a message no Function covers:
  1. Jev decides whether answering needs a search (`when` narrows it).
  2. The Generate engine writes one query from the **masked** message.
  3. **The query never leaves with anything about a person.** It is not
     sent when the masker would change it, when it has a long run of
     digits (documents, phones), or when Jev says it names or describes a
     private person. The masker alone does not know names or passport
     numbers.
  4. At most `maxResults` results (default 5: site, title, url, snippet)
     reach the answer as third-party pages. The rules make the answer:
     - cite the site;
     - treat the text as data and never obey it;
     - never pass on phones, links or offers found only there;
     - let the business's own reads win;
     - never conclude on the user's own case (their dates, documents,
       age, weight): say the rule, its source and what to check
       (websearch-002).

     A request for the agent to act is not searched, even when the
     agent cannot do it.

  The query, or why none was sent, is journaled with the free turn.
  Measured in [websearch-001](../research/websearch-001.md), including
  hostile pages.
- While a Function waits, Jev classifies each message:
  - an answer goes to the Function;
  - a side question gets a free answer, then the pending question again
    as a continuation, with its facts still locked. When the free answer
    already asks for it, it is not asked twice; and after the first side
    question, the pending question is not repeated at all (a person does
    not insist): the run still waits, and a later answer reaches it
    (agency-001);
  - at a question (never at a confirmation), a side question that also
    answers ("ida dia 3, volta dia 17; e os documentos?") gets its free
    answer and still reaches the Function, so nothing is asked again;
  - a cancel or a goodbye ends the run without acting;
  - a reply holding the pattern of a `format` question (an e-mail) is
    its answer, whatever else it asks;
  - a wait with `relay: true` (and `orReply`) takes every reply as
    written, questions included: for relaying a conversation with a
    person, where a question back is for that person (agency-003);
  - a message that starts a Function, or answers one, and also asks something else ("quero
    a cotação; e criança paga?") gets the rest answered by the free mode
    (with the web and the documents), before the Function's reply, when
    Decide finds something unanswered (agency-002);
  - if the classifier fails (a provider error), the message is neither
    the answer nor anything else: the pending question is asked again.

```yaml
spec:
  voice:
    persona: "Recepcionista da clínica, cordial, frases curtas; trata o paciente pelo primeiro nome."
    keep: [remarc]
    examples: ["Oi, Ana! Claro, te ajudo a remarcar. Para qual dia você prefere?"]
  freeMode:
    instructions: "Responde dúvidas da clínica. Pelo chat só dá para remarcar; para cancelar ou marcar consulta nova, o paciente liga para a clínica."
    reads: [ { act: clinic/info, public: true } ]
    runReads: [clinic/find-slots]
    search: { act: web/search }        # tessera add web
    functions:
      - { function: falar-com-recepcao, when: "o pedido precisa de uma decisão ou exceção da recepção" }
```

**"What can you do?"** By default (`freeMode.self`, on unless set to
`false`; #60), the free mode knows what the agent does for the
person asking, built by code from your definitions
([self-001](../research/self-001.md)):
- the Functions on their menu and the free mode's `functions`;
- what each asks for;
- whether a person approves it, and which queue;
- whether the user confirms first;
- follow-ups that come only after another Function.

So it answers "o que você faz?", "vocês reservam?" and "precisa de
aprovação?" truthfully, says what to send to start, and never offers what
is not there. Another audience's Functions never appear.
- Measured: the free mode's capability errors went from 17 of 27 with
  nothing to 4 of 26. That is as good as a hand-written list, and it
  stays right when a Function is removed (2 of 28, against 12 of 15 for
  the stale list).
- With `freeMode.tasks`, a team member's self also says their open
  tasks are answered with the buttons or the link they received.
- **Prefer it to listing capabilities in `instructions`:** a
  hand-written list goes stale.
- **Write each Function's `description` as what the agent does**
  ("Shows the week's discounts"), not who does it ("The owner reads…").
  It is what the agent says about itself.

**Agent mode** (experimental: [RFC-0010](../rfcs/0010-modo-agente.md),
ADR-0007, measured in agency-004):

```yaml
  freeMode:
    agent: { maxSteps: 5 }      # the tool calls one turn may make
```

- With no run pending, a model plans each turn in steps, instead of the
  router and the free answer. Each step calls one tool or replies. The
  tools are:
  - the Functions on the caller's menu, plus `freeMode.functions`;
  - `web`, when `search` is set; a query about a person is never sent;
  - `documents`, when `knowledge` is set.
- **A Function it calls runs as any run:** its questions, confirmation,
  approval and idempotency. The planner may give it inputs the user
  said, but only inputs with an `extract` source and a scalar type. A
  value that is a placeholder or has the wrong type is left for the
  Function to resolve.
  - When the Function waits (a question, a confirmation, a person), the
    turn ends with its message.
  - When it finishes, its messages go to the user and the planner goes
    on (another question, the next step).
- **Its reply passes the same checks** as a free answer: the
  unsupported-fact check and the claims check. A reply that fails is
  planned once more, then dropped: `whenUnsure` or `messages.unsure`.
- Each step (tool, inputs, search query) is journaled with the run it
  started or with the turn's note.
- If the model cannot plan, the router answers as before. Evals fail
  such a turn, so a stale cassette cannot test the router instead.
- **One Function a turn**, and not the same Function with the same
  inputs as on the previous message. Functions' idempotency keys assume
  one run per message. A repeat would show the same thing again; the
  planner answers from what was shown instead.
- A draft the checks drop is not silence (agency-005): the free answer
  takes its place (with a web search when the planner made none, and its
  quoted fallback). After a Function answered part of the message, the
  free answer covers the rest; if none stands, `messages.unsure` says so.

**"I'll send it later"** (agency-005): while a process waits for the
user, a message that puts the answer off ("mando os dados amanhã") is not
taken as the answer. The process keeps waiting, the question is not
asked again, and what the message asks is answered. The user then hears
`messages.later`.


the business's policies, FAQ and price lists as Markdown or text files in
the agent's repository.

```yaml
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: politicas, namespace: clinica }
spec:
  description: Políticas, preços e FAQ.
  sources: [ { files: ["docs/*.md"] } ]      # relative to this file
  audiences: [equipe]                         # optional: who may read it
---
# in the free mode (the Agent's or an audience's):
  freeMode:
    knowledge: { bases: [politicas] }          # check: on by default
```

- Before a free answer, the message and the words a document would use
  for it are searched. The best passages the caller's audience may read
  reach the model, and which ones is journaled.
- With `check` (the default), an answer stating a fact no passage or
  read supports is rewritten once, then dropped. What the agent itself
  already told in the conversation (a quote, a search result) and the web
  pages it found count as support; what the user said does not. When
  every draft is dropped, `freeMode.whenUnsure: <Function>` starts that
  Function (asking the team, as a person who does not know would);
  without it, and with no documents to quote, a web
  page's own sentence is quoted with its site, checked word for word;
  otherwise the user hears `messages.unsure` ("não consigo te
  confirmar com segurança"), not `notUnderstood`.
- With `relevance` (the default), passages that do not answer the
  message are dropped first. A rule about another treatment, age or plan
  is not taken for an answer, and the handoff sends it to a person
  (knowledge-002: misleading replies halved on new data).
- Measured: answerable questions 3 → 54 of 54; 0 unsupported facts in
  90; an injected note in a document was not obeyed; a staff-only base
  never reached a patient.
- **Files may be Markdown, text, PDF or DOCX** ([pdf-001](../research/pdf-001.md)).
  - A PDF's paragraphs and headings are guessed from its layout. A DOCX's
    headings come from its heading styles.
  - A scanned PDF has no text, and admission refuses it (`NO_TEXT`):
    export it with its text.
  - Two-column PDFs read poorly. Prefer one column, or Markdown.
- **Documents from a system, not the repository** ([plugin-001](../research/plugin-001.md)).
  A source can be a plugin that lists documents and fetches each one.
  SharePoint is built in (`tessera add sharepoint --tenant … --site …`):

  ```yaml
  sources:
    - list:  { act: sharepoint/list-files, with: { folder: "Politicas" } }
      fetch: { act: sharepoint/download, with: { id: "${ inputs.id }" } }
      refresh: 24h
  ```

  - It is read when the agent loads, then every `refresh`.
  - A failed read keeps the documents read before.
  - For another system, the list action returns `{name, id?, modified?,
    text?, base64?}`, and fetch returns `{base64}` or `{text}`.
- **Documents inside a process** ([retrieve-001](../research/retrieve-001.md)).
  A `retrieve` step gives a Function the passages the free mode would
  find, and a `generate` step answers from them:

  ```yaml
  steps:
    - { id: docs, retrieve: { bases: [politicas], query: "event.message" } }
    - id: resposta
      generate:
        instructions: "Answer only from PASSAGES; never a price that is not in them."
        state: { message: "${ event.message }", passages: "${ steps.docs.output }" }
  output: { template: "{{ steps.resposta.output }}" }
  ```

  - The passages are recorded in the run, so a resumed run uses the same
    ones.
- **Write the documents for the patient, one topic per heading.** Keep
  personal data out: passages are not masked.
- Put staff-only policies in their own base, with `audiences`.

**Lessons from the team's answers** (`tessera learn`,
[learn-001](../research/learn-001.md)). Give the free mode a Function
that asks the reception (`escalate` with `task: answer`, relaying the
answer), and a lessons `KnowledgeBase` (a Markdown file) next to the
documents. Then, from time to time:

```bash
tessera learn --db tessera-runs.db --namespace clinica --out proposals.json   # propose, read-only
tessera learn accept --proposals proposals.json --ids <id>,<id> --to docs/licoes.md
```

- Answers about a customer's own case are rejected.
- Contradicting answers are held as conflicts.
- A lesson needs at least 2 answered cases.
- Each accepted lesson keeps its provenance and a review date: price 30
  days, policy 180, fact 365.
- **Read every proposal before accepting it:** learn-001 saw one
  customer's instalment judged general. You are the check.
- **Or let the team approve lessons in its channel** ([learn-003](../research/learn-003.md)):

  ```yaml
  kind: KnowledgeBase
  metadata: { name: licoes }
  spec: { description: Lições da recepção., lessons: { queue: recepcao, every: 168h } }
  ```

  - Every week the agent proposes lessons from the team's answers.
  - Each one is an approval task with the exact text, on WhatsApp,
    Slack, Teams or e-mail.
  - Approved lessons are used at once. Denied ones do not come back.
  - A change to an active lesson says "Substitui:" and replaces it.
  - The lessons live in `<dataDir>/lessons/<namespace>/<base>.md`.
  - Lessons due for review (within 14 days) are put to the queue:
    "Esta lição continua valendo?". Approved, they are renewed; denied or
    unanswered, they expire and are no longer used (renew-001).
- **Keep the lessons alive** ([lifecycle-001](../research/lifecycle-001.md)).
  A lesson past its review date is no longer used; the question goes to
  a person again. Run the review from time to time:

  ```bash
  tessera learn review --db tessera-runs.db --namespace clinica --lessons docs/licoes.md --apply
  tessera learn renew --lessons docs/licoes.md --ids <still true>
  tessera learn retire --lessons docs/licoes.md --ids <dead> --reason dead
  tessera learn --db tessera-runs.db --namespace clinica --lessons docs/licoes.md   # new answers vs lessons
  tessera learn accept --proposals p.json --ids <new> --supersede <old> --to docs/licoes.md
  ```

  - `--apply` suspends lessons users keep disputing.
  - A lesson a user disputes right after hearing it is not repeated in
    that conversation: the question goes to a person, and the dispute is
    journaled for the review (dispute-001).
  - Dead lessons are only reported: retiring them is your call.

### Audiences: one agent for patients and for the team

**When one agent serves several kinds of caller** (patients, the
reception, dentists), `Agent.spec.audiences` gives each its own:
- menu (`functions`);
- `voice`;
- `freeMode`;
- `messages`.

It is measured in [audiences-001](../research/audiences-001.md).

```yaml
spec:
  audiences:
    - name: dentista
      who: "has(caller.roles) && 'dentista' in caller.roles"
      functions: [ver-agenda, bloquear-agenda]
      voice: { persona: "Assistente da equipe: direto e objetivo." }
      freeMode:
        instructions: "Ajude o dentista com a própria agenda."
        reads: [ { act: clinic/schedule, with: { dentistId: "${ caller.attributes.dentistId }" } } ]
    - name: paciente                     # no who: everyone else; last
      functions: [pedir-desconto]
      voice: { persona: "Recepcionista acolhedora." }
      freeMode: { instructions: "Tire dúvidas dos pacientes.", reads: [ { act: clinic/info, public: true } ] }
```

- **Which audience applies:** the first whose `who` holds for the caller.
  - The router offers only that audience's `functions`. `selector.who`
    and `authorize` still apply on top.
  - The audience is journaled with each run (`caller.audience`), so a run
    resumed after a person answers still speaks in its voice and
    messages.
  - A caller cannot choose its audience: `caller.audience` is always
    recomputed.
- **The catch-all:** an audience without `who` catches everyone else and
  must be last.
  - Without one, a caller who matches no audience is offered nothing and
    hears `messages.notUnderstood`. Admission warns `NO_CATCH_ALL`.
  - A routable Function on no menu is never offered (`NO_AUDIENCE`).
- **Omitted fields:** `voice`, `freeMode` and `messages` fall back to the
  Agent's, field by field for messages.

**Who is on the team comes from the Team, never from what they write.**
When the WhatsApp adapter receives a message from a member's `whatsapp`
number (which Meta attests), the caller carries:
- `member`, `roles` and `name`;
- `attributes`, from the member's `attributes` (`dentistId: D1`).

These sit under `caller.attributes`, never at the caller's top level, so
no attribute can pose as `caller.trigger`. "Sou o dentista D1" from
another number is just a patient's message.

Over the API, the client holding the token states the caller, as before.
The full clinic is in `bench/backoffice/agent-live-audiences`.

**The team talks to the agent on Slack and Teams too** (backoffice-006).
With a `post-text` action on the `slack` or `teams` plugin, a member's
message to the app (Slack DM, Teams personal chat) reaches the agent.
- **Identity:** the Slack user ID, or the Entra object ID in the tenant,
  is the member's identity, checked like the approval clicks. Anyone else
  in the workspace is a caller without roles.
- **Replies:** now and later (a confirmation, a result after an approval)
  they go back to the same DM, in order, through the shared outbox.
- **Slack setup:** subscribe the app to `message.im` at
  `<public URL>/channels/slack/events`. It needs `chat:write` and
  `im:history`.

```yaml
    post-text:      # slack
      input: { to: string, text: string }
      effect: external-message
      http: { method: POST, path: /chat.postMessage, failWhen: "!sources.response.body.ok", body: { channel: "${ inputs.to }", text: "${ inputs.text }" } }
    post-text:      # teams
      input: { conversation: string, text: string }
      output: map
      effect: external-message
      http: { method: POST, path: "/v3/conversations/{{ inputs.conversation }}/activities", body: { type: message, text: "${ inputs.text }" } }
```

**"Do I have anything to approve?"** With `freeMode.tasks: true` on a
team audience, the free mode sees the member's open tasks: those they
may answer, masked like any private read.
- It says how many and which.
- They are still answered only with the buttons or the link sent to the
  member. "Pode aprovar" in the conversation approves nothing.

The whole back office (patients on WhatsApp, the team on Slack and
Teams, the owner's report) is in `bench/backoffice/agent-live-backoffice`.

**What the team sees of a conversation** (audiences-002). By default, a
member answering a task sees its text only. An audience declares more:

```yaml
    - name: equipe
      sees: [ { audience: paciente, conversation: masked } ]   # or full
```

- Its members then see the conversation behind a patient's task: its
  last lines, on the channels' task messages and in `freeMode.tasks`.
- `masked` hides e-mails, phones and documents. A model always gets the
  masked form.
- Every view is journaled in the task's run
  (`<site>/viewed/<member>/<channel>`), as the audit.
- The task's own text is the Function's: do not put personal data in it.

**Autonomy** (`spec.autonomy`, or per audience):
- **`autonomous`**, the default: as today.
- **`approve-actions`:** before every `irreversible` or `external-message`
  effect, the `humanInTheLoop` queue approves the action and its
  arguments. A denial ends the Function, and the user hears
  `messages.denied`.
- **`draft`:** that, and every message a run sends the user waits for a
  person to approve its exact text. Denied, nothing is sent.
  - A draft audience has no free mode (admission), since free answers
    could not be reviewed.
  - The platform's fixed messages (`notUnderstood`, `withAPerson`) still
    go out alone: write them in the users' language.

These levels need `humanInTheLoop`. The live clinic is
`bench/backoffice/agent-live-autonomy`.

## 3. The EvalSuite: prove it in whole conversations

An `EvalSuite` plays conversations against the agent with plugins
replaced by **sandboxes**: Starlark functions named after the actions,
holding state like the real system
([`evals/`](../../examples/traveler/evals)).

```yaml
scenarios:
  - id: built-over-messages
    caller: { homeCity: "São Paulo" }
    simulator: scripted
    turns:
      - user: "quero viajar"
        expect: { routed: search-flights, status: WaitingForUser, says: ["Para onde"] }
      - user: "Lisboa"
        expect: { says: ["dia da ida"] }
      - user: "sexta que vem"
        expect: { status: Succeeded, says: ["Encontrei estas opções"] }
    expect:
      effects: [ { action: booking/search-flights, count: 1 } ]
      budget: { generateCalls: 3 }
```

Every scenario also checks the invariants no agent may break (no
irreversible effect without confirmation or approval, no duplicate
effect, no lost message). A sandbox can fail like the real system
(`definite("NOT_FOUND", ...)`, `ambiguous("timeout")`) — test those paths:
they are where agents do damage.

```bash
tessera test examples/traveler --decide typesafe:jev-1.13.0 \
  --generate openrouter:deepseek/deepseek-v4.1-flash               # live, budgeted
tessera test ... --cassette c.jsonl --cassette-mode record          # record once
tessera test ... --cassette c.jsonl --cassette-mode replay          # CI: offline, free
```

Run the suite on both Decide backends: it finds differences you would
otherwise meet in production ([evals-backends-001](../research/evals-backends-001.md)).

## 4. Talk to it

```bash
tessera chat examples/traveler --namespace traveler --caller homeCity="São Paulo"
```

Runs are durable: stop the chat, start it again with the same `--db` and
`--conversation`, and the conversation continues where it was. `/human`
answers the tasks the agent sends to a person.

## Checklist before connecting a real system

- [ ] `tessera validate` has no errors and you read every warning.
- [ ] Every irreversible action has an idempotency key, a `confirm` (or a
      human approval), and `onError` that verifies before retrying.
- [ ] The `confirm` text names everything the action acts on: which
      appointment *and* the new time, not only the new time
      (`CONFIRM_INCOMPLETE`). The user says yes to what they read: in
      [market-bench-001](../research/market-bench-001.md) a confirmation
      that named only the time moved the wrong appointment.
- [ ] The `confirm` is plainly a question the user answers: "Posso
      remarcar ... para ...? Responda sim para confirmar.", not "Confirmo a
      remarcação ...?", which also reads as "I confirm" and made simulated
      patients leave before answering (market-bench-003).
- [ ] Long option lists are narrowed first: ask the day, then the times of
      that day, instead of listing two weeks of slots.
- [ ] A "no, X instead" at a confirmation starts the Function again from
      that reply: make sure the Function can resolve its inputs from a
      single message (an `ask` with `choices` reads the triggering
      message first).
- [ ] `selector.description` also says what the Function does **not** do,
      when a neighbouring request looks like it: "not for booking a new
      appointment or cancelling". Without that, "can you book a new
      cleaning?" reached the reschedule Function (freemode-001).
- [ ] The templates carry the detail a person would give: which
      appointment, the full date, who with. The voice may not add facts,
      so detail missing from the template is missing from the
      conversation, and the blind judge penalised exactly that
      (freemode-001).
- [ ] The voice's `keep` names the action's verb.
- [ ] Offers are narrowed to what the user already said (a day, a
      period): an input for "manhã/tarde" and a computed input with the
      matching times. Offering morning times to "de tarde" was most of the
      blind judge's clarity losses and factual flags. Removing it halved
      the losses and cut the flags from 57 to 5 (voice-002).
- [ ] Channels send their message ID (`POST /v1/messages` `messageId`):
      a retried message is then not handled twice.
- [ ] The EvalSuite covers the happy path, a refusal, an unknown input,
      and the plugin's definite and ambiguous failures.
- [ ] It passes live on both Decide backends, and replays in CI.
- [ ] Secrets are references; `egress.hosts` lists only what the plugin
      needs.
- [ ] The namespace has an `Agent` whose messages are in the users'
      language.

## Independent decisions in one Jev request

Use `decideBatch` when several finite questions depend on the same known state.
Each named question supports `yesNo`, `choice`, or `score`, exactly like `decide`.
The engine masks the shared state and journals one request for the whole batch.
Independent questions run together; a question that needs another answer belongs
in a later step. See `testdata/decide-batch/function.yaml` and
`tessera explain Function.spec.steps.decideBatch`.

Read `steps.<id>.output.answers.<name>.value` and `.probability`.
`minConfidence` is the winning probability, applied separately to each answer.
With `onLowConfidence: continue`, uncertain answers have `value: null`;
confident answers remain available. The aggregate `output.value` is null unless
all answers meet the threshold. With `fail`, any uncertain answer invokes
`onError`. Provider errors and missing/malformed answers always invoke `onError`.
Never interpret null as permission or as absence of a user restriction.

The same error rule applies to individual `decide` steps: `onLowConfidence`
handles a valid but uncertain answer, never a provider failure or malformed
response. Those failures use the step's `onError`; without a recovery strategy,
the run fails. The engine enforces this regardless of author policy.

The engine does not merge consecutive `decide` steps automatically. Even with
independent inputs, the first step may fail or wait for a person before the next
question should be asked. Declaring `decideBatch` explicitly gives the engine
permission to evaluate all questions together, with one durable result. Business
scope, clarification wording and conversation order remain author decisions.

For weekday preferences, ask whether a restriction is explicit and which of the
seven weekdays it permits, in one batch. A confidently absent restriction needs
no weekday filter; an uncertain explicitness answer needs clarification. When a
restriction is present, require all seven answers confident. Filter actual slot
timestamps deterministically with `getDayOfWeek('America/Sao_Paulo')` (Sunday 0).
A bounded search window and its explanation belong in the agent's business
policy; the engine supplies no default calendar range. Empty results describe
only the searched window. Use Generate for open text and dates, not to choose
among finite weekdays or slots.

### Completing a clarification

A message asking for clarification does not itself suspend a Function. Use a
reply wait when the next message must continue that run. A measured weekday
flow asks for a complete weekday restatement, waits with
`orReply: true`, and reclassifies once in a Jev batch. It separately checks that
the reply supplies a preference, so “I do not know yet” cannot become unrestricted.
Unresolved replies and timeout stop without selecting days.

Its contract replaces weekday preferences with a full restatement. It does not
support arbitrary partial additions, preserve date/time constraints, or book
appointments. Those need explicit author policy and tests. See
`docs/research/clarification-001.md`; this example is not a universal engine rule.

### Retaining preferences during side questions

For a scheduling preference collection, opt a reply wait into
`includeSideMessages: true` (requires `orReply: true`). The host uses one Jev
request with three questions: reply type, a factual question in the **current**
message, and withdrawal from the pending rescheduling. Earlier answered
questions are context, not new questions to answer. Ordinary waits and
confirmations keep their existing classifier.

When the wait ends, `output.sideMessages` contains the raw user messages
whose side requests were handled during that wait, in chronological order.
Combine initial message, this list and `output.reply` explicitly when
extracting preferences. Later corrections supersede earlier preferences.
The list is scoped to the wait and exact caller, survives journal replay,
and is limited to eight messages and 32 KiB; overflow is an error, never
silent truncation. It does not resume the wait, extend its deadline or
confirm an effect. On timeout, `reply` remains null even if side messages
exist. Always check for an actual reply before parsing.

This opt-in policy was measured on synthetic clinic rescheduling, not on
arbitrary business domains. See `docs/research/preference-001.md` for failed
variants, held-out evidence and remaining classification limits.
