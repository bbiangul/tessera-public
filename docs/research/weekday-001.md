# Weekday 001: independent Jev decisions in one request

Base `03b79b9`; preregistration: `weekday-001-preregistration.md`. The owner
prioritizes Jev and batching for speed. This round separates two questions:
can one request reliably classify weekday preferences, and can a partner use
that capability to avoid a pointless date question in clinic conversations?

## Decision experiment

Twelve frozen selection messages, twice each, identical questions in serial
and batch; twelve different frozen replication messages, twice each. Jev
`jev-1.13.0`, four concurrent sample conversations, full-mask wall latency.
These are 24 observations per arm, **12 unique messages**, not 24 independent
holdout messages. The replication set was not used to choose prompts.

| Arm | Accepted correct masks | Confident forbidden days | Decide calls | p50 | p95 | Provider cost USD |
| --- | --- | --- | --- | --- | --- | --- |
| Seven questions, serial | 22/24 | 0 | 168 | 1857 ms | 2267 ms | .003137904 |
| Seven questions, batch | 22/24 | 0 | 24 | 272 ms | 790 ms | .001486800 |
| Eight questions, serial | 24/24 | 0 | 192 | 2220.5 ms | 4229 ms | .003572352 |
| Eight questions, batch | 24/24 | 0 | 24 | 294.5 ms | 789 ms | .001646064 |
| Eight questions, batch replication | 24/24 | 0 | 24 | 259 ms | 864 ms | .001647744 |

Seven questions failed the preregistered 95% acceptance gate: a numeric date
with no named weekday produced the correct mask but insufficient confidence.
No threshold was lowered. The registered amendment adds an independent
question: does the user explicitly restrict weekdays for the **new** slot?
A confident no means no weekday restriction; an uncertain answer stops. A
confident yes requires all seven weekday answers to meet .7. The eighth
question travels in the same request. Greetings, an old appointment's day
and a FAQ question are not new scheduling restrictions.

The amended batch passes selection and replication. Compared with the
matching eight-question serial arm, it uses one eighth as many requests,
with 86.7% lower median full-mask latency and 53.9% lower billed cost on
this selection workload. This does not establish end-to-end conversation
speed or general model accuracy. Runs were separate arms, not interleaved;
provider load may affect timing. Counts are application-level Decide calls;
transport retry attempts were not separately instrumented. All calls used a metered ledger. The
prototype retained costs and answers but omitted token counts; the promised
token breakdown cannot be recovered from these artifacts.

Initial scoring read `Answer.Choice` for a Noul answer, where Choice is
empty. Original rows are preserved; `summarize.py` recalculates from recorded
Noul probabilities without new calls. It also rejects errors/incomplete
answers. The initial seven-question harness is retained as text. The final
runnable harness is `bench/weekdaybench`; no live calls occur in unit tests.

## Shipped runtime contract

`decideBatch` declares named independent questions and one shared state.
State expressions are resolved and masked once; Jev receives every question
in one request. The durable journal stores all answers at one site, so
resuming after a wait does not call the model again. Supported question
kinds are the same as `decide`: choice, yes/no and ordered score.

Each answer carries value, winning probability, distribution and model.
With `continue`, uncertain values become null and the aggregate value is
null if any answer is uncertain. Confident individual answers remain usable,
which permits the explicitness gate above. Provider errors, missing answers,
wrong types or malformed probabilities invoke onError even with continue.
The existing single-question contract is unchanged. There is no automatic
batching of dependent steps and no built-in 31-day search policy.

The public weekday helper returns known/restricted/allowed. Tests verify
that uncertainty cannot become an unrestricted calendar. Unit tests cover
one call, state masking, durable resume, confidence failure/continuation,
provider failure, missing/malformed answers and admission rejection of
self-dependencies or mixed question kinds.

Four public demonstration conversations pass live and replay, using two
Decide calls per conversation (router plus eight-question batch), no Generate
calls and deterministic timezone-aware filtering. Recorded live cost is
$0.000334 rounded. Their cassette is part of `make test-evals`. These four
cases are integration checks, not another held-out accuracy estimate.

## Reproduction

Run `python3 bench/results/weekday-001/summarize.py` for offline scores.
With a Jev key in TYPESAFE_API_KEY, run the harness with `-dataset`, `-out`,
`-mode batch|serial`, `-repeats 2`, `-max-cost .2`. Use a new output path;
never overwrite frozen observations. The original seven-question results
require the retained original harness; the current one asks eight questions.

