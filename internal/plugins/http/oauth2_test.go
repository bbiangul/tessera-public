package http

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

// backoffice-004 (T9): connection.oauth2 gets one token by the
// client-credentials grant and reuses it; an API's 401 drops it; a token
// endpoint that refuses fails the call definitely, before it is sent.
func TestOAuth2ClientCredentials(t *testing.T) {
	var tokens, calls atomic.Int32
	var refuse atomic.Bool
	var lastAuth atomic.Value
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if refuse.Load() || r.PostForm.Get("client_secret") != "app-secret" || r.PostForm.Get("grant_type") != "client_credentials" || r.PostForm.Get("scope") != "https://api.botframework.com/.default" {
				w.WriteHeader(401)
				_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided."}`)
				return
			}
			n := tokens.Add(1)
			_, _ = io.WriteString(w, `{"token_type":"Bearer","expires_in":3599,"access_token":"tok-`+string(rune('0'+n))+`"}`)
		default:
			calls.Add(1)
			lastAuth.Store(r.Header.Get("Authorization"))
			if r.Header.Get("Authorization") == "Bearer tok-1" && calls.Load() == 3 {
				w.WriteHeader(401) // revoked
				return
			}
			_, _ = io.WriteString(w, `{"id":"ok"}`)
		}
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "teams", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress: &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL, OAuth2: &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "TEAMS_APP_ID", ClientSecret: "TEAMS_APP_PASSWORD",
			Scope: "https://api.botframework.com/.default"}},
		Actions: map[string]plugins.ActionSpec{"ping": {Effect: plugins.EffectExternalMessage, HTTP: &plugins.HTTPCall{Method: "POST", Path: "/v3/ping"}}},
	}}
	p, err := NewWithOptions(m, secrets{"TEAMS_APP_ID": "app-id", "TEAMS_APP_PASSWORD": "app-secret"}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	ctx := context.Background()
	call := func(k string) error {
		_, err := r.Call(ctx, act.Call{Action: "teams/ping", IdempotencyKey: k})
		return err
	}
	if err := call("a"); err != nil {
		t.Fatal(err)
	}
	if err := call("b"); err != nil || tokens.Load() != 1 || lastAuth.Load() != "Bearer tok-1" {
		t.Fatalf("one token for both calls: %v, %d tokens, %v", err, tokens.Load(), lastAuth.Load())
	}
	if err := call("c"); err == nil {
		t.Fatal("the API refused the token")
	}
	if err := call("d"); err != nil || tokens.Load() != 2 || lastAuth.Load() != "Bearer tok-2" {
		t.Fatalf("after a 401 a new token: %v, %d tokens, %v", err, tokens.Load(), lastAuth.Load())
	}
	// A refused token request: the call is not sent, and fails definitely
	// with the endpoint's reason, never the secret.
	forgetToken(p)
	refuse.Store(true)
	before := calls.Load()
	err = call("e")
	var definite *act.DefiniteError
	if !errors.As(err, &definite) || definite.Code != "AUTH" || calls.Load() != before {
		t.Fatalf("a refused token fails definitely before the call: %v (calls %d→%d)", err, before, calls.Load())
	}
	if strings.Contains(err.Error(), "app-secret") || !strings.Contains(err.Error(), "Invalid client secret") {
		t.Errorf("the reason, without the secret: %v", err)
	}
}

// backoffice-005 (E8): with refreshToken the grant is refresh_token
// (Gmail acting as the mailbox that consented once).
func TestOAuth2RefreshToken(t *testing.T) {
	var tokens atomic.Int32
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != "1//refresh" || r.PostForm.Get("client_id") != "cid" {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Bad Request"}`)
				return
			}
			tokens.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"ya29.x","expires_in":3599,"scope":"https://www.googleapis.com/auth/gmail.send","token_type":"Bearer"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer ya29.x" {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{"id":"m1"}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "email", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress: &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL, OAuth2: &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "GMAIL_CLIENT_ID", ClientSecret: "GMAIL_CLIENT_SECRET",
			RefreshToken: "GMAIL_REFRESH_TOKEN"}},
		Actions: map[string]plugins.ActionSpec{"send": {Effect: plugins.EffectExternalMessage, HTTP: &plugins.HTTPCall{Method: "POST", Path: "/send"}}},
	}}
	p, err := NewWithOptions(m, secrets{"GMAIL_CLIENT_ID": "cid", "GMAIL_CLIENT_SECRET": "cs", "GMAIL_REFRESH_TOKEN": "1//refresh"}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	for _, k := range []string{"a", "b"} {
		if _, err := r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: k}); err != nil {
			t.Fatal(err)
		}
	}
	if tokens.Load() != 1 {
		t.Errorf("one token for both sends: %d", tokens.Load())
	}
}

// forgetToken drops whatever token is cached (tests).
func forgetToken(p *Plugin) {
	p.oauth.mu.Lock()
	defer p.oauth.mu.Unlock()
	p.oauth.token = ""
}
