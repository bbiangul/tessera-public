# ADR-0005: durable timers on our own journal; Temporal is the reserve, Restate is out

- **Status:** Accepted
- **Date:** 2026-09-23
- **Context:** ADR-0004 left the engine for long waits (phase 2: `wait`,
  timers of days, `Trigger`s, `Workflow`s) to a spike before phase 2, with
  RFC-0001's criteria: single binary, versioning of runs that live 15 days,
  Go SDK quality, license, the cost of millions of waiting runs, timers in
  the agent's time zone. The spike ran an outbound sales cadence (D+1 at 10h on
  the next business day, then 48h later) on our journal and on Temporal.

## Decision

Timers live in our journal: a `wait` step, `WaitingForTime` runs with a
`wake_at` column (a partial index holds only sleeping runs), and
`Runtime.WakeDue` (RFC-0003, "wait"). Temporal stays the reserve behind
`journal.Store` if cluster mode (phase 4) needs what it offers; Restate is
excluded by its license.

## What was measured

**Our journal** (`go run ./internal/tools/timerbench`, SQLite on a laptop,
synchronous=FULL, one fsync per record):

| Measure | 20 thousand sleeping runs | 1 million sleeping runs |
| --- | --- | --- |
| Find the due runs when none is due | 0.2 ms | 0.02 ms |
| Find and load the due runs (11 / 264 due) | 0.3 ms | 6 ms |
| Disk per sleeping run (run, 3 records, index) | 1.1 KB | 850 bytes (854 MB in all) |
| Writing sleepers (5 fsynced writes each) | 2,400/s | 1,600/s |

End to end through the runtime (start the cadence, sleep, wake, send the
next message): about 2,000 runs started and 1,500–1,800 woken per second
on one node. For scale: a million leads each woken once a business day,
spread over 8 hours, is 35 wakes per second, about 40 times less than one
node does. The scan cost does not grow with sleeping runs that are not
due, because the index holds only sleepers and is ordered by wake time.

Correctness, as tests: sleeping and waking at the right local time across
a weekend (`TestWaitSleepsAndWakes`), no double wake from concurrent or
repeated schedulers (`TestWakeIsIdempotent`), a new process on the same
database finishing the cadence (`TestWaitSurvivesRestarts`), and a crash
injected at every journal write of the whole cadence with the same
messages and no duplicate (`TestWaitCrashAtEveryAppend`). `tessera chat`
wakes runs every second.

**Temporal** (`spikes/temporal-cadence`, Go SDK v1.49.0, its in-process
test environment): the same cadence is ~40 lines, runs in 0.75 s with time
skipping, and its versioning (`workflow.GetVersion`) works. The server was
not run: it is a separate service, and the only single-process form
(the embedded server on SQLite) is documented as for development and
testing only; production needs PostgreSQL, MySQL or Cassandra
([docs](https://docs.temporal.io/self-hosted-guide/embedded-server)).

**Restate** was not run: its server is under the Business Source License
1.1, which forbids offering it as a "Public Restate Platform Service"
where third parties register and invoke their own deployments
([license](https://github.com/restatedev/restate/blob/main/LICENSE)). That
is close to what the Tessera cloud is.

## Against the criteria

| Criterion | Our journal | Temporal | Restate |
| --- | --- | --- | --- |
| Single binary (open source starts with one command) | Yes: the same static binary | No: a server plus a database in production | No: a separate server (Rust) |
| Runs that live 15 days across releases | Each run is pinned to its generation (RFC-0003); nothing to write | By hand, `GetVersion` per change, or worker versioning | Deployment versions |
| Go SDK quality | n/a: definitions are YAML, not code | Mature (v1.49), 79 modules in a minimal program (Tessera has 59) | Younger Go SDK |
| License | Ours | MIT | BSL 1.1: restricts a managed platform |
| Millions of waiting runs | Measured above (disk per run, due lookup) | Not measured (needs the server) | Not measured |
| Time-zone timers, business days | `atTime`, `addBusinessDays` in CEL | Code in the workflow (`workflow.Now`, own helpers) | Code |
| Semantics we need (effect classes, verification, outbox, HIL) | Already there | Would sit on top, unchanged | Would sit on top |

## What would change this

- Cluster mode (phase 4) needs leases, sharding of the wake scan and
  timers across nodes. If building that costs more than running Temporal,
  the seam is `journal.Store` plus `WakeDue`, and runs keep their
  semantics.
- A measured limit: when a single node cannot wake the due runs of its
  tenants in time (the benchmark gives the rate), or the database grows
  past what one SQLite file should hold.