Replay the public runtime example:

```
./bin/tessera test examples/decide-batch --cassette bench/cassettes/decide-batch-weekdays.jsonl --cassette-mode replay
```

`make ci` passes (30 tested packages, 86 replayed conversation scenarios)
and `make lint-extra` passes. Initial CI exposed an unrelated wall-clock
assumption in `TestFireAndDevClock`: crossing the daily cron time creates a
scheduled run beside the manually fired one. The test now identifies its
manual run by ID instead of requiring exactly one result. No scheduler
behavior changed; the original failing CI log is preserved.

## Fresh external maintenance partner

A new zero-history partner received only the public binary/docs/skill, generic
examples (including the measured helper) and client-owned declarative clinic
definitions. No platform source, prior conversation, internal benchmark cases
or reference solution was supplied. The work was maintenance, not blind
new-agent authoring. The parent requested inherited regressions but supplied
no functional repair or evaluator feedback. All 59 supplied files remained
unchanged; a prescribed shell sandbox denies source reads (verified). This
is a shared-host protocol, not whole-agent VM isolation.

The partner compared direct disclosed 31-day search (A) with asking for a
calendar range first (B), freezing nine development cases before live calls.
Both first versions passed 6/9: the partner had made the explicitness gate
stricter than the supplied helper, and an exclusion produced uncertainty for
Sunday. The partner restored the helper's gate and added instructions not
to infer weekday availability from company opening hours. The .7 floor stayed.
Those changed prompts are **not** the prompts of the 24/24 classifier
replication above; their evidence is the partner's scoped conversation tests.

Both revised policies pass 9/9. A needs one user message versus B's two to
present real Saturdays from an initial request, or two versus three when
Saturday comes as a follow-up. Thus A wins the preregistered turn-count tie
breaker with the same functional/safety outcomes. A later additional
month-boundary case also passes both policies. Final B passes 10/10; chosen
A passes **37/37 live and replay**, including all **27 inherited cases with
unchanged assertions**. Live A consists of a 36-case recording plus one
additional boundary recording. The initial incomplete-cassette replay miss
is preserved; final replay matches all transcripts and plugin calls.

The partner strengthened assertions to require reaching confirmation before
withdrawal/correction and exclude forbidden slots. Fixtures use a corrected
Gregorian ordinal rather than adding to day-of-month; checked on 1461 dates
including a leap year. The explicit-month fixture was narrowed to exactly
two Saturdays. Retained initial failures, full attempts, costs and comparison
are in `partner/REPORT.md`; this is development selection, not a new held-out
conversation-parity estimate.

Authorization, action/confirmation/idempotency, same-dentist preconditions,
plugin verification and error handling are identical to the supplied client.
No real clinic writes occur in these tests. The selected definitions ship as
`examples/clinic-weekdays` with their recorded cassette in CI. The 31-day
policy remains declared in this example; it is not a new runtime default.
The original partner delivery has four UNUSED_FUNCTION warnings: its
`ordinal` helper is public and therefore listed as an action handler. This
is a missing public-doc convention, not a validator false positive. Integration
CI requires warning-free examples, so the parent renamed only that helper
and its call sites to `_ordinal` in the shipped example, documented the
convention in the public testing guide, and reran the 37-case cassette.
Calendar behavior, agent definitions and original evidence stay unchanged.
The initial integration-CI warning failure is retained. Final example
validation has zero errors and zero warnings.

Raw replies are captured with public `wait.orReply` and passed explicitly
to the helper. This is bounded by a 24-hour wait; arbitrary side questions
at that point are not established by the new comparison. The suspected
WaitingForTime/WaitingForUser regression did not occur in the externally
observed inherited assertions. Generate remains the smallest supplied model
for extraction/text; this change does not migrate every existing finite
classification elsewhere in the inherited clinic to Jev.

## Cost, backlog and remaining scope

Recorded test-provider spend is **$0.048074244336 of $3**, including all
partner live attempts ($0.036249816336). Coding-agent inference is excluded.
Replay adds no provider charges. See `cost.json` and the partner's attempt
table. The prototype lacks token totals as noted above.

Batch primitive: #39. Conversation maintenance advances #36; #36 and parent
#24 remain open. Market seed 43 and off-script seed 41 remain unused.
Further work should measure side questions/corrections during preference
capture and then use fresh authors/tasks for any broad promotion claim.
Historical authoring-003/004 and availability-001 artifacts remain unchanged.
