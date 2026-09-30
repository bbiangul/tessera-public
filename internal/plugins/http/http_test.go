package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

type secrets map[string]string

func (s secrets) Secret(name string) (string, error) {
	if v, ok := s[name]; ok {
		return v, nil
	}
	return "", ErrNoSecret
}

const secretValue = "s3cr3t-value"

// local lets tests reach httptest servers on 127.0.0.1.
var local = Options{AllowPrivateNetworks: true}

// manifest for a fake Booking-like API served by srv.
func manifest(srv *httptest.Server, auth *plugins.Auth) *plugins.PluginManifest {
	u, _ := url.Parse(srv.URL)
	return &plugins.PluginManifest{
		Metadata: plugins.PluginMeta{Name: "booking", Version: "1.0.0"},
		Spec: plugins.PluginSpec{
			Egress:     &plugins.Egress{Hosts: []string{u.Hostname()}},
			Connection: &plugins.Connection{BaseURL: srv.URL + "/api/v1", Auth: auth},
			Actions: map[string]plugins.ActionSpec{
				"search": {Effect: plugins.EffectRead, Input: map[string]string{"to": "string", "adults": "int"},
					HTTP: &plugins.HTTPCall{Method: "GET", Path: "/flights/{{ inputs.to }}",
						Query:   map[string]any{"adults": "${ inputs.adults }", "sort": "BEST", "stops": nil, "tag": []any{"a", "b"}},
						Headers: map[string]string{"x-rapidapi-host": "booking.example"},
						Output:  "sources.response.body.data.offers"}},
				"book": {Effect: plugins.EffectIrreversible, Input: map[string]string{"token": "string"},
					HTTP: &plugins.HTTPCall{Method: "POST", Path: "/book", Body: map[string]any{"token": "${ inputs.token }", "source": "tessera"}}},
			},
		},
	}
}

func registry(t *testing.T, srv *httptest.Server, auth *plugins.Auth) *act.Registry {
	t.Helper()
	p, err := NewWithOptions(manifest(srv, auth), secrets{"BOOKING_KEY": secretValue}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	p.Backoff = time.Millisecond
	p.Timeout = 300 * time.Millisecond
	r := act.NewRegistry()
	p.Register(r)
	return r
}

func TestGetWithQueryPathAndAuth(t *testing.T) {
	var got *nethttp.Request
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		got = r
		_, _ = io.WriteString(w, `{"data": {"offers": [{"token": "t1", "price": 1234.5, "stops": 1}]}}`)
	}))
	defer srv.Close()
	r := registry(t, srv, &plugins.Auth{Header: "x-rapidapi-key", Secret: "BOOKING_KEY"})
	out, err := r.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "São Paulo GRU", "adults": int64(2)}})
	if err != nil {
		t.Fatal(err)
	}
	offers := out.([]any)
	if o := offers[0].(map[string]any); o["token"] != "t1" || o["stops"] != int64(1) || o["price"] != 1234.5 {
		t.Errorf("output %#v", o)
	}
	if got.URL.EscapedPath() != "/api/v1/flights/S%C3%A3o%20Paulo%20GRU" {
		t.Errorf("path %q: a value must stay one segment", got.URL.EscapedPath())
	}
	q := got.URL.Query()
	if q.Get("adults") != "2" || q.Get("sort") != "BEST" || q.Has("stops") || len(q["tag"]) != 2 {
		t.Errorf("query %v", q)
	}
	if got.Header.Get("x-rapidapi-key") != secretValue || got.Header.Get("x-rapidapi-host") != "booking.example" {
		t.Errorf("headers %v", got.Header)
	}
}

func TestPostSendsOnceWithIdempotencyKey(t *testing.T) {
	var calls int32
	var body map[string]any
	var key string
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewDecoder(r.Body).Decode(&body)
		key = r.Header.Get("Idempotency-Key")
		w.WriteHeader(502)
	}))
	defer srv.Close()
	r := registry(t, srv, nil)
	_, err := r.Call(context.Background(), act.Call{Action: "booking/book", Args: map[string]any{"token": "t1"}, IdempotencyKey: "k-1"})
	var amb *act.AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("a 5xx after a POST may have happened: want ambiguous, got %v", err)
	}
	if calls != 1 {
		t.Errorf("an effect was sent %d times; the transport must never retry it", calls)
	}
	if body["token"] != "t1" || body["source"] != "tessera" || key != "k-1" {
		t.Errorf("body %v, key %q", body, key)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status   int
		definite bool
		code     string
		calls    int32 // for the read action
	}{
		{404, true, "NOT_FOUND", 1},
		{403, true, "FORBIDDEN", 1},
		{400, true, "INVALID", 1},
		{429, true, "RATE_LIMITED", 3},
		{500, false, "", 3},
		{302, true, "REDIRECT", 1},
	}
	for _, c := range cases {
		var calls int32
		srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			atomic.AddInt32(&calls, 1)
			if c.status == 302 {
				w.Header().Set("Location", "https://elsewhere.example/")
			}
			w.WriteHeader(c.status)
		}))
		_, err := registry(t, srv, nil).Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
		srv.Close()
		var d *act.DefiniteError
		if isDef := errors.As(err, &d); isDef != c.definite || (isDef && d.Code != c.code) {
			t.Errorf("%d: got %v", c.status, err)
		}
		if calls != c.calls {
			t.Errorf("%d: %d calls, want %d (reads retry transient failures)", c.status, calls, c.calls)
		}
	}
}

