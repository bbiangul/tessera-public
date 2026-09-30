# Contributing

Thanks for helping. The short version:

1. Read [AGENTS.md](AGENTS.md) — it applies to people too.
2. Read the RFC section for the area you change
   ([RFC-0001](docs/rfcs/0001-plataforma-declarativa-de-agentes.md)).
3. `make ci` must pass: gofmt, `go vet`, the file-size limit, unit tests with
   `-race`, and the build.
4. Behavior changes come with tests ([testing guide](docs/guides/testing.md)).
   Claims about models come with a benchmark ([ADR-0002](docs/adr/0002-measure-before-believing.md)).
5. Architecture decisions get an ADR in `docs/adr/`; larger designs get an RFC
   in `docs/rfcs/`.

## Commit messages

Imperative mood, one line under 72 characters, then a body that explains why.
Reference the RFC or ADR when a change implements one.
