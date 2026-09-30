# lifecycle-001: lessons that expire, die, contradict and harm (RFC-0008 part C)

- **Date:** 2026-09-27.
- **Pre-registration:** `lifecycle-001-preregistration.md` (commit
  45530e9, before the code).
- **Code:** 5e09398, plus one fix found by this measure (below).
- **Data:** `bench/results/lifecycle-001/`.
- **Spend:** about $0.03 (agents, Jev). No judge: every check is
  deterministic.

## What was built

| Piece | What it does |
| --- | --- |
| Lesson metadata | A lesson's HTML comment is its passages' metadata, never their text. |
| The index | Expired (past the review date), suspended and retired lessons are not retrieved. |
| The journal | Each free turn records the id of every lesson it was given. |
| `tessera learn review` | Reports due, expired, dead (90 days unused) and harmful lessons. Harmful means disputed by the user right after being used, at least twice and in at least 30% of its uses; Jev reads the next message. `--apply` suspends the harmful ones. |
| `renew`, `retire` | Update the lessons file: a new review date, or retired with a reason. |
| `accept --supersede` | Retires the lesson a new one replaces. |
| `learn --lessons` | Reports a proposal that contradicts or reconfirms an active lesson, instead of proposing it. |
| Tests | 9 mutations caught, 2 after strengthening the tests. |

## Result

| # | Result |
| --- | --- |
| X1 | **Supported.** The expired Invisalign price (R$ 9.800): today's binary said it in 9 of 9 replies; the new one in 0 of 9, handing every one to the reception, whose answer can teach it again. |
| X2 | **Supported.** The valid sedation price was said without a handoff in 9 of 9. |
| X3 | **Supported, in 3 of 3 review runs.** The wrong children lesson was harmful (used 4 times, disputed 2, 3 and 2 times) and suspended by `--apply`. The sedation lesson was used twice, disputed 0 times, and never suspended. |
| X4 | **Supported.** After the suspension, "atendem criança de 1 ano?" was handed off in 3 of 3, never "1 ano". Sedation was still answered in 3 of 3. |
| X5 | **Supported.** The bebedouro lesson was reported dead in every run; sedation and children never. The expired Invisalign lesson was also dead: unused because expired. |
| X6 | **Partly supported.** The R$ 11.500 answers were reported as contradicting the R$ 9.800 lesson in 3 of 3 runs, and not proposed. "Reconfirms" could not be tested: the valid sedation lesson answered its questions itself, so the reception was never asked and nothing reconfirmed it. It is covered by a unit test only. |
| X7 | **Supported.** `make ci`, `make lint-extra`, and a unit test showing the index never returns metadata. |

**The supersede flow, run end to end, found a bug, now fixed.**
- **The bug:** the new Invisalign lesson had the same id as the one it
  replaced (the same question). `accept` took it as already present,
  wrote nothing, and still retired the old lesson. That would have left
  no active lesson.
- **The fix:**
  - the new lesson gets its own id (the date appended);
  - nothing is retired unless something was written;
  - both cases are unit-tested.
- **Re-run:** the old lesson was retired ("superseded by …-20260927"),
  and the new one is active with its review date.

## What this means

1. **The lifecycle works on every path measured:**
   - old prices stop being said;
   - wrong lessons are suspended by what users say;
   - unused lessons are reported;
   - changed answers are held for a person.

   Nothing stays in the agent's mouth only because it was once learned.
2. **The agent defended a wrong lesson against the user.** When a
   patient said "me disseram que é a partir de 3 anos", the reply was
   "pela nossa política, atendemos a partir de 1 ano". A learned lesson
   carries less authority than the business's documents. When a user
   disputes it, the agent should pass it to a person rather than insist.
   Opened as an issue.
3. **Harm is detected after the fact.** Here it took 3 disputed
   conversations, and in production that means real patients misled
   first. A dispute could also hand off at once and report the lesson;
   that is the same issue.

## Limits

- One clinic, synthetic lessons.
- Dispute detection was measured on clear disputes only.
- The dead threshold (90 days) and the harm threshold (2 disputes, 30%)
  are the RFC's defaults, not tuned.
