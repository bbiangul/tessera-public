# drift-009: the drift report and promotion (RFC-0005 steps 1 and 4)

- **Date:** 2026-09-26.
- **Pre-registration:** `drift-009-preregistration.md`.
- **Issue:** #26.
- **Data:** `bench/results/drift-009/`:
  - per-condition runs and reports;
  - `reports-registered.json`, the report as registered;
  - `reports.json`, after the fix below;
  - `promoted-D4/`, `promoted-D6/`.
- **Setup:** seed 55, 12 tasks × 1 repeat per condition, journals kept
  (not committed).
- **Spend:** ~$0.25; the report makes no model calls.

## Result

| # | Hypothesis | Result |
| --- | --- | --- |
| L1 | The expected signal in 6/6 drift conditions | **Not supported as registered: 4/6**. **6/6** after a report fix, declared post hoc |
| L2 | No false alarm on D0 | **Supported**: 0 contracts, 0 renames, 0 repairs, 0 refusals |
| L3 | The promoted binding, no improvising: D4 and D6 ≥ 90% | **Supported: 12/12 each.** The promoted agents' reports say "no drift signals" |

**L1:**
- **Found as registered:**
  - D1: the refusal naming `reason`, and `reason=(asked)` repaired;
  - D2: R$ 50, and `acceptFee` with the agreement question;
  - D3: the phone;
  - D5: R$ 500, with no successful repair.
- **D4 and D6:** the rename `label ← description, start ← startsAt`
  appeared 12/12, never a decoy field, with a binding-output proposal.
  But no `CONTRACT` line: when the read is repaired it does not fail, so
  the journal holds only the repair.
- **The fix, post hoc, report-side only:** a `…/repaired` record means
  the type was broken. The report now lists it under contracts as
  "missing label, start (repaired by renaming)". Re-run on the same
  journals: 6/6.

**L3, promotion:**
- The D4 and D6 proposals were the same expression:
  `sources.response.body.map(x, {'dentistId': x.dentistId, 'id': x.id, 'label': x.description, 'patientId': x.patientId, 'slotId': x.slotId, 'start': x.startsAt})`.
- Pasted into the plugin of a copy with `improvise` removed
  (`bench/market/tessera-promoted-d4`, `-d6`), it passed D4 12/12 and D6
  12/12 (the decoy fields ignored). The drift report on those runs was
  empty: the agent is up to date again, with no improvisation.

## What this means

RFC-0005's four steps are in place:

| Step | What it does | Where |
| --- | --- | --- |
| 1, notice | `tessera drift` lists refusals with the system's words, broken contracts and repairs per action, with no false alarm on an unchanged API | this report |
| 2, repair reads | declared types; rename-only repair | drift-008 |
| 3, repair actions | additions only; the user's value or agreement; a person approves | drift-007 |
| 4, promote | a repeated read repair becomes a binding change that makes improvising unnecessary; a request repair becomes a note for the author | this report |

**Honest limit:** for request repairs (D1/D2) the proposal is a note, not
a patch. Turning "a fee now applies" into a Function needs an input or a
confirmation designed by the author (or the authoring skill), not a
generated flag.
