# EvalSuites on both Decide backends (001)

- **Date:** 2026-09-23
- **Question:** RFC-0001's phase 1 exit criterion is "the traveler passes
  its EvalSuite on both Decide backends". Does it, and what differs?
- **Setup:** `tessera test`, live, Generate = DeepSeek V4.1 Flash (routing
  and extraction for the traveler). Decide = Jev jev-1.13.0, or the
  `llm-constrained` backend with DeepSeek V4.1 Flash (first-token logprobs).
  Plugins are the suites' sandboxes.

| Suite | Jev | LLM (DeepSeek V4.1 Flash) |
| --- | --- | --- |
| traveler / flights (4 scenarios) | 4/4, $0.0004 | 4/4, $0.0005 |
| clinica / reschedule (3 scenarios) | 3/3, $0.0002 | 2/3, $0.0004 |

The one failure: "quero remarcar a consulta de segunda" with the options
"Seg 21/09 às 9h" and "Ter 22/09 às 14h". Jev resolved it to Monday's
appointment; the LLM backend answered below the threshold, so the agent
asked, and the scenario's script did not expect a question. The LLM
backend was also slower where it had to ask twice (11.5 s against 1.3 s
for the same scenario with Jev).

## Findings

1. **The exit criterion holds for the traveler** on both backends.
2. **The backends differ where it matters for routing by confidence**:
   matching a weekday to a dated option is the kind of decision earlier
   Decide benchmarks (not published) showed Jev doing better. The LLM backend failed safe: it asked instead of guessing.
3. **Running both backends found an authoring bug** in the example: the
   message for an unresolved appointment said "no appointments found"
   even when the patient simply had not picked one. `onUnresolved` can now
   read the input's sources, and the example tells the two cases apart
   (scenario `unclear-choice`).

## Next

Suites are small; the numbers say "works on these conversations", not
"works". Growing the traveler suite from `TRAVELER_TESTS.md` (round trips,
children, loyalty, follow-up questions) comes with the remaining traveler
Functions.
