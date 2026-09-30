package expr

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Value is a literal field value that may contain "${ expr }" strings, at any
// depth inside maps and lists (RFC-0001: rule 2).
type Value struct {
	lit   any
	exprs map[string]*Expr // by path, for References
	tree  any              // lit with *Expr in place of "${ }" strings
}

// CompileValue walks v and compiles every string whose entire content is
// "${ expr }". A string that contains "${" anywhere else is an error: values
// are never half-substituted, which is how the legacy platform engine ended up
// splicing user text into shell scripts.
func (e *Engine) CompileValue(v any) (*Value, error) {
	val := &Value{lit: v, exprs: map[string]*Expr{}}
	tree, err := e.compileTree(v, "$", val.exprs)
	if err != nil {
		return nil, err
	}
	if len(val.exprs) > MaxInterpolations {
		return nil, fmt.Errorf("more than %d ${ } expressions in one value", MaxInterpolations)
	}
	val.tree = tree
	return val, nil
}

func (e *Engine) compileTree(v any, path string, exprs map[string]*Expr) (any, error) {
	switch v := v.(type) {
	case string:
		inner, isExpr, err := splitDollar(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if !isExpr {
			return v, nil
		}
		x, err := e.Compile(inner, Any)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		exprs[path] = x
		return x, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			c, err := e.compileTree(item, path+"."+k, exprs)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			c, err := e.compileTree(item, fmt.Sprintf("%s[%d]", path, i), exprs)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case json.Number:
		return Number(v), nil
	}
	return v, nil // bools, nil: literal
}

// Number converts a JSON number literal the way it was written: 7 is an
// int64, 7.0 and 7.5 are float64.
func Number(n json.Number) any {
	if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
		return i
	}
	f, _ := n.Float64()
	return f
}

// splitDollar returns the inner expression when s is exactly "${ ... }".
func splitDollar(s string) (string, bool, error) {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "${") && strings.HasSuffix(t, "}") && strings.Count(t, "${") == 1 {
		return t[2 : len(t)-1], true, nil
	}
	if strings.Contains(s, "${") {
		return "", false, fmt.Errorf("%q mixes text and ${ }: an expression must be the whole value; build text inside it, e.g. \"${ 'prefix-' + inputs.id }\"", s)
	}
	return "", false, nil
}

// Resolve evaluates every expression and returns the value with results in
// place. Results keep their type: "${ inputs.days }" can resolve to an int.
func (v *Value) Resolve(ctx context.Context, act Activation) (any, error) {
	return resolveTree(ctx, v.tree, act)
}

func resolveTree(ctx context.Context, v any, act Activation) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch v := v.(type) {
	case *Expr:
		return v.Eval(ctx, act)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			r, err := resolveTree(ctx, item, act)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			r, err := resolveTree(ctx, item, act)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

// References lists the scope paths read by all expressions in the value.
func (v *Value) References() []string {
	set := map[string]bool{}
	for _, x := range v.exprs {
		for _, r := range x.References() {
			set[r] = true
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
