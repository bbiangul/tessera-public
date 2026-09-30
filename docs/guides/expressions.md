# Calendar expressions

Expressions use CEL: `${ ... }` returns a typed value; `{{ ... }}` embeds
values in a text template. Schema fields of type Expr (such as `value`,
`choices` and `authorize`) use bare CEL without delimiters; check `explain`
for the field type. Use explicit IANA zones, e.g. America/Sao_Paulo.

| Function | Result |
| --- | --- |
| `date('2026-10-12', zone)` | Timestamp at the start of that local date |
| `formatDate(timestampValue, zone)` | String YYYY-MM-DD; second argument is a zone, not a format pattern |
| `startOfDay(timestampValue, zone)` | Timestamp at the first instant of that local day |
| `atTime(timestampValue, '10:30', zone)` | Timestamp on the same local day at that clock time |
| `addBusinessDays(timestampValue, 3, zone)` | Timestamp after three weekdays; weekends excluded, holidays not known |
| `isWeekend(timestampValue, zone)` | Boolean |

`addBusinessDays` accepts negative days and at most 3660 in either direction.
Convert an RFC3339 API string using CEL `timestamp(value)` before passing it
to a timestamp helper. For example,
`formatDate(timestamp('2026-10-12T15:00:00Z'), 'America/Sao_Paulo')`
returns `2026-10-12`. `date` accepts YYYY-MM-DD, not natural language.
Use extraction for natural-language dates; do not assume these helpers parse them.
