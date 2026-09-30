package admission

import (
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// MetricsPlugin is the built-in plugin that lets Functions read business
// numbers (a weekly report, an alert): local and read-only, so it runs for
// real in EvalSuites, like sql.
const MetricsPlugin = "metrics"

// LessonsPlugin is the built-in plugin that writes an approved lesson into
// its base (learn-003).
const LessonsPlugin = "lessons"

// BuiltinDocs are the plugins every load includes. Declaring a plugin with
// the same name is a duplicate.
func BuiltinDocs() []Doc {
	lessons := Doc{File: "<built-in>", Kind: "PluginManifest", Plugin: &plugins.PluginManifest{
		APIVersion: "plugins.tessera.dev/v1alpha1", Kind: "PluginManifest",
		Metadata: plugins.PluginMeta{Name: LessonsPlugin, Version: "1.0.0"},
		Spec: plugins.PluginSpec{Actions: map[string]plugins.ActionSpec{
			// accept: append an approved lesson to its base's file (idempotent
			// by the approval run).
			"accept": {
				Effect: plugins.EffectIdempotentWrite, Output: "map",
				Input: map[string]string{"base": "string", "id": "string", "question": "string", "fact": "string", "kind": "string",
					"cases": "int", "members": "string", "runs": "string", "supersede": "string"},
			},
			// renew: set a lesson's review date to now plus its validity
			// (renew-001).
			"renew": {
				Effect: plugins.EffectIdempotentWrite, Output: "map",
				Input: map[string]string{"base": "string", "id": "string"},
			},
		}},
	}}
	return append([]Doc{lessons}, Doc{File: "<built-in>", Kind: "PluginManifest", Plugin: &plugins.PluginManifest{
		APIVersion: "plugins.tessera.dev/v1alpha1", Kind: "PluginManifest",
		Metadata: plugins.PluginMeta{Name: MetricsPlugin, Version: "1.0.0"},
		Spec: plugins.PluginSpec{Actions: map[string]plugins.ActionSpec{
			// funnel: a Funnel now, per cohort and in total (reached,
			// pending, lost, conversion, median and p90 days, stuck).
			"funnel": {
				Effect: plugins.EffectRead, Input: map[string]string{"funnel": "string"}, Output: "map",
			},
			// experiment: an Experiment now (arms, evidence, verdict, SRM).
			"experiment": {
				Effect: plugins.EffectRead, Input: map[string]string{"experiment": "string"}, Output: "map",
			},
		}},
	}})
}
