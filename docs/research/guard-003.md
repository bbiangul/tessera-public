# guard-003: c2 at 0.5 on new data (#58)

- **Date:** 2026-09-26.
- **Pre-registration:** `guard-003-preregistration.md`.
- **Data:** `bench/results/guard-003/`.
- **Spend:** about $0.30 (G5's agents and judge), plus cents for Jev.

## Result

**Test set v3.** There are 60 new replies, asked 3 times each.

| | Self-descriptions rejected (of 45) | Other true replies rejected (of 45) | Claims missed (of 90) |
| --- | --- | --- | --- |
| c0 at 0.5 (today) | 3 | 0 | **29** |
| **c2 at 0.5** | **0** | 1\* | **3** |

\* A Jev HTTP 520. An error counts as a claim in production, so it is
counted as a false positive.

**G4, end to end.** Traveler `listed`, 10 new messages about the agent,
3 repeats:

| | "Desculpe, não entendi" |
| --- | --- |
| c0 | **18 of 30** |
| c2 | **0 of 30** |

The c2 replies were read. They answer the questions truthfully ("não
monto pacotes… o que eu faço é buscar passagens e hospedagem") and none
claims an action.

**G5, failures.** Seed 58, tessera-drift, D1–D4 × 2 repeats:
- false claims: 2 of 96;
- explained: 72 of 72 (D1–D3).

drift-006 had 0 of 96, with the same bar of ≤ 4.

**Hypotheses**

| # | Result |
| --- | --- |
| G2a | **Not supported as written.** It needed c2 ≤ c0 − 5. c0 rejected only 3 self-descriptions on this set, so the bar could not be met. c2 rejected 0. |
| G2b | **Supported:** 1 ≤ 0 + 2. |
| G3 | **Supported:** 3 ≤ 29 + 2. |
| G4 | **Supported:** 0 of 30, against c0's 18. |
| G5 | **Supported:** 2 of 96. |

## Across the three sets

These are all data c2 was not chosen on: guard-002's test and
guard-003's test.

| | False positives | Claims missed |
| --- | --- | --- |
| c0 at 0.5 | 6 + 3 = 9 of 144 | 15 + 29 = **44 of 144** |
| c2 at 0.5 | 4 + 1 = 5 of 144 | 2 + 3 = **5 of 144** |

**Today's guard lets about one claim in three through** when the claim
is about an approval, a request, a call or a completed change:
- "Seu pedido de troca foi aprovado pela loja"
- "Avisei o professor que você vai faltar"
- "Liguei para a clínica e confirmei o horário"

drift-002 and drift-006 measured it only on rescheduling claims.

## Decision

The registered rule needs G2a, and G2a is not met as written. The bar
was mis-specified: it assumed c0 would reject at least 5 of 45
self-descriptions.

**Not shipped by me: this is the owner's call.** The evidence favours
c2 on every measure:
- fewer true replies rejected;
- about 9× fewer claims missed;
- 0 of 30 against 18 of 30 end to end;
- G5 within its bar.

**The change is ready:**
- It is on the branch `guard-c2`: the guard's question gains two
  sentences, and the threshold stays 0.5.
- Shipping it needs the example cassettes re-recorded live. Their Jev
  requests change: 9 `clinic-weekdays` scenarios miss today with it.

## Shipped (2026-09-26, the owner's decision)

The owner decided to ship c2 at 0.5 despite G2a's unreachable bar. It
is in `internal/host/freemode.go`, and the threshold stays 0.5.

**Four example cassettes were re-recorded live:**
- clinic-weekdays: 53/53 pass;
- traveler: 24/24;
- clinic-context: 3/3;
- clinic-confirmation: 7/7.

Their guard requests changed. A missing recording also counts as a
claim, so it showed up as "não entendi" in replay. The other examples
replay unchanged, and `make ci` passes.
