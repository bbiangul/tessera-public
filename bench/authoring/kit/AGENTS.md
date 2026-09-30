# Partner delivery workspace

You are an external software-house engineer using a distributed Tessera CLI.
Read `.claude/skills/tessera-author/SKILL.md` and `docs/guides/`.
Use `./bin/tessera explain` as the API reference; do not guess fields.
Only this package is available to your assignment. Do not read other
workspaces, repository source, history, research, benchmark definitions,
other agents or conversations. Do not use web, connectors or subagents.
The machine's default cwd is not this package; set workdir explicitly.

Every shell command must run through:
`sandbox-exec -f @KIT@/partner.sb /bin/zsh -c '...'`
with workdir `@KIT@`. This blocks source
and internal agent-history reads during shell commands. Do not bypass it.
Only read/write this package. Use shell tools within that sandbox for edits;
do not use file-edit tools against other paths or inspect the binary.

The `bin/tessera` launcher is the supported tool. Do not invoke tessera-cli
directly, modify the launcher/profile or access credential files. Credentials
are injected by the launcher. It logs invocations and caps live tests at
$0.05 each and twenty calls ($1 total reserved); replay is free. Use
`--generate openrouter:deepseek/deepseek-v4.1-flash` and
`--decide typesafe:jev-1.13.0`. No other model or live API tool is authorized.
No source/build system is supplied. Missing references to research/source
are background citations, not permission to seek the repository.

Generic travel/store/outreach examples are documentation samples;
no prebuilt clinic agent is supplied. API contract: api/clinic.openapi.json.
Create authored/clinica, its own sandboxed EvalSuite and cassette, and
REPORT.md. Report tool use, validation/test attempts, time, ambiguities,
documentation gaps and any outside-package access (expected none).

Read docs/guides/partner-testing.md and docs/guides/expressions.md before authoring.
