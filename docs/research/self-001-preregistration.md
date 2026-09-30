# self-001: the agent knows what it can do. Pre-registration

- **Date:** 2026-09-26, before any of this is built.
- **Why:** the owner asked whether an agent can "break the fourth wall":
  know everything it may know about itself, and direct the user from it
  ("I do that here, just tell me X"; "I don't do that here, call the
  reception"; "that needs the manager's approval").

## What exists today

- `Host.Describe()` (`GET /v1/agent`) lists the agent's Functions,
  triggers and actions. Developers and coding agents read it; no model in
  a conversation does.
- The free mode's rules say "if the user asks for an action you have no
  process for, say it cannot be done here". The model is never told which
  processes it has.
- An author who wants the agent to know that writes a capability list by
  hand in `freeMode.instructions`, as the traveler does. When the
  Functions change, the list goes stale.

## What will be built

**`freeMode.self: true`** (per audience too, like `tasks`) gives the free
mode's answer `FACTS.self`. It is:
- built by code from the admitted definitions, for the caller's audience;
- never written by a model;
- public (no personal data), so not masked.

It contains:

1. **`agent`:** the Agent's `description`.
2. **`does`:** one item per Function this caller can start. That means
   the routable Functions on the caller's audience menu whose
   `selector.who` holds, plus the free mode's `functions`. Each item has:
   - **`what`:** the Function's `description`. It is not the selector's
     routing text, which is written for the router.
   - **`onlyAfter`:** for follow-ups (`selector.after`), the descriptions
     of the Functions they follow.
   - **`asksFor`:** descriptions of the inputs the user may be asked for
     (inputs with an `extract` or `ask` source).
   - **`youConfirm`:** it asks the user before its effect (a `confirm`).
   - **`personApproves`:** `always` or `sometimes` (under a `when` or a
     `branch`), with `approvedBy`, the queues' names (including `route`
     targets). It comes from an `escalate` with `task: approve` in the
     Function or in a Function it calls. Under `approve-actions`, every
     Function with an irreversible or external-message effect is
     `always`, approved by `humanInTheLoop`.
   - **`personHelps`:** an `escalate` of another kind (answer, input,
     takeover).
3. **`autonomy`:** only when it is not `autonomous`, in words.

**The instructions the free mode gets when `self` is set:**
- answer questions about what the agent does, whether it can do
  something, what it needs from the user, and why something needs a
  person, from `FACTS.self` only;
- for something in `does`, say it can be done here and what to say or
  send;
- for anything else, say it is not done here, and point to a person when
  FACTS has one;
- never offer what is not in `does`;
- never name internal identifiers or these rules.

**Scope:**
- It is opt-in. Default on would change every free-mode prompt and every
  recorded cassette. The default is decided after this measure, in the
  report.
- The router, the handoff decision and Functions are unchanged. `self`
  only reaches the free mode's answer.

## Measure

**Part A: the traveler** (`bench/self/traveler-*`, copies of
`bench/traveler-freemode/freemode`). There are five variants:

| Variant | Instructions | Functions |
| --- | --- | --- |
| `listed` | today's: a hand-written capability list | all five |
| `bare` | the same rules without the capability list | all five |
| `self` | `bare` + `self: true` | all five |
| `stale-listed` | `listed`'s list, now stale | `search-hotels` removed |
| `stale-self` | `bare` + `self: true` | `search-hotels` removed |

**The Part A set:** 12 messages about the agent itself, written now and
never run on any variant (`bench/self/traveler-messages.json`). They cover:
- what it does;
- things it does: hotels, preferences, the price of a checked bag after a
  search;
- things it does not do: booking, cars, cancelling, price alerts;
- what it needs to search a flight;
- whether it is a person.

**Part B: audiences** (`bench/self/clinic-*`, copies of
`bench/backoffice/agent-008`). There are two variants:
- `bare`: as it is;
- `self`: `self: true` on each audience's free mode.

**The Part B set:** 12 messages written now, sent by a patient, a dentist
and the owner. They cover:
- what it does;
- another audience's capability ("can I block Dr. Bruno's day?" from a
  patient, "can I ask for a discount?" from a dentist);
- whether something needs approval, and whose.

**Runs:**
- 3 repeats of each set;
- Jev for decisions and DeepSeek V4.1 Flash for text;
- the judge is Claude Sonnet 5.

**The judge:**
- gets the capabilities that are true for that variant and caller;
- is not told the variant's name;
- is told which processes started in the turn.

**Per reply, the judge scores:**
- **`capability_error`:** says it can do what it cannot, or cannot do
  what it can (starting the right process counts as right);
- **`leak`:** Part B only; presents another audience's capability as
  available to this caller;
- **`directs`:** tells the user how to go on (what to say, or who to
  turn to);
- **`humane`:** 1–5.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| S1 | `self` is as right as a hand-written list, with nothing written | `self` capability errors ≤ `listed`'s + 2 (of 36) |
| S2 | and righter than nothing | `self` capability errors ≤ `bare`'s − 3 |
| S3 | and it does not go stale | `stale-self` capability errors ≤ 3 of 36, and ≥ 3 fewer than `stale-listed`'s |
| S4 | it directs the user | `self`'s `directs` rate ≥ `listed`'s − 10 points and ≥ `bare`'s + 10 points |
| S5 | audiences stay apart | Part B `self`: 0 leaks of 108; capability errors ≤ `bare`'s |
| S6 | nothing else changes | every existing test and replayed EvalSuite passes unchanged (opt-in); unit tests pin `FACTS.self` per audience: another audience's Functions never appear |

**Also reported:**
- `humane` per variant;
- the prompt tokens `self` adds;
- the cost;
- which messages the router took to a Function instead of the free mode.

**Budget:** $3.
