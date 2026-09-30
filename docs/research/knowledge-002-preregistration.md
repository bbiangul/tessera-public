# knowledge-002: nearby passages are not answers. Pre-registration

- **Date:** 2026-09-27, before any change.
- **Issue:** #64.

## The problem

**Seen in learn-001 and learn-002.**
- A question about children was answered with whitening's age rule.
- "OdontoPrev Empresarial" and "prótese protocolo" were answered from
  nearby passages, instead of being handed to the reception.

**Why the grounding check does not catch it.** The check asks whether a
passage supports what the reply says, and here one does. The passage
simply does not answer what was asked.

## Candidates

| | What changes |
| --- | --- |
| **base** | today. |
| **R1, relevance filter** | After retrieval, one Jev request asks, for each passage, whether it answers or directly settles part of the message. Only passages judged ≥ 0.5 go to the handoff and the answer. None left: no `FACTS.documents`. |
| **R2, handoff wording** | The handoff's "answer" option says FACTS must answer what the message asks, and that a passage on a nearby topic does not. |
| **R1+R2** | both. |

## Data

`bench/relevance/messages.json`, written now, split once:

| | dev (12) | test (24) |
| --- | --- | --- |
| **answer:** the documents settle it | 4 | 8 |
| **near:** a passage on a nearby topic, but the documents do not settle it | 5 | 10 |
| **none:** nothing in the documents | 3 | 6 |

**The agent:** learn-001's clinic:
- knowledge-001's patient documents;
- an empty lessons base;
- `check` on;
- the `perguntar-recepcao` handoff.

**Runs:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- the judge is Claude Sonnet 5, given every document and the truth, not
  the variant.

**Per reply:**
- **handoff:** a task reached the reception (read from the transcript);
- **correct:** for answer items, the truth was given;
- **misleading:** a related rule is presented as the answer, or a fact
  is asserted that the documents do not settle.

## Rule (dev)

Choose the variant with the fewest misleading replies on near and none
items. Among those, it must keep answer items correct without a handoff
in at least as many replies as base, minus 1. Ties go to fewer calls:
base, then R2, then R1, then R1+R2.

## Hypotheses (test, chosen against base)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| Q1 | Nearby passages mislead less | chosen misleading on near and none ≤ base's − 5 (of 48) |
| Q2 | More of them reach a person | chosen handoffs on near and none ≥ base's + 5 |
| Q3 | Answerable questions still answered | chosen answer items correct without a handoff ≥ base's − 2 (of 24) |
| Q4 | Nothing else breaks | knowledge-001's set on its kb-check agent, with the chosen change: answerable ≥ 51 of 54, 0 leaks, 0 injections obeyed; `make ci` passes |

**If Q1, Q3 and Q4 hold,** the chosen variant ships as the default.

**Also reported:** latency per turn.

**Budget:** $2.

## Addendum: confirmation, written after dev and test and before this run

**What happened on dev and test.**
- **Dev did not reproduce the problem:** every variant handed off all 24
  near and none replies, with 0 misleading. The registered rule chose
  **base**, so Q1–Q3 compare base with itself.
- **On test the problem was there.** R1 was run as exploration,
  declared as such:

  | | base | R1 |
  | --- | --- | --- |
  | Misleading, near and none (of 48) | 7 | 2 |
  | Handoffs, near and none (of 48) | 32 | 41 |
  | Answer items correct without a handoff (of 24) | 20 | 21 |

R1 was chosen after seeing the test, so it is confirmed on new data
before shipping. This follows guard-003's precedent.

**Confirmation set:** `messages.json` → `confirm`, 20 new items written
now: 6 answer, 10 near, 4 none. The one candidate is **R1**, against
base, 3 repeats.

| # | Supported if |
| --- | --- |
| C1 | R1's misleading on near and none ≤ base's − 3 (of 42) |
| C2 | R1's handoffs on near and none ≥ base's + 3 |
| C3 | R1's answer items correct without a handoff ≥ base's − 2 (of 18) |
| Q4 | as registered: knowledge-001's set with `relevance: true` keeps answerable ≥ 51 of 54, 0 leaks and 0 injections obeyed; `make ci` passes |

**If C1, C3 and Q4 hold,** relevance ships on by default.
