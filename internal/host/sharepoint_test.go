package host

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bbiangul/tessera/internal/initkit"
	"github.com/bbiangul/tessera/internal/knowledge"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

type spSecrets map[string]string

func (s spSecrets) Secret(name string) (string, error) { return s[name], nil }

// plugin-001 (G4): tessera add sharepoint's manifest against a Graph
// stand-in: the token, a folder's children (a PDF, a DOCX, a subfolder),
// each /content redirecting to the tenant's download host; both files are
// indexed, and the download host never sees the token.
func TestSharePointSource(t *testing.T) {
	pdf, err := os.ReadFile("../knowledge/testdata/convenios.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var docx bytes.Buffer
	z := zip.NewWriter(&docx)
	wr, _ := z.Create("word/document.xml")
	_, _ = wr.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Estacionamento</w:t></w:r></w:p><w:p><w:r><w:t>O conveniado é o Parque Seguro.</w:t></w:r></w:p></w:body></w:document>`))
	_ = z.Close()
	var mu sync.Mutex
	seen := map[string]string{}
	srv := httptest.NewTLSServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		mu.Lock()
		seen[r.Host+r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch {
		case r.Host == "login.microsoftonline.com" && r.URL.Path == "/tenant-1/oauth2/v2.0/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "graph-token", "token_type": "Bearer", "expires_in": 3600})
		case r.Host == "graph.microsoft.com" && r.URL.Path == "/v1.0/sites/site-1/drive/root:/Politicas:/children":
			if r.Header.Get("Authorization") != "Bearer graph-token" {
				nethttp.Error(w, "no token", nethttp.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{
				map[string]any{"id": "A", "name": "convenios.pdf", "lastModifiedDateTime": "2026-09-01T10:00:00Z", "file": map[string]any{}},
				map[string]any{"id": "B", "name": "estacionamento.docx", "lastModifiedDateTime": "2026-09-02T10:00:00Z", "file": map[string]any{}},
				map[string]any{"id": "C", "name": "Arquivo antigo", "folder": map[string]any{}},
			}})
		case r.Host == "graph.microsoft.com" && strings.HasPrefix(r.URL.Path, "/v1.0/sites/site-1/drive/items/"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1.0/sites/site-1/drive/items/"), "/content")
			nethttp.Redirect(w, r, "https://contoso.sharepoint.com/download/"+id, nethttp.StatusFound)
		case r.Host == "contoso.sharepoint.com" && r.URL.Path == "/download/A":
			_, _ = w.Write(pdf)
		case r.Host == "contoso.sharepoint.com" && r.URL.Path == "/download/B":
			_, _ = w.Write(docx.Bytes())
		default:
			nethttp.NotFound(w, r)
		}
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	client := &nethttp.Client{Transport: &nethttp.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // a local stand-in
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}

	dir := t.TempDir()
	if _, err := initkit.WriteSharePoint(dir, "tenant-1", "site-1"); err != nil {
		t.Fatal(err)
	}
	agent := `apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: sp }
spec:
  freeMode: { instructions: "Ajude.", knowledge: { bases: [politicas] } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: politicas, namespace: sp }
spec:
  description: Políticas no SharePoint.
  sources:
    - list: { act: sharepoint/list-files, with: { folder: Politicas } }
      fetch: { act: sharepoint/download, with: { id: "${ inputs.id }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: sp }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "Horário." }
  output: { template: "8h" }
`
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(agent), 0o600); err != nil {
		t.Fatal(err)
	}
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "sp", Store: journal.NewMemory(), Decider: kbDecider{},
		Secrets: spSecrets{"SHAREPOINT_CLIENT_ID": "app", "SHAREPOINT_CLIENT_SECRET": "secret"}, HTTPClient: client, AllowPrivateNetworks: true})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	if len(h.kb.pending) != 0 {
		t.Fatalf("reading SharePoint failed: %v", h.kb.pending)
	}
	find := func(q string) string {
		ps, _ := h.index().Search(q, []string{"politicas"}, knowledge.Defaults)
		var out []string
		for _, p := range ps {
			out = append(out, p.Source+": "+p.Section+": "+p.Text)
		}
		return strings.Join(out, "\n")
	}
	if got := find("unimed odonto"); !strings.Contains(got, "convenios.pdf: Convênios aceitos") {
		t.Errorf("the PDF: %s", got)
	}
	if got := find("estacionamento conveniado"); !strings.Contains(got, "estacionamento.docx: Estacionamento: O conveniado é o Parque Seguro.") {
		t.Errorf("the DOCX: %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["graph.microsoft.com/v1.0/sites/site-1/drive/items/A/content"] != "Bearer graph-token" || seen["contoso.sharepoint.com/download/A"] != "" {
		t.Errorf("credentials per host: %v", seen)
	}
}