func TestSecretsNeverLeak(t *testing.T) {
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		time.Sleep(time.Second) // longer than the plugin's timeout
	}))
	defer srv.Close()
	r := registry(t, srv, &plugins.Auth{Query: "key", Secret: "BOOKING_KEY"})
	_, err := r.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
	if err == nil || strings.Contains(err.Error(), secretValue) {
		t.Fatalf("error %v must exist and must not contain the secret", err)
	}
	var amb *act.AmbiguousError
	if !errors.As(err, &amb) || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("a timeout is ambiguous: %v", err)
	}

	p, _ := NewWithOptions(manifest(srv, &plugins.Auth{Header: "Authorization", Prefix: "Bearer ", Secret: "MISSING"}), secrets{}, srv.Client(), local)
	reg := act.NewRegistry()
	p.Register(reg)
	_, err = reg.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
	var d *act.DefiniteError
	if !errors.As(err, &d) || d.Code != "NO_SECRET" {
		t.Errorf("missing secret: %v", err)
	}
}

func TestEgressAndLimits(t *testing.T) {
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = w.Write([]byte(`"` + strings.Repeat("x", MaxResponseBytes) + `"`))
	}))
	defer srv.Close()
	m := manifest(srv, nil)
	m.Spec.Egress.Hosts = []string{"api.booking.example"}
	p, _ := NewWithOptions(m, secrets{}, srv.Client(), local)
	reg := act.NewRegistry()
	p.Register(reg)
	_, err := reg.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
	var d *act.DefiniteError
	if !errors.As(err, &d) || d.Code != "EGRESS" {
		t.Errorf("a host outside egress must be refused before connecting: %v", err)
	}
	_, err = registry(t, srv, nil).Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
	if !errors.As(err, &d) || d.Code != "TOO_LARGE" {
		t.Errorf("oversized response: %v", err)
	}
}

func TestUnreachableIsDefinite(t *testing.T) {
	srv := httptest.NewServer(nethttp.NotFoundHandler())
	r := registry(t, srv, nil)
	srv.Close() // nothing listens any more
	_, err := r.Call(context.Background(), act.Call{Action: "booking/book", Args: map[string]any{"token": "t"}})
	var d *act.DefiniteError
	if !errors.As(err, &d) || d.Code != "UNREACHABLE" {
		t.Errorf("a refused connection never reached the server: want definite, got %v", err)
	}
}

func TestPrivateAddressesAreRefusedByDefault(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { atomic.AddInt32(&calls, 1) }))
	defer srv.Close()
	p, err := New(manifest(srv, nil), secrets{}, srv.Client()) // default: no private networks
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	_, err = r.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}})
	var d *act.DefiniteError
	if !errors.As(err, &d) || calls != 0 {
		t.Errorf("127.0.0.1 must be refused before connecting: %v, %d calls", err, calls)
	}
}

// Review finding: a processed effect was reported as "did not happen".
func TestEffectFailuresAfterSendingAreAmbiguous(t *testing.T) {
	for name, handler := range map[string]nethttp.HandlerFunc{
		"303 after create": func(w nethttp.ResponseWriter, r *nethttp.Request) {
			w.Header().Set("Location", "/booking/1")
			w.WriteHeader(303)
		},
		"200 not JSON": func(w nethttp.ResponseWriter, r *nethttp.Request) { _, _ = io.WriteString(w, "OK") },
		"200 too large": func(w nethttp.ResponseWriter, r *nethttp.Request) {
			_, _ = w.Write([]byte(`"` + strings.Repeat("x", MaxResponseBytes) + `"`))
		},
	} {
		srv := httptest.NewServer(handler)
		_, err := registry(t, srv, nil).Call(context.Background(), act.Call{Action: "booking/book", Args: map[string]any{"token": "t"}, IdempotencyKey: "k"})
		srv.Close()
		var amb *act.AmbiguousError
		if !errors.As(err, &amb) {
			t.Errorf("%s: an irreversible POST that reached the server must be ambiguous, got %v", name, err)
		}
	}
}

