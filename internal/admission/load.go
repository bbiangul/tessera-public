// Package admission checks resources before anything runs (RFC-0001,
// "Control plane"). It has two layers:
//
//  1. Load: every document is parsed and validated against the JSON Schema
//     of its kind. Unknown fields, missing fields and wrong types fail here,
//     with the path of the problem.
//  2. Check: the documents are checked together. Expressions compile,
//     references resolve, actions exist in a plugin manifest, and the
//     effect rules hold (idempotency keys, safe retries).
//
// Every problem is an Issue with a stable code, a path and a suggested fix,
// so people and coding agents can act on it.
package admission

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	jsv "github.com/santhosh-tekuri/jsonschema/v6"
	yamlv3 "go.yaml.in/yaml/v3"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/api/schemas"
)

// Severity of an issue. Errors block publication; warnings do not.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one problem found by admission.
type Issue struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	File     string   `json:"file,omitempty"`
	// Resource is Kind/namespace/name when known.
	Resource string `json:"resource,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
}

func (i Issue) String() string {
	var b strings.Builder
	if i.File != "" {
		b.WriteString(i.File + ": ")
	}
	if i.Resource != "" {
		b.WriteString(i.Resource + ": ")
	}
	if i.Path != "" {
		b.WriteString(i.Path + ": ")
	}
	fmt.Fprintf(&b, "%s [%s] %s", i.Severity, i.Code, i.Message)
	if i.Fix != "" {
		b.WriteString(" — fix: " + i.Fix)
	}
	return b.String()
}

// Doc is one loaded resource.
type Doc struct {
	File       string
	Index      int // position within the file, from 0
	Kind       string
	Function   *agents.Function
	Plugin     *plugins.PluginManifest
	EvalSuite  *agents.EvalSuite
	Agent      *agents.Agent
	Trigger    *agents.Trigger
	Subject    *agents.Subject
	Funnel     *agents.Funnel
	Experiment *agents.Experiment
	Queue      *agents.HumanQueue
	Team       *agents.Team
	Knowledge  *agents.KnowledgeBase
}

// ID returns Kind/namespace/name.
func (d Doc) ID() string {
	switch {
	case d.Function != nil:
		return "Function/" + d.Function.Metadata.Namespace + "/" + d.Function.Metadata.Name
	case d.Plugin != nil:
		return "PluginManifest/" + d.Plugin.Metadata.Name
	case d.EvalSuite != nil:
		return "EvalSuite/" + d.EvalSuite.Metadata.Namespace + "/" + d.EvalSuite.Metadata.Name
	case d.Agent != nil:
		return "Agent/" + d.Agent.Metadata.Namespace + "/" + d.Agent.Metadata.Name
	case d.Trigger != nil:
		return "Trigger/" + d.Trigger.Metadata.Namespace + "/" + d.Trigger.Metadata.Name
	case d.Subject != nil:
		return "Subject/" + d.Subject.Metadata.Namespace + "/" + d.Subject.Metadata.Name
	case d.Funnel != nil:
		return "Funnel/" + d.Funnel.Metadata.Namespace + "/" + d.Funnel.Metadata.Name
	case d.Experiment != nil:
		return "Experiment/" + d.Experiment.Metadata.Namespace + "/" + d.Experiment.Metadata.Name
	case d.Queue != nil:
		return "HumanQueue/" + d.Queue.Metadata.Namespace + "/" + d.Queue.Metadata.Name
	case d.Team != nil:
		return "Team/" + d.Team.Metadata.Namespace + "/" + d.Team.Metadata.Name
	case d.Knowledge != nil:
		return "KnowledgeBase/" + d.Knowledge.Metadata.Namespace + "/" + d.Knowledge.Metadata.Name
	}
	return d.Kind
}

// Loader parses and schema-validates documents.
type Loader struct {
	schemas map[string]*jsv.Schema
}

// NewLoader compiles the embedded schemas.
func NewLoader() (*Loader, error) {
	l := &Loader{schemas: map[string]*jsv.Schema{}}
	for kind := range schemas.Kinds {
		raw, err := schemas.Get(kind)
		if err != nil {
			return nil, err
		}
		doc, err := jsv.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		c := jsv.NewCompiler()
		url := "tessera:///" + kind + ".json"
		if err := c.AddResource(url, doc); err != nil {
			return nil, err
		}
		s, err := c.Compile(url)
		if err != nil {
			return nil, fmt.Errorf("compile %s schema: %w", kind, err)
		}
		l.schemas[kind] = s
	}
	return l, nil
}

// LoadPaths loads every .yaml, .yml and .json file under the given files or
// directories.
func (l *Loader) LoadPaths(paths []string) ([]Doc, []Issue) {
	var docs []Doc
	var issues []Issue
	for _, p := range paths {
		files, err := expand(p)
		if err != nil {
			issues = append(issues, Issue{Severity: SeverityError, Code: "READ_ERROR", File: p, Message: err.Error()})
			continue
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				issues = append(issues, Issue{Severity: SeverityError, Code: "READ_ERROR", File: f, Message: err.Error()})
				continue
			}
			d, is := l.Load(f, data)
			docs = append(docs, d...)
			issues = append(issues, is...)
		}
	}
	return append(docs, BuiltinDocs()...), issues
}

func expand(p string) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{p}, nil
	}
	var out []string
	err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Like the Go tools, a directory walk skips testdata and hidden
		// directories: fixtures are not resources. A path given explicitly
		// is always loaded.
		if d.IsDir() && path != p && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".yaml", ".yml", ".json":
			if !d.IsDir() {
				out = append(out, path)
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// Limits that keep a hostile or careless file from exhausting admission.
const (
	MaxFileBytes    = 1 << 20 // 1 MiB per file
	MaxDocsPerFile  = 200
	MaxStringLength = 64 << 10 // 64 KiB per scalar value
)

var utf8BOM = []byte("\xef\xbb\xbf")

// Load parses one file, which may hold several YAML documents. Documents are
// split by a real YAML decoder, so "--- # comment", CRLF line endings and a
// byte-order mark all work; a document is never skipped without an issue.
func (l *Loader) Load(file string, data []byte) ([]Doc, []Issue) {
	failFile := func(code, msg, fix string) ([]Doc, []Issue) {
		return nil, []Issue{{Severity: SeverityError, Code: code, File: file, Message: msg, Fix: fix}}
	}
	if len(data) > MaxFileBytes {
		return failFile("TOO_LARGE", fmt.Sprintf("file is %d bytes; the limit is %d", len(data), MaxFileBytes), "split it into several files")
	}
	data = bytes.TrimPrefix(data, utf8BOM)

	var docs []Doc
	var issues []Issue
	dec := yamlv3.NewDecoder(bytes.NewReader(data))
	for index := 0; ; index++ {
		if index >= MaxDocsPerFile {
			return docs, append(issues, Issue{Severity: SeverityError, Code: "TOO_LARGE", File: file,
				Message: fmt.Sprintf("more than %d documents in one file", MaxDocsPerFile), Fix: "split it into several files"})
		}
		var node yamlv3.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// The decoder cannot continue past a syntax error.
			return docs, append(issues, Issue{Severity: SeverityError, Code: "PARSE_ERROR", File: fileRef(file, index), Message: err.Error(), Fix: "fix the YAML syntax"})
		}
		if isEmptyDocument(&node) {
			continue
		}
		if long := longScalar(&node); long != "" {
			issues = append(issues, Issue{Severity: SeverityError, Code: "TOO_LARGE", File: fileRef(file, index),
				Message: fmt.Sprintf("a value is longer than %d bytes (starts with %q)", MaxStringLength, long), Fix: "shorten it"})
			continue
		}
		js, err := toJSON(&node)
		if err != nil {
			issues = append(issues, Issue{Severity: SeverityError, Code: "PARSE_ERROR", File: fileRef(file, index), Message: err.Error(),
				Fix: "fix the YAML; each key may appear once"})
			continue
		}
		d, is := l.loadOne(file, index, js)
		issues = append(issues, is...)
		if d != nil {
			if d.Function != nil {
				d.Function.InputOrder = mappingKeys(&node, "spec", "inputs")
			}
			docs = append(docs, *d)
		}
	}
	return docs, issues
}

// mappingKeys returns the keys of the mapping at path, in written order.
func mappingKeys(n *yamlv3.Node, path ...string) []string {
	if n.Kind == yamlv3.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	for _, key := range path {
		if n.Kind != yamlv3.MappingNode {
			return nil
		}
		var next *yamlv3.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				next = n.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		n = next
	}
	if n.Kind != yamlv3.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i].Value)
	}
	return keys
}

func isEmptyDocument(n *yamlv3.Node) bool {
	if n.Kind == 0 {
		return true
	}
	if n.Kind == yamlv3.DocumentNode {
		return len(n.Content) == 0 || (n.Content[0].Kind == yamlv3.ScalarNode && n.Content[0].Tag == "!!null")
	}
	return false
}

// longScalar returns the start of the first scalar over MaxStringLength.
func longScalar(n *yamlv3.Node) string {
	if n.Kind == yamlv3.ScalarNode && len(n.Value) > MaxStringLength {
		return n.Value[:20]
	}
	for _, c := range n.Content {
		if s := longScalar(c); s != "" {
			return s
		}
	}
	return ""
}

func (l *Loader) loadOne(file string, index int, raw []byte) (*Doc, []Issue) {
	fail := func(code, path, msg, fix string) (*Doc, []Issue) {
		return nil, []Issue{{Severity: SeverityError, Code: code, File: fileRef(file, index), Path: path, Message: msg, Fix: fix}}
	}
	js := raw
	var head struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(js, &head); err != nil {
		return fail("PARSE_ERROR", "", "a resource must be an object with apiVersion and kind", "")
	}
	if head.APIVersion == "" && head.Kind == "" {
		return nil, []Issue{{Severity: SeverityWarning, Code: "NOT_A_RESOURCE", File: fileRef(file, index),
			Message: "no apiVersion or kind; skipped", Fix: "remove the file from the paths being validated, or add apiVersion and kind"}}
	}
	schema, ok := l.schemas[head.Kind]
	if !ok {
		kinds := make([]string, 0, len(l.schemas))
		for k := range l.schemas {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		return fail("UNKNOWN_KIND", "kind", fmt.Sprintf("unknown kind %q", head.Kind), "use one of "+strings.Join(kinds, ", "))
	}

	inst, err := jsv.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return fail("PARSE_ERROR", "", err.Error(), "")
	}
	if err := schema.Validate(inst); err != nil {
		var ve *jsv.ValidationError
		if !errorsAs(err, &ve) {
			return fail("SCHEMA_ERROR", "", err.Error(), "")
		}
		return nil, schemaIssues(fileRef(file, index), head.Kind, ve)
	}

	d := &Doc{File: file, Index: index, Kind: head.Kind}
	dec := json.NewDecoder(bytes.NewReader(js))
	dec.DisallowUnknownFields()
	// Numbers inside free-form values (with, state, caller) keep the form
	// they were written in: 7 is an int, 7.0 a float. Without this every
	// literal would reach plugins as a float.
	dec.UseNumber()
	switch head.Kind {
	case "Function":
		d.Function = &agents.Function{}
		err = dec.Decode(d.Function)
	case "PluginManifest":
		d.Plugin = &plugins.PluginManifest{}
		err = dec.Decode(d.Plugin)
	case "EvalSuite":
		d.EvalSuite = &agents.EvalSuite{}
		err = dec.Decode(d.EvalSuite)
	case "Agent":
		d.Agent = &agents.Agent{}
		err = dec.Decode(d.Agent)
	case "Trigger":
		d.Trigger = &agents.Trigger{}
		err = dec.Decode(d.Trigger)
	case "Subject":
		d.Subject = &agents.Subject{}
		err = dec.Decode(d.Subject)
	case "Funnel":
		d.Funnel = &agents.Funnel{}
		err = dec.Decode(d.Funnel)
	case "Experiment":
		d.Experiment = &agents.Experiment{}
		err = dec.Decode(d.Experiment)
	case "HumanQueue":
		d.Queue = &agents.HumanQueue{}
		err = dec.Decode(d.Queue)
	case "Team":
		d.Team = &agents.Team{}
		err = dec.Decode(d.Team)
	case "KnowledgeBase":
		d.Knowledge = &agents.KnowledgeBase{}
		err = dec.Decode(d.Knowledge)
	}
	if err != nil {
		return fail("DECODE_ERROR", "", err.Error(), "")
	}
	return d, nil
}

func fileRef(file string, index int) string {
	if index == 0 {
		return file
	}
	return fmt.Sprintf("%s#%d", file, index)
}

var (
	reAdditional = regexp.MustCompile(`additional properties? (.+) not allowed`)
	reMissing    = regexp.MustCompile(`missing propert(?:y|ies) (.+)`)
)

// schemaIssues turns a validation error into one issue per real cause. The
// library's flat output only carries a generic wrapper, so the detailed tree
// is walked and its leaves reported. A failing document always yields at
// least one issue: it is never dropped silently.
func schemaIssues(file, kind string, ve *jsv.ValidationError) []Issue {
	var leaves []jsv.OutputUnit
	var walk func(u jsv.OutputUnit)
	walk = func(u jsv.OutputUnit) {
		if len(u.Errors) == 0 {
			if u.Error != nil {
				leaves = append(leaves, u)
			}
			return
		}
		for _, c := range u.Errors {
			walk(c)
		}
	}
	walk(*ve.DetailedOutput())

	var issues []Issue
	seen := map[string]bool{}
	for _, u := range leaves {
		msg := u.Error.String()
		path := pointerToPath(u.InstanceLocation)
		key := path + "|" + msg
		if seen[key] {
			continue
		}
		seen[key] = true
		is := Issue{Severity: SeverityError, Code: "INVALID_FIELD", File: file, Path: path, Message: msg,
			Fix: fmt.Sprintf("see: tessera explain %s", explainPath(kind, path))}
		if m := reAdditional.FindStringSubmatch(msg); m != nil {
			is.Code, is.Message = "UNKNOWN_FIELD", "unknown field "+m[1]
			is.Fix = fmt.Sprintf("remove it or check the spelling; see: tessera explain %s", explainPath(kind, path))
		} else if m := reMissing.FindStringSubmatch(msg); m != nil {
			is.Code, is.Message = "MISSING_FIELD", "missing required field "+m[1]
		}
		issues = append(issues, is)
	}
	if len(issues) == 0 {
		issues = append(issues, Issue{Severity: SeverityError, Code: "INVALID_FIELD", File: file,
			Path: pointerToPath("/" + strings.Join(ve.InstanceLocation, "/")), Message: ve.Error()})
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
	return issues
}

// pointerToPath turns "/spec/inputs/newSlot/resolve/1/ask" into
// "spec.inputs.newSlot.resolve[1].ask".
func pointerToPath(ptr string) string {
	if ptr == "" || ptr == "/" {
		return ""
	}
	var b strings.Builder
	for _, seg := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		if _, err := strconv.Atoi(seg); err == nil {
			b.WriteString("[" + seg + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteString(".")
		}
		b.WriteString(seg)
	}
	return b.String()
}

// explainPath keeps only the schema path part: map keys and indexes differ
// per document and explain works on field names.
func explainPath(kind, path string) string {
	path = listIndex.ReplaceAllString(path, "")
	if path == "" {
		return kind
	}
	return kind + "." + path
}

var listIndex = regexp.MustCompile(`\[[0-9]+\]`)

func errorsAs(err error, target **jsv.ValidationError) bool {
	ve, ok := err.(*jsv.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}
