# Partner testing notes

Keep resource definitions in their own folder (for example authored/agencia). Write JSON test reports, logs
and cassette recordings in evidence/ outside the resource tree: recursive
resource loading treats JSON/YAML files as resources, not arbitrary reports.
Starlark sandbox files may live beside the EvalSuite that references them.

`expect.effects` counts successful effect calls, not attempts. For an action
that mutates sandbox state and then returns `ambiguous("timeout")`, a probe
can establish that the change happened without a successful write result.
That scenario can correctly have zero successful writes. Assert the final
behavior and invariants as well; a zero count alone does not prove safety.
`never` is for actions that must not be attempted at all. Use the CLI's
`explain EvalSuite.spec.scenarios.expect` for the exact schema.

Record the whole suite into one cassette, then replay it. New recordings
scope calls by namespace, suite and scenario, so identical prompts in
separate scenarios do not require separate cassette files.

For calendar helper signatures see expressions.md. Research, repository
unit tests and prebuilt clinic examples are not included in this package;
the CLI explain command is the field reference. Credentials and budgets
are supplied by the launcher; do not read credential files.

For an irreversible action's probe, `verify.with` names the read action
inside the same plugin, such as `appointments`, not `clinic/appointments`.
The qualified form is used by Function calls, not this plugin-local field.