func TestPlatformHeadersWin(t *testing.T) {
	var keys []string
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		keys = append(keys, r.Header.Values("Idempotency-Key")...)
	}))
	defer srv.Close()
	m := manifest(srv, nil)
	book := m.Spec.Actions["book"]
	book.HTTP.Headers = map[string]string{"idempotency-key": "static"}
	m.Spec.Actions["book"] = book
	p, _ := NewWithOptions(m, secrets{}, srv.Client(), local)
	r := act.NewRegistry()
	p.Register(r)
	for range 20 {
		_, _ = r.Call(context.Background(), act.Call{Action: "booking/book", Args: map[string]any{"token": "t"}, IdempotencyKey: "per-call"})
	}
	for _, k := range keys {
		if k != "per-call" {
			t.Fatalf("a manifest header replaced the idempotency key: %q", keys)
		}
	}
}

func TestPathValuesStayOneSegment(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { atomic.AddInt32(&calls, 1) }))
	defer srv.Close()
	r := registry(t, srv, nil)
	for _, bad := range []string{"..", ".", "../../admin", "a/b", `a\b`} {
		_, err := r.Call(context.Background(), act.Call{Action: "booking/search", Args: map[string]any{"to": bad, "adults": int64(1)}})
		var d *act.DefiniteError
		if !errors.As(err, &d) || d.Code != "INVALID" {
			t.Errorf("%q: want INVALID, got %v", bad, err)
		}
	}
	if calls != 0 {
		t.Errorf("%d requests reached the server", calls)
	}
}

