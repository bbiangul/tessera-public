// Package schemas holds the JSON Schemas generated from the API types, with
// their Go doc comments as descriptions. The files are committed and
// embedded; TestSchemasAreUpToDate fails when the types change and the files
// were not regenerated (make schema).
package schemas

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

//go:embed *.schema.json
var files embed.FS

// Module is the Go module path, used to match doc comments to types.
const Module = "github.com/bbiangul/tessera"

// Kinds maps each kind to its schema file.
var Kinds = map[string]string{
	"Function":       "function.schema.json",
	"PluginManifest": "pluginmanifest.schema.json",
	"EvalSuite":      "evalsuite.schema.json",
	"Agent":          "agent.schema.json",
	"Trigger":        "trigger.schema.json",
	"Subject":        "subject.schema.json",
	"Funnel":         "funnel.schema.json",
	"Experiment":     "experiment.schema.json",
	"HumanQueue":     "humanqueue.schema.json",
	"Team":           "team.schema.json",
	"KnowledgeBase":  "knowledgebase.schema.json",
}

// Get returns the embedded schema for a kind.
func Get(kind string) ([]byte, error) {
	name, ok := Kinds[kind]
	if !ok {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	return files.ReadFile(name)
}

// Generate reflects the API types into schemas. It must run with the working
// directory at the repository root, because doc comments are read from the
// source files under ./api.
func Generate() (map[string][]byte, error) {
	r := &jsonschema.Reflector{}
	if err := r.AddGoComments(Module, "api", jsonschema.WithFullComment()); err != nil {
		return nil, fmt.Errorf("read doc comments: %w", err)
	}
	out := map[string][]byte{}
	for kind, v := range map[string]any{"Function": &agents.Function{}, "PluginManifest": &plugins.PluginManifest{}, "EvalSuite": &agents.EvalSuite{}, "Agent": &agents.Agent{}, "Trigger": &agents.Trigger{}, "Subject": &agents.Subject{}, "Funnel": &agents.Funnel{}, "Experiment": &agents.Experiment{}, "HumanQueue": &agents.HumanQueue{}, "Team": &agents.Team{}, "KnowledgeBase": &agents.KnowledgeBase{}} {
		s := r.Reflect(v)
		s.Title = kind
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return nil, err
		}
		out[Kinds[kind]] = append(data, '\n')
	}
	return out, nil
}
