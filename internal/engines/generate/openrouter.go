package generate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bbiangul/tessera/internal/httpx"
)

// OpenRouter generates with any chat model behind OpenRouter's
// OpenAI-compatible API.
type OpenRouter struct {
	APIKey  string
	Model   string // e.g. "deepseek/deepseek-v4.1-flash"
	BaseURL string // default https://openrouter.ai/api/v1
	// Providers pins the upstream providers, in order, with no fallback.
	Providers []string
	// Temperature; default 0.
	Temperature float64
	// StructuredOutputs asks the provider to constrain decoding to the
	// schema (response_format json_schema). The reply is validated locally
	// either way.
	StructuredOutputs bool
	// KeepReasoning leaves the model's reasoning on; off by default, since
	// extraction does not need it and it multiplies cost and latency.
	KeepReasoning bool
	Client        *httpx.Client
}

func (o *OpenRouter) Name() string { return "openrouter" }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Choices  []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
}

const systemText = "You are a component of a software system. Follow the INSTRUCTIONS using only the STATE. " +
	"Do not invent facts that are not in the STATE."

const systemJSON = systemText + " Reply with only one JSON value that matches the SCHEMA: no prose, no code fence."

// Prompt renders the user message for a request.
func Prompt(req Request) (string, error) {
	state, err := json.Marshal(req.State)
	if err != nil {
		return "", fmt.Errorf("%w: state is not JSON: %v", ErrInvalidRequest, err)
	}
	p := "INSTRUCTIONS:\n" + req.Instructions + "\n\nSTATE (JSON):\n" + string(state)
	if req.Schema != nil {
		schema, _ := json.Marshal(req.Schema)
		p += "\n\nSCHEMA (JSON Schema):\n" + string(schema)
	}
	return p, nil
}

func (o *OpenRouter) Generate(ctx context.Context, req Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	prompt, err := Prompt(req)
	if err != nil {
		return nil, err
	}
	system := systemText
	if req.Schema != nil {
		system = systemJSON
	}
	msgs := []message{{Role: "system", Content: system}, {Role: "user", Content: prompt}}
	start := time.Now()
	resp := &Response{}
	for attempt := 1; attempt <= 2; attempt++ {
		resp.Attempts = attempt
		out, st, err := o.call(ctx, req, msgs)
		if err == nil {
			resp.Usage.UnreportedCalls += st.MaybeBilled
		}
		resp.Model, resp.Provider = out.Model, out.Provider
		resp.Usage.InputTokens += out.Usage.PromptTokens
		resp.Usage.OutputTokens += out.Usage.CompletionTokens
		resp.Usage.CostUSD += out.Usage.Cost
		resp.Latency = time.Since(start)
		if err != nil {
			return resp, err
		}
		if len(out.Choices) == 0 || out.Choices[0].Message.Content == nil {
			return resp, errors.New("generate: empty reply")
		}
		reply := *out.Choices[0].Message.Content
		if out.Choices[0].FinishReason == "length" {
			return resp, fmt.Errorf("generate: reply cut at %d tokens", maxTokens(req))
		}
		if req.Schema == nil {
			resp.Text = reply
			return resp, nil
		}
		v, perr := ParseObject(reply, req.Schema)
		if perr == nil {
			resp.Value = v
			return resp, nil
		}
		if attempt == 2 {
			return resp, perr
		}
		// One repair: show the model its reply and what failed.
		msgs = append(msgs,
			message{Role: "assistant", Content: reply},
			message{Role: "user", Content: "That reply is not valid: " + perr.Error() + ". Reply again with only the corrected JSON."})
	}
	return resp, nil // unreachable
}

func maxTokens(req Request) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 1024
}

func (o *OpenRouter) call(ctx context.Context, req Request, msgs []message) (chatResponse, httpx.Stats, error) {
	base := o.BaseURL
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	body := map[string]any{
		"model":       o.Model,
		"temperature": o.Temperature,
		"max_tokens":  maxTokens(req),
		"usage":       map[string]any{"include": true},
		"messages":    msgs,
	}
	if !o.KeepReasoning {
		body["reasoning"] = map[string]any{"enabled": false}
	}
	if req.Schema != nil && o.StructuredOutputs {
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "output", "strict": true, "schema": req.Schema},
		}
	}
	if len(o.Providers) > 0 {
		body["provider"] = map[string]any{"order": o.Providers, "allow_fallbacks": false}
	}
	client := o.Client
	if client == nil {
		client = &httpx.Client{}
	}
	var out chatResponse
	st, err := client.PostJSON(ctx, base+"/chat/completions", map[string]string{"Authorization": "Bearer " + o.APIKey}, body, &out)
	return out, st, err
}
