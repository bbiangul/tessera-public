# ADR-0004: our own journal for phase 1; the long-wait engine is decided before phase 2

- **Status:** Accepted; the long-wait question is answered by ADR-0005
- **Date:** 2026-09-23
- **Context:** RFC-0001 phase 0 asks for a spike to choose the durable
  engine: Temporal, Restate, or our own over SQLite/Postgres.

## Decision

Phase 1 runs on our own journal (RFC-0003): a run is re-executed from the
top over an append-only list of records keyed by stable sites, stored in
SQLite. The choice of an engine for **long waits** (phase 2: `Workflow`,
`wait`, timers of days, `Trigger`s) is made before phase 2 starts, with
the RFC's criteria, and the journal interface (`journal.Store`) is the
seam where an engine would plug in.

## Why

- **Phase 1 has no long waits.** A Function run waits for a user or a
  person; the wait is a row, not a timer. That is the part the journal
  already does, measured:
  - crash injection at every journal write of a whole conversation
    recovers with no duplicated effect or message (`TestCrashAtEveryAppend`);
  - replaying 51 records costs 125 µs (`BenchmarkReplay50`), so "replay
    from the top" is not a cost at phase 1 sizes.
- **Single binary.** The open-source build must start with one command
  (RFC-0001 criterion 1); SQLite through a pure-Go driver keeps it static.
- **The semantics are ours either way.** Effect classes, verification,
  Indeterminate, the transactional outbox and the confirmation rules are
  in the runtime, not in the engine; adopting Temporal or Restate later
  changes storage and scheduling, not these.

## What is not decided, and how it will be

Temporal and Restate were not benchmarked; this ADR does not claim our
journal is better than either. Before phase 2, a spike runs an outbound sales
cadence (D+1…D+15 with business hours) on each candidate and on our
journal with a timer table, against the RFC's criteria: single binary,
code versioning of runs that live 15 days, Go SDK quality, license, cost of
millions of waiting runs, time-zone timers.
