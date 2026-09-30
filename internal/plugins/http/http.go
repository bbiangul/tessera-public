// Package http is the built-in http plugin (RFC-0001, "Plugins que vêm com
// a plataforma"): it implements every manifest action that declares an
// http call, so a REST API becomes typed, checked actions without code.
//
// Safety rules:
//   - Requests only go to hosts in the manifest's egress list; redirects are
//     not followed.
//   - Credentials come from a SecretStore by name and are never logged or
//     put in errors.
//   - Reads are retried on transient failures. Effects are sent once: a
//     timeout or a 5xx after the request left may mean it happened, so the
//     outcome is reported as ambiguous and the runtime verifies or asks a
//     person (RFC-0003).
package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
)

// SecretStore resolves secret names to values.
type SecretStore interface {
	Secret(name string) (string, error)
}

// EnvSecrets reads TESSERA_SECRET_<NAME> (self-hosted, phase 1).
type EnvSecrets struct{}

// ErrNoSecret means a referenced secret is not set.
var ErrNoSecret = errors.New("secret is not set")

func (EnvSecrets) Secret(name string) (string, error) {
	v := os.Getenv("TESSERA_SECRET_" + name)
	if v == "" {
		return "", fmt.Errorf("%w: %s (set TESSERA_SECRET_%s)", ErrNoSecret, name, name)
	}
	return v, nil
}

// Limits of one request.
const (
	MaxResponseBytes = 8 << 20
	DefaultTimeout   = 20 * time.Second
	readAttempts     = 3
)

// action is one compiled http action.
type action struct {
	name     string
	spec     plugins.ActionSpec
	path     *expr.Template
	query    *expr.Value
	body     *expr.Value
	headers  map[string]string
	output   *expr.Expr
	failWhen *expr.Expr
}

// Options configure a plugin.
type Options struct {
	// AllowPrivateNetworks lets requests reach loopback, private and
	// link-local addresses. Off by default: a hostname in egress that
	// resolves to an internal address (or the cloud metadata service) is
	// refused at connection time. Self-hosted installs that call an API
	// inside their own network turn it on explicitly.
	AllowPrivateNetworks bool
}

// Plugin serves one manifest's http actions.
type Plugin struct {
	bytesClient *nethttp.Client // bytes bindings: follows allowed redirects
	manifest    *plugins.PluginManifest
	base        *url.URL
	hosts       map[string]bool
	actions     map[string]*action
	secrets     SecretStore
	client      *nethttp.Client
	oauth       tokenCache
	// Timeout per request; DefaultTimeout when zero.
	Timeout time.Duration
	// Backoff between read retries; tests shorten it.
	Backoff time.Duration
}

// New compiles a manifest's http actions. The manifest must have passed
// admission.
func New(m *plugins.PluginManifest, secrets SecretStore, client *nethttp.Client) (*Plugin, error) {
	return NewWithOptions(m, secrets, client, Options{})
}

