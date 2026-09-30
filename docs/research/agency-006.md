# agency-006: agent mode's open points. H is not adopted as better

- **Date:** 2026-09-29.
- **Pre-registration:** `agency-006-preregistration.md` (64af2f7).
- **Data:** `bench/results/agency-006/`.
- **Variants:**
  - **G:** agency-005's shipped agent mode, `tessera-G6`: d16dc62 plus
    only the stall-line patch;
  - **H:** e0913e3, `tessera-H2`, after both dev rounds.
- **Spend:** about $3.5.

## Result (sealed test: 16 new personas × 3)

| | G | H |
| --- | --- | --- |
| Goal met (of 48) | 38 (79%) | 36 (75%) |
| Conversations with a false statement | 8 | 7 |
| Information lost | 20 | 26 |
| Redundant messages | 37 | 29 |
| Natural (1–5) | 3.46 | 3.69 |
| Trust (of 48) | 30 | 32 |
| Ended at a question the team was not scripted to answer | 10 | 10 |
| Ended at a booking the team was not scripted to complete | 2 | 0 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0064 | $0.0065 |

| # | Hypothesis | Result |
| --- | --- | --- |
| R1 | H lost ≤ 70% of G's | **Not supported:** 26 against 20 |
| R2 | H questions left with the team ≤ 60% of G's | **Not supported:** 10 and 10 |
| R3 | H goals ≥ G's | **Not supported:** 36 and 38 |
| R4 | H's conversations with a false statement ≤ G's | **Supported:** 7 and 8 |
| R5 | 0 invariant failures; median cost ≤ $0.02 | **Supported** |
| R6 | CI, lint, suites | **Supported:** 38/38 live and replayed |

**Decision (the pre-registered rule): H is not adopted as better than
G.** The differences in goals (2 of 48) and loss are within what one
persona's three runs move (p05 alone: 10 lost).

**What was kept, and why.** Each fix of H corrects a reproduced failure
and has a test that fails without it:
- **The booking asked for the user's details when one person travels,**
  so a grandfather's name went on his grandson's booking (n05).
- **A car rental page was quoted** to "is travel insurance required?"
  (n09).
- **"Meus dados…, os da minha esposa amanhã" lost the given details**
  (agency-005 m03).
- **A 15-year-old flying alone was quoted with a parent** (m13).
- **A booking's status went out four times** (m14).

They stay in the traveler, which is now H. This is a deviation from the
rule's letter, which spoke of replacing the default: G has the booking
and quote bugs above. **By this measurement, H is not better overall.**

**Per persona (goals of 3, G → H):**
- **H gained on:** p07 (compare two weeks: 0 → 3), p06, and p03 and p04
  (lost 3 → 1, 6 → 2).
- **H lost on:**
  - p05 (a 14-year-old flying alone: 2 → 0). H counts him right, then
    the customer asks the team whether the airlines take a 14-year-old
    alone, and the scripted team has no answer;
  - p11 and p09, where a booking waiting for details keeps asking for
    them;
  - p13 (does a baby pay at the hotel: 2 → 0, a question for the team).

## What the study says about the instrument

- **One conversation in five ends because nobody answers the team.**
  Here that was 10 of 48 in each variant. The customer asks something
  only the agency knows, such as an airline's rule for minors or a
  hotel's baby policy. The agent rightly asks the team, the scripted
  team has no answer, and the conversation ends.
- A real agency's team answers. **Until the simulated team can, agent
  mode's comparisons are mostly about who reaches the team first.**
- **Next, the instrument:** a simulated team member that answers from a
  written sheet of the world's facts (airline rules, hotels' policies,
  fees), and says it does not know otherwise. It is measured first for
  answering only from the sheet.

## What the study says about the agent

- **A booking waiting for details keeps asking** while the customer asks
  about something else (p09, p11). With a run pending, side questions
  still take the router-era path. The planner for the pending branch
  (RFC-0010) is not built. It is the next agent change.
