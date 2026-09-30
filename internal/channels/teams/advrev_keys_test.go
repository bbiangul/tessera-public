package teams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"crypto/rand"
	"crypto/rsa"
)

// ADVREV-1: a failing (or slow) key fetch is retried on every request,
// under the keySet mutex, so an unauthenticated caller turns each request
// into outbound fetches and serializes every other Teams request behind them.
func TestAdvrevKeyFetchNoBackoff(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	var openidHits atomic.Int32
	var fail atomic.Bool
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openid", func(w http.ResponseWriter, _ *http.Request) {
		openidHits.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jwks_uri": srv.URL + "/keys"})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "k1", "endorsements": []string{"msteams"},
			"n": base64.RawURLEncoding.EncodeToString(priv.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())}}})
	})
	srv = httptest.NewTLSServer(mux)
	defer srv.Close()

	clock := time.Now()
	var mu sync.Mutex
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	ks := &keySet{openID: srv.URL + "/openid", client: srv.Client(), now: now}
	ctx := context.Background()
	if _, _, err := ks.get(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	// 6 minutes later Microsoft's endpoint is down; an attacker sends 50
	// requests with a made-up kid (no valid token needed: get runs before
	// the signature check).
	mu.Lock()
	clock = clock.Add(6 * time.Minute)
	mu.Unlock()
	fail.Store(true)
	before := openidHits.Load()
	for i := 0; i < 50; i++ {
		_, _, _ = ks.get(ctx, "attacker-kid")
	}
	t.Logf("unknown kid, fetch failing: 50 requests -> %d outbound fetches", openidHits.Load()-before)

	// After 24h with the endpoint down, every legitimate request refetches too.
	mu.Lock()
	clock = clock.Add(24 * time.Hour)
	mu.Unlock()
	before = openidHits.Load()
	for i := 0; i < 10; i++ {
		if _, _, err := ks.get(ctx, "k1"); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("known kid, keys >24h old, fetch failing: 10 requests -> %d outbound fetches", openidHits.Load()-before)
	if n := openidHits.Load() - before; n > 1 {
		t.Errorf("no backoff after a failed fetch: %d fetches for 10 requests", n)
	}
}

// ADVREV-1b: the mutex is held during the network fetch: a legitimate
// request with a known, fresh key waits for an attacker-triggered fetch.
func TestAdvrevKeyFetchBlocksKnownKey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	var slow atomic.Bool
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openid", func(w http.ResponseWriter, r *http.Request) {
		if slow.Load() {
			time.Sleep(3 * time.Second)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jwks_uri": srv.URL + "/keys"})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "k1", "endorsements": []string{"msteams"},
			"n": base64.RawURLEncoding.EncodeToString(priv.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())}}})
	})
	srv = httptest.NewTLSServer(mux)
	defer srv.Close()
	clock := time.Now()
	ks := &keySet{openID: srv.URL + "/openid", client: srv.Client(), now: func() time.Time { return clock }}
	ctx := context.Background()
	if _, _, err := ks.get(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(6 * time.Minute)
	slow.Store(true)
	go func() { _, _, _ = ks.get(ctx, "attacker-kid") }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if _, _, err := ks.get(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	t.Logf("a legitimate request with a fresh known key waited %v", d)
	if d > time.Second {
		t.Errorf("known-key verification blocked behind an attacker-triggered fetch for %v", d)
	}
}
