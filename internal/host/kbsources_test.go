package host

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/knowledge"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const kbPluginAgent = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: docs, version: 0.1.0 }
spec:
  actions:
    list: { input: {}, output: list, effect: read }
    get: { input: { id: string }, output: map, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: kp }
spec:
  messages: { notUnderstood: "NAO-ENTENDI" }
  freeMode: { instructions: "Ajude.", knowledge: { bases: [politicas] } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: politicas, namespace: kp }
spec:
  description: Políticas.
  sources:
    - list: { act: docs/list }
      fetch: { act: docs/get, with: { id: "${ inputs.id }" } }
      refresh: 1h
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: horarios, namespace: kp }
spec:
  description: Opening hours.
  selector: { intent: horarios, description: "Horário." }
  output: { template: "8h" }
`

// docsWorld is a document library the sandboxed actions serve.
type docsWorld struct {
	mu        sync.Mutex
	files     map[string][2]string // id -> name, modified
	content   map[string]string    // id -> text
	listFail  bool
	fetchFail map[string]bool
	lists     int
	fetches   map[string]int
}

func (w *docsWorld) handlers() map[string]act.Handler {
	return map[string]act.Handler{
		"docs/list": func(context.Context, act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.lists++
			if w.listFail {
				return nil, errors.New("graph is down")
			}
			var out []any
			for id, f := range w.files {
				out = append(out, map[string]any{"id": id, "name": f[0], "modified": f[1]})
			}
			return out, nil
		},
		"docs/get": func(_ context.Context, c act.Call) (any, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			id := c.Args["id"].(string)
			w.fetches[id]++
			if w.fetchFail[id] {
				return nil, errors.New("download failed")
			}
			return map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte(w.content[id]))}, nil
		},
	}
}

func (w *docsWorld) set(id, name, modified, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.files[id] = [2]string{name, modified}
	w.content[id] = text
}

// plugin-001: plugin sources are read at load and refreshed by Tick; a
// changed document is served in its new version, a removed one is gone,
// an unchanged one is not fetched again, and a failure keeps what was
// read before.
func TestKnowledgePluginSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(kbPluginAgent), 0o600); err != nil {
		t.Fatal(err)
	}
	w := &docsWorld{files: map[string][2]string{}, content: map[string]string{}, fetchFail: map[string]bool{}, fetches: map[string]int{}}
	w.set("1", "estacionamento.md", "v1", "# Estacionamento\n\nO conveniado é o Parque Seguro, R$ 12.\n")
	w.set("2", "wifi.md", "v1", "# Wi-fi\n\nTemos wi-fi na sala de espera.\n")
	w.set("3", "foto.png", "v1", "not a document")
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	h, issues, err := Load(Options{Paths: []string{dir}, Namespace: "kp", Store: journal.NewMemory(), Decider: kbDecider{}, Handlers: w.handlers(),
		Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return now } }})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ctx := context.Background()
	search := func(q string) string {
		ps, _ := h.index().Search(q, []string{"politicas"}, knowledge.Defaults)
		var s []string
		for _, p := range ps {
			s = append(s, p.Text)
		}
		return strings.Join(s, "|")
	}
	if !strings.Contains(search("estacionamento"), "R$ 12") || !strings.Contains(search("sala espera"), "wi-fi") || w.fetches["3"] != 0 {
		t.Fatalf("at load: %q %q, png fetched %d", search("estacionamento"), search("sala espera"), w.fetches["3"])
	}

	// Not due: nothing is called.
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if w.lists != 1 {
		t.Errorf("listed %d times before refresh was due", w.lists)
	}

	// Due: the changed document is fetched and served; the unchanged one
	// is not fetched again; the removed one is gone.
	w.set("1", "estacionamento.md", "v2", "# Estacionamento\n\nO conveniado é o Parque Seguro, agora R$ 15.\n")
	w.mu.Lock()
	delete(w.files, "2")
	w.mu.Unlock()
	now = now.Add(61 * time.Minute)
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if got := search("estacionamento"); !strings.Contains(got, "R$ 15") || strings.Contains(got, "R$ 12") {
		t.Errorf("after refresh: %q", got)
	}
	if got := search("sala espera"); got != "" {
		t.Errorf("a removed document is still served: %q", got)
	}
	w.set("4", "cafe.md", "v1", "# Café\n\nTemos café.\n")
	now = now.Add(61 * time.Minute)
	if _, err := h.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if w.fetches["1"] != 2 || w.fetches["4"] != 1 {
		t.Errorf("fetches %v (the unchanged document fetched again?)", w.fetches)
	}

	// A failed list keeps everything; it is retried within 5 minutes.
	w.mu.Lock()
	w.listFail = true
	w.mu.Unlock()
	now = now.Add(61 * time.Minute)
	if _, err := h.Tick(ctx); err == nil || !strings.Contains(err.Error(), "keeping 2 document(s)") {
		t.Errorf("a failed list: %v", err)
	}
	if !strings.Contains(search("estacionamento"), "R$ 15") || !strings.Contains(search("cafe"), "café") {
		t.Errorf("a failed list emptied the base")
	}
	lists := w.lists
	now = now.Add(4 * time.Minute)
	_, _ = h.Tick(ctx)
	if w.lists != lists {
		t.Errorf("retried before 5 minutes")
	}
	now = now.Add(2 * time.Minute)
	_, _ = h.Tick(ctx)
	if w.lists != lists+1 {
		t.Errorf("not retried after 5 minutes")
	}

	// A failed fetch keeps that document's previous version.
	w.mu.Lock()
	w.listFail = false
	w.fetchFail["1"] = true
	w.mu.Unlock()
	w.set("1", "estacionamento.md", "v3", "# Estacionamento\n\nR$ 99.\n")
	now = now.Add(61 * time.Minute)
	if _, err := h.Tick(ctx); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Errorf("a failed fetch: %v", err)
	}
	if got := search("estacionamento"); !strings.Contains(got, "R$ 15") {
		t.Errorf("a failed fetch lost the previous version: %q", got)
	}
}
