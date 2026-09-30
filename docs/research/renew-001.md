# renew-001: lessons renewed in the team's channel

- **Date:** 2026-09-28.
- **Pre-registration:** `renew-001-preregistration.md` (commit 0812cc7,
  before the code).
- **Data:** `bench/results/renew-001/live.txt`.
- **Spend:** under $0.01.

## What was built

**Due lessons get a renewal task.** A lesson of a learning base is due
when its review date is within 14 days or past. It gets one `approve`
task per review date from a generated Function, `renew-<base>`:
- "Esta lição continua valendo?";
- the question, the answer, the review date;
- "A equipe respondeu o mesmo N vez(es)", when `learn` found the answer
  repeated.

**Approved:** `lessons/renew` sets the review to now plus the kind's
validity (price 30 days, policy 180, fact 365), the status to active, and
records `renewedBy`. The index is rebuilt.

**Denied:** nothing changes. An expired lesson stays unused, and the task
is not sent again for that date.

**Tests:** 6 mutations caught, one of them after adding a suspended
lesson to the test.

## Result

| # | Result |
| --- | --- |
| W1 | **Supported.** Expired and due-in-10-days lessons got one task each. A lesson due in 60 days and a suspended one got none. A later tick added none. The lesson the team answered again carried "respondeu o mesmo 2 vez(es)". |
| W2 | **Supported.** Renewed to exactly now + 365 days (fact) and now + 30 days (price), with `renewedBy=rita`. The expired café lesson, unused before, was given to the free mode after renewal. |
| W3 | **Supported.** Gil (no role) got `NotAllowed`. A denied renewal left the lesson unchanged and was not asked again. |
| W4 | **Partly supported (live).** On learn-002's journal, with learn-003's four lessons due yesterday: 4 renewal tasks, and after renewal the four held-out questions were answered correctly (4 of 4). Only 2 of the 4 tasks carried "respondeu o mesmo", against ≥ 3 registered: `learn` did not match the nota fiscal and animals answers to their lessons in this run. The test does not record why. |
| W5 | **Supported:** `make ci`, `make lint-extra`. |

## What this means

**Renewal is time-driven and works.** Every due lesson is put to a
person, and approval restores it the same minute.

**The repeated-answer line is best effort.** It depends on `learn`
recognising the team's answers as the same lesson, which here happened
2 times out of 4. A task without it is still correct; it only shows less
evidence.

**Also slow:** the live tick took about 6 minutes. `learn` asks Jev
several questions per answered case and per pair of cases, and nothing
is cached between ticks. That is fine weekly, but it would matter on a
large journal.
