package initkit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWriteCreatesAWorkingRepository(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent")
	files, err := Write(dir, Options{Namespace: "clinica", Image: "ghcr.io/bbiangul/tessera:1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"agent.yaml", "functions/anotar-recado.yaml", "evals/recados.evalsuite.yaml",
		"AGENTS.md", "Dockerfile", ".gitignore", ".claude/skills/tessera-author/SKILL.md",
		".tessera/guides/authoring.md", ".tessera/examples/traveler/agent.yaml"} {
		if !contains(files, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, f := range files {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if strings.Contains(string(b), "@NS@") || strings.Contains(string(b), "@IMAGE@") {
			t.Errorf("%s keeps a placeholder", f)
		}
	}
	docker, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if !strings.Contains(string(docker), "FROM ghcr.io/bbiangul/tessera:1.2.3") || !strings.Contains(string(docker), `"--namespace", "clinica"`) {
		t.Errorf("Dockerfile:\n%s", docker)
	}
	skill, _ := os.ReadFile(filepath.Join(dir, ".claude/skills/tessera-author/SKILL.md"))
	if strings.Contains(string(skill), "./bin/tessera") || strings.Contains(string(skill), "`docs/guides/") {
		t.Error("the skill must point at the new repository's copies")
	}
}

// Every relative link in the copied documents resolves inside the new
// repository; the others became plain text.
func TestNoBrokenLinks(t *testing.T) {
	dir := t.TempDir()
	files, err := Write(dir, Options{Namespace: "x", Image: "i"})
	if err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`\]\(([^)\s]+)\)`)
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, f))
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			target := m[1]
			if strings.Contains(target, ":") || strings.HasPrefix(target, "#") {
				continue
			}
			file, _, _ := strings.Cut(target, "#")
			if _, err := os.Stat(filepath.Join(dir, filepath.Dir(f), file)); err != nil {
				t.Errorf("%s: broken link %s", f, target)
			}
		}
	}
}

func TestWriteNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, Options{Namespace: "x", Image: "i"}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("want a refusal, got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "agent.yaml")); string(b) != "mine" {
		t.Error("the file was overwritten")
	}
}

func TestNamespaceIsChecked(t *testing.T) {
	for _, ns := range []string{"", "Clinica", "minha clinica", "1abc", "a/b"} {
		if _, err := Write(filepath.Join(t.TempDir(), "a"), Options{Namespace: ns, Image: "i"}); err == nil {
			t.Errorf("%q must be refused", ns)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
