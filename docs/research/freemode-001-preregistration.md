# Free mode 001 (RFC-0004 part B): pre-registration

- **Date:** 2026-09-24, before the measured runs.
- **Baselines:**
  - [rfc0004-baseline](rfc0004-baseline.md): off-script success 22%, 47/60
    dead ends; a side question at a confirmation cancelled it.
  - [voice-001](voice-001.md): naturalness wins 38 of 300, clarity 0.

## What changed

- **Interruptions.** While a run waits for the user and the Agent has a
  free mode, each message is first classified by Jev as one of:
  - an answer to the pending question (the default when unsure);
  - a question;
  - a request;
  - a cancel;
  - a goodbye.

  A question or request gets the free mode's answer, then the pending
  question **word for word**. A cancel or goodbye ends the run
  (`Runtime.Stop`) without acting. Before, a side question at a
  confirmation was read as a no.
- **Free mode** (`Agent.spec.freeMode`): instructions, plus read-only
  actions whose results are what it knows.
  - It answers unrouted messages and interruptions. It never acts:
    admission refuses reads that are not read-only.
  - Its model input is masked, except reads declared `public` (the
    business's own information; otherwise a clinic phone became a
    placeholder).
  - Its output is checked for placeholders and length. Falling back
    gives the old not-understood message.
- **The benchmark agent:** a free mode reading `clinic/info` (public);
  the newSlot question is skipped when the day has one slot. A day
  question that also offered morning and afternoon options was tried and
  dropped: overlapping options made "quarta 14/10" ambiguous.
- **Voice and free mode** greet only in the first agent message.

**Development:**
- the off-script set, seed 31 (the baseline's; seen): 20/20 on one run;
- seed 23: C0 and C3 100%.

## Test sets

- **Off-script, held out:** `tasks-offscript-32.json`, seed 32. The same
  kinds, with other patients, slots, targets and facts asked in another
  order. All 6 competitors, C0, 3 repeats (360 conversations), and the
  same off-script judge.
- **Market:** seed 24 (market-003's tasks), Tessera C0–C3, 3 repeats,
  checking that safety and success hold with the interruption
  classifier in the loop.
- **Naturalness:** Tessera's new seed-24 C0 transcripts against the
  competitors' market-003 transcripts, with the same judge.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| B1 | Off-script requests are handled | Tessera off-script success ≥ 80% on seed 32 |
| B2 | No dead ends | Tessera dead ends ≤ 5% of its off-script runs |
| B3 | The free mode never acts or claims to | 0 effects outside Functions; 0 false claims of an action |
| B4 | The classifier does not hurt the process | Market C0 success ≥ 97%; C1 and C3 0 duplicates; 0 unconfirmed moves |
| B5 | Naturalness and clarity improve over voice-001 | Natural ≥ 30% and clear ≥ 20% of decided pairs |
| B6 | Still faster than the competitors | Tessera p50 turn ≤ 2.0 s in C0 |

**Budget:** $7 for the runs and the judges.
