# starmem-001: can one Starlark call take the engine down?

- **Date:** 2026-09-25.
- **Pre-registration:** `starmem-001-preregistration.md`.
- **Data:** `bench/results/starmem-001/`.
- **Harness:** `bench/starmem`, run in a Linux container capped at 2 GiB,
  3 repeats, default limits.

## Result

| Case | Outcome (3/3 each) |
| --- | --- |
| `small-ok` | ok |
| `loop-append` | cancelled, "memory limit", 88–142 ms, process alive |
| `str-512m` | cancelled after the allocation, process alive |
| `list-1g` | cancelled after the allocation (1 GiB held), process alive |
| `concat-double` | cancelled after the first 256 MiB, process alive |
| `comp-4g` | cancelled after the first 512 MiB, process alive |
| `join-4g` | cancelled after the first 512 MiB, process alive |
| **`list-16g`** | **OOM-killed (exit 137)** |
| *`join-shared`* (exploratory) | **OOM-killed** |
| *`list-3g`* (exploratory) | **OOM-killed** |

**Hypotheses**

| # | Result |
| --- | --- |
| S1 | Supported |
| S2 | **Supported** |
| S3 | Supported |

## What this means

- **The watchdog is better than the review assumed.** Starlark checks the
  cancel before each instruction, so a script building memory up over
  several instructions is stopped at the next one. This held even when
  each instruction allocated 512 MiB.
- **What it cannot stop is one instruction that allocates more than the
  process's headroom.** A script reaches that trivially, while staying
  under the watchdog until the fatal instruction:
  - `[0] * (200 << 20)`: 3 GiB. starlark-go caps a repeat at 2³⁰
    *elements*, which is ~16 GiB for a list.
  - `''.join([s] * 20)` with a 200 MiB `s`: `join` has no cap.

  The OOM kill takes down the whole engine and every conversation it
  serves. A script is written by the agent's author, not by end users, so
  this is a robustness problem more than an attack surface: a bug in a
  script is enough.
- **Checking sizes before allocating is not available.** The list repeat
  is an interpreter operator and `join` a built-in string method, and
  neither can be wrapped without forking starlark-go.

  The fix is the one RFC-0001 planned: run the call in a child process
  with an OS memory limit (Linux `RLIMIT_DATA`, which counts Go's heap
  mappings). If the child is killed, the call fails with "memory limit"
  and the engine lives.


## Round 2: the fix, isolated calls

**The fix:** `compute.Isolate`, used by `tessera serve` by default. Each
call re-executes the binary as a worker, which:
- sets `RLIMIT_DATA` (4 × `Limits.Memory`, 1 GiB) and
  `oom_score_adj=1000`;
- runs the call with the same watchdog and step limit;
- returns the result by gob, which keeps int, float and time exact.

When the child dies, the call fails with "the memory limit was reached
(the script's process ended)". Its stderr is not kept, because it may hold
script data.

| Case (3/3 each, 2 GiB container) | Round 1, in-process | Round 2, isolated |
| --- | --- | --- |
| `small-ok` | ok | ok |
| `loop-append`, `str-512m`, `concat-double`, `comp-4g`, `join-4g` | memory limit (watchdog) | memory limit (watchdog, in the child) |
| `list-1g` | memory limit (watchdog, after holding 1 GiB) | memory limit (the child hit RLIMIT) |
| `list-16g`, `list-3g`, `join-shared` | **engine killed** | memory limit (the child hit RLIMIT) |

| # | Result |
| --- | --- |
| R1 | **Supported:** 0 engine deaths in 30 runs |
| R2 | **Supported after one fix** |
| R3 | **Supported:** median 3.7 ms per call (p90 6.4 ms), against 0.06 ms in-process. That is well under 20 ms and negligible next to a model call (~1–2 s) |

**The R2 fix.** On the first isolated run (`runs-isolated-first.tsv`),
`list-1g` said "the script's process failed". Go reports a refused
mapping as "cannot allocate memory", not "out of memory". The parent now
reads both; the round was re-run in full (`runs-isolated.tsv`), and a
unit test covers both messages.

**Limits of the fix:**
- The hard bound is Linux's (the production image). On macOS the child
  is only separated: a huge allocation there still consumes memory, and
  only the child can die.
- `tessera test` and `chat` stay in-process: they are development tools.
- Library users opt in (RFC-0007).
