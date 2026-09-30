# renew-001: lessons renewed in the team's channel. Pre-registration

- **Date:** 2026-09-28, before any of this is built.
- **RFC:** RFC-0008, part C: "Tempo: vai para revisão: a fila dona
  recebe 'isto ainda vale?'".
- **Follows:** learn-003, which only proposed new lessons and
  contradictions.

## What will be built

1. **A renew task for every due lesson.** In a learning base, a lesson
   is due when its review date is within 14 days or already past. It gets
   one `approve` task in the base's queue from a generated Function,
   `renew-<base>`:
   - "Esta lição continua valendo?";
   - the question, the answer, the review date;
   - when `learn` found the team giving the same answer again, "A equipe
     respondeu o mesmo N vez(es)".
2. **Approved:** the built-in `lessons/renew` sets the review date to now
   plus the kind's validity, sets `status=active`, and records
   `renewedBy`. The index is rebuilt, so an expired lesson is used again.
3. **Denied:** nothing changes. The lesson expires, or stays expired, and
   is not used. `learn review` still lists it, for retiring.
4. **Once per review date:** the run's ID is
   `renew/<ns>/<base>/<id>/<review>`. After a renewal, the new date
   brings a new task only when that date is due in turn.

## Measure

**Functional, deterministic** (host tests, fake engines, a Team):

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W1 | Due lessons, and only those, get one task | an expired lesson and one due in 10 days get a task each; one due in 60 days gets none; later ticks add none; a lesson the team answered again carries "respondeu o mesmo" |
| W2 | Approval renews | the review date moves to now plus the validity, with `renewedBy`; the expired lesson is given to the free mode again |
| W3 | Denial and roles hold | a denied renewal changes nothing and is not asked again for that date; a member without the role cannot approve |

**Live** (`TESSERA_LIVE=1`): learn-002's phase-1 journal, with a lessons
file holding learn-003's four approved lessons, their review date set to
yesterday.

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W4 | End to end | the first tick opens 4 renew tasks; ≥ 3 carry "respondeu o mesmo" (the journal repeats them); approving all four makes the held-out questions answered again (≥ 3 of 4) |

| # | Hypothesis | Supported if |
| --- | --- | --- |
| W5 | Nothing else changes | `make ci` and `make lint-extra` pass |

**Budget:** $0.5.