// NewWithOptions is New with options.
func NewWithOptions(m *plugins.PluginManifest, secrets SecretStore, client *nethttp.Client, opts Options) (*Plugin, error) {
	if m.Spec.Connection == nil {
		return nil, fmt.Errorf("plugin %s: no connection", m.Metadata.Name)
	}
	base, err := url.Parse(m.Spec.Connection.BaseURL)
	if err != nil {
		return nil, err
	}
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	p := &Plugin{manifest: m, base: base, hosts: map[string]bool{}, actions: map[string]*action{}, secrets: secrets, Backoff: 300 * time.Millisecond}
	if m.Spec.Egress != nil {
		for _, h := range m.Spec.Egress.Hosts {
			p.hosts[strings.ToLower(h)] = true
		}
	}
	if client == nil {
		client = &nethttp.Client{}
	}
	c := *client
	c.CheckRedirect = func(*nethttp.Request, []*nethttp.Request) error { return nethttp.ErrUseLastResponse }
	if !opts.AllowPrivateNetworks {
		base, _ := c.Transport.(*nethttp.Transport)
		if base == nil {
			base = nethttp.DefaultTransport.(*nethttp.Transport)
		}
		t := base.Clone()
		dialer := &net.Dialer{Timeout: 10 * time.Second, Control: refusePrivate}
		t.DialContext = dialer.DialContext
		c.Transport = t
	}
	p.client = &c
	// Bytes bindings follow redirects (plugin-001): at most 3, https, each
	// to an allowed host. The transport (and its private-address refusal)
	// is the same; Go drops Authorization when the host changes.
	bc := c
	bc.CheckRedirect = func(req *nethttp.Request, via []*nethttp.Request) error {
		if len(via) > 3 {
			return errors.New("more than 3 redirects")
		}
		if req.URL.Scheme != "https" && !opts.AllowPrivateNetworks {
			return errors.New("redirect to a non-https URL")
		}
		if !p.allowed(req.URL.Hostname()) {
			return fmt.Errorf("redirect to %s, not in the plugin's egress list", req.URL.Hostname())
		}
		return nil
	}
	p.bytesClient = &bc
	for name, spec := range m.Spec.Actions {
		if spec.HTTP == nil {
			continue
		}
		a := &action{name: name, spec: spec, headers: spec.HTTP.Headers}
		if a.path, err = e.CompileTemplate(spec.HTTP.Path); err != nil {
			return nil, fmt.Errorf("%s.path: %w", name, err)
		}
		if a.query, err = e.CompileValue(orEmpty(spec.HTTP.Query)); err != nil {
			return nil, fmt.Errorf("%s.query: %w", name, err)
		}
		if spec.HTTP.Body != nil {
			if a.body, err = e.CompileValue(spec.HTTP.Body); err != nil {
				return nil, fmt.Errorf("%s.body: %w", name, err)
			}
		}
		if spec.HTTP.FailWhen != "" {
			if a.failWhen, err = e.Compile(spec.HTTP.FailWhen, expr.Bool); err != nil {
				return nil, fmt.Errorf("%s.failWhen: %w", name, err)
			}
		}
		if spec.HTTP.Output != "" {
			if a.output, err = e.Compile(spec.HTTP.Output, expr.Any); err != nil {
				return nil, fmt.Errorf("%s.output: %w", name, err)
			}
		}
		p.actions[name] = a
	}
	return p, nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// Register adds the plugin's http actions to a registry.
func (p *Plugin) Register(r *act.Registry) { p.RegisterExcept(r, nil) }

// RegisterExcept adds the http actions whose plugin/action name is not in
// skip, so a test or a recording can stand in for some of them.
func (p *Plugin) RegisterExcept(r *act.Registry, skip map[string]act.Handler) {
	names := make([]string, 0, len(p.actions))
	for n := range p.actions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		full := p.manifest.Metadata.Name + "/" + n
		if _, skipped := skip[full]; skipped {
			continue
		}
		a := p.actions[n]
		r.Register(full, func(ctx context.Context, c act.Call) (any, error) {
			return p.call(ctx, a, c)
		})
	}
}

func (p *Plugin) call(ctx context.Context, a *action, c act.Call) (any, error) {
	req, err := p.request(ctx, a, c)
	if err != nil {
		return nil, err
	}
	attempts := 1
	if a.spec.Effect == plugins.EffectRead {
		attempts = readAttempts
	}
	var last error
	for i := range attempts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, &act.DefiniteError{Code: "CANCELLED", Message: "stopped before retrying"}
			case <-time.After(p.Backoff * time.Duration(1<<(i-1))):
			}
			if req, err = p.request(ctx, a, c); err != nil { // bodies are single-use
				return nil, err
			}
		}
		out, retry, err := p.do(req, a)
		if err == nil {
			return out, nil
		}
		last = err
		if !retry {
			break
		}
	}
	return nil, last
}

