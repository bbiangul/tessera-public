# starmem-001: can one Starlark call take the engine down? Pre-registration

- **Date:** 2026-09-25, before any run.
- **Issue:** #6.
- **Background:**
  - The watchdog (`internal/engines/compute/watch.go`) samples the heap
    every 5 ms and cancels a call that grows it by more than 256 MiB.
  - It cannot interrupt a single allocation.
  - starlark-go caps one repeat at 2³⁰ elements. That is 1 GiB for a
    string, but ~16 GiB for a list (16 bytes per element), and `+` and
    `join` have no cap.
- **Method:**
  - `bench/starmem` runs one attack against `compute.Script.Call` with the
    default limits, inside a Linux container capped at 2 GiB (a typical
    pod).
  - Exit 137 (OOM-killed) means the engine process died, and every
    conversation on it with it.
  - 3 repeats per case.

## Cases

| Case | What it does |
| --- | --- |
| `loop-append` | Many small allocations (the watchdog's test) |
| `str-512m` | One 512 MiB string |
| `list-1g` | One 1 GiB list |
| `list-16g` | One ~16 GiB list, under starlark's cap |
| `concat-double` | 256 MiB, doubled 3 times in 3 steps (2 GiB) |
| `comp-4g` | Eight 512 MiB strings in a comprehension |
| `join-4g` | One 4 GiB join |
| `small-ok` | Control |

## Hypotheses

| # | Hypothesis |
| --- | --- |
| S1 | `loop-append` is cancelled with "memory limit", and the process survives in 3/3 |
| S2 | At least one single-allocation case (`list-16g`, `join-4g`) OOM-kills the process in 3/3. This is #6's claim; if it fails, #6 is lower priority than the review thought |
| S3 | `small-ok` succeeds in 3/3 |

**Fix direction** (for a second round, measured with the same harness):
- Only if S2 holds.
- Candidates:
  1. run scripts in a child process with an OS limit (setrlimit
     RLIMIT_AS / a cgroup);
  2. check sizes before allocating, by wrapping the repeat, concat and
     join paths. The second is cheaper but only covers what we find.

## Round 2 (registered after round 1, before any isolated run)

**Setup:**
- the fix: `compute.Isolate`, with calls in a child process bounded by
  `RLIMIT_DATA` and `oom_score_adj=1000`;
- the same harness with `-isolated`, the same 10 cases, 3 repeats, the
  same 2 GiB container.

| # | Hypothesis |
| --- | --- |
| R1 | 0 engine deaths in 30 runs; `list-16g`, `list-3g` and `join-shared` end with "memory limit" |
| R2 | Every case that survived round 1 ends the same way (same result or error kind) |
| R3 | Median added latency per call below 20 ms (`small-ok`, 50 calls, in-process against isolated, in the container). This is the cost of one exec per call; a worker pool is the fallback if it fails |
