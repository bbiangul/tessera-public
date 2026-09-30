# Market benchmark 004: the product as it is today, pre-registration

- **Date:** 2026-09-25, before any run on the sets below.
- **Code measured:** `b54c27b`, frozen in a separate worktree. Another
  agent is changing the code at the same time; nothing here depends on
  that work.
- **Why:** the last direct comparison with every competitor was
  market-bench-003 (seed 24). Since then:
  - the voice became warm by default (voice-004/005);
  - its rewrite context changed (voice-007);
  - the free mode changed (freemode-003..005);
  - `decideBatch` and preference capture landed (weekday-001,
    preference-001).

  The claimed naturalness parity (55%) was measured before voice-007 and
  never re-measured. This round measures the product as it is, on sets
  nobody has run, against every competitor at once.

## Test sets (fresh)

- **Market seed 44:** `bench/market/tasks-044.json`, 20 tasks, generated
  for this round.
- **Off-script seed 42:** `bench/market/tasks-offscript-42.json`, 20 tasks,
  generated for this round.
- Seeds 43 and 41 are left to authoring-005, which another agent is
  running.

## Stacks

- **tessera-context:** `bench/market/tessera-context`, the hand-written
  reference of the authoring rounds.
- **tessera-weekdays:** `examples/clinic-weekdays`, the newest clinic
  (preferences, `decideBatch`).
- **The LLM competitors:** plain, langgraph, langgraph-sqlite, agents-sdk,
  agents-sdk-sqlite. The same model (DeepSeek V4.1 Flash), the same
  harness and the same simulator as before.

## Runs

1. **Market:** all 7 stacks, C0–C3, 3 repeats: 1,680 conversations.
2. **Naturalness:** each Tessera stack's C0 transcripts against the five
   competitors' C0 transcripts **from this same run** (300 pairs per
   Tessera stack). Claude Sonnet 5, blind, both orders.
3. **Off-script:** all 7 stacks, C0, 3 repeats on seed 42 (420
   conversations), and the off-script judge.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| S1 | Tessera's fault guarantees hold | Both Tessera stacks: 0 duplicate moves and 0 unconfirmed moves in C0–C3 |
| S2 | The LLM stacks still duplicate under act-then-timeout | At least one LLM stack duplicates in ≥ 10% of fired C1 runs |
| S3 | Success is at parity | Both Tessera stacks ≥ 97% in C0 and ≥ 95% in each of C1–C3 |
| S4 | Naturalness parity holds under today's voice | tessera-context's natural share of decided pairs ≥ 45% |
| S5 | Off-script holds | Both Tessera stacks: success ≥ 95%, 0 false claims |
| S6 | Cost and speed | Tessera's cost per successful C0 run ≤ 50% of the LLM median, and its p50 turn ≤ the LLM median's |

**Also reported, without thresholds:**
- clarity and efficiency shares;
- the judge's factual flags, checked by hand;
- tessera-weekdays against tessera-context on every metric.

**Budget:** $10 (runs about $3, three judges about $5.5).

## Addendum (before running it): a diagnostic of S4

The first judge gave tessera-context 32% natural, against voice-005's 55%.
That round differs in two ways:
- the code: voice-007 changed the rewrite context, and freemode-003..005
  changed the free mode;
- the comparison: competitor transcripts fresh from this same run,
  instead of market-002/003's older ones.

**Diagnostic D1:** voice-005's code (`be653f1`) with its agent
`bench/market/tessera-warm`, C0, 3 repeats on seed 44, judged against this
round's competitor transcripts.

**Reading, decided now:**
- D1 ≥ 45%: the drop comes from the code changes since voice-005;
- D1 within 10 points of 32%: the comparison set, not the code, explains
  it, and voice-005's 55% did not generalise to fresh competitor
  transcripts;
- in between: both contribute; reported as such.

Budget: +$2.5.

### D1 result, and addendum D2 (before running it)

D1 gave 47% (40–55), against tessera-context's 32% (26–39) on the same
tasks and competitors. By the reading above, the drop comes from changes
since voice-005. Those changes include both the runtime and the agent
definition (`tessera-warm` → `tessera-context`).

**Diagnostic D2:** today's code (`b54c27b`) with voice-005's agent
`bench/market/tessera-warm`, C0, 3 repeats on seed 44, judged against the
same competitor transcripts.

**Reading:**
- D2 within 7 points of D1 (≥ 40%): the runtime is not the cause; the
  definition change (`tessera-context`) is;
- D2 within 7 points of 32% (≤ 39%): the runtime changes are the cause,
  voice-007 being the first suspect;
- otherwise, both.

Budget: +$2.5.

### D2 result, and addendum D3 (before running it)

D2 gave 33% (26–41): today's runtime with voice-005's agent scores like
tessera-context (32%). The runtime changes since `be653f1` are the cause,
not the agent definition.

**Diagnostic D3:** a bisection by measurement, with no code change. The
commit just before voice-007's context isolation (`6793acf`), with
`bench/market/tessera-warm`, C0, 3 repeats on seed 44, the same
competitors and judge.

**Reading:**
- D3 ≥ 40%: voice-007 (`1d07796`) explains the drop;
- D3 ≤ 39%: the cause is earlier (voice-006 `ba72306`, freemode-003
  `2e78d82` or freemode-004 `6793acf`), to be bisected further.

Budget: +$2.5.

**Deviation, off-script:** `bench/scripts/market.sh` kills the subshell
but not the Python server under it. The market phase's servers outlived
it, with their data folders deleted, and the off-script phase reached
them:
- the in-memory competitors ran on identical, stale servers;
- the SQLite competitors failed every run ("unable to open database
  file").

The two SQLite competitors were run again on clean servers (the local
copy of the script starts Python with `exec`; the repo script is
unchanged) and replace the failed runs. The first rerun, before the
leftover servers were found, landed on the wrong servers and was
discarded.
