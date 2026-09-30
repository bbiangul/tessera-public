// Package tessera holds the reference material tessera init copies into a
// new agent's repository (RFC-0007, step 1): the authoring skill, the
// guides and the traveler example. An agent is a folder in the customer's own
// repository, so the material travels in the binary.
//
// The public Go API comes later (RFC-0007, step 3).
package tessera

import "embed"

// Kit is the reference material, at its paths in this repository.
//
//go:embed .claude/skills/tessera-author/SKILL.md
//go:embed docs/guides/authoring.md docs/guides/testing.md docs/guides/validation.md docs/guides/expressions.md
//go:embed examples/traveler/README.md examples/traveler/agent.yaml examples/traveler/functions examples/traveler/plugins examples/traveler/evals examples/traveler/scripts
var Kit embed.FS
