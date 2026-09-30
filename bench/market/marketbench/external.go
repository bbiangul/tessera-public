package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bbiangul/tessera/internal/ledger"
)

// external is a competitor served by bench/market/competitors/server.py.
type external struct {
	url, session   string
	ledger, global *ledger.Ledger
	price          price
	http           *http.Client
}

func newExternal(ctx context.Context, url, session string, t task, l, global *ledger.Ledger, p price) (*external, error) {
	e := &external{url: url, session: session, ledger: l, global: global, price: p, http: &http.Client{Timeout: 5 * time.Minute}}
	return e, e.post(ctx, "/session", map[string]any{"session": session, "patientId": t.PatientID, "patientName": t.PatientName}, nil)
}

func (e *external) Send(ctx context.Context, text, _ string) ([]string, error) {
	var out struct {
		Replies                   []string
		InputTokens, OutputTokens int
		Calls                     int
	}
	if e.global.Exceeded() || e.ledger.Exceeded() {
		return nil, ledger.ErrBudgetExceeded
	}
	start := time.Now()
	err := e.post(ctx, "/turn", map[string]any{"session": e.session, "text": text}, &out)
	// One ledger entry per turn, a failed one too (the server reports the
	// usage it had): tokens from the framework, priced like everyone's.
	en := ledger.Entry{Engine: "agent", Backend: "openrouter", InputTokens: out.InputTokens, OutputTokens: out.OutputTokens, Latency: time.Since(start)}
	en.CostUSD = e.price.costOf(en)
	_ = e.ledger.Add(en)
	if err != nil {
		return nil, err
	}
	for i := 1; i < out.Calls; i++ { // count the model calls
		_ = e.ledger.Add(ledger.Entry{Engine: "agent", Backend: "openrouter"})
	}
	return out.Replies, nil
}

func (e *external) Crash(ctx context.Context) error {
	return e.post(ctx, "/crash", map[string]any{"session": e.session}, nil)
}

func (e *external) Close() {}

func (e *external) post(ctx context.Context, path string, body, out any) error {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", e.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if out != nil {
			_ = json.Unmarshal(data, out) // usage, when the server had it
		}
		return fmt.Errorf("%s %d: %s", path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// price is the model's list price on OpenRouter, per token. Every
// competitor's DeepSeek calls are priced with it from their tokens, so
// costs compare; Jev's cost is added as Typesafe reports it.
type price struct{ in, out float64 }

func listPrice(ctx context.Context, key, model string) (price, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return price{}, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return price{}, err
	}
	for _, m := range out.Data {
		if m.ID == model {
			in, err1 := strconv.ParseFloat(m.Pricing.Prompt, 64)
			o, err2 := strconv.ParseFloat(m.Pricing.Completion, 64)
			if err1 != nil || err2 != nil {
				return price{}, fmt.Errorf("price of %s: %q %q", model, m.Pricing.Prompt, m.Pricing.Completion)
			}
			return price{in, o}, nil
		}
	}
	return price{}, fmt.Errorf("no model %s on OpenRouter", model)
}

// costOf prices an entry: DeepSeek by tokens at the list price, the rest
// (Jev) as reported.
func (p price) costOf(e ledger.Entry) float64 {
	if e.Backend == "openrouter" {
		return float64(e.InputTokens)*p.in + float64(e.OutputTokens)*p.out
	}
	return e.CostUSD
}