func TestFailWhenReadsTheBody(t *testing.T) {
	answer := `{"ok": false, "error": "channel_not_found"}`
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = io.WriteString(w, answer)
	}))
	defer srv.Close()
	m := manifest(srv, nil)
	m.Spec.Actions["post"] = plugins.ActionSpec{Effect: plugins.EffectExternalMessage, Input: map[string]string{"text": "string"},
		HTTP: &plugins.HTTPCall{Method: "POST", Path: "/chat.postMessage", Body: map[string]any{"text": "${ inputs.text }"},
			FailWhen: "!sources.response.body.ok", Output: "sources.response.body.ts"}}
	p, err := NewWithOptions(m, secrets{}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	_, err = r.Call(context.Background(), act.Call{Action: "booking/post", Args: map[string]any{"text": "oi"}})
	var def *act.DefiniteError
	if !errors.As(err, &def) || def.Code != "REFUSED" || !strings.Contains(def.Message, "channel_not_found") {
		t.Fatalf("a 200 with ok false is a refusal: %v", err)
	}
	irr := m.Spec.Actions["post"]
	irr.Effect = plugins.EffectIrreversible
	m.Spec.Actions["charge"] = irr
	p2, err := NewWithOptions(m, secrets{}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r2 := act.NewRegistry()
	p2.Register(r2)
	var amb *act.AmbiguousError
	if _, err := r2.Call(context.Background(), act.Call{Action: "booking/charge", Args: map[string]any{"text": "oi"}}); !errors.As(err, &amb) {
		t.Fatalf("for an irreversible action the body cannot prove nothing happened: %v", err)
	}
	answer = `{"ok": true, "ts": "1.2"}`
	if out, err := r.Call(context.Background(), act.Call{Action: "booking/post", Args: map[string]any{"text": "oi"}}); err != nil || out != "1.2" {
		t.Fatalf("%v %v", out, err)
	}
}

// drift-002: what a refusing server said, in its words, for the user.
func TestErrorDetail(t *testing.T) {
	for body, want := range map[string]string{
		`{"error": "online rescheduling is suspended: call (11) 3030-4040"}`: "online rescheduling is suspended: call (11) 3030-4040",
		`{"error": {"code": 7, "message": "a fee applies"}}`:                 "a fee applies",
		`{"message": "slot  taken\n today"}`:                                 "slot taken today",
		`{"code": 42}`:                                                       "",
		`<html><body>Forbidden</body></html>`:                                "",
		`plain words`:                                                        "plain words",
	} {
		if got := errorDetail([]byte(body)); got != want {
			t.Errorf("%s: %q, want %q", body, got, want)
		}
	}
	if got := errorDetail([]byte(`{"error": "` + strings.Repeat("é", 400) + `"}`)); len(got) > 304 || !strings.HasSuffix(got, "…") {
		t.Errorf("clipped at 300 bytes on a rune boundary: %d", len(got))
	}
}

// drift-007: a repair adds body fields; it can never change one, and a
// read without a body cannot take any.
func TestRepairExtraOnlyAdds(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"data": {"offers": []}}`)
	}))
	defer srv.Close()
	r := registry(t, srv, nil)
	if _, err := r.Call(context.Background(), act.Call{Action: "booking/book", Args: map[string]any{"token": "t1"}, IdempotencyKey: "k", Extra: map[string]any{"acceptFee": true}}); err != nil {
		t.Fatal(err)
	}
	if body["acceptFee"] != true || body["token"] != "t1" {
		t.Errorf("body %v", body)
	}
	var def *act.DefiniteError
	for name, c := range map[string]act.Call{
		"changes a field": {Action: "booking/book", Args: map[string]any{"token": "t1"}, IdempotencyKey: "k", Extra: map[string]any{"token": "t2"}},
		"no body":         {Action: "booking/search", Args: map[string]any{"to": "x", "adults": int64(1)}, Extra: map[string]any{"x": 1}},
	} {
		if _, err := r.Call(context.Background(), c); !errors.As(err, &def) {
			t.Errorf("%s: want a definite refusal, got %v", name, err)
		}
	}
}

// backoffice-002: the documented send-buttons binding sends Meta's
// interactive button message.
func TestWhatsAppButtonsBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"messages":[{"id":"wamid.x"}]}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "whatsapp", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL},
		Actions: map[string]plugins.ActionSpec{"send-buttons": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"to": "string", "text": "string", "buttons": "list"},
			HTTP: &plugins.HTTPCall{Method: "POST", Path: "/messages", Body: map[string]any{
				"messaging_product": "whatsapp", "to": "${ inputs.to }", "type": "interactive",
				"interactive": map[string]any{"type": "button", "body": map[string]any{"text": "${ inputs.text }"},
					"action": map[string]any{"buttons": "${ inputs.buttons.map(b, {'type': 'reply', 'reply': {'id': b.id, 'title': b.title}}) }"}},
			}}}},
	}}
	p, err := NewWithOptions(m, secrets{}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	if _, err := r.Call(context.Background(), act.Call{Action: "whatsapp/send-buttons", IdempotencyKey: "k", Args: map[string]any{"to": "5511900000001", "text": "Desconto de 10%?",
		"buttons": []any{map[string]any{"id": "task:h:approve", "title": "Aprovar"}, map[string]any{"id": "task:h:deny", "title": "Recusar"}}}}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(body)
	want := `{"interactive":{"action":{"buttons":[{"reply":{"id":"task:h:approve","title":"Aprovar"},"type":"reply"},{"reply":{"id":"task:h:deny","title":"Recusar"},"type":"reply"}]},"body":{"text":"Desconto de 10%?"},"type":"button"},"messaging_product":"whatsapp","to":"5511900000001","type":"interactive"}`
	if string(got) != want {
		t.Errorf("body\n%s\nwant\n%s", got, want)
	}
}

// backoffice-002: the documented send-task-template binding sends Meta's
// template message with the task in the body and the task ids as the
// quick replies' payloads.
func TestWhatsAppTaskTemplateBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"messages":[{"id":"wamid.x"}]}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "whatsapp", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL},
		Actions: map[string]plugins.ActionSpec{"send-task-template": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"to": "string", "text": "string", "buttons": "list"},
			HTTP: &plugins.HTTPCall{Method: "POST", Path: "/messages", Body: map[string]any{
				"messaging_product": "whatsapp", "to": "${ inputs.to }", "type": "template",
				"template": map[string]any{"name": "tarefa_aprovacao", "language": map[string]any{"code": "pt_BR"},
					"components": "${ [{'type': 'body', 'parameters': [{'type': 'text', 'text': inputs.text}]}, {'type': 'button', 'sub_type': 'quick_reply', 'index': '0', 'parameters': [{'type': 'payload', 'payload': inputs.buttons[0].id}]}, {'type': 'button', 'sub_type': 'quick_reply', 'index': '1', 'parameters': [{'type': 'payload', 'payload': inputs.buttons[1].id}]}] }"},
			}}}},
	}}
	p, err := NewWithOptions(m, secrets{}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	if _, err := r.Call(context.Background(), act.Call{Action: "whatsapp/send-task-template", IdempotencyKey: "k", Args: map[string]any{"to": "5511900000001", "text": "Desconto de 10%?",
		"buttons": []any{map[string]any{"id": "task:h:approve", "title": "Aprovar"}, map[string]any{"id": "task:h:deny", "title": "Recusar"}}}}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(body)
	want := `{"messaging_product":"whatsapp","template":{"components":[{"parameters":[{"text":"Desconto de 10%?","type":"text"}],"type":"body"},{"index":"0","parameters":[{"payload":"task:h:approve","type":"payload"}],"sub_type":"quick_reply","type":"button"},{"index":"1","parameters":[{"payload":"task:h:deny","type":"payload"}],"sub_type":"quick_reply","type":"button"}],"language":{"code":"pt_BR"},"name":"tarefa_aprovacao"},"to":"5511900000001","type":"template"}`
	if string(got) != want {
		t.Errorf("body\n%s\nwant\n%s", got, want)
	}
}
