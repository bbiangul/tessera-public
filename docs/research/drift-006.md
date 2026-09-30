# drift-006: explained failures become the default

- **Date:** 2026-09-25.
- **Pre-registration:** `drift-006-preregistration.md`.
- **Issues:** #26, #51.
- **Data:** `bench/results/drift-006/`.
- **Setup:** seed 52, 12 tasks × D0–D4 × 2 repeats.
- **Spend:** ~$0.35 for the agents, $0.73 for the judge, ~$0.1 for the
  example gate and re-recording.

## Result

| | D0 | D1 new field | D2 fee | D3 call instead | D4 renamed |
| --- | --- | --- | --- | --- | --- |
| **tessera-on** | 24/24 | false 0, explained 24 | fee 24; false 0, explained 24 | phone 24; false 0, explained 24 | false 0; to the queue 24/24 |
| **tessera-off** | 24/24 | false 0, explained 0 | fee 0; false 0, explained 0 | phone 0; false 7, explained 0 | false 0; to the queue 24/24 |
| **plain** | 24/24 | false 5, explained 19 | fee 24; false 7, explained 13 | phone 23; false 2, explained 23 | 24/24 success |

**Unsafe actions:** 0 for all.

**Median agent turn:** on 2.7 s, off 1.8 s.

| # | Hypothesis | Result |
| --- | --- | --- |
| H1 | D3 phone: on ≥ 70%, off ≤ 10% | **Supported** (100% / 0%) |
| H2 | D2 fee: on ≥ 70%, off ≤ 10% | **Supported** (100% / 0%) |
| H3 | False claims D1–D4: on ≤ 4/96 and ≤ off | **Supported** (0/96; off 7/96; plain 14/96) |
| H4 | Explained D1–D3 ≥ 70% | **Supported** (72/72) |
| H5 | D0 ≥ 90%, 0 unsafe | **Supported** |
| H6 | D4 reaches the queue ≥ 90% | **Supported** (24/24 both) |

**Replication:** drift-005 (seed 51) and drift-006 (seed 52) agree.
Explain on gives 0/96 false claims and every refusal explained, in both.

**The example gate** (`default-gate/`, with the rule declared in the
pre-registration): every example passes live. The two exceptions are
not caused by explain:
- a Jev HTTP 520;
- `clinic-weekdays` `correct-mixed`, flaky in both modes (off 6/10, on
  9/10).

## What changes

**`explainFailures` is on by default.** When an action fails because the
system refused it:
- the user hears that it was not done, what the system said and their
  next step;
- the claims guard keeps the free mode from saying it registered or
  passed on something;
- a failed run is handed to the team's queue (#52).

The example cassettes were re-recorded live with it on. The cost is
about 0.8 s more per agent turn: the free mode's claims check, and the
rewrite on failures.

**What stays unsolved, for RFC-0005's next steps:**
- **D1/D2**, a new field or accepting a fee. No stack succeeds: the tool
  schemas are fixed. Steps 2–3 (a Function written at runtime) are the
  way.
- **D4**, renamed fields. The loops adapt; Tessera hands the case to a
  person.
