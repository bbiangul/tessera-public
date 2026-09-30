# Clinic weekday availability

This example accepts requests such as “qualquer sábado”, including a reply to
a request for preferences. It classifies weekdays through an eight-question
Jev batch, extracts calendar dates with Generate, and filters actual returned
slots deterministically in America/Sao_Paulo.

With weekday preferences but no explicit date range, this clinic's policy
searches today plus the next 30 days and tells the user both bounds. Explicit
ranges take precedence. Empty results describe only the searched interval.
The engine has no default 31-day window. Appointment ownership, same dentist,
confirmation, idempotency and verification of ambiguous effects remain enforced.

`search-request` collects up to three preference replies with 24-hour waits.
Each opts into `includeSideMessages`: answered side user messages survive
in the journal and are explicitly included in later extraction, in order.
A mixed preference+FAQ gets its factual answer before a later search
acknowledgment; corrections replace earlier preferences. The interruption
classifier asks three independent Jev questions in one request. Weekdays
still use their existing eight-question batch. Neither context nor a search
acknowledgment confirms an effect. Unknown preferences are asked again;
exhausting the three replies stops resolution explicitly.

```
./bin/tessera validate testdata/dias-uteis
./bin/tessera test testdata/dias-uteis --generate openrouter:deepseek/deepseek-v4.1-flash --cassette bench/cassettes/clinic-weekdays.jsonl --cassette-mode replay
```

53 scenarios cover the 37 inherited regressions plus 16 preference cases.
The final four include two explicitly documented oracle corrections (a
single available slot enters Confirming and displays a localized date).
Fixtures consistently allow Saturday hours; historical contradictory
fixtures and failed candidates remain under bench/results/preference-001.
The sandbox calendar helper is private (`_ordinal`), so validation has no
unused-action warnings. The preserved partner delivery used the public name
`ordinal`; this packaging-only rename does not change calendar behavior.
Tests use synthetic patients and a sandbox, never a real appointment system.
Set up the HTTP plugin and its environment secret for your own integration.

This was authored by a fresh partner agent with public CLI/docs/skills and
client-owned definitions, without platform source or prior conversation.
The direct-search policy saved one user reply versus asking for a range first
in the tested initial/follow-up Saturday conversations. It is a scoped
maintenance result, not a claim of broad conversation parity. Full attempts,
comparison policy and limitations: docs/research/weekday-001.md and
bench/results/weekday-001/partner/REPORT.md.

Preference capture measurements and limitations: docs/research/preference-001.md.
