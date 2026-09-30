package host

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/knowledge"
)

// Knowledge from files and plugins (plugin-001). Files are read when the
// agent loads. Plugin sources are read then too, and again every refresh
// by Tick. The index is rebuilt and swapped in whole, so an answer never
// sees a half-built one. A failed read keeps what was read before: a
// failure never empties a base.

type kbState struct {
	mu        sync.RWMutex
	ix        *knowledge.Index
	fileBases []knowledge.Base
	sources   []*kbSource
	pending   []error // read failures at load, returned by the first Tick
}

type kbSource struct {
	base                string
	list, fetch         string
	listWith, fetchWith *expr.Value
	refresh             time.Duration
	next                time.Time
	docs                map[string]kbDoc // by id, else name
}

type kbDoc struct {
	name, modified, text string
}

// retryAfter bounds how long a failed plugin source waits for a retry.
const retryAfter = 5 * time.Minute

// loadKnowledge compiles the namespace's knowledge bases: their files and
// plugin sources, and each base's audiences. nil when there are none.
func loadKnowledge(docs []admission.Doc, ns string, lessonFiles map[string]string) (*kbState, map[string][]string, error) {
	e, err := expr.New()
	if err != nil {
		return nil, nil, err
	}
	kb := &kbState{}
	auds := map[string][]string{}
	for _, d := range docs {
		b := d.Knowledge
		if b == nil || b.Metadata.Namespace != ns {
			continue
		}
		auds[b.Metadata.Name] = b.Spec.Audiences
		base := knowledge.Base{Name: b.Metadata.Name, Dir: filepath.Dir(d.File)}
		for _, s := range b.Spec.Sources {
			if s.List == nil {
				fs, err := knowledge.Resolve(d.File, s.Files)
				if err != nil {
					return nil, nil, err
				}
				base.Files = append(base.Files, fs...)
				continue
			}
			src := &kbSource{base: b.Metadata.Name, list: s.List.Act, refresh: 24 * time.Hour, docs: map[string]kbDoc{}}
			if s.Refresh != "" {
				if src.refresh, err = time.ParseDuration(s.Refresh); err != nil {
					return nil, nil, err
				}
			}
			if src.listWith, err = e.CompileValue(withOrEmpty(s.List.With)); err != nil {
				return nil, nil, fmt.Errorf("knowledge base %s: list.with: %w", b.Metadata.Name, err)
			}
			if s.Fetch != nil {
				src.fetch = s.Fetch.Act
				if src.fetchWith, err = e.CompileValue(withOrEmpty(s.Fetch.With)); err != nil {
					return nil, nil, fmt.Errorf("knowledge base %s: fetch.with: %w", b.Metadata.Name, err)
				}
			}
			kb.sources = append(kb.sources, src)
		}
		if f := lessonFiles[b.Metadata.Name]; f != "" {
			base.Files = append(base.Files, f) // approved lessons (learn-003)
		}
		kb.fileBases = append(kb.fileBases, base)
	}
	if len(kb.fileBases) == 0 {
		return nil, nil, nil
	}
	return kb, auds, nil
}

func withOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// index is the current knowledge index, nil without knowledge bases.
func (h *Host) index() *knowledge.Index {
	if h.kb == nil {
		return nil
	}
	h.kb.mu.RLock()
	defer h.kb.mu.RUnlock()
	return h.kb.ix
}

// rebuild indexes the files and the plugin sources' documents and swaps
// the new index in. The old one is closed a minute later, after answers
// that were using it are done.
func (h *Host) rebuild() error {
	kb := h.kb
	var bases []knowledge.Base
	for _, b := range kb.fileBases {
		for _, s := range kb.sources {
			if s.base != b.Name {
				continue
			}
			for _, d := range s.docs {
				b.Docs = append(b.Docs, knowledge.Doc{Source: d.name, Text: d.text})
			}
		}
		bases = append(bases, b)
	}
	ix, err := knowledge.Build(bases)
	if err != nil {
		return err
	}
	kb.mu.Lock()
	old := kb.ix
	kb.ix = ix
	kb.mu.Unlock()
	if old != nil {
		time.AfterFunc(time.Minute, func() { _ = old.Close() })
	}
	return nil
}

