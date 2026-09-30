# Independent decisions in one request

`function.yaml` demonstrates two kinds of question sharing masked state.
`weekdays.function.json` is a reusable helper that returns:
- `known`: whether a safe weekday mask could be determined;
- `restricted`: true for an explicit restriction, false for a confident absence,
  null for uncertainty;
- `allowed`: Sunday=0 through Saturday=6, empty on uncertainty.

Pass the actual user text as `message`; it defaults to the message that started
the Function. Do not assume that `event.message` changes when a user replies.
Questions about a follow-up need that reply explicitly. `wait.orReply` exposes
its reply as output; an `ask` with choices returns the chosen object instead.

`filter.yaml` uses the mask to filter a fixed synthetic three-day calendar in
the business timezone. It does not extract calendar ranges, query a real clinic
or perform bookings. Uncertainty produces a clarification, not an unrestricted
search. A real agent must preserve explicit dates, use its own disclosed search
window, and confirm any irreversible action separately.

```
./bin/tessera validate testdata/decide-batch
./bin/tessera test testdata/decide-batch --cassette bench/cassettes/decide-batch-weekdays.jsonl --cassette-mode replay
```

The cassette contains four live Jev conversations. Each makes two requests:
one for routing and one with all eight independent weekday questions.
