# self-001: the agent knows what it can do

- **Date:** 2026-09-26.
- **Pre-registration:** `self-001-preregistration.md` (commit d70f826,
  before the code).
- **Data:** `bench/results/self-001/`.
- **Run:** `bench/scripts/self.sh`.
- **Spend:** $0.03 in runs, $0.86 in judging (including the second
  analysis).

## What was built

- **`freeMode.self: true`**, on the Agent's free mode or an audience's,
  gives the free mode's answer `FACTS.self` (`internal/host/self.go`).
  It is built by code from the admitted definitions:
  - `agent`: the Agent's description.
  - `does`: one item per Function this caller can start. It covers the
    routable Functions on the audience's menu whose `selector.who`
    holds, and the free mode's `functions`. Each item has:
    - `what`: the Function's `description`;
    - `onlyAfter`: for follow-ups;
    - `asksFor`: the inputs the user must give;
    - `youConfirm`: set when there is a `confirm`;
    - `personApproves` (`always` or `sometimes`) and `approvedBy`: from
      `escalate: approve` anywhere in the Function or in Functions it
      calls, including `route` targets. Under `approve-actions`, the
      `humanInTheLoop` queue too;
    - `personHelps`: set for other escalations.
  - `autonomy`: when it is not `autonomous`.
- **Rules** added to the free mode, only when set:
  - answer questions about itself from `FACTS.self`;
  - say what to send to start something;
  - never offer what is not in `does`.
- **Unit tests:** `internal/host/self_test.go`.
  - `FACTS.self` is pinned per audience: another audience's Functions
    never appear, and `selector.who` filters.
  - Approvals are covered in a called Function, conditionally and with
    routing; so are `approve-actions`, follow-ups, `asksFor` and the
    free mode's functions.
  - It is absent without `self`.
  - 8 mutations were planted, and all 8 were caught.

**A deviation from the pre-registration, before any test-set run.** A
plumbing check on a development message (not in either set) told a
dentist to give "the id of the dentist, like D1". That input falls back
to the caller's own id. So `asksFor` now lists only inputs whose last
source is `extract` or `ask`, not every input with such a source.

## Result

**Part A: the traveler. Capability errors in 36 replies (12 messages ×
3 repeats).**

| Variant | Errors | From the free mode | From "não entendi" | From a Function the router started | Directs |
| --- | --- | --- | --- | --- | --- |
| `listed` (hand-written list) | 18 | 4 / 13 | 11 / 15 | 3 / 8 | 24 |
| `bare` (no list) | 20 | 17 / 27 | 0 / 0 | 3 / 9 | 33 |
| **`self`** | **10** | **4 / 26** | 2 / 3 | 4 / 7 | 29 |
| `stale-listed` (hotels removed, list kept) | 30 | 12 / 15 | 15 / 15 | 3 / 6 | 18 |
| **`stale-self`** (hotels removed) | **7** | **2 / 28** | 2 / 2 | 3 / 6 | 30 |

In each "from" column, the first number is the errors and the second
is the replies of that kind.

**Humane (1–5):**
- `listed` 2.78
- `bare` 3.14
- `self` 3.39
- `stale-listed` 2.53
- `stale-self` 3.56

**Part B: the clinic's audiences (patient, dentist, owner), 36 replies
per variant.**

| Variant | Errors (registered truth) | Errors (truth v2) | From the free mode (v2) | Leaks | Directs |
| --- | --- | --- | --- | --- | --- |
| `bare` | 17 | 17 | 8 / 27 | 0 | 21 |
| `self` | 17 | 15 | 6 / 27 | **0** | 20 (v2: 21) |

**Hypotheses**