// readSources reads the plugin sources that are due (all of them when
// force), and says whether any document changed.
func (h *Host) readSources(ctx context.Context, now time.Time, force bool) (bool, []error) {
	changed := false
	var errs []error
	for _, s := range h.kb.sources {
		if !force && now.Before(s.next) {
			continue
		}
		c, err := h.readSource(ctx, s, now)
		changed = changed || c
		errs = append(errs, err...)
	}
	return changed, errs
}

func (h *Host) readSource(ctx context.Context, s *kbSource, now time.Time) (bool, []error) {
	fail := func(err error) (bool, []error) {
		wait := min(retryAfter, s.refresh)
		s.next = now.Add(wait)
		return false, []error{fmt.Errorf("knowledge base %s: %s: %w (keeping %d document(s); retry in %s)", s.base, s.list, err, len(s.docs), wait)}
	}
	args, err := s.listWith.Resolve(ctx, expr.Activation{})
	if err != nil {
		return fail(err)
	}
	a, _ := args.(map[string]any)
	out, err := h.Runtime.Actor.Call(ctx, act.Call{Action: s.list, Args: a})
	if err != nil {
		return fail(err)
	}
	items, ok := out.([]any)
	if !ok {
		return fail(errors.New("the list action did not return a list"))
	}
	var errs []error
	docs := map[string]kbDoc{}
	for _, it := range items {
		item, _ := it.(map[string]any)
		name, _ := item["name"].(string)
		if name == "" || !knowledge.Supported(name) {
			continue // not a document the index reads
		}
		key := name
		if id := fmt.Sprint(item["id"]); item["id"] != nil && id != "" {
			key = id
		}
		modified := ""
		if item["modified"] != nil {
			modified = fmt.Sprint(item["modified"])
		}
		prev, had := s.docs[key]
		content := item
		if item["text"] == nil && item["base64"] == nil {
			switch {
			case had && modified != "" && prev.modified == modified:
				docs[key] = prev // unchanged: not fetched again
				continue
			case s.fetch == "":
				continue
			}
			fargs, err := s.fetchWith.Resolve(ctx, expr.Activation{"inputs": item})
			if err == nil {
				fa, _ := fargs.(map[string]any)
				var got any
				if got, err = h.Runtime.Actor.Call(ctx, act.Call{Action: s.fetch, Args: fa}); err == nil {
					content, _ = got.(map[string]any)
					if t, ok := got.(string); ok {
						content = map[string]any{"text": t}
					}
				}
			}
			if err != nil {
				if had {
					docs[key] = prev // the previous version stays
				}
				errs = append(errs, fmt.Errorf("knowledge base %s: %s %q: %w", s.base, s.fetch, name, err))
				continue
			}
		}
		text, err := contentText(name, content)
		if err != nil {
			if had {
				docs[key] = prev
			}
			errs = append(errs, fmt.Errorf("knowledge base %s: %q: %w", s.base, name, err))
			continue
		}
		docs[key] = kbDoc{name: name, modified: modified, text: text}
	}
	changed := len(docs) != len(s.docs)
	for k, d := range docs {
		if p, ok := s.docs[k]; !ok || p != d {
			changed = true
		}
	}
	s.docs = docs
	s.next = now.Add(s.refresh)
	return changed, errs
}

// contentText is a fetched or listed document's text: {text} as it is,
// {base64} decoded and extracted by the name's type.
func contentText(name string, c map[string]any) (string, error) {
	if t, ok := c["text"].(string); ok {
		return knowledge.TextBytes(name, []byte(t))
	}
	b64, _ := c["base64"].(string)
	if b64 == "" {
		return "", errors.New("no text or base64 in the answer")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	return knowledge.TextBytes(name, data)
}
