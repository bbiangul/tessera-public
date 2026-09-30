# RFC-0004 baseline: pre-registration

- **Date:** 2026-09-24, written before any of these measurements ran.
- **Why:** [RFC-0004](../rfcs/0004-conversa-modo-livre-e-autoria.md) measures
  today's Tessera before building. Each part is judged against this
  baseline.
- **Owner's defaults** for the RFC's open questions: the voice starts on
  the small model; the naturalness judge is automatic, with a human
  sample optional later; the first authoring task is synthetic, so no
  customer data goes to external services.

## 1. Blind naturalness

**Material:** market-003's C0 conversations (`bench/results/market-003`,
seed 24): for each task and repeat, Tessera's transcript paired with each
competitor's. That is 5 competitors × 60 = 300 pairs.

**Judge:** `anthropic/claude-sonnet-5` through OpenRouter, temperature 0.
It is another model family than every competitor's (DeepSeek), so it
cannot prefer its own style. It sees:
- the two transcripts, labelled A and B;
- the patient's goal;
- the tool server's final state and effect log, the ground truth for
  facts.

It does not see which competitor is which. Each pair is judged **twice,
with A and B swapped**. A verdict counts only when both orders agree;
otherwise it is a tie.

**Questions:**
1. **Natural:** which sounds more like a good human receptionist.
2. **Clear:** which is easier to follow.
3. **Efficient:** which reaches the goal with fewer needless steps.
4. **Factual errors:** for each side, whether it states something false
   against the ground truth (a date, a time, what was done), with a
   one-line reason.

**Metrics:**
- Tessera's win, tie and loss rate on each question, per competitor and
  pooled, with 95% Wilson intervals on the decided pairs.
- Factual error rate per side.
- The judge's position consistency: the share of pairs where both orders
  agree.

**Hypotheses:**

| # | Hypothesis | Supported if |
| --- | --- | --- |
| N1 | Tessera loses on naturalness today | Tessera's pooled win rate on "natural" is below 30% of decided pairs |
| N2 | Tessera is not less clear | Win rate on "clear" is 40% or more of decided pairs |
| N3 | Tessera makes no more factual errors | Its factual error rate is at most the competitors' |

**Budget:** $8 hard cap for the judge.

## 2. Off-script tasks

**Question:** what does each stack do with requests its process does not
cover?

**Clinic facts:** the tool server gains `GET /clinic/info`, fixed facts:
address, parking, opening hours, prices, accepted insurance, phone.
- The competitors get it as one more read tool, `clinic_info`, the same
  description for all.
- Tessera's plugin declares the action, but today no Function can use it.
  That is the honest baseline: today Tessera answers only what a Function
  declares. After Part B, its free mode reads the same action.

**Tasks:** `bench/market/tasks-offscript.json`, seed 31, 20 tasks.

| Kind | n | What the patient does | Handled well when |
| --- | --- | --- | --- |
| side-question | 6 | Asks to reschedule and, midway, asks one fact ("tem estacionamento?") | The fact is answered correctly and the reschedule ends right |
| info-only | 4 | Only asks facts (price, insurance, address) | Answered correctly, nothing changes |
| uncovered-action | 4 | Asks to cancel (no tool cancels) | No claim that it was done; a way forward (phone or a person); nothing changes |
| small-talk | 2 | Greets, thanks, says goodbye around a short reschedule | The reschedule ends right; the small talk gets a natural answer |
| new-request-midway | 2 | Midway asks to book a new cleaning (no tool books) | No false claim; the reschedule is finished or explicitly set aside |
| over-reach | 2 | Asks to reschedule the whole family's appointments | Only the patient's own changes; the rest refused politely |

**Judgement:**
- The tool server's final state checks what changed, as before.
- The same judge (Claude Sonnet 5), given the clinic facts and the final
  state, answers per transcript:
  - `answered`: every answerable question got a correct answer;
  - `false_claim`: the agent said something false or claimed an action it
    did not do;
  - `dead_end`: it replied "não entendi" or equivalent to something it
    could have answered or redirected;
  - `humane`: 1–5.
- A task **succeeds** when the state is right, `answered` holds, there is
  no `false_claim` and no `dead_end`.

**Runs:** all 6 competitors, C0 only, 3 repeats: 360 conversations.

**Hypotheses:**

| # | Hypothesis | Supported if |
| --- | --- | --- |
| O1 | Today's Tessera handles off-script requests badly | Success below 50%, with dead ends in most side-question and info-only runs |
| O2 | The LLM stacks handle them well | Success of 80% or more |
| O3 | Nobody makes false claims about the uncovered action | 0 false claims of cancelling |

**Budget:** $6 for runs and judge.

## 3. Authoring by prompt

**Question:** today, with only the docs, how much does it take a coding
agent to build a Tessera agent from one briefing, and how good is the
result?

**Author:** a Claude Code subagent (Claude Opus 5.5, the model of this
session) in an isolated git worktree, with only the briefing below.
- **Allowed:** the docs, `AGENTS.md`, `tessera explain`, the other
  examples, the CLI.
- **Not allowed:** `examples/clinica` and `bench/market/` except the
  API's `openapi.json`. Those hold a finished reschedule Function it
  would copy.
- It may call Jev and DeepSeek for its own evals, with at most $1.

**The briefing, verbatim:**

> Build a Tessera agent for a dental clinic's chat, in Brazilian
> Portuguese: logged-in patients reschedule their own appointments. The
> clinic's API is described in `bench/market/toolserver/openapi.json`
> (base URL https://clinic.bench). Policy: only the patient's own
> appointments may be moved; before moving, state the new date and time
> and get an explicit yes; never move an appointment twice for one
> request; if no slot fits, say so and never invent one. The caller
> identity has `role: patient` and `patientId`. Put the definitions in
> `authored/clinica/` (namespace `clinica-bench`, the Function routable),
> with an EvalSuite that covers the policy, and make `tessera validate`
> and `tessera test` pass.

**Measured:**
- wall time, the author's tokens and tool calls, how often `validate`
  and `test` were run, and the questions it could not settle alone
  (which count as human interventions);
- then the produced agent in the market harness on market-003's tasks
  (seed 24), Tessera only, C0–C3, 3 repeats. The same harness and judge,
  compared with the hand-written agent's 100%.

**Hypotheses:**

| # | Hypothesis | Supported if |
| --- | --- | --- |
| A1 | Today one briefing yields a valid agent without human help | `validate` has no errors, its own EvalSuite passes, within 60 minutes, with 0 interventions |
| A2 | Without the benchmark's lessons, the result is worse than the hand-written agent | C0 success below 90% on the held-out tasks |
| A3 | The fault guarantees do not depend on the author | 0 duplicate moves in C1 and C3 |

**Budget:** the author's evals $1. The harness run is covered by the $6
above.

## Deviations

See [rfc0004-baseline](rfc0004-baseline.md#deviations): the naturalness
judge runs with reasoning off, and the judges write and resume per
verdict.
