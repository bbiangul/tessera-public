# ADR-0001: Provisional name, license and repository language

- **Status:** accepted (provisional by design)
- **Date:** 2026-09-22

## Context

The project needs a name to start, and the final name is not decided. Waiting
would block work. `agentd`/`agentctl`, the placeholders in RFC-0001, collide
with an existing open-source CLI.

## Decision

- Working name **Tessera** (a small tile in a mosaic: agents built from small
  typed pieces). CLI `tessera`, server `tesserad`, API group
  `agents.tessera.dev`, module `github.com/bbiangul/tessera`.
- Every user-visible use of the name reads from `internal/project`, so a rename
  is one file plus the module path (a mechanical `sed` plus `go mod edit`).
- License **Apache-2.0**, the suggestion in RFC-0001, until the final
  decision. Change it before the first public release if needed.
- Code, comments and repository docs are in **English**, because the project
  is open source. RFC-0001 stays in Portuguese for now and will be translated.

## Consequences

- The final name must be chosen before the API group is frozen at `v1beta1`.
- A GitHub search on 2026-09-22 found no significant project named Tessera in
  the agent space; this must be rechecked, with domain and trademark, before
  the rename window closes.
