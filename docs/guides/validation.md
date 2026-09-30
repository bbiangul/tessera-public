# Validation and explain

`tessera validate` checks resources before anything runs, in two layers.

1. **Schema** (per document): unknown fields, missing fields, wrong types,
   values outside an enum. The schema is generated from `api/` and embedded
   in the binary.
2. **Semantics** (all documents together):
   - expressions and templates compile, with the right result type;
   - every `inputs.x`, `sources.x` and scope reference resolves; sources can
     only read earlier sources of the same input; a step can only read
     earlier steps, and the action, output and `onError` read all of them;
     inputs and preconditions run before any step and cannot read them;
   - inputs do not depend on each other in a cycle;
   - `plugin/action` exists in a `PluginManifest` among the files, and the
     arguments match its parameters;
   - sources only call `read` actions;
   - irreversible actions have an idempotency key (and a warning when they
     have no `confirm`);
   - `onError` never repeats or reroutes an irreversible action whose
     outcome is unknown without a `probe` first, and `probe` is only allowed
     when the plugin can verify.

Every issue has a stable `code`, a `path` like
`spec.inputs.newSlot.resolve[1].ask`, a message and, when possible, a fix.
`-o json` prints the same for coding agents. Exit code 1 means at least one
error; warnings do not fail.

| Code | Meaning |
| --- | --- |
| `READ_ERROR`, `TOO_LARGE` | The file cannot be read, or passes a limit: 1 MiB per file, 200 documents per file, 64 KiB per string |
| `PARSE_ERROR` | Not valid YAML/JSON, including a key repeated in one mapping |
| `NOT_A_RESOURCE` (warning) | A document with no `apiVersion` or `kind`; it is skipped |
| `UNKNOWN_KIND` | An `apiVersion`/`kind` we do not know |
| `UNKNOWN_FIELD`, `MISSING_FIELD`, `INVALID_FIELD` | Schema problems |
| `SCHEMA_ERROR`, `DECODE_ERROR` | Internal: the schema or the typed decoding failed on a document the schema accepted. Please report it |
| `DUPLICATE` | Two documents declare the same plugin or function |
| `INVALID_EXPR`, `INVALID_TEMPLATE` | An expression or template does not compile |
| `UNRESOLVED_REF`, `SCOPE_NOT_ALLOWED` | A reference points to nothing, or to a scope not available there (plugin `authorize` rules may only read the action's own parameters and `caller`, `agent`, `env`, `event`, `conversation`) |
| `UNKNOWN_TYPE` | An input or parameter type that is neither basic (`string`, `int`, `float`, `bool`, `timestamp`, `duration`, `map`, `any`), `list<T>`, nor a type declared by a plugin |
| `INPUT_CYCLE` | Inputs need each other to resolve |
| `RECURSION` | A function calls itself |
| `UNKNOWN_PLUGIN`, `UNKNOWN_ACTION`, `UNKNOWN_FUNCTION` | A call to something that does not exist |
| `UNKNOWN_PARAM`, `MISSING_PARAM` | Arguments do not match the action's parameters |
| `EFFECT_NOT_ALLOWED` | Resolving an input would change the world: a source or an input's fallback calls an action that is not a read, or a Function that can (followed through `call`, `fromFunction`, branches and fallbacks); or a read action uses a method other than GET (a query API that reads with a POST body declares `http.safe: true`); or a queue's `notify` names an action that is not `external-message` |
| `WINDOW_RISK` | Free text (a `respond`, `ask`, `freeText` action, output template, or a `call` into a Function that sends one) on a path where the channel's window may be closed: a Trigger or Funnel start, waits reaching the window, an `escalate`, a `call` that waits, the timeout path of `wait orReply`. Add an `onError` fallback act that sends a template, or send a template on that path |
| `CONFIRM_INCOMPLETE` (warning) | A `confirm` does not mention every input or step the action acts on (its `with` reads them): the user may confirm something else. Name each, e.g. which appointment and the new time |
| `NOTIFY_USER_CHANNEL` | A queue's `notify` uses the users' channel plugin (one with `spec.channel`): notifications would skip its consent and window rules. Declare the team's channel as its own plugin |
| `IDEMPOTENCY_REQUIRED` | An irreversible action, including a fallback `act`, without an idempotency key |
| `UNSAFE_RETRY`, `UNSAFE_FALLBACK`, `PROBE_UNSUPPORTED` | Error handling that could repeat an effect; after a fallback, later items are judged against the fallback's action |
| `SOURCE_KIND`, `STEP_KIND`, `ONE_STRATEGY`, `DUPLICATE_ID` | Structural rules: one kind per source, step, onError item and branch case; unique ids; a branch ends with `else` |
| `EGRESS_NOT_ALLOWED` | A plugin's connection points at a host missing from its `egress.hosts`, or at a private, loopback or link-local IP (at runtime, a name that resolves to one is refused too unless private networks are allowed explicitly) |
| `LITERAL_SECRET` | A credential written into a definition (a header or query parameter such as `Authorization`, `x-api-key`, `key`); use `connection.auth` with a secret name |
| `INVALID_SCHEMA` | The JSON Schema of an `extract` or `generate` step does not compile |
| `INVALID_SCRIPT` | A Starlark script is missing, too large (1 MiB), or does not compile; its function's parameters are checked against `with` (`UNKNOWN_PARAM`, `MISSING_PARAM`) |
| `UNUSED_FIELD` | A field that has no effect where it is (e.g. `with` without an action or function to receive it) |
| `NO_CONFIRMATION` (warning) | An irreversible action with no confirm question |
| `UNUSED_FUNCTION` (warning) | A sandbox function that matches no action of its plugin (a typo in the name) |

An `ask` must always state `minConfidence` and `onLowConfidence`: the schema
requires them (`MISSING_FIELD`), so no question runs without an explicit
routing decision. A `fallback` can `act` with arguments (`with`) and an
`idempotencyKey`, checked like any other call.

`tessera explain Kind[.path]` documents any field: type, whether it is an
expression or a template, allowed values, description and sub-fields. Map
keys and list positions can be skipped (`Function.spec.inputs.resolve`).

## Differences from RFC-0001 v0.2

- `fallback: { value: null }` became `fallback: { useNull: true }`: a YAML
  `null` cannot be told apart from a missing field.
- A branch's last case is written `- else: true` next to its target
  (`call`, `respond` or `escalate`), instead of `- else: { escalate: ... }`,
  so every case has the same shape.
- A Starlark `compute` step names its script file, relative to the file
  that declares the Function: `starlark: { script: score.star, function:
  score, with: {...} }`. The script is part of the run's generation.
