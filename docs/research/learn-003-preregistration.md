# learn-003: lessons approved in the team's channel. Pre-registration

- **Date:** 2026-09-28, before any of this is built.
- **RFC:** RFC-0008, part B: "aprovação (tarefa approve num canal da
  equipe, com o texto exato e a evidência)".

## What will be built

1. **`KnowledgeBase.spec.lessons: {queue?, every?, min?}`** marks a base
   that learns.
   - Its lessons live in a file the server keeps:
     `<dataDir>/lessons/<namespace>/<base>.md`, in a temporary directory
     without a data dir.
   - The file is indexed with the base's other sources.
   - `queue` defaults to the Agent's `humanInTheLoop`, `every` to 168h
     (weekly) and `min` to 2.
2. **A generated Function per learning base, `learn-<base>`:**
   - it escalates an `approve` task to the queue, with the exact lesson in
     the text:
     - the question;
     - the answer;
     - "Substitui: …" when it contradicts an active lesson;
     - how many answers it rests on;
   - then it calls a built-in action, `lessons/accept`
     (idempotent-write), that appends the lesson, with provenance and
     `approvedBy`, and rebuilds the index.

   So every approval surface already built applies:
   - WhatsApp buttons, Slack, Teams, e-mail links;
   - the task's hash binding;
   - the queue's roles;
   - SLA and escalation;
   - a denial ends the run.
3. **`Tick`**, every `every`, runs `learn` over the server's own journal,
   with the base's current lessons. It starts one `learn-<base>` run for:
   - each proposal;
   - each contradiction of an active lesson, which superseded on
     approval.

   **A proposal whose evidence runs were all already in an earlier
   `learn-<base>` run** (approved, denied or pending) is not sent again.
   A denied lesson does not come back reworded.

## Measure

**Functional, deterministic** (host tests, fake engines, a Team with
roles):

| # | Hypothesis | Supported if |
| --- | --- | --- |
| C1 | Proposals become tasks, once | at the first due tick, one approve task per proposal in the queue, with its question and answer in the text; a second tick, or one before `every`, adds none |
| C2 | Approved lessons are used | approving by a member with the queue's role appends the lesson (with `approvedBy`) and the free mode's next answer is given it |
| C3 | Denials and roles hold | a denial writes nothing and the lesson is not proposed again, even reworded; a member without the role cannot approve (existing rule, pinned here) |
| C4 | Contradictions supersede | a proposal contradicting an active lesson says "Substitui:" and, approved, retires the old lesson (superseded) and adds the new |

**Live** (`TESSERA_LIVE=1`, real Jev and DeepSeek, learn-002's phase-1
journal copied into the server's store):

| # | Hypothesis | Supported if |
| --- | --- | --- |
| C5 | End to end with real engines | the first tick opens tasks for learn-002's general topics (≥ 3 of the 4). Approved through `AnswerTask` as a member, the free mode then answers 3 of the 4 held-out questions m01, m03, m04, m05 from them. No task holds a patient's name. |

| # | Hypothesis | Supported if |
| --- | --- | --- |
| C6 | Nothing else changes | `make ci` and `make lint-extra` pass |

**Budget:** $0.5.
