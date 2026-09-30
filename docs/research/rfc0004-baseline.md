# RFC-0004 baseline: today's Tessera on naturalness, off-script requests and authoring

- **Date:** 2026-09-24
- **Pre-registration:** [rfc0004-baseline-preregistration](rfc0004-baseline-preregistration.md),
  with its deviations.
- **Code measured:** commit `9435964`. Results are in
  `bench/results/naturalness-000`, `offscript-000` and `authoring-000`.
  Spend: about $2.0 for the naturalness judge, $1.1 for the off-script
  judge, and about $0.5 for the runs.

## Summary

| Measure | Today's Tessera | LLM stacks |
| --- | --- | --- |
| Blind naturalness: wins / losses / ties, 300 pairs | **0 / 281 / 19** | — |
| Blind clarity: wins / losses / ties | **0 / 240 / 60** | — |
| Blind efficiency: wins / losses / ties | 21 / 42 / 237 | — |
| Off-script success (360 conversations) | **22%** (13–34%) | 95–100% |
| Off-script dead ends ("não entendi" to something answerable) | **47 of 60** | 0 |
| Authoring by one prompt: time to a valid, tested agent | **5.7 min**, 0 human interventions | — |
| The authored agent in the market harness (C0 / C1 / C2 / C3) | 98% / 100% / 98% / 100%, 0 duplicates | — |

**In one line:** the safety core and authoring already work. The
conversation is where Tessera loses, clearly: it is less natural, less
clear, and it dead-ends on anything its Functions do not declare. Parts A
and B of RFC-0004 are where the work is. Part C is less urgent than we
thought.

## 1. Blind naturalness (N1–N3)

The judge was Claude Sonnet 5 (another family than every competitor),
seeing each pair twice with the sides swapped. Its verdicts agree across
both orders in 94% (natural) and 83% (clear) of pairs.

- **N1 supported: Tessera loses on naturalness.** 0 wins out of 281
  decided pairs, the same against every competitor. The judge's reasons
  repeat: the others use the patient's name, give details (the dentist,
  the full date), format the message and sound warmer. Tessera is "dry".
- **N2 not supported: Tessera is also less clear.** 0 wins out of 240
  decided pairs. Tessera had been expected to be as clear.
- **Efficiency:** mostly ties (237). Tessera wins 21 and loses 42.
- **N3, factual errors:** the judge flagged 70 Tessera transcripts
  against 14 of the others. **On review none of the reasons is a false
  date, time or action.** They are two real defects of another kind:
  1. **Relevance:** the patient asks for the afternoon and Tessera lists
     every time of the day, morning included. Tessera narrows by day, not
     by period (t06, t08, t10, t12, t04).
  2. **Loops:** a patient who declines and says goodbye gets the same
     question again (t13, t16).

  N3 is recorded as **not supported as judged**. The two defects go to
  Parts A and B.

## 2. Off-script requests (O1–O3)

| Competitor | Success | Dead ends | False claims | Humane (1–5) |
| --- | --- | --- | --- | --- |
| tessera | **22%** | 47 | 1 | **1.8** |
| plain | 95% | 0 | 2 | 4.6 |
| langgraph / -sqlite | 100% | 0 | 0 | 4.7 |
| agents-sdk / -sqlite | 100% | 0 | 0 | 4.7–4.8 |

- **O1 supported.** Tessera by kind:
  - side-question 0/18;
  - info-only 0/12;
  - uncovered-action 0/12;
  - small-talk 6/6;
  - over-reach 6/6 (only the patient's own appointment moves);
  - new-request-midway 1/6.
- **O2 supported:** the LLM stacks handle these well.
- **O3 supported:** nobody claimed to have cancelled an appointment. The
  3 false claims are judge nitpicks ("hoje é sexta-feira" is true in the
  scenario) and one Tessera transcript (t18) whose reason does not match
  the transcript. None is a false action.

**The worst finding: a side question at a confirmation becomes a
cancellation.** "Antes de confirmar, vocês aceitam Odontoprev?" is read
as a no, so Tessera replies "Tudo bem, não mudei nada". From then on the
router answers every repetition with the same sentence, and the patient
asks four times.

This comes from the confirmation logic plus the lack of any answer to
undeclared questions. RFC-0004 Part B's interruption handling targets
exactly this. A new request midway ("marcar uma limpeza?") restarts the
reschedule instead of saying that cleanings cannot be booked here.

## 3. Authoring by one prompt (A1–A3)

| | Result |
| --- | --- |
| Wall time | 5.7 min (340 s) |
| Author tokens / tool calls | 141k / 35 |
| `validate` / `test` runs | 6 / 4 |
| Live eval spend | $0.002 |
| Human interventions | **0** (8 questions decided alone, listed in the author's report) |
| Result | 4 resources, 0 errors, 0 warnings; 9 EvalSuite scenarios passing |

- **A1 supported.**
- **A2 not supported:** the authored agent scored 98% in C0 on the
  held-out tasks, against 100% for the hand-written one. The premise was
  wrong: the lessons were already in the authoring checklist, written
  after market-003, and the author used them. It asked the day first,
  confirmed with a question naming both appointments, and made one
  idempotent call.
- **A3 supported:** 0 duplicates in C1 and C3.

The author's difficulties, which are the fixes Part C needs:
1. What `onError` does was only learnable from the runtime code.
2. The same for how a choice question reads the first message.
3. The `Confirming` status is not in the guides; it caused 5 failed
   expectations.
4. The testing guide points to an example that was not there, and plugin
   types are not explained.

**Limits:**
- The author is the same model family as this session, starting from a
  clean context.
- Its task is the one the checklist was written from, so it is an easy
  case for the docs.
- A new domain would test authoring for real.

## What this changes in RFC-0004

1. **Part A (voice) and Part B (free mode and interruptions) are clearly
   needed and come first.** The gaps are large (0 naturalness wins, 22%
   off-script). Part B must first fix the "question at the confirmation
   becomes a cancel" behaviour.
2. **Two defects the judge found go into A/B:** narrowing by period as
   well as day, and not repeating a question after a goodbye.
3. **Part C shrinks to its concrete gaps:**
   - document `onError`, choice questions and statuses;
   - explain plugin types;
   - fix the testing guide.

   Then re-measure on a new domain. The rest of Part C (plugin import,
   goal-based simulated users) stays, measured against a harder task.

## Deviations

1. **The naturalness judge runs with reasoning off, 800 output tokens.**
   With reasoning on and 400 tokens it spent the whole budget thinking and
   returned nothing: two crashes, at pair 11, costing $0.18. The 10 pairs
   judged with reasoning on were judged again, so all 300 share one
   setting. The off-script judge started after the fix.
2. **The judges write each verdict as it arrives and resume**
   (a harness fix).
