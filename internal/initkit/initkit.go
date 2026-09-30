// Package initkit writes a new agent's repository: tessera init (RFC-0007,
// step 1). The agent is a folder in the customer's repository. It gets a
// small working agent to start from, the authoring skill for Claude Code,
// the guides and reference examples (under .tessera/), and a Dockerfile
// on the Tessera image.
package initkit

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bbiangul/tessera"
)

//go:embed all:skeleton
var skeleton embed.FS

//go:embed sharepoint
var sharepoint embed.FS

//go:embed web
var web embed.FS

// Options shape the new repository.
type Options struct {
	// Namespace is the agent's namespace: lowercase letters, digits and
	// dashes.
	Namespace string
	// Image is the Tessera image the Dockerfile starts from.
	Image string
}

var namespaceRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Write creates dir with a new agent and returns the files it wrote,
// sorted. It never overwrites: dir must not exist or be empty.
func Write(dir string, o Options) ([]string, error) {
	if !namespaceRe.MatchString(o.Namespace) {
		return nil, fmt.Errorf("namespace %q: use lowercase letters, digits and dashes, starting with a letter", o.Namespace)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty: tessera init never overwrites", dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files := map[string][]byte{}
	err := fs.WalkDir(skeleton, "skeleton", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skeleton.ReadFile(p)
		if err != nil {
			return err
		}
		s := strings.NewReplacer("@NS@", o.Namespace, "@IMAGE@", o.Image).Replace(string(b))
		files[strings.TrimSuffix(strings.TrimPrefix(p, "skeleton/"), ".tmpl")] = []byte(s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(tessera.Kit, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := tessera.Kit.ReadFile(p)
		if err != nil {
			return err
		}
		to := kitPath(p)
		if strings.HasSuffix(p, ".md") {
			b = []byte(rewriteLinks(string(b), p, to))
		}
		if strings.HasSuffix(p, "SKILL.md") {
			b = []byte(skillPaths.Replace(string(b)))
		}
		files[to] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	var written []string
	for name, b := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return nil, err
		}
		written = append(written, name)
	}
	sort.Strings(written)
	return written, nil
}

// kitPath is where a file of Tessera's repository goes in the new one.
func kitPath(p string) string {
	switch {
	case strings.HasPrefix(p, "docs/guides/"):
		return ".tessera/guides/" + strings.TrimPrefix(p, "docs/guides/")
	case strings.HasPrefix(p, "examples/"):
		return ".tessera/" + p
	}
	return p // the skill keeps its path
}

// skillPaths points the skill at the new repository's copies.
var skillPaths = strings.NewReplacer(
	"docs/guides/", ".tessera/guides/",
	"./bin/tessera", "tessera",
)

var mdLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

// rewriteLinks keeps relative links that point into the kit, moved to their
// new place, and turns the others (research reports, RFCs, source code)
// into plain text naming Tessera's repository.
func rewriteLinks(doc, from, to string) string {
	return mdLink.ReplaceAllStringFunc(doc, func(m string) string {
		parts := mdLink.FindStringSubmatch(m)
		text, target := parts[1], parts[2]
		if strings.Contains(target, ":") || strings.HasPrefix(target, "#") {
			return m // a URL or an anchor
		}
		file, anchor, _ := strings.Cut(target, "#")
		repoPath := path.Clean(path.Join(path.Dir(from), file))
		if !inKit(repoPath) {
			return text + " (in Tessera's repository: " + repoPath + ")"
		}
		rel, err := filepath.Rel(filepath.FromSlash(path.Dir(to)), filepath.FromSlash(kitPath(repoPath)))
		if err != nil {
			return text
		}
		if anchor != "" {
			anchor = "#" + anchor
		}
		return "[" + text + "](" + filepath.ToSlash(rel) + anchor + ")"
	})
}

func inKit(p string) bool {
	if _, err := fs.Stat(tessera.Kit, p); err == nil {
		return true
	}
	return false
}
