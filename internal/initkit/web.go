package initkit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WebProviders are the built-in web search plugins (#49): one contract,
// search {query} -> [{site, title, url, snippet}], so switching provider
// is replacing one file.
func WebProviders() []string {
	entries, _ := fs.ReadDir(web, "web")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(out)
	return out
}

// WriteWeb writes the web plugin for provider into dir/plugins/web.yaml
// and returns its path. It never overwrites.
func WriteWeb(dir, provider string) (string, error) {
	b, err := web.ReadFile("web/" + provider + ".yaml")
	if err != nil {
		return "", fmt.Errorf("no web provider %q: use one of %s", provider, strings.Join(WebProviders(), ", "))
	}
	target := filepath.Join(dir, "plugins", "web.yaml")
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%s exists: tessera add never overwrites", target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, b, 0o644)
}

// WriteSharePoint writes the SharePoint plugin (Microsoft Graph, for
// KnowledgeBase sources; plugin-001) into dir/plugins/sharepoint.yaml,
// with the tenant and site ids. It never overwrites.
func WriteSharePoint(dir, tenant, site string) (string, error) {
	if tenant == "" || site == "" {
		return "", errors.New("--tenant and --site are required (the Entra tenant id, and the site id: GET /v1.0/sites/{host}:/sites/{name})")
	}
	b, err := sharepoint.ReadFile("sharepoint/sharepoint.yaml")
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(b), "TENANT_ID", tenant), "SITE_ID", site)
	target := filepath.Join(dir, "plugins", "sharepoint.yaml")
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%s exists: tessera add never overwrites", target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, []byte(text), 0o644)
}
