# Replay 001: isolate EvalSuite recordings (#32)

Base 7a16d69, 2026-09-24. No model prompt or production conversation change.
Reproduce identical requests returning different responses in two scenarios.
Scope new EvalSuite recordings by namespace/suite/scenario, with scope in
both stored metadata and key. Preserve nth-call ordering within a scenario.
Legacy wholly unscoped targets remain replayable with their existing reset
semantics; never fall back to an unscoped response when a target contains
scoped recordings. Benchmarks outside EvalSuites keep unscoped behavior.

Gates: deterministic tests demonstrate baseline failure and corrected
independence under reverse order, selection, repeated calls and partial
re-recording; missing scope/request/extra occurrence must miss. Verify both
Decide and Generate wiring, namespace isolation and old-cassette support.
Run clinic-context and clinic-confirmation live into one file per suite,
then replay complete suites and every scenario alone; all ten must match
recorded transcripts. Budget $0.10 per suite, total $0.20. Retain existing
per-scenario files as historical evidence; switch CI to the combined files.
Run make ci and make lint-extra. No new market benchmark required because
this changes only test recording/replay, not model requests or runtime.
