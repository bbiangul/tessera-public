# Market benchmark 004: the product as it is today, results

- **Date:** 2026-09-25
- **Pre-registration:** [market-bench-004-preregistration](market-bench-004-preregistration.md),
  with addenda D1–D3 written before each diagnostic ran.
- **Code measured:** `b54c27b`, in a separate worktree. The diagnostics
  used `be653f1` (D1) and `6793acf` (D3). No code was changed.
- **Results:** `bench/results/market-004/`:
  - `market/{context,weekdays}`;
  - `naturalness/{context,weekdays}`;
  - `offscript/`;
  - `diagnostic-d1..d3`.
- **Spend:** about $14:
  - runs about $2.5;
  - five naturalness judges about $10.8;
  - the off-script judge $0.79.

  The pre-registration budgeted $10, plus $2.5 for each diagnostic.

## 1. Market, seed 44 (fresh), C0–C3, 3 repeats

C1–C3 count only the runs where the fault fired (48–49 of 60).

| Stack | C0 | C1 success | C1 duplicates | C1 re-sent after applied | C2 | C3 | Unconfirmed moves | Cost / C0 run | p50 C0 turn |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **tessera-context** | **100%** | **100%** | **0 / 48** | **0** | 100% | 100% | **0** | **$0.0005** | **2.44 s** |
| **tessera-weekdays** | **100%** | **100%** | **0 / 48** | **0** | 100% | 100% | **0** | $0.0008 | 2.66 s |
| plain | 100% | 85% | 15 / 48 | 46 | 94% | 100% | 2 (C3) | $0.0012 | 2.92 s |
| langgraph | 100% | 75% | 15 / 48 | 42 | 98% | 100% | 0 | $0.0011 | 2.71 s |
| langgraph-sqlite | 100% | 85% | 13 / 48 | 40 | 100% | 100% | 0 | $0.0012 | 3.11 s |
| agents-sdk | 100% | 79% | 18 / 48 | 46 | 100% | 100% | 0 | $0.0012 | 2.92 s |
| agents-sdk-sqlite | 100% | 81% | 18 / 48 | 44 | 100% | 100% | 0 | $0.0012 | 2.63 s |

The plain loop also made 2 duplicates in C3 (retried delivery).

## 2. Naturalness (blind, both orders, 300 pairs per row, against this run's competitors)

| Tessera | Natural W / L / T | Natural, share of decided | Clear | Efficient | Tessera factual flags |
| --- | --- | --- | --- | --- | --- |
| tessera-context (today) | 55 / 116 / 129 | **32%** (26–39) | 15% | 22% | 2 |
| tessera-weekdays (today) | 1 / 257 / 42 | **0%** (0–2) | 0% | 65% | 5 |
| D1: voice-005's code, `tessera-warm` | 71 / 79 / 150 | 47% (40–55) | 22% | 12% | 2 |
| D3: `6793acf`, just before voice-007, `tessera-warm` | 81 / 73 / 146 | **53%** (45–60) | 29% | 15% | 1 |
| D2: today's code, `tessera-warm` | 53 / 106 / 141 | **33%** (26–41) | 16% | 19% | 1 |

**The cause of the drop (the bisection, readings fixed before each
run):**
- D1 ≥ 45%: the drop came from changes after voice-005, not from the
  fresh competitor transcripts.
- D2 ≈ tessera-context: the runtime, not the agent definition.
- D3 ≥ 40%: the change is **voice-007 (`1d07796`, "Isolate warm voice
  context")**. Before it the warm voice wins 53% of decided natural pairs;
  after it, 33%.

voice-007 removed the dialogue text from the warm rewrite's context, to
stop a false completion promise (#34). On this benchmark that fix costs
about 20 points of naturalness and 13 of clarity. The drop was not
measured when it shipped: voice-007 compared its candidates on promise
counts and a within-Tessera tie-break, and made no pairwise comparison
against the competitors (the HANDOVER says so).

**tessera-weekdays at 0%** is its definition, not the runtime: its
persona is a list of prohibitions. Its messages come out formal ("Responda
sim para confirmar ou não para desistir da remarcação"), with an emoji in
5% of them against about 70% for tessera-context. It has the shortest
flow, and the best efficiency, 65%.

## 3. Off-script, seed 42 (fresh), C0, 3 repeats

| Stack | Success | False claims | Dead ends | Humane (1–5) |
| --- | --- | --- | --- | --- |
| **tessera-context** | **60/60** | **0** | **0** | 4.63 |
| **tessera-weekdays** | **60/60** | **0** | **0** | 4.55 |
| plain | 59/60 | 1 | 0 | 4.80 |
| langgraph | 57/60 | 3 | 0 | 4.67 |
| langgraph-sqlite | 60/60 | 0 | 0 | 4.77 |
| agents-sdk | 60/60 | 0 | 0 | 4.70 |
| agents-sdk-sqlite | 60/60 | 0 | 0 | 4.70 |

## Hypotheses

| # | Verdict |
| --- | --- |
| S1: both Tessera stacks with 0 duplicates, 0 unconfirmed moves | **Supported.** |
| S2: an LLM stack duplicates in ≥ 10% of C1 | **Supported:** 27–38% of fired C1 runs, in every stack. |
| S3: Tessera ≥ 97% in C0 and ≥ 95% in C1–C3 | **Supported:** 100% everywhere. |
| S4: tessera-context natural ≥ 45% | **Not supported:** 32%. The diagnostics show the parity existed until `6793acf` (53%) and was lost with voice-007. |
| S5: off-script ≥ 95%, 0 false claims | **Supported:** 60/60, 0, both stacks. |
| S6: cost ≤ 50% of the LLM median, and p50 ≤ theirs | **Supported for tessera-context** (42%; 2.44 s against 2.92 s). **tessera-weekdays: cost 67%**, p50 supported (2.66 s). |

## Deviations

1. **The off-script phase met leftover servers.**
   `bench/scripts/market.sh` kills the subshell, not the Python server
   under it, so the market phase's servers outlived it, with their data
   folders deleted.
   - In the off-script phase the in-memory competitors ran on those
     identical servers (the same code, the same tool server address).
   - The SQLite competitors failed all 120 runs ("unable to open database
     file").

   Those were re-run on clean servers (a local copy of the script that
   starts Python with `exec`) and replace the failed runs. A first rerun,
   before the leftovers were found, landed on the wrong servers and was
   discarded. **The repository script still has this bug.**
2. **Seeds 44 and 42 instead of 43 and 41.** Those were already reserved
   for authoring-005 by another agent, so both rounds stay independent.

## What this suggests (for the people who change the code)

1. **voice-007 traded naturalness for the #34 fix without measuring it.**
   A fix that keeps both is needed:
   - keep the dialogue context, and check the rewrite for claims the
     draft does not support (a guard like the free answer's);
   - or narrow the context the other way: remove only what caused the
     promise.

   It should be judged pairwise against these same competitor
   transcripts (seed 44), with #34's regression scenarios. D3 (53%) is
   the target to recover.
2. **Pairwise naturalness against the competitors** should be part of the
   gate for any change to the voice or free mode. Cost: about $2.2 per
   judge.
3. **tessera-weekdays needs a warm persona** before it is used as a
   reference. The authoring lesson from authoring-002 applies to it too.
4. **Fix `bench/scripts/market.sh`:** start the Python servers with `exec`
   (or kill the process group).
