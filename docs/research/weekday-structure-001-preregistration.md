# Weekday structure 001 — classify operands, compute the set

2026-09-25, base a6933a9. Compare E (9 independent final-day questions/guard)
with G and H, which ask for a base set and per-day explicit roles, then compute
(base union inclusions) minus exclusions deterministically. Retain E's exact
ambiguity guard in both candidates. Jev 1.13.0, one batch, .7 threshold.
G: base + scope +7 role choices (include/exclude/none),9 questions.
H: base + scope +14 inclusion/exclusion booleans,16 questions.
No model calculates the resulting combination. Named ranges are classified as
positive per-day mentions; groups are represented by the base. Unknown operands
or conflicting include/exclude for a day must clarify, never default to all days.

Development reuses the consumed ambiguity-003 dev and replication sets, with
unique ids, plus four explicit group/individual-union requests (two wrappers).
That is60 distinct messages x2 repeats =120 trials/arm. This is development,
not fresh evaluation. Freeze a new20-message replication set before calls:
include clear groups/exclusions/corrections/date/context/union/ranges and vague
or contradictory requests. Two repeats =40 trials/arm.

Gate:100% clear exact masks and ambiguous clarification, zero provider errors,
unsafe accepted sets or unnecessary clarification. Select passing G/H by fewer
questions then median latency. Replicate selected and E; stop if neither passes.
No adaptive prompt repair/reruns. Record assembled masks and raw operands.
Integration/promotion requires a separately registered frozen conversation gate.

Budget .10 per arm/set, up to five runs, .50 total. Synthetic Jev only, metered;
no Generate/judge. Freeze executable/source/data/protocol hashes before calls.
Public helpers remain unchanged during measurement. Unit-test set composition,
correction-independent operand handling, missing/uncertain/conflicting operands.
