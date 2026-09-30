# learn-003: lessons approved in the team's channel

- **Date:** 2026-09-28.
- **Pre-registration:** `learn-003-preregistration.md` (commit 393b388,
  before the code).
- **Code:** 06e5516.
- **Data:** `bench/results/learn-003/live.txt`.
- **Spend:** under $0.01 (Jev and DeepSeek in the live check).

## What was built

**A KnowledgeBase with `lessons: {queue?, every?, min?}` learns:**
- its lessons live in `<dataDir>/lessons/<namespace>/<base>.md`;
- that file is indexed with the base's sources.

**`Tick` proposes lessons every `every`** (default weekly) from the
server's own journal. Each proposal becomes a run of a generated
Function, `learn-<base>`:
1. An `approve` task goes to the queue with the exact lesson: question,
   answer, "Substitui: …" for a contradiction, and how many answers it
   rests on.
2. The built-in `lessons/accept` appends the lesson with `approvedBy`,
   supersedes the old one, and rebuilds the index. It is idempotent by
   the approval's conversation.

**Every approval surface already built applies:**
- WhatsApp buttons, Slack, Teams, e-mail links;
- the hash binding;
- the queue's roles;
- SLA and escalation.

**A proposal is not sent again** when every run it rests on was already
in an earlier approval, whether approved, denied or pending. A denied
lesson does not come back reworded.

## Result

| # | Result |
| --- | --- |
| C1 | **Supported.** The first tick opened one approve task per proposal: estacionamento, café, and a wi-fi contradiction whose text read "Substitui: Temos sim…". Later ticks, before and after `every`, added none. |
| C2 | **Supported.** Approved by Rita (who holds the role): written with `approvedBy=rita`, and given to the next free answer. |
| C3 | **Supported.** Gil, without the queue's role, got `NotAllowed`. A denied lesson was not written, and was not proposed again at the next due tick. |
| C4 | **Supported.** The approved contradiction retired the old wi-fi lesson (`status=retired reason=superseded by=…`) and added the new one. |
| C5 | **Supported (live).** On learn-002's phase-1 journal with real Jev and DeepSeek, the tick opened 4 tasks, one per general topic, none naming a patient. Approved through `AnswerTask`, the held-out questions were answered from them. The keyword check counted 3 of 4, because it looked for "cão-guia" and the reply said "cães-guia"; read by hand, all 4 are correct. Details below. |
| C6 | **Supported:** `make ci`, `make lint-extra`. 8 mutations caught, one after adding a case where a retried accept supersedes. |

**The live check's held-out answers:**
- "Não trabalhamos com laser… gel e luz LED";
- "uma limpeza dura cerca de 40 minutos";
- "emitimos nota fiscal: ela sai no dia do pagamento";
- "só cães-guia podem entrar".

**Two mistakes of mine in the unit test, fixed:**
- `AnswerTask` reports a refused member as an outcome (`NotAllowed`),
  not an error, and the first test expected an error.
- A retry case named the wrong lesson id.

## What it closes

The learning loop no longer needs a terminal:
1. the team answers patients through the agent;
2. once a week the agent proposes lessons;
3. the team approves each one in its own channel, by the exact text;
4. the agent uses it the same minute.

`tessera learn review --lessons <dataDir>/lessons/<ns>/<base>.md` keeps
the lifecycle (lifecycle-001) on the same file.

## Limits

- Only proposals and contradictions become tasks. A reconfirmation (the
  same answer again, which would renew a lesson) is not yet sent as a
  renew task.
- The lessons file is on the server's data volume, not in the agent's
  repository. A backup of the data dir is a backup of the lessons.
