# agency-005: agent mode's three gaps. Pre-registration

- **Date:** 2026-09-28, before the fixes are built.
- **Follows:** agency-004 (agent mode 58% of goals, the router 50%).

## Question

Do three fixes to agent mode bring it to the bar agency-004 set? The
three gaps are:
1. **The rest of a message:** when a process answered part of a message
   and the planner's add-on reply fails a check, the rest is dropped
   without a word.
2. **A public question goes to the team:** when its reply fails a check,
   the question goes to the team, though the web answers it.
3. **A pending process loops:** "I'll send it later" is taken as the
   answer and the same question comes again.

## Measure

- **Instrument:** as agency-004 after its deviations.
  - the llm simulator;
  - the world's sandboxes;
  - the blind Claude Sonnet 5 judge;
  - `report.py`;
  - scripted team answers given to the task of their shape.
- **World:**
  - seven new bookings, for the personas that ask about one;
  - three new web pages: minors without their parents, pets in the
    cabin, Fortaleza's climate.

  They are facts of the world, the same for every variant.
- **Data:** `bench/agency/personas-005.json`, written now.
  - **dev:** 12 new personas;
  - **test:** 16 new personas, sealed.
  - Every persona whose goal only a person meets has team answers.
  - At most two dev tuning rounds.
- **Variants:**
  - **D:** the router (`bench/agency/d4`, `tessera-D4b`), for reference;
  - **E:** agent mode as measured in agency-004 (`bench/agency/e4`,
    `tessera-E2`, 276de1d);
  - **F:** E with the three fixes.
- **Runs:** 3 repeats on test.

## Hypotheses (on test)

| # | Hypothesis | Supported if |
| --- | --- | --- |
| Q1 | More goals | F goal ≥ 70% and ≥ E + 8 points |
| Q2 | Less lost | F's information lost ≤ 70% of E's |
| Q3 | Not less true | F's conversations with a false statement ≤ E's |
| Q4 | Fewer needless handoffs | F's conversations ending at a team task the scenario did not script ≤ 60% of E's |
| Q5 | Safe and affordable | 0 invariant failures in F; median agent cost ≤ $0.02 |
| Q6 | Nothing else breaks | with agent mode off: `make ci`, `make lint-extra`, the traveler suites live as before |

**Decision rule:** agent mode becomes the traveler's default if all of
these hold:
- Q1, Q3 and Q5;
- F's goal is at least D's plus 10 points.

Otherwise it stays experimental, and the report says where it lost.

**Budget:** $8.

## Deviation (recorded before any F test conversation was run)

Dev round 2 let a turn run a Function again with other inputs. F-dev2 had
6 invariant failures (duplicate effects): the traveler's idempotency
keys assume one run per message, so a second quote in the same turn wrote
under the first's keys.

The change was reverted (a Function runs once a turn, and a test pins
it). A third dev run (`F-dev3`) checks the corrected build only. It
tunes nothing; its purpose is 0 invariant failures before the test.
