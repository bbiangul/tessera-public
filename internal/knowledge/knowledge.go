// Package knowledge indexes an agent's documents and searches them
// (RFC-0008 part A, knowledge-001). Documents are split into passages by
// headings and paragraphs, each keeping its source, section and the
// document's hash, and indexed with SQLite FTS5 (BM25) in memory: they
// live in the agent's repository, so the index is rebuilt when the agent
// loads and holds no state of its own.
package knowledge

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	_ "modernc.org/sqlite" // FTS5 included
)

// Passage is one searchable piece of a document.
type Passage struct {
	Base    string `json:"base"`
	Source  string `json:"source"`  // the file, relative to the base's resource
	Section string `json:"section"` // heading path, "Convênios aceitos > Carteirinha"
	Text    string `json:"text"`
	Hash    string `json:"hash"` // of the whole document: its version
	// Meta is a lesson's metadata (RFC-0008 part C), from the HTML
	// comment under its heading: id, kind, learned, review, status...
	Meta map[string]string `json:"meta,omitempty"`
}

// Base is one KnowledgeBase's documents.
type Base struct {
	Name  string
	Dir   string   // what Files are relative to
	Files []string // resolved paths
	Docs  []Doc    // documents already read (a plugin source)
}

// Doc is a document already read into text.
type Doc struct {
	Source string // its name, as shown in passages
	Text   string
}

// Options are the search's parameters, chosen on a dev set
// (knowledge-001) and then fixed as Defaults.
type Options struct {
	K int // passages returned
	// Keep, when set, skips passages it refuses (an expired or suspended
	// lesson) while the best K are chosen.
	Keep      func(Passage) bool
	Stopwords bool // drop Portuguese and English function words
	Prefix    bool // match words of 4+ letters by prefix (plurals, verb forms)
	All       bool // every word must match (AND) instead of any (OR)
}

// Defaults are knowledge-001's frozen parameters.
var Defaults = Options{K: 4, Stopwords: true, Prefix: true}

// Index is the searchable passages of an agent's bases.
type Index struct {
	db       *sql.DB
	passages []Passage
}

