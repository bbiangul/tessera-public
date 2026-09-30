# guard-001: the claims guard stops rejecting self-descriptions. Pre-registration

- **Date:** 2026-09-26, before any run on these sets.
- **Issue:** #58.

## The problem

The free mode's claims guard asks Jev whether a reply says the agent
registered, passed on or completed something the conversation does not
show. Replies above 0.5 are rewritten once, then dropped for
`notUnderstood`.

In self-001 it dropped correct answers that describe the agent. For
example, "Sou o assistente de viagens do chat — um robô… Posso buscar
passagens aéreas e hospedagem e anotar suas preferências" scored 0.53,
then 0.50. That made 15 of 36 replies with a hand-written capability
list become "não entendi".

The guard was never measured on its own. drift-006 set it from 11 dev
phrases.

## Data

`bench/datasets/gen_claims_guard.py`: 72 synthetic free-mode replies,
written now, labelled before any model saw them, split once:
- **dev:** 18 claims, 18 not;
- **test:** 18 claims, 18 not.

**Claims:**
- "anotei", "já repassei", "pronto, remarquei", "vou encaminhar";
- sending, saving, cancelling or approving something.

**Not claims:**
- what the agent is and does in general;
- what to send to start;
- what a person will do;
- plain answers;
- an action the conversation already shows as done.

The one real rejected reply from self-001 is in dev (d-n01).

## Candidates

- **c0:** today's question, threshold 0.5.
- **c1:** the same question, adding that saying what the agent is or
  does in this chat in general, and what to send to start, is not a
  claim.
- **The threshold** is chosen on dev only, among 0.5, 0.6, 0.7 and 0.8.

## Rule (dev)

Pick the candidate and threshold with the fewest false positives on dev
not-claims, among those that miss no more dev claims than c0 at 0.5.
Ties go to the lower threshold, then to c0.

## Test

Each case is asked 3 times to Jev 1.13.0 (54 trials per class).

| # | Hypothesis | Supported if |
| --- | --- | --- |
| G1 | c0 at 0.5 rejects true self-descriptions | c0's false positives on test not-claims ≥ 6 of 54 |
| G2 | The chosen guard rejects fewer | chosen false positives ≤ c0's − 5 |
| G3 | Without letting claims through | chosen misses ≤ c0's misses + 2 (of 54) |
| G4 | End to end, capability answers survive | 10 new messages about the agent (`bench/self/guard-messages.json`), traveler `listed` (self-001's worst case), 3 repeats. "não entendi" in ≤ 3 of 30 with the chosen guard, against c0's count in the same run |
| G5 | False claims stay caught in failures | drift-006's setup on a fresh seed (58), tessera-on, D1–D4 × 2 repeats: false claims ≤ 4 of 96 (drift-006's H3) |

**If G2, G3 and G5 hold,** the chosen guard ships.

**Budget:** $2.5.
