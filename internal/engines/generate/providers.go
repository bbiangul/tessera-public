package generate

import "strings"

// DefaultProviders pin a model to the providers it was measured on when a
// spec names none. On 2026-09-25 OpenRouter served deepseek-v4.1-flash
// through 27 providers (fp4, fp8, full precision); unpinned, one run of the
// extraction benchmark went through 6 of them and never DeepSeek, which
// every benchmark had measured, and the cheapest provider got 76.7% of the
// records right against 98.9–100% (docs/research/provider-001.md, #5).
var DefaultProviders = map[string][]string{
	"deepseek/deepseek-v4.1-flash": {"DeepSeek"},
}

// AnyProvider, as "model@any", leaves the choice to OpenRouter.
const AnyProvider = "any"

// ParseModel splits "model[@Provider[,Provider]]". Without "@", a model in
// DefaultProviders is pinned to them; "@any" pins nothing. pinned says
// whether the providers came from DefaultProviders.
func ParseModel(spec string) (model string, providers []string, defaulted bool) {
	model, prov, ok := strings.Cut(spec, "@")
	switch {
	case !ok:
		if p := DefaultProviders[model]; len(p) > 0 {
			return model, append([]string(nil), p...), true
		}
		return model, nil, false
	case prov == "" || strings.EqualFold(prov, AnyProvider):
		return model, nil, false
	default:
		return model, strings.Split(prov, ","), false
	}
}
