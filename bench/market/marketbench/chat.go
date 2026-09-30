package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bbiangul/tessera/internal/ledger"
)

// chatClient calls OpenRouter's chat completions with tools: the patient
// simulator and the plain tool-calling loop use it. Every call is metered
// in the ledger under engine.
type chatClient struct {
	key, model string
	providers  []string
	ledger     *ledger.Ledger // this run's
	global     *ledger.Ledger // the budget
	http       *http.Client
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatReply struct {
	Message chatMessage
	Usage   ledger.Entry
}

func (c *chatClient) complete(ctx context.Context, engine string, msgs []chatMessage, tools []map[string]any) (chatReply, error) {
	if c.global != nil && c.global.Exceeded() {
		return chatReply{}, ledger.ErrBudgetExceeded
	}
	body := map[string]any{"model": c.model, "temperature": 0, "max_tokens": 1024, "messages": msgs,
		"usage": map[string]any{"include": true}, "reasoning": map[string]any{"enabled": false}}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if len(c.providers) > 0 {
		body["provider"] = map[string]any{"order": c.providers, "allow_fallbacks": false}
	}
	raw, _ := json.Marshal(body)
	start := time.Now()
	var out struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
		Choices  []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			Cost             float64 `json:"cost"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			lastErr = fmt.Errorf("openrouter %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return chatReply{}, fmt.Errorf("openrouter: %w", err)
		}
		if out.Error != nil || len(out.Choices) == 0 {
			return chatReply{}, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
		}
		e := ledger.Entry{Engine: engine, Backend: "openrouter", Model: out.Model, Provider: out.Provider,
			InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens, CostUSD: out.Usage.Cost, Latency: time.Since(start)}
		_ = c.ledger.Add(e)
		return chatReply{Message: out.Choices[0].Message, Usage: e}, nil
	}
	return chatReply{}, lastErr
}