// request builds the HTTP request for a call.
func (p *Plugin) request(ctx context.Context, a *action, c act.Call) (*nethttp.Request, error) {
	actv := expr.Activation{"inputs": c.Args, "caller": c.Caller}
	// A value becomes exactly one path segment: "..", "." or a slash would
	// let an input (often typed by a user) reach another endpoint with the
	// plugin's credentials.
	badSegment := false
	path, err := a.path.Render(ctx, actv, func(v string) string {
		if v == "." || v == ".." || strings.ContainsAny(v, "/\\") {
			badSegment = true
		}
		return url.PathEscape(v)
	})
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": path: " + err.Error()}
	}
	if badSegment {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": a path value cannot be '.', '..' or contain a slash"}
	}
	u := *p.base
	// path is already escaped per segment; RawPath keeps a "/" inside a
	// value as %2F instead of turning it into a new segment.
	u.RawPath = strings.TrimSuffix(p.base.EscapedPath(), "/") + path
	if u.Path, err = url.PathUnescape(u.RawPath); err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": path is not valid"}
	}
	if !p.allowed(u.Hostname()) {
		return nil, &act.DefiniteError{Code: "EGRESS", Message: "host " + u.Hostname() + " is not in the plugin's egress list"}
	}
	q, err := a.query.Resolve(ctx, actv)
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": query: " + err.Error()}
	}
	values := url.Values{}
	for k, v := range q.(map[string]any) {
		switch v := v.(type) {
		case nil:
		case []any:
			for _, x := range v {
				values.Add(k, scalar(x))
			}
		default:
			values.Set(k, scalar(v))
		}
	}
	var body io.Reader
	if len(c.Extra) > 0 && a.body == nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": a repair can only add body fields to an action that sends a body"}
	}
	if a.body != nil {
		b, err := a.body.Resolve(ctx, actv)
		if err != nil {
			return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": body: " + err.Error()}
		}
		if len(c.Extra) > 0 {
			m, ok := b.(map[string]any)
			if !ok {
				return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": a repair can only add fields to an object body"}
			}
			for k, v := range c.Extra {
				if _, taken := m[k]; taken {
					return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": a repair may not change the field " + k}
				}
				m[k] = v
			}
		}
		data, err := json.Marshal(b)
		if err != nil {
			return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": body is not JSON"}
		}
		body = bytes.NewReader(data)
	}
	// Canonical names, so "idempotency-key" in a manifest and the
	// platform's Idempotency-Key are one header; the platform's own
	// headers are set last and always win.
	headers := nethttp.Header{}
	headers.Set("Accept", "application/json")
	for k, v := range a.headers {
		headers.Set(k, v)
	}
	if body != nil {
		headers.Set("Content-Type", "application/json")
	}
	if au := p.manifest.Spec.Connection.Auth; au != nil {
		secret, err := p.secrets.Secret(au.Secret)
		if err != nil {
			return nil, &act.DefiniteError{Code: "NO_SECRET", Message: err.Error()}
		}
		if au.Header != "" {
			headers.Set(au.Header, au.Prefix+secret)
		} else {
			values.Set(au.Query, au.Prefix+secret)
		}
	}
	if p.manifest.Spec.Connection.OAuth2 != nil {
		tok, err := p.bearer(ctx)
		if err != nil {
			return nil, err
		}
		headers.Set("Authorization", "Bearer "+tok)
	}
	if c.IdempotencyKey != "" {
		headers.Set("Idempotency-Key", c.IdempotencyKey)
	}
	u.RawQuery = values.Encode()
	req, err := nethttp.NewRequestWithContext(ctx, a.spec.HTTP.Method, u.String(), body)
	if err != nil {
		return nil, &act.DefiniteError{Code: "INVALID", Message: a.name + ": " + err.Error()}
	}
	req.Header = headers
	return req, nil
}

