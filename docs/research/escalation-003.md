# Escalation 003: results

- **Date:** 2026-09-24
- **Pre-registration:** [escalation-003-preregistration](escalation-003-preregistration.md).
- **Code measured:** commit `4b92254`. Results are in
  `bench/results/escalation-003`.
- **Spend:** $3.01: runs $0.49, judge $2.52.

## Results on `test3` (19 new questions × 20 repeats)

| Stack | Unneeded escalation (answer + partial) | on partial | Coach recall | Mean per-question SD | Correct | False claims | Cost/run | p50 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| agents-sdk | 0% (0–1) | 0% | 100% | 0.00 | 100% (95/95) | 12 | $0.00035 | 1.58 s |
| plain | 2% (1–4) | 4% | 100% | 0.02 | 100% (95/95) | 3 | $0.00035 | 1.47 s |
| langgraph | 4% (2–7) | 10% | 100% | 0.03 | 98% (93/95) | 4 | $0.00035 | 1.44 s |
| tessera (routed) | 31% (25–37) | 60% | 22% | 0.03 | 92% (87/95) | 0 | $0.00009 | 1.46 s |
| **tessera, free-mode handoff** | **0%** (0–1) | **0%** | **100%** | **0.00** | 95% (90/95) | 5 | $0.00017 | 1.52 s |

- **Recall is now 100%.** All four processes the coach starts went to the
  coach in 20 of 20 runs: adding an owner, relocating, a long closure,
  handing the center to a son.
- **The routed Tessera fell to 22% recall:** its router does not
  recognise those processes from the message alone.
- **All 5 of the handoff's false claims are one question** (u16, "can we
  open at 8am on Saturdays?"):
  - the decision was right: answer, P(ask-coach) low;
  - the written answer then invented a rule, "opening earlier is an
    exception that needs your coach's approval".

  The writer contradicted the decision, although it was told nobody else
  needs to decide.

## Hypotheses

| # | Verdict |
| --- | --- |
| K1: unneeded ≤ half the best LLM stack's | **Supported only on paper:** 0% against 0% (Agents SDK). On this set the LLM stacks did not over-escalate either, so there was no difference to find. |
| K2: coach recall ≥ 90% | **Supported:** 100%. |
| K3: correct ≥ LLM lowest − 5; false claims ≤ LLM median | **Not supported:** correct 94.7% against the 92.9% bar, but 5 false claims against a median of 4, all on u16. |
| K4: SD ≤ LLM highest | **Supported:** 0.00. |
| K5: cost ≤ 50% of the LLM median | **Supported:** 48%. |

**The claim does not go on the list:** K3 failed, and K1 held without
a difference.

## Why this set found no difference

The phenomenon of [escalation-000](escalation-000.md) is questions whose
rule sits next to a clause that hands something to the coach. The lease
is the real case: the rule says "review before signing", with approvals
nearby. On those, the LLM stacks asked the coach in 45–74% of runs.

`test3`'s partial questions happen to have no such clause next to them:
- recruiting before site approval;
- extra hours;
- a background check before an offer;
- a short closure;
- early insurance.

So the LLM stacks answered them. The questions were written before any
run and are kept. The lesson is that the question set must be built to
hold the pattern being measured, as a stated stratum, not by chance.

## All three held-out sets together (post hoc, not a result)

These numbers pool runs of different Tessera versions (001, 002, 003)
and question sets of different difficulty. They are here for the
picture, not as a test of any hypothesis.

| Stack | Unneeded escalation | Coach recall |
| --- | --- | --- |
| plain | 11.5% (99 of 860) | 100% |
| agents-sdk | 11.9% (102 of 860) | 100% |
| langgraph | 17.7% (152 of 860) | 100% |
| tessera (routed) | 42.1% (362 of 860) | 69% |
| **tessera, free-mode handoff** | **4.7%** (40 of 860) | 89% (100% on the latest version) |

## Where this leaves #25

**Shown on held-out questions, three times:**
- The escalation decision declared in the free mode is perfectly stable:
  0 flaky questions in 1,220 runs.
- It costs about half as much as the LLM stacks.
- Where the over-escalation pattern exists (sets 1 and 2), it cuts
  unneeded escalations by more than half, and goes from 3–20 of 20 to 0
  of 20 on the real case's analogues.
- With the policy naming every kind of handoff, its recall is 100%.

**Not shown yet:** equal answer quality. The writer can still contradict
the decision (u16). That is a separate component, and it can be checked:
when the decision is to answer, a reply that sends the user to a person
or invents an approval is a contradiction the platform can detect before
sending.

**Next (escalation-004), when this is picked up again:**
1. A guard: when the handoff decided to answer, one Decide yes/no asks
   "does the reply say something needs someone's approval or send the
   user to a person?". A yes regenerates the reply once, then sends the
   template "I can answer that" path.
2. A fourth held-out set built with a declared stratum of "partial next
   to a coach clause" questions, so the pattern is present by design.
3. The claim rule unchanged: K1, K2 and K3.
