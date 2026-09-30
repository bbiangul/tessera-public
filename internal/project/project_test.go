package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
)

// The API groups are also spelled in the API packages, struct tags and
// examples, which cannot all read a constant. A rename must update them all;
// this finds the ones missed.
func TestAPIGroupsAreSpelledConsistently(t *testing.T) {
	versions := map[string]string{
		"../../api/agents/v1alpha1/types.go":  APIGroup + "/v1alpha1",
		"../../api/plugins/v1alpha1/types.go": PluginAPIGroup + "/v1alpha1",
	}
	if agents.APIVersion != APIGroup+"/v1alpha1" || plugins.APIVersion != PluginAPIGroup+"/v1alpha1" {
		t.Errorf("API packages use %s and %s", agents.APIVersion, plugins.APIVersion)
	}
	for file, want := range versions {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "enum="+want) {
			t.Errorf("%s does not restrict apiVersion to %s", file, want)
		}
	}
	files, _ := filepath.Glob("../../examples/*/*.yaml")
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			v, ok := strings.CutPrefix(strings.TrimSpace(line), "apiVersion:")
			if v = strings.TrimSpace(v); ok && v != agents.APIVersion && v != plugins.APIVersion {
				t.Errorf("%s: unknown apiVersion %q", f, v)
			}
		}
	}
}

func TestNamesAgree(t *testing.T) {
	if !strings.EqualFold(Name, CLI) || Daemon != CLI+"d" {
		t.Errorf("Name %q, CLI %q and Daemon %q disagree", Name, CLI, Daemon)
	}
	for _, g := range []string{APIGroup, PluginAPIGroup} {
		if !strings.Contains(g, "."+CLI+".") {
			t.Errorf("API group %q does not carry the name %q", g, CLI)
		}
	}
}
