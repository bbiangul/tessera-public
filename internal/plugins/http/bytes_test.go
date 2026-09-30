package http

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

// hostsClient sends every host to srv, so tests can use real names
// (graph.test, dl.files.test) on one local TLS server.
func hostsClient(srv *httptest.Server) *nethttp.Client {
	addr := srv.Listener.Addr().String()
	return &nethttp.Client{Transport: &nethttp.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // a local test server
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
}

// plugin-001: a bytes binding returns the body as base64, follows
// redirects to allowed hosts without credentials on another host, and
// refuses the rest; a JSON binding still follows none.
func TestBytesAndRedirects(t *testing.T) {
	var mu sync.Mutex
	auth := map[string]string{} // host -> Authorization seen
	loops := 0
	srv := httptest.NewTLSServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		mu.Lock()
		auth[r.Host] = r.Header.Get("Authorization")
		mu.Unlock()
		switch {
		case r.URL.Path == "/file":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte{0, 1, 2, 'p', 'd', 'f'})
		case r.URL.Path == "/moved":
			nethttp.Redirect(w, r, "https://dl.files.test/file", nethttp.StatusFound)
		case r.URL.Path == "/elsewhere":
			nethttp.Redirect(w, r, "https://evil.test/file", nethttp.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/loop"):
			mu.Lock()
			loops++
			mu.Unlock()
			nethttp.Redirect(w, r, "https://graph.test/loop"+r.URL.Path, nethttp.StatusFound)
		case r.URL.Path == "/json-moved":
			nethttp.Redirect(w, r, "https://graph.test/file", nethttp.StatusFound)
		}
	}))
	defer srv.Close()
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "files", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{"graph.test", "*.files.test"}},
		Connection: &plugins.Connection{BaseURL: "https://graph.test", Auth: &plugins.Auth{Header: "Authorization", Secret: "TOKEN", Prefix: "Bearer "}},
		Actions: map[string]plugins.ActionSpec{
			"get":       {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/file", Response: "bytes"}},
			"moved":     {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/moved", Response: "bytes"}},
			"elsewhere": {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/elsewhere", Response: "bytes"}},
			"loop":      {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/loop", Response: "bytes"}},
			"json":      {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/json-moved"}},
		},
	}}
	p, err := NewWithOptions(m, secrets{"TOKEN": "s3cret"}, hostsClient(srv), local)
	if err != nil {
		t.Fatal(err)
	}
	p.Backoff = 0
	r := act.NewRegistry()
	p.Register(r)
	call := func(a string) (any, error) {
		return r.Call(context.Background(), act.Call{Action: "files/" + a, Args: map[string]any{}})
	}
	out, err := call("get")
	body, _ := out.(map[string]any)
	if err != nil || body["base64"] != base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 'p', 'd', 'f'}) || body["contentType"] != "application/pdf" || body["size"] != int64(6) {
		t.Fatalf("bytes: %v %v", out, err)
	}
	if out, err = call("moved"); err != nil || out.(map[string]any)["size"] != int64(6) {
		t.Fatalf("redirect: %v %v", out, err)
	}
	mu.Lock()
	if auth["graph.test"] != "Bearer s3cret" || auth["dl.files.test"] != "" {
		t.Errorf("credentials per host: %v", auth)
	}
	mu.Unlock()
	if _, err := call("elsewhere"); err == nil {
		t.Errorf("a redirect outside egress was followed")
	}
	mu.Lock()
	loops = 0
	mu.Unlock()
	if _, err := call("loop"); err == nil {
		t.Errorf("more than 3 redirects: %v", err)
	}
	mu.Lock()
	if loops != 4*readAttempts { // each attempt: the first request and 3 redirects
		t.Errorf("redirects followed: %d requests", loops)
	}
	mu.Unlock()
	if _, err := call("json"); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("a JSON binding followed a redirect: %v", err)
	}
}

// plugin-001: the bytes client keeps the private-address refusal.
func TestBytesRefusesPrivate(t *testing.T) {
	srv := httptest.NewTLSServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { _, _ = w.Write([]byte("x")) }))
	defer srv.Close()
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "files", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{"127.0.0.1"}},
		Connection: &plugins.Connection{BaseURL: srv.URL},
		Actions:    map[string]plugins.ActionSpec{"get": {Output: "map", Effect: plugins.EffectRead, HTTP: &plugins.HTTPCall{Method: "GET", Path: "/f", Response: "bytes"}}},
	}}
	p, err := NewWithOptions(m, secrets{}, srv.Client(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	p.Backoff = 0
	r := act.NewRegistry()
	p.Register(r)
	if _, err := r.Call(context.Background(), act.Call{Action: "files/get", Args: map[string]any{}}); err == nil {
		t.Errorf("a private address was reached")
	}
}
