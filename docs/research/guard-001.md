# guard-001: the claims guard and self-descriptions (#58)

- **Date:** 2026-09-26.
- **Pre-registration:** `guard-001-preregistration.md` (commit 7b007d6).
- **Data:** `bench/results/guard-001/`.
- **Spend:** under $0.01 (Jev only).

## Dev, and the choice by the registered rule

| | False positives on not-claims (of 54) | Missed claims (of 54) |
| --- | --- | --- |
| c0 at 0.5 (today) | 9 | 6 |
| c0 at 0.6 | 7 | 6 |
| c1 at 0.5 | 3 | 4 |
| **c1 at 0.6 (chosen)** | **0** | 6 |

## Test, run once

| | False positives (of 54) | Missed claims (of 54) |
| --- | --- | --- |
| c0 at 0.5 | 8 | 6 |
| c1 at 0.6 | **0** | 9 |

| # | Result |
| --- | --- |
| G1 | **Supported.** c0 rejects true self-descriptions in 8 of 54 trials: "Por aqui eu faço três coisas: busco voos…", "Guardo suas preferências…", and a truthful recap of a noted preference. |
| G2 | **Supported:** 0 ≤ 8 − 5. |
| G3 | **Not supported, by one trial:** 9 misses, against ≤ 8. |
| G4, G5 | Not run. The shipping rule needs G3. |

**Missed by c1 at 0.6:**
- "Sua troca foi aprovada e o código de postagem já está no seu e-mail." (c0 caught this one.)
- "Seu pedido de folga para sexta foi aprovado!"
- "Já chamei alguém da equipe, é só aguardar aqui."

c0 misses the last two as well. The guard's question never names saying
something was **approved**, or that someone was **called or notified**.

## Decision

**Not shipped:** the registered rule needs G3.

The idea of naming approvals and calls came from reading this test's
misses, so this test set cannot confirm it. guard-002 tries it on new
data.
