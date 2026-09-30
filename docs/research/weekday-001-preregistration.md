# Weekday 001: Jev preference decisions in one batch

Base 03b79b9. User prioritizes Jev wherever possible and batching for speed.
Classify seven independent allowed-weekday booleans with Jev-1.13.0, then
filter timestamps deterministically in the business timezone. No LLM chooses
weekdays or slots. Compare identical questions in seven serial requests vs
one batch. Dates/ranges remain the existing Generate extraction; weekdays
are closed-set decisions. Never turn uncertainty into an unrestricted search.

Freeze selection/replication messages before calls. Selection 12 messages x2
per mode; replication 12 different messages x2 for the selected mode. Require
>=95% complete masks correct, zero confidently allowed forbidden weekdays;
confidence per boolean >=.7, otherwise stop/clarify. Batch wins only if these
accuracy gates pass and p50 full-mask latency beats serial. Report request
counts, billed tokens/cost, p50/p95; synthetic results, no blanket latency claim.

If batch passes, expose an explicit declarative decideBatch step with a shared
masked state and named independent questions, one durable recorded request.
No implicit grouping of dependent steps. Every answer must satisfy the batch
confidence floor before output values are usable; errors/missing answers never
silently fill defaults. Unit tests verify masking, one call/replay, low confidence
and provider failure. Keep the existing single decide contract unchanged.

Compare two public authoring policies on controlled fixtures: direct search
within a disclosed 31-day window vs asking for a date/range first. The window
is this synthetic agent's policy, not an engine default. Preserve explicit
ranges and weekday restrictions; empty results mention only the searched
window. Selection gates: answer available Saturdays without weekday leakage,
zero writes without confirmation, and fewer unnecessary user turns. Fresh
external diagnostic subagent gets only updated public CLI/docs and declarative
client definitions, no engine sources or inherited history. This is maintenance
validation, not a blind new-authoring/parity study. Market43/offscript41 stay
unused unless a separate broad gate is explicitly preregistered.

Budget $3 test-provider calls, including up to $1 reserved external author
calls. Freeze historical evidence. All live calls metered; replay free. Report
failed candidates/deviations, update #36 and handover, run CI/static checks.

Initial scorer incorrectly read Choice for Noul answers (whose Choice is
empty); corrected offline from recorded Noul probabilities, no new calls.
Raw rows preserved. Original serial and batch both 22/24 accepted masks,
zero unsafe allowances; numeric-date-only case abstains in both. Batch p50
272ms vs serial1857ms, but neither passes the 95% gate. No promotion.

Register candidate B before new calls: add one independent Jev Noul question
in the SAME batch asking whether a weekday restriction is explicit. If its
answer is confidently false, no weekday filter applies; otherwise require
all seven weekday answers confident. Uncertain meta-answer stops/clarifies.
This never infers a weekday from a numeric date or lowers the .7 threshold.
Repeat same dev selection, both serial and batch (now 8 questions), then use
the still untouched replication set if gates pass. Preserve first results.
