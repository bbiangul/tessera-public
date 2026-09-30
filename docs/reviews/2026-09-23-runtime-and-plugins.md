# Adversarial review: runtime, plugins, host, evals (2026-09-23)

Two independent reviewers were asked to break the code written that day:
the runtime (RFC-0003) and journal, and the http plugin, admission, host,
Starlark engine, Generate engine and EvalSuites. They reported 30
findings; 22 were reproduced with tests, the rest reasoned from the code.
Every finding below is fixed, and each fix has a regression test that
fails on the code before it (checked by running the new tests against the
previous commit).

| # | Finding | Severity | Fix |
| --- | --- | --- | --- |
| R1 | After a fallback act failed, later verify/retry acted on the original action: a booking could happen twice | High | Later strategies act on the fallback's call |
| R2 | Any answer to an escalated action counted as success ("Pronto!" when nothing happened) | High | The answer must say whether it happened; unclear answers are asked again |
| R3 | A finished run's unacked messages were lost | High | `Resume` returns them; the host delivers recovered runs' messages (`Reply.Earlier`) |
| R4 | A store failure after a reply was recorded blocked the conversation | High | The same reply delivered again finishes the execution |
| R5 | Unknown outcome + `onError: [fail]` ended without a person | High | An unknown outcome always reaches Indeterminate |
| R6 | Native verification ran only with an explicit `probe` item | Medium | Verification by the declared mode runs first, always |
| R7 | Plain-text human answers were read as "no" | Medium | Clear yes/no text is understood; anything else is asked again |
| R8 | Input order changed the path but not the generation | Medium | Order is hashed into the generation |
| R9 | Typed plugin outputs (`[]string`) wedged runs | Medium | Converted through JSON; unrecordable outputs become a definite error |
| R10 | `fail()` text (personal data) reached run errors | Medium | Only codes survive (`NO_PLACE`) |
| R11 | Two first messages started two runs | Medium | The host serializes each conversation |
| R12 | A recovered run's question could be "answered" by an earlier message | Medium | The question is sent; the message is not used as its answer |
| R13 | Changed limits changed a running run's path | Medium | Each run keeps the limits it started with |
| R14 | Values from `with`, fallbacks and escalations were not type-checked | Low | Checked; a wrong escalation answer is asked again |
| R15 | Intents did not record their arguments | Low | Recorded |
| R16 | YAML 1.1 conversion renamed an input `n` to `false` | Medium | The loader converts YAML 1.2 nodes itself |
| P1 | A processed POST could be reported as "did not happen" (303, non-JSON 200, oversized body, cancellation) | High | Ambiguous for anything but reads |
| P2 | A manifest header could replace the per-call Idempotency-Key | High | Canonical names, platform headers last; admission refuses shadowing |
| P3 | Path traversal through `..` in inputs | High | `.`, `..` and slashes are refused in path values |
| P4 | Egress to internal addresses (metadata service) | Medium | Refused at admission (IP literals) and after DNS at runtime, unless allowed explicitly |
| P5 | Literal-secret check bypasses (constants, lists, body, prefix, header names) | Medium | Recursive, constant-aware, pattern-based |
| P6 | Manifest expressions could read scopes the runtime does not give them | Medium | Scopes per context; read actions must use GET |
| P7 | A `fromFunction` source could run a Function with effects | Medium | Refused at admission, transitively |
| P8/P9 | Host concurrency and recovery | Medium | See R11/R12 |
| P10 | Starlark memory unbounded | Medium | **Yes** (2026-09-25, #6): `serve` runs script calls in a child process bounded by `RLIMIT_DATA` on Linux; one huge allocation kills only the child (starmem-001) |
| P11 | Starlark dict order came from Go's random map order | Medium | Sorted insertion |
| P12 | The confirmation invariant accepted any human answer | Medium | Tied to the effect's own confirmation or an explicit approval |
| P13 | Cassette replay repeated old answers | Medium | Asking more than recorded is a miss; replay restarts per scenario |
| P14 | JSON Schema `$ref` loaded local files | Low | External references refused |
| P15 | Failed model calls were not charged | Low | **Partly**: charged at least their estimate; the HTTP client still retries model POSTs on network errors, and each attempt may be billed |

Open items from this review are tracked in the code comments where they
live (`internal/engines/compute/starlark.go`, `internal/httpx`).
