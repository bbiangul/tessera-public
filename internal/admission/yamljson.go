package admission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	yamlv3 "go.yaml.in/yaml/v3"
)

// toJSON converts a YAML 1.2 document to JSON. Doing it on the node, and
// not through a YAML 1.1 library, keeps y, n, on, off, yes and no (and the
// country code NO) as the strings they are; a key written twice is an
// error, never last-wins; numbers keep the form they were written in.
func toJSON(n *yamlv3.Node) ([]byte, error) {
	var b bytes.Buffer
	if err := writeJSON(&b, n, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// maxYAMLDepth bounds nesting, aliases included (a "billion laughs"
// document expands aliases exponentially).
const maxYAMLDepth = 100

func writeJSON(b *bytes.Buffer, n *yamlv3.Node, depth int) error {
	if depth > maxYAMLDepth {
		return fmt.Errorf("line %d: nested deeper than %d (aliases included)", n.Line, maxYAMLDepth)
	}
	switch n.Kind {
	case yamlv3.DocumentNode:
		if len(n.Content) == 0 {
			b.WriteString("null")
			return nil
		}
		return writeJSON(b, n.Content[0], depth+1)
	case yamlv3.AliasNode:
		return writeJSON(b, n.Alias, depth+1)
	case yamlv3.MappingNode:
		b.WriteByte('{')
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yamlv3.ScalarNode {
				return fmt.Errorf("line %d: a key must be a plain value", k.Line)
			}
			if k.Value == "<<" && k.ShortTag() == "!!merge" {
				return fmt.Errorf("line %d: merge keys (<<) are not supported; write the fields", k.Line)
			}
			if seen[k.Value] {
				return fmt.Errorf("line %d: key %q is written twice in the same mapping", k.Line, k.Value)
			}
			seen[k.Value] = true
			if i > 0 {
				b.WriteByte(',')
			}
			key, _ := json.Marshal(k.Value)
			b.Write(key)
			b.WriteByte(':')
			if err := writeJSON(b, v, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	case yamlv3.SequenceNode:
		b.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, c, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case yamlv3.ScalarNode:
		return writeScalar(b, n)
	}
	return fmt.Errorf("line %d: unsupported YAML node", n.Line)
}

func writeScalar(b *bytes.Buffer, n *yamlv3.Node) error {
	switch n.ShortTag() {
	case "!!null":
		b.WriteString("null")
	case "!!bool":
		v, err := strconv.ParseBool(strings.ToLower(n.Value))
		if err != nil {
			return fmt.Errorf("line %d: %q is not a boolean", n.Line, n.Value)
		}
		b.WriteString(strconv.FormatBool(v))
	case "!!int":
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return fmt.Errorf("line %d: %q is not an integer that fits in 64 bits", n.Line, n.Value)
		}
		b.WriteString(strconv.FormatInt(i, 10))
	case "!!float":
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return fmt.Errorf("line %d: %q is not a finite number", n.Line, n.Value)
		}
		// Keep the written form when it is valid JSON (7.0 stays a float).
		if json.Valid([]byte(n.Value)) {
			b.WriteString(n.Value)
		} else {
			s := strconv.FormatFloat(f, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			b.WriteString(s)
		}
	default: // !!str, !!timestamp, !!binary: text as written
		s, _ := json.Marshal(n.Value)
		b.Write(s)
	}
	return nil
}