func scalar(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// do sends one request. It reports whether a retry is sensible (for reads).
//
// Once a request may have left, a failure of an action that is not a read
// is ambiguous unless the server said it did nothing (a 4xx other than 408,
// or 429): a redirect after a create, a body we cannot read, or our own
// cancellation mid-flight all leave the effect possibly done.
func (p *Plugin) do(req *nethttp.Request, a *action) (any, bool, error) {
	effect := a.spec.Effect != plugins.EffectRead
	sentButUnclear := func(code, msg string, retry bool) (any, bool, error) {
		if effect {
			return nil, false, &act.AmbiguousError{Message: a.name + ": " + msg}
		}
		return nil, retry, &act.DefiniteError{Code: code, Message: a.name + ": " + msg}
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	client := p.client
	if a.spec.HTTP.Response == "bytes" {
		client = p.bytesClient
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		if notSent(err) {
			return nil, true, &act.DefiniteError{Code: "UNREACHABLE", Message: a.name + ": could not connect"}
		}
		if req.Context().Err() != nil {
			return sentButUnclear("CANCELLED", "cancelled while the request was in flight", false)
		}
		return nil, true, &act.AmbiguousError{Message: a.name + ": no response (" + errKind(err) + ")"}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, true, &act.AmbiguousError{Message: a.name + ": the response was cut"}
	}
	if len(data) > MaxResponseBytes {
		return sentButUnclear("TOO_LARGE", fmt.Sprintf("response larger than %d bytes", MaxResponseBytes), false)
	}
	status := resp.StatusCode
	if status == nethttp.StatusUnauthorized && p.manifest.Spec.Connection.OAuth2 != nil {
		// Revoked or rotated: the next call gets a new one.
		p.dropToken(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	}
	switch {
	case status >= 300 && status < 400:
		return sentButUnclear("REDIRECT", fmt.Sprintf("redirect %d is not followed", status), false)
	case status == 429:
		// Rate limited: the request was refused, not processed.
		return nil, true, &act.DefiniteError{Code: "RATE_LIMITED", Message: a.name + ": rate limited"}
	case status == 408 || status >= 500:
		return nil, true, &act.AmbiguousError{Message: fmt.Sprintf("%s: server answered %d", a.name, status)}
	case status >= 400:
		code := "INVALID"
		switch status {
		case 401, 403:
			code = "FORBIDDEN"
		case 404:
			code = "NOT_FOUND"
		case 409:
			code = "CONFLICT"
		}
		return nil, false, &act.DefiniteError{Code: code, Message: fmt.Sprintf("%s: server answered %d", a.name, status),
			Detail: errorDetail(data), Public: p.manifest.Spec.Connection.PublicErrors}
	}
	var body any
	if a.spec.HTTP.Response == "bytes" {
		body = map[string]any{"base64": base64.StdEncoding.EncodeToString(data), "contentType": resp.Header.Get("Content-Type"), "size": int64(len(data))}
	} else if len(bytes.TrimSpace(data)) > 0 {
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		if err := d.Decode(&body); err != nil {
			return sentButUnclear("BAD_RESPONSE", "response is not JSON", false)
		}
		body = plain(body)
	}
	sources := expr.Activation{"sources": map[string]any{
		"response": map[string]any{"status": int64(status), "body": body},
	}}
	if a.failWhen != nil {
		failed, err := a.failWhen.Eval(req.Context(), sources)
		if err != nil {
			return sentButUnclear("BAD_RESPONSE", "done, but failWhen could not be read: "+err.Error(), false)
		}
		if failed == true {
			start := strings.TrimSpace(string(data))
			if len(start) > 200 {
				start = start[:200] + "…"
			}
			msg := fmt.Sprintf("%s: the server refused it: %s", a.name, start)
			if a.spec.Effect == plugins.EffectIrreversible {
				// A 2xx body can mean "pending" as well as "refused": for an
				// irreversible effect, verification decides, not the body.
				return nil, false, &act.AmbiguousError{Message: msg}
			}
			return nil, false, &act.DefiniteError{Code: "REFUSED", Message: msg}
		}
	}
	if a.output == nil {
		return body, false, nil
	}
	out, err := a.output.Eval(req.Context(), sources)
	if err != nil {
		return sentButUnclear("BAD_RESPONSE", "done, but its output could not be read: "+err.Error(), false)
	}
	return out, false, nil
}

// ErrPrivateAddress is returned when a connection would reach an internal
// address.
var ErrPrivateAddress = errors.New("refusing to connect to a private, loopback or link-local address")

// refusePrivate runs after DNS resolution, so a public name that resolves
// to an internal address is refused too.
// errorDetail is what a refusing server said, in its words: a string
// field of a JSON body (error, message, detail, title, error.message), or
// a short text body. At most 300 bytes.
func errorDetail(data []byte) string {
	var body any
	text := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &body) == nil {
		text = ""
		if m, ok := body.(map[string]any); ok {
			for _, k := range []string{"error", "message", "detail", "title", "error_description"} {
				switch v := m[k].(type) {
				case string:
					text = v
				case map[string]any:
					text, _ = v["message"].(string)
				}
				if text != "" {
					break
				}
			}
		}
	} else if strings.HasPrefix(text, "<") {
		text = "" // an HTML error page says nothing to a user
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 300 {
		cut := 300
		for cut > 0 && text[cut]&0xC0 == 0x80 {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}

func refusePrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return ErrPrivateAddress
	}
	return nil
}

// notSent reports errors that happen before a request leaves: DNS and
// refused connections. Nothing was processed.
func notSent(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func errKind(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "connection lost"
}

func plain(v any) any {
	switch v := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return i
		}
		f, _ := v.Float64()
		return f
	case []any:
		for i := range v {
			v[i] = plain(v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = plain(v[k])
		}
	}
	return v
}

// allowed says whether host is in the egress list: exactly, or under a
// "*.domain" pattern.
func (p *Plugin) allowed(host string) bool {
	host = strings.ToLower(host)
	if p.hosts[host] {
		return true
	}
	for h := range p.hosts {
		if strings.HasPrefix(h, "*.") && strings.HasSuffix(host, h[1:]) {
			return true
		}
	}
	return false
}