| # | Result | Detail |
| --- | --- | --- |
| S1 | **Supported** | `self` made 10 errors, against `listed`'s 18 (the bar was ≤ 20). Nothing was written by hand. |
| S2 | **Supported** | `self` made 10, against `bare`'s 20 (the bar was ≤ 17). |
| S3 | **Not supported as registered** | `stale-self` made 7, against the ≤ 3 registered. The free mode itself erred twice. The rest came from things `self` does not reach: the router took "me avisa se baixar" to a flight search three times, and the fixed "não entendi" (which still says hotels) came out twice. The comparison part holds: 7 against `stale-listed`'s 30. |
| S4 | **Not supported** | `self` directed in 29 replies, above `listed − 10` (24 − 3.6). It did not reach `bare + 10`: `bare` directed in 33. `bare` points somewhere almost every time, but where it points is wrong in 17 of its 27 free answers. The measure did not ask where to. |
| S5 | **Supported** | 0 leaks. Errors were ≤ `bare`'s: 17 against 17 as registered, 15 against 17 with the corrected truth. The pre-registration wrote "0 leaks of 108"; the set is 12 × 3 = 36 per variant. |
| S6 | **Supported** | `make ci` and `make lint-extra` pass. It is opt-in, so no cassette changed. |

## What the errors are

1. **The claims guard throws good answers away.** This is the largest
   single cause, and it is not `self`'s.
   - 15 of `listed`'s 36 replies were "Desculpe, não entendi". So were
     15 of `stale-listed`'s, 3 of `self`'s and 2 of `stale-self`'s.
   - The free mode had written a correct answer. One recorded example:
     "Sou o assistente de viagens do chat — um robô, não uma pessoa.
     Posso buscar passagens aéreas e hospedagem…".
   - The `claimsAction` guard (drift-006, #51) then rejected it twice,
     with Jev at 0.53 and 0.50 against a 0.5 threshold. The caller then
     falls back to `notUnderstood`.
   - A hand-written list makes the model state capabilities ("eu busco,
     eu anoto"), which the guard reads as claims. `self`'s rules
     ("what the user should say to start") trigger it less.
   - Opened as #58: the guard needs a measured fix, not a change
     inside this measure.
2. **Questions about a Function get routed to it.**
   - "me avisa se o preço baixar" started a flight search in every
     variant.
   - In the clinic, "se eu pedir um desconto, quem decide?", "se eu
     bloquear um dia precisa de aprovação?" and "e folga, como funciona
     pedir?" started the Function every time. Its input has no `ask`,
     so it failed, and the user heard "Algo deu errado". That is 9 of
     36 in both variants.
   - This is router-003's misrouting again. `self` never sees these
     messages (#59).
3. **The harness truth was wrong once.**
   - The registered truth said a dentist's day off is approved by the
     management only.
   - Under the dentists' `approve-actions`, the runtime also asks the
     reception, and `self` said so ("a gerência e a recepção aprovam").
   - The judge counted it as an error. `--truth-v2` corrects it: that
     is the "truth v2" column, as a second analysis.
4. **Third-person descriptions leak into speech.**
   - The owner's `self` item reads "The owner reads the week's
     discounts". The reply said "a dona lê os descontos" to the owner,
     and omitted her open tasks (`freeMode.tasks` is not in `self`).
   - Writing descriptions as capabilities ("Shows the week's
     discounts") is now in the authoring guide. Adding `tasks` to `self`
     is a follow-up (#60).

**Cost:**
- About 650 input tokens per free answer over `bare`; about 150 over
  `listed`.
- DeepSeek's cost per 12 messages rose by about $0.0003.

## What this means

1. **The agent knowing itself works, and it cannot go stale.**
   - The free mode's own capability errors went from 17/27 with nothing
     to 4/26 with `self`.
   - That is as good as a hand-written list (4/13) that nobody has to
     write.
   - When a Function is removed, `self`'s errors stay low (2/28), while
     the stale list's go up (12/15).
2. **Most remaining errors are elsewhere:** the claims guard (#58) and
   routing questions about a capability to it (#59).
3. **Default:** it stays opt-in in the engine for now. Default on would
   change every recorded cassette, and the guard problem affects every
   answer that describes the agent. The authoring skill and guide now
   recommend `self: true` for every free mode. Default on is proposed
   once the guard is fixed.

## Default on (#60, 2026-09-26)

After guard-003 fixed the claims guard (#58), `freeMode.self` is on
unless set to `false`. With `freeMode.tasks`, a member's `self` also
lists their open tasks.

**Gate:** every example passes live, and four cassettes were
re-recorded (clinic-weekdays, traveler, clinic-context,
clinic-confirmation).

The one live failure was `clinic-weekdays`
`preference-final-replication/correct-mixed`, the scenario already
known to be flaky. drift-006's rule applies: over 10 live runs it passed
8 with `self` on and 5 with it off.
