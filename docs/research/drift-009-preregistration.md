# drift-009: the drift report and promotion (RFC-0005 steps 1 and 4). Pre-registration

- **Date:** 2026-09-26, before any run on seed 55.
- **Issue:** #26.

## The feature

**`tessera drift <defs> --db <journals> --namespace ns`**, read-only.
Per action it reports:
- refusals with the system's words;
- `CONTRACT` failures (missing fields);
- read renames (`…/repaired`);
- request repairs (fields, what was asked, what the user agreed to,
  approvals, successes).

**Proposals** (step 4), never applied:
- **binding-output:** a read repaired the same way at least twice gets an
  `http.output` expression mapping the new fields back to the declared
  type;
- **author-note:** a request repair becomes a note (a new input asked;
  the fee named in the confirm; never a flag that skips the user).

For this, the journal now records the failing action on errors and on
repair proposals.

## Method

- **Journals:** `bench/market/tessera-improvise` (types, `repairReads`,
  `repair`) on fresh seed 55, 12 cooperative tasks × 1 repeat per
  condition D0–D6. `market.sh -keep-db 1` keeps the per-run journals,
  one folder per condition.
- **The report** runs on each condition's journals separately, with
  `tessera-improvise`'s definitions.

**Scored mechanically**, the expected signal per condition:

| Condition | Expected |
| --- | --- |
| D1 | a refusal naming `reason`, and a repair adding `reason=(asked)` that succeeded |
| D2 | a refusal naming R$ 50, and a repair adding `acceptFee` with an agreement question that succeeded |
| D3 | a refusal naming the phone 3030-4040 |
| D4 | a `CONTRACT` on list-appointments missing label and start, a rename `label ← description, start ← startsAt`, and a binding-output proposal |
| D5 | a refusal naming R$ 500, and no succeeded repair |
| D6 | as D4, and the rename never uses `createdAt` or `createdDescription` |

**Promotion test:** the D4 binding-output proposal, applied to a copy of
`tessera-improvise` with `improvise` removed, run on D4. The D6 proposal,
applied the same way, run on D6. Same 12 tasks, 1 repeat each.

## Hypotheses

| # | Hypothesis |
| --- | --- |
| L1 | The report shows the expected signal in 6 of 6 drift conditions |
| L2 | D0's report has 0 contracts, 0 renames and 0 repairs. Refusals, if any, are listed and counted as false alarms unless the system's words are the ordinary "slot not available" |
| L3 | With the promoted binding and no improvising: D4 ≥ 90% and D6 ≥ 90% success; the promoted agents' reports show 0 contracts and 0 renames |

**Budget:** $3.
