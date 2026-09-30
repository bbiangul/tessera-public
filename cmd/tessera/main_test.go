package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/project"
)

// serve isolates script calls by running this binary as the worker, so the
// test binary must be one too, as main is.
func TestMain(m *testing.M) {
	compute.RunWorkerIfRequested()
	os.Exit(m.Run())
}

func TestVersion(t *testing.T) {
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != project.CLI+" "+project.Version+"\n" {
		t.Errorf("version output = %q", got)
	}
}

func TestBuildTargets(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "k1")
	t.Setenv("OPENROUTER_API_KEY", "k2")
	tgs, err := buildTargets([]string{"typesafe:jev-latest", "llm:deepseek/deepseek-v4.1-flash"}, true, []string{"DeepSeek"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tgs) != 2 || tgs[0].Label != "typesafe:jev-latest" || tgs[1].Label != "llm:deepseek/deepseek-v4.1-flash@DeepSeek" {
		t.Errorf("targets = %+v", tgs)
	}
	if tgs[0].Decider.Name() != "typesafe" || tgs[1].Decider.Name() != "llm-constrained" {
		t.Errorf("backends = %s, %s", tgs[0].Decider.Name(), tgs[1].Decider.Name())
	}

	for spec, want := range map[string]string{
		"jev":         "want backend:model",
		"typesafe:":   "want backend:model",
		"gpt:gpt-5.5": "unknown backend",
	} {
		if _, err := buildTargets([]string{spec}, true, nil, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", spec, err, want)
		}
	}

	t.Setenv("TYPESAFE_API_KEY", "")
	if _, err := buildTargets([]string{"typesafe:jev-latest"}, true, nil, false); err == nil {
		t.Error("want error when the key is missing")
	}
	if _, err := buildTargets([]string{"typesafe:jev-latest"}, true, nil, true); err != nil {
		t.Errorf("replay needs no key: %v", err)
	}
}

func TestBenchDecideRequiresFlags(t *testing.T) {
	root := newRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"bench", "decide"})
	if err := root.Execute(); err == nil {
		t.Fatal("want error without --dataset and --target")
	}
}

// tessera init's agent passes tessera validate as written.
func TestInitThenValidate(t *testing.T) {
	dir := t.TempDir() + "/minha-clinica"
	for _, args := range [][]string{{"init", dir}, {"validate", dir}} {
		root := newRoot()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		if args[0] == "init" && !strings.Contains(out.String(), "namespace minha-clinica") {
			t.Errorf("the namespace comes from the folder: %s", out.String())
		}
		if args[0] == "validate" && !strings.Contains(out.String(), "0 errors, 0 warnings") {
			t.Errorf("validate: %s", out.String())
		}
	}
}

// Every built-in web provider validates once switched on (#49).
func TestAddWebThenValidate(t *testing.T) {
	for _, p := range []string{"tinyfish", "searxng", "tavily", "brave"} {
		dir := t.TempDir() + "/agente"
		run := func(args ...string) string {
			root := newRoot()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("%s %v: %v\n%s", p, args, err, out.String())
			}
			return out.String()
		}
		run("init", dir)
		run("add", "web", "--provider", p, "--dir", dir)
		agent := dir + "/agent.yaml"
		b, err := os.ReadFile(agent)
		if err != nil {
			t.Fatal(err)
		}
		on := strings.Replace(string(b), "  freeMode:\n", "  freeMode:\n    search: { act: web/search }\n", 1)
		if on == string(b) {
			t.Fatal("the skeleton has no freeMode")
		}
		if err := os.WriteFile(agent, []byte(on), 0o644); err != nil {
			t.Fatal(err)
		}
		if out := run("validate", dir); !strings.Contains(out, "0 errors, 0 warnings") {
			t.Errorf("%s: %s", p, out)
		}
	}
}
