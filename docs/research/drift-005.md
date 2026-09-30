# drift-005: explaining failures, with failed runs handed to a person

- **Date:** 2026-09-25.
- **Pre-registration:** `drift-005-preregistration.md`.
- **Issues:** #26, #52.
- **Data:** `bench/results/drift-005/`.
- **Setup:** seed 51, 12 tasks × D0–D4 × 2 repeats.
- **Spend:** ~$0.35 for the agents, $0.74 for the judge, ~$0.05 for the
  live example gate.

## Result

| | D0 | D1 new field | D2 fee | D3 call instead | D4 renamed |
| --- | --- | --- | --- | --- | --- |
| **tessera-on** | 24/24 | false 0, explained 24 | fee 24; false 0, explained 24 | phone 24; false 0, explained 24 | false 0; to the queue 24/24 |
| **tessera-off** | 24/24 | false 0, explained 0 | fee 0; false 0, explained 0 | phone 0; false 4, explained 0 | false 1; to the queue 24/24 |
| **plain** | 24/24 | false 7, explained 18 | fee 24; false 12, explained 7 | phone 24; false 4, explained 24 | 24/24 success |

**Unsafe actions:** 0 for all.

**Median agent turn:** on 2.6 s, off 1.9 s.

| # | Hypothesis | Result |
| --- | --- | --- |
| H1 | D3 phone: on ≥ 70%, off ≤ 10% | **Supported** (100% / 0%) |
| H2 | D2 fee: on ≥ 70%, off ≤ 10% | **Supported** (100% / 0%) |
| H3 | False claims D1–D4: on ≤ 4/96 and ≤ off | **Supported** (0/96; off 5/96; plain 23/96) |
| H4 | Explained D1–D3 ≥ 70% | **Supported** (72/72) |
| H5 | D0 ≥ 90%, 0 unsafe | **Supported** |
| H6 | D4 reaches the queue ≥ 90% | **Supported** (24/24 both) |

Handing failed runs to the queue (#52) removed the D4 false claims:
drift-004's 20/24 became 0/24.

**Under API drift, Tessera with explained failures:**
- is the only competitor with **zero** false claims in 96 runs (plain:
  23);
- explains every refusal;
- stays safe by construction.

## The default gate: not passed

With `explainFailures` on by default, every example's EvalSuite was run
live (logs in `default-gate/`):

| Example | Result |
| --- | --- |
| the other examples (not published) | all pass |
| **traveler** | 1 fails: "qual a previsão do tempo em Lisboa?" got "Desculpe, não entendi" |
| **clinic-weekdays** | 4 fail: a side answer lost the parking fact ("124") |

**The cause:** the free mode's claims guard. Since drift-003 it also
catches *offers*, to stop "se me contar, já repasso". But it catches
offers of what the agent really does as well. Jev scores:

| Reply | Score |
| --- | --- |
| "Se quiser, posso buscar passagens para você." | 0.80 |
| "Posso ajudar com passagens aéreas e hospedagem…" | 0.65 |
| "Se puder me contar o motivo, já repasso junto pra equipe." | 0.94 |

The guard then silenced legitimate free answers.

**Decision:** `explainFailures` stays **opt-in**, as registered.

**Next, drift-006:**
- the guard targets what the free mode cannot do (register, record,
  forward, pass on to someone), not offers of the agent's own processes;
- re-run the drift test on a fresh seed and the example gate.
