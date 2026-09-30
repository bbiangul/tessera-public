// Package project holds the project's naming, in one place.
//
// The project name is provisional (see docs/adr/0001-provisional-name.md).
// Everything user-visible that carries the name must read it from here, so a
// rename is a one-file change plus the Go module path.
package project

const (
	// Name is the human-readable project name.
	Name = "Tessera"
	// CLI is the command-line tool binary name.
	CLI = "tessera"
	// Daemon is the server binary name.
	Daemon = "tesserad"
	// APIGroup is the API group for agent resources, e.g. agents.tessera.dev/v1alpha1.
	APIGroup = "agents.tessera.dev"
	// PluginAPIGroup is the API group for plugin resources.
	PluginAPIGroup = "plugins.tessera.dev"
)

// Version is set at build time with -ldflags "-X .../internal/project.Version=...".
var Version = "dev"