// Build reads and indexes the bases' documents.
func Build(bases []Base) (*Index, error) {
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one in-memory database
	if _, err := db.Exec(`CREATE VIRTUAL TABLE passages USING fts5(section, text, tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("knowledge: %w", err)
	}
	ix := &Index{db: db}
	add := func(b Base, source, text string) error {
		sum := sha256.Sum256([]byte(text))
		for _, p := range Split(text) {
			p.Base, p.Source, p.Hash = b.Name, source, hex.EncodeToString(sum[:8])
			if _, err := db.Exec(`INSERT INTO passages(rowid, section, text) VALUES (?, ?, ?)`, len(ix.passages)+1, p.Section, p.Text); err != nil {
				return fmt.Errorf("knowledge: %w", err)
			}
			ix.passages = append(ix.passages, p)
		}
		return nil
	}
	for _, b := range bases {
		for _, f := range b.Files {
			text, err := Text(f)
			if err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("knowledge base %s: %w", b.Name, err)
			}
			rel, err := filepath.Rel(b.Dir, f)
			if err != nil {
				rel = filepath.Base(f)
			}
			if err := add(b, filepath.ToSlash(rel), text); err != nil {
				_ = db.Close()
				return nil, err
			}
		}
		for _, d := range b.Docs {
			if err := add(b, d.Source, d.Text); err != nil {
				_ = db.Close()
				return nil, err
			}
		}
	}
	return ix, nil
}

// Lesson returns the passage of a lesson by its id.
func (ix *Index) Lesson(id string) (Passage, bool) {
	for _, p := range ix.passages {
		if p.Meta["id"] == id {
			return p, true
		}
	}
	return Passage{}, false
}

// Close releases the index.
func (ix *Index) Close() error { return ix.db.Close() }

// Len is the number of passages.
func (ix *Index) Len() int { return len(ix.passages) }

// Search returns the best passages for query among the given bases.
func (ix *Index) Search(query string, bases []string, o Options) ([]Passage, error) {
	if o.K <= 0 {
		o.K = Defaults.K
	}
	q := Query(query, o)
	if q == "" || len(bases) == 0 {
		return nil, nil
	}
	// Section words weigh twice a body word: a heading names the topic.
	rows, err := ix.db.Query(`SELECT rowid FROM passages WHERE passages MATCH ? ORDER BY bm25(passages, 2.0, 1.0)`, q)
	if err != nil {
		return nil, fmt.Errorf("knowledge search: %w", err)
	}
	defer rows.Close()
	var out []Passage
	for rows.Next() && len(out) < o.K {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if p := ix.passages[id-1]; slices.Contains(bases, p.Base) && (o.Keep == nil || o.Keep(p)) {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// Query turns a message into an FTS5 query: its words, quoted, joined by
// OR (or AND), stopwords dropped and long words matched by prefix.
func Query(message string, o Options) string {
	var terms []string
	seen := map[string]bool{}
	for _, w := range words(message) {
		if len([]rune(w)) < 2 || (o.Stopwords && stopwords[w]) || seen[w] {
			continue
		}
		seen[w] = true
		t := `"` + w + `"`
		if o.Prefix && len([]rune(w)) >= 4 {
			// A prefix of the word's stem: "sensiveis" also finds
			// "sensibilidade" poorly, but "remarcar" finds "remarcação".
			t = `"` + stem(w) + `"*`
		}
		terms = append(terms, t)
	}
	op := " OR "
	if o.All {
		op = " AND "
	}
	return strings.Join(terms, op)
}

// words lowercases, removes diacritics and splits on anything that is
// not a letter or a digit.
func words(s string) []string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// stem cuts a few common Portuguese endings, keeping at least 4 letters.
func stem(w string) string {
	r := []rune(w)
	for _, suf := range []string{"coes", "cao", "mente", "ados", "adas", "ando", "endo", "ar", "er", "ir", "as", "es", "os", "is", "a", "e", "o", "s"} {
		s := []rune(suf)
		if len(r)-len(s) >= 4 && strings.HasSuffix(w, suf) {
			return string(r[:len(r)-len(s)])
		}
	}
	return w
}

// Split cuts a Markdown or text document into passages: one per
// paragraph or list, under its heading path; short neighbours in the same
// section are joined up to about 700 characters.
func Split(doc string) []Passage {
	// HTML comments are metadata, not text. A lesson's comment is kept as
	// its section's Meta.
	doc = lessonComment.ReplaceAllString(doc, "\n\x00$1\n")
	doc = comments.ReplaceAllString(doc, "")
	var out []Passage
	var heads []string
	var meta map[string]string
	var block, cur []string
	section := func() string { return strings.Join(heads, " > ") }
	flushBlock := func() {
		if len(block) == 0 {
			return
		}
		p := strings.TrimSpace(strings.Join(block, "\n"))
		block = nil
		if p == "" {
			return
		}
		if len(cur) > 0 && len(strings.Join(cur, "\n\n"))+len(p) > 700 {
			out = append(out, Passage{Section: section(), Text: strings.Join(cur, "\n\n"), Meta: meta})
			cur = nil
		}
		cur = append(cur, p)
	}
	flushSection := func() {
		flushBlock()
		if len(cur) > 0 {
			out = append(out, Passage{Section: section(), Text: strings.Join(cur, "\n\n"), Meta: meta})
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "\x00") {
			meta = ParseMeta(t[1:])
			continue
		}
		if level := headingLevel(t); level > 0 {
			flushSection()
			meta = nil
			title := strings.TrimSpace(strings.TrimLeft(t, "#"))
			if level-1 < len(heads) {
				heads = heads[:level-1]
			}
			for len(heads) < level-1 {
				heads = append(heads, "")
			}
			heads = append(heads, title)
			continue
		}
		if t == "" {
			flushBlock()
			continue
		}
		block = append(block, t)
	}
	flushSection()
	for i := range out {
		out[i].Section = strings.Trim(strings.ReplaceAll(out[i].Section, " >  > ", " > "), " >")
	}
	return out
}

var (
	comments      = regexp.MustCompile(`(?s)<!--.*?-->`)
	lessonComment = regexp.MustCompile(`<!-- (lesson [^\n]*?) -->`)
)

// ParseMeta reads "lesson id=a kind=price review=2026-10-27" into
// {"lesson": "", "id": "a", ...}. Values have no spaces.
func ParseMeta(s string) map[string]string {
	m := map[string]string{}
	for _, f := range strings.Fields(s) {
		k, v, _ := strings.Cut(f, "=")
		m[k] = v
	}
	return m
}

// Active says whether a passage may be used at now: not a lesson, or a
// lesson that is neither suspended nor retired and whose review date has
// not passed (RFC-0008 part C).
func Active(p Passage, now time.Time) bool {
	if p.Meta == nil {
		return true
	}
	if s := p.Meta["status"]; s != "" && s != "active" {
		return false
	}
	if r, err := time.Parse("2006-01-02", p.Meta["review"]); err == nil && now.After(r.Add(24*time.Hour)) {
		return false
	}
	return true
}

func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n == len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a o as os um uma uns umas de do da dos das no na nos nas em por para pra pro com sem e ou que se ja nao sim eu voce vc voces vcs ele ela eles elas meu minha meus minhas seu sua seus suas nosso nossa isso isto esse essa este esta aquele aquela aqui ai la tem ter tenho temos ha ser sou foi era sao esta estou estao fazer faz fazem posso pode podem consigo consegue da dá the of to and or is are in on for with what how can do does i you me mim ao aos tambem mais muito qual quais quanto quantos quanta quando onde como porque qualquer alguma algum depois antes entao so`) {
		stopwords[w] = true
	}
}

// RewriteRules ask the Generate engine for search words: the words a
// business document would use for what the message asks ("arrancar o
// dente" is "extração"; "onde fica" is "endereço, rua"). They are added to
// the message's own words (knowledge-001: kept only if they raise dev
// recall by 10 points or more).
const RewriteRules = `MESSAGE is a customer's question to a business. Write the words the business's own documents (policies, FAQ, price list) would most likely use for what it asks: the topic's usual names and synonyms, in the language of MESSAGE.
Rules: at most 10 words, no sentences, nothing about the person (no names, numbers or contact details). Reply with the words only.`

// Resolve expands a KnowledgeBase's file globs, relative to the file the
// resource was written in, to the documents they match (sorted, unique).
func Resolve(resourceFile string, globs []string) ([]string, error) {
	dir := filepath.Dir(resourceFile)
	seen := map[string]bool{}
	var out []string
	for _, g := range globs {
		if !filepath.IsAbs(g) {
			g = filepath.Join(dir, g)
		}
		m, err := filepath.Glob(g)
		if err != nil {
			return nil, err
		}
		for _, f := range m {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

// Supported says whether a file is a document the index reads.
func Supported(file string) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".md", ".markdown", ".txt", ".pdf", ".docx":
		return true
	}
	return false
}
