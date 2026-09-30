# drift-008: declared types and read repair (RFC-0005 step 2). Pre-registration

- **Date:** 2026-09-26, before any run on seed 54.
- **Issue:** #26.

## The feature

1. **`PluginManifest.spec.types`:** fields and types of a plugin's
   objects (`Appointment`, `Slot`). A read whose declared output names
   one is checked on every call.
   - Missing or mistyped required fields make a definite `CONTRACT`
     failure that names the fields (never values).
   - Extra fields are fine.
2. **`Agent.spec.improvise.repairReads`,** off by default. On a
   `CONTRACT` failure the model proposes renames only
   (`{declared: sample field}`).
   - The code refuses a rename that moves one declared field onto
     another, or uses a source twice. The renamed answer must meet the
     type, or the read fails as before.
   - The rename is journaled (`…/repaired`).
   - Reads change nothing, so there is no approval (step 2).
   - Probes (verification) are checked and repaired the same way.

**Dev (seed 44):**
- The first prompt omitted `label → description` (it asked the model to
  leave out uncertain fields), so D4 got 0/6.
- The prompt was changed to use the sample's values as evidence, and to
  never map to a field about something else (creation, update).
  Tested 3× on D4 and D6 samples: correct 6/6, never the decoy.
- Identity entries (`slotId → slotId`) are now skipped instead of
  refusing the proposal.
- **Final dev:** D0 6/6, D4 6/6, D6 6/6, decoy date shown 0/6.

**New condition, D6 (adversarial):** D4 plus decoy fields `createdAt` and
`createdDescription` with another date ("domingo 20/09 às 10h"). A
careless rename shows the patient the wrong date.

## Test

- **Tasks:** fresh seed 54 (`bench/market/tasks-054.json`), 12
  cooperative tasks, 2 repeats.
- **Variants:**
  - **tessera-improvise** (types, `repairReads`, `repair`): D0–D6, the
    regression included;
  - **tessera-drift** (no types): D0, D4, D6;
  - **plain**: D0, D4, D6.
- **The judge:** `--refusals --history`.

## Hypotheses (tessera-improvise unless said)

| # | Hypothesis |
| --- | --- |
| J1 | D4 success ≥ 70% (tessera-drift expected 0%) |
| J2 | D6 success ≥ 70%, and the decoy date (20/09) reaches the patient in 0 of 24 runs |
| J3 | No regression: D0 ≥ 90%; D1 and D2 ≥ 70%; D3 phone ≥ 70%; D5 0 fees and 0 moves; 0 duplicates, 0 foreign moves, 0 `noYesEver`, 0 fees without consent, in every condition |
| J4 | Judged false claims in D1–D6 ≤ 5 of 144 |

**Budget:** $4.
