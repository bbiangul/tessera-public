# drift-008: declared types and read repair (RFC-0005 step 2)

- **Date:** 2026-09-26.
- **Pre-registration:** `drift-008-preregistration.md`.
- **Issue:** #26.
- **Data:** `bench/results/drift-008/`.
- **Setup:** seed 54, 12 tasks × 2 repeats.
- **Spend:** ~$0.4 for the agents, $0.60 for the judge.

## Result

| | D0 | D1 new field | D2 fee | D3 call | D4 renamed | D5 R$ 500 | D6 renamed + decoy |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **tessera-improvise** | 24/24 | 20/24 | 24/24 | phone 24/24 | **24/24** | 0 moves, 0 fees | **22/24**, decoy shown 0 |
| tessera-drift (no types) | 24/24 | — | — | — | 0/24 | — | 0/24 |
| plain | 24/24 | — | — | — | 24/24 | — | 24/24 |

**Safety (tessera-improvise), every condition:** 0 duplicates, 0
foreign moves, 0 `noYesEver`, 0 fees without consent.

**Judged false claims** (`--history`): 0/144 for tessera-improvise in
D1–D6; 0 for the others.

**Median agent turn:** 2.7 s (tessera-drift 1.9 s).

| # | Hypothesis | Result |
| --- | --- | --- |
| J1 | D4 ≥ 70% | **Supported** (100%; tessera-drift 0%) |
| J2 | D6 ≥ 70%, decoy shown 0/24 | **Supported** (92%, 0/24) |
| J3 | No regression | **Supported**: D0 100%, D1 83%, D2 100%, D3 100%, D5 0 moves or fees, 0 unsafe |
| J4 | False claims ≤ 5/144 | **Supported** (0) |

**The misses:**
- **D6:** 2 were Jev HTTP 520 errors (infrastructure).
- **D1:** 3 of 4 were the patient insisting on a time not offered and
  giving up, a slot mismatch unrelated to drift. 1 had no repair
  proposed: the agent explained the missing reason and handed the case
  to the team.

## What this means

**With declared types, a changed API answer is noticed** (`CONTRACT`,
naming the fields) instead of breaking expressions later.
- With `repairReads`, renamed fields are mapped back, by renames only,
  and checked against the type.
- D4, the one drift where the LLM loops beat Tessera (0% vs 100% in
  drift-001–007), is now closed.
- The decoy (a creation date next to the appointment date) never
  reached the patient.

**Across the six drifts,** Tessera with `improvise` now:
- completes what the loops complete (D0, D3, D4, D6);
- completes what no loop can (D1 new field, D2 fee);
- refuses what it must (D5);
- makes no unsafe action and no judged false claim.

**Off by default:** `improvise.repairReads` and `repair`. Types are
opt-in per plugin; existing plugins are unchanged.

**Left in RFC-0005:**
- step 1's developer report (`tessera drift`) and step 4 (promoting
  repairs). The journal now holds what they need: `CONTRACT` failures,
  `…/repaired` renames, repair approvals.
