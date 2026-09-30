package journal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The codec stores the plain Go values expressions work with (see
// expr.Eval) as JSON that decodes back to the same types. Plain JSON would
// turn int64 into float64 and time.Time into a string, and a replayed run
// would then see different values than the original run and could take a
// different path. Records are always decoded before use, on the first run
// too, so both runs see exactly the same values.
//
// Encoding:
//
//	int64, int, uint64 -> 12           float64 -> 12.0 (always a '.' or 'e')
//	time.Time -> {"$time": "RFC 3339 with nanoseconds and offset"}
//	time.Duration -> {"$duration": "1h30m0s"}
//	[]byte -> {"$bytes": "base64"}
//	map keys starting with "$" are escaped with a second "$".

// ErrUnsupported is returned for values the codec cannot represent.
var ErrUnsupported = errors.New("value cannot be recorded")

// Encode returns the canonical encoding of v. Map keys are sorted, so equal
// values have equal encodings.
func Encode(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := encode(&b, v, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// maxDepth bounds nesting, so a cyclic or hostile value cannot recurse
// without end.
const maxDepth = 64

func encode(b *bytes.Buffer, v any, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("%w: nested deeper than %d", ErrUnsupported, maxDepth)
	}
	switch v := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case string:
		writeString(b, v)
	case int:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case int64:
		b.WriteString(strconv.FormatInt(v, 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case uint64:
		b.WriteString(strconv.FormatUint(v, 10))
	case json.Number: // a literal from a definition, kept as written
		if _, err := strconv.ParseFloat(v.String(), 64); err != nil {
			return fmt.Errorf("%w: number %q", ErrUnsupported, v)
		}
		b.WriteString(v.String())
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: %v", ErrUnsupported, v)
		}
		s := strconv.FormatFloat(v, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		b.WriteString(s)
	case time.Time:
		b.WriteString(`{"$time":`)
		writeString(b, v.Format(time.RFC3339Nano))
		b.WriteByte('}')
	case time.Duration:
		b.WriteString(`{"$duration":`)
		writeString(b, v.String())
		b.WriteByte('}')
	case []byte:
		b.WriteString(`{"$bytes":`)
		writeString(b, base64.StdEncoding.EncodeToString(v))
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, x := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encode(b, x, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if strings.HasPrefix(k, "$") {
				writeString(b, "$"+k)
			} else {
				writeString(b, k)
			}
			b.WriteByte(':')
			if err := encode(b, v[k], depth+1); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("%w: type %T", ErrUnsupported, v)
	}
	return nil
}

func writeString(b *bytes.Buffer, s string) {
	out, _ := json.Marshal(s) // cannot fail for a string
	b.Write(out)
}

// Decode reverses Encode.
func Decode(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var raw any
	if err := d.Decode(&raw); err != nil {
		return nil, err
	}
	if d.More() {
		return nil, errors.New("trailing data after the value")
	}
	return decode(raw)
}

func decode(v any) (any, error) {
	switch v := v.(type) {
	case json.Number:
		s := v.String()
		if strings.ContainsAny(s, ".eE") {
			return strconv.ParseFloat(s, 64)
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i, nil
		}
		return strconv.ParseUint(s, 10, 64)
	case []any:
		for i := range v {
			x, err := decode(v[i])
			if err != nil {
				return nil, err
			}
			v[i] = x
		}
		return v, nil
	case map[string]any:
		if len(v) == 1 {
			for k, x := range v {
				if s, ok := x.(string); ok {
					switch k {
					case "$time":
						return time.Parse(time.RFC3339Nano, s)
					case "$duration":
						return time.ParseDuration(s)
					case "$bytes":
						return base64.StdEncoding.DecodeString(s)
					}
				}
			}
		}
		out := make(map[string]any, len(v))
		for k, x := range v {
			dx, err := decode(x)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(k, "$$") {
				k = k[1:]
			} else if strings.HasPrefix(k, "$") {
				return nil, fmt.Errorf("unknown tag %q", k)
			}
			out[k] = dx
		}
		return out, nil
	}
	return v, nil
}

// Normalize returns v as it will look after a round trip through the
// journal. Code that records a value uses the normalized one from then on.
func Normalize(v any) (any, error) {
	data, err := Encode(v)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}
