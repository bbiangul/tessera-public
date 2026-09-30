package http

import (
	"context"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

func advPlugin(t *testing.T, h nethttp.HandlerFunc, refresh bool) (*Plugin, *act.Registry) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	o := &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "CID", ClientSecret: "CSECRET"}
	if refresh {
		o.RefreshToken = "RT"
	}
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "email", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL, OAuth2: o},
		Actions:    map[string]plugins.ActionSpec{"send": {Effect: plugins.EffectExternalMessage, HTTP: &plugins.HTTPCall{Method: "POST", Path: "/send"}}},
	}}
	p, err := NewWithOptions(m, secrets{"CID": "cid", "CSECRET": "super-secret-value", "RT": "1//refresh-v1"}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	return p, r
}

// ADV-4: a 401 answered to a request that carried an OLD token drops the
// token another goroutine just fetched (dropToken is unconditional).
func TestAdvStale401DropsFreshToken(t *testing.T) {
	var n atomic.Int32
	var gate = make(chan struct{})
	var inflight = make(chan struct{})
	p, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if req.URL.Path == "/token" {
			k := n.Add(1)
			fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, k)
			return
		}
		if req.Header.Get("Authorization") == "Bearer tok-1" {
			if req.Header.Get("Idempotency-Key") == "slow" {
				close(inflight)
				<-gate // held until tok-2 was fetched
			}
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}, false)
	ctx := context.Background()
	done := make(chan struct{})
	go func() { _, _ = r.Call(ctx, act.Call{Action: "email/send", IdempotencyKey: "slow"}); close(done) }()
	<-inflight
	_, _ = r.Call(ctx, act.Call{Action: "email/send", IdempotencyKey: "fast"}) // 401 with tok-1 -> drop
	if _, err := r.Call(ctx, act.Call{Action: "email/send", IdempotencyKey: "fresh"}); err != nil {
		t.Fatal(err) // fetched tok-2, works
	}
	close(gate)
	<-done // the slow tok-1 request now 401s and drops tok-2
	p.oauth.mu.Lock()
	cur := p.oauth.token
	p.oauth.mu.Unlock()
	t.Logf("token fetches=%d, cached token after stale 401=%q", n.Load(), cur)
	if cur != "tok-2" {
		t.Errorf("DEFECT: a stale 401 (request sent with tok-1) dropped the freshly fetched tok-2")
	}
}

// ADV-5: a rotated refresh_token in the token response is ignored; with a
// provider that rotates (and invalidates the old one), the second refresh
// fails and every later call fails.
func TestAdvRefreshTokenRotationIgnored(t *testing.T) {
	var mu sync.Mutex
	valid := "1//refresh-v1"
	gen := 0
	_, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if req.URL.Path == "/token" {
			_ = req.ParseForm()
			mu.Lock()
			defer mu.Unlock()
			if req.PostForm.Get("refresh_token") != valid {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"refresh token already used"}`)
				return
			}
			gen++
			valid = fmt.Sprintf("1//refresh-v%d", gen+1)
			fmt.Fprintf(w, `{"access_token":"at-%d","expires_in":61,"refresh_token":%q}`, gen, valid)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}, true)
	ctx := context.Background()
	if _, err := r.Call(ctx, act.Call{Action: "email/send", IdempotencyKey: "a"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // life = 61s - 60s = 1s
	_, err := r.Call(ctx, act.Call{Action: "email/send", IdempotencyKey: "b"})
	t.Logf("second refresh: %v", err)
	if err != nil {
		t.Errorf("DEFECT: the rotated refresh_token was not kept")
	}
}

// ADV-6: the token endpoint's text/JSON error is echoed into the error
// message: an endpoint (or a WAF/proxy in front of it) that echoes the
// request leaks client_secret / refresh_token into logs and to the agent.
func TestAdvTokenErrorEchoesSecret(t *testing.T) {
	for _, body := range []string{
		"", // text echo
		`{"error":"invalid_request: grant_type=refresh_token&client_secret=%s"}`,
	} {
		_, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
			b, _ := io.ReadAll(req.Body)
			w.WriteHeader(400)
			if body == "" {
				_, _ = io.WriteString(w, "bad request: "+string(b))
			} else {
				fmt.Fprintf(w, body, req.FormValue("client_secret"))
			}
		}, true)
		_, err := r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: "a"})
		t.Logf("error: %v", err)
		if strings.Contains(err.Error(), "super-secret-value") || strings.Contains(err.Error(), "refresh-v1") {
			t.Errorf("DEFECT: the error message carries a credential")
		}
	}
}

// ADV-7: error_description is neither length-bounded nor single-lined
// (only "\r\n" is cut), unlike errorDetail's 300 bytes.
func TestAdvTokenErrorDescriptionUnbounded(t *testing.T) {
	_, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		w.WriteHeader(400)
		fmt.Fprintf(w, `{"error":"x","error_description":%q}`, "line1\nFAKE LOG LINE level=error\n"+strings.Repeat("A", 60000))
	}, false)
	_, err := r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: "a"})
	t.Logf("error length %d, has newline %v", len(err.Error()), strings.Contains(err.Error(), "\n"))
	if len(err.Error()) > 1000 || strings.Contains(err.Error(), "\n") {
		t.Errorf("DEFECT: unbounded, multi-line error_description")
	}
}

// ADV-8: expires_in as a JSON string (Entra v1 endpoint, some IdPs) is
// read as "no access_token"; a response without expires_in caches nothing.
func TestAdvExpiresInShapes(t *testing.T) {
	var fetches atomic.Int32
	_, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if req.URL.Path == "/token" {
			fetches.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"at","token_type":"Bearer","expires_in":"3599"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}, false)
	_, err := r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: "a"})
	t.Logf("string expires_in: %v", err)
	if err != nil {
		t.Errorf("DEFECT: a valid access_token refused because expires_in is a string")
	}
	var n atomic.Int32
	_, r2 := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if req.URL.Path == "/token" {
			n.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"at","token_type":"Bearer"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}, false)
	for i := 0; i < 5; i++ {
		_, _ = r2.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: fmt.Sprint(i)})
	}
	t.Logf("no expires_in: %d token fetches for 5 calls", n.Load())
	if n.Load() > 1 {
		t.Errorf("DEFECT: without expires_in the token is fetched on every call")
	}
}

// Held: concurrent calls with drops under -race; one fetch shared.
func TestAdvConcurrentBearerRace(t *testing.T) {
	var fetches atomic.Int32
	p, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		if req.URL.Path == "/token" {
			fetches.Add(1)
			time.Sleep(20 * time.Millisecond)
			_, _ = io.WriteString(w, `{"access_token":"at","expires_in":3600}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}, false)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%10 == 0 {
				forgetToken(p)
			}
			_, _ = r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: fmt.Sprint(i)})
		}(i)
	}
	wg.Wait()
	t.Logf("fetches: %d", fetches.Load())
}

// Held: a token endpoint that redirects elsewhere is not followed.
func TestAdvTokenRedirectNotFollowed(t *testing.T) {
	var hit atomic.Bool
	other := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { hit.Store(true) }))
	defer other.Close()
	_, r := advPlugin(t, func(w nethttp.ResponseWriter, req *nethttp.Request) {
		nethttp.Redirect(w, req, other.URL+"/steal", nethttp.StatusTemporaryRedirect)
	}, false)
	_, err := r.Call(context.Background(), act.Call{Action: "email/send", IdempotencyKey: "a"})
	if err == nil || hit.Load() {
		t.Fatalf("redirect followed: %v %v", err, hit.Load())
	}
	t.Logf("%v", err)
}
