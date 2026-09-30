package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
)

// defaultTokenLife is how long a token without expires_in is kept.
const defaultTokenLife = 5 * time.Minute

// tokenCache keeps one OAuth2 access token (connection.oauth2), and the
// refresh token when the provider rotated it.
type tokenCache struct {
	mu      sync.Mutex
	token   string
	until   time.Time
	refresh string // a rotated refresh token, kept in memory only
}

// bearer returns a valid access token, fetching one by the
// client-credentials (or refresh-token) grant when there is none or it is
// about to expire. A failure means the API call was never sent: it fails
// definitely.
func (p *Plugin) bearer(ctx context.Context) (string, error) {
	p.oauth.mu.Lock()
	defer p.oauth.mu.Unlock()
	if p.oauth.token != "" && time.Now().Before(p.oauth.until) {
		return p.oauth.token, nil
	}
	o := p.manifest.Spec.Connection.OAuth2
	id, err := p.secrets.Secret(o.ClientID)
	if err != nil {
		return "", &act.DefiniteError{Code: "NO_SECRET", Message: err.Error()}
	}
	secret, err := p.secrets.Secret(o.ClientSecret)
	if err != nil {
		return "", &act.DefiniteError{Code: "NO_SECRET", Message: err.Error()}
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {id}, "client_secret": {secret}}
	credentials := []string{secret}
	if o.RefreshToken != "" {
		refresh := p.oauth.refresh
		if refresh == "" {
			if refresh, err = p.secrets.Secret(o.RefreshToken); err != nil {
				return "", &act.DefiniteError{Code: "NO_SECRET", Message: err.Error()}
			}
		}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refresh)
		credentials = append(credentials, refresh)
	}
	fail := func(msg string) error {
		return &act.DefiniteError{Code: "AUTH", Message: p.manifest.Metadata.Name + ": OAuth2 token: " + redact(msg, credentials)}
	}
	u, err := url.Parse(o.TokenURL)
	if err != nil || !p.hosts[strings.ToLower(u.Hostname())] {
		return "", fail("the token URL's host is not in egress")
	}
	if o.Scope != "" {
		form.Set("scope", o.Scope)
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fail(err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fail("no response (" + errKind(err) + ")")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != nethttp.StatusOK {
		var e struct {
			Description string `json:"error_description"`
		}
		detail := errorDetail(data)
		if json.Unmarshal(data, &e) == nil && e.Description != "" {
			detail = oneLineCapped(e.Description) // Entra appends trace ids on new lines
		}
		// The endpoint's words, never a credential: some echo the request.
		return "", fail(fmt.Sprintf("the token endpoint answered %d: %s", resp.StatusCode, detail))
	}
	var t struct {
		AccessToken  string          `json:"access_token"`
		ExpiresIn    json.RawMessage `json:"expires_in"`
		RefreshToken string          `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &t); err != nil || t.AccessToken == "" {
		return "", fail("the token endpoint answered without an access_token")
	}
	life := defaultTokenLife
	if secs, ok := seconds(t.ExpiresIn); ok {
		life = time.Duration(secs)*time.Second - time.Minute
		if life < 0 {
			life = 0
		}
	}
	if o.RefreshToken != "" && t.RefreshToken != "" {
		// The provider rotated the refresh token: the old one may no
		// longer work. Kept in memory; a restart uses the secret again.
		p.oauth.refresh = t.RefreshToken
	}
	p.oauth.token, p.oauth.until = t.AccessToken, time.Now().Add(life)
	return t.AccessToken, nil
}

// seconds reads expires_in as a number or a numeric string (Entra's v1
// endpoint sends "3599").
func seconds(raw json.RawMessage) (int64, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(f), true
}

// redact removes credentials, plain and URL-encoded, from a message.
func redact(msg string, credentials []string) string {
	for _, c := range credentials {
		if len(c) < 4 {
			continue
		}
		for _, form := range []string{c, url.QueryEscape(c), url.PathEscape(c)} {
			msg = strings.ReplaceAll(msg, form, "***")
		}
	}
	return msg
}

// oneLineCapped keeps a text on one line and at most 300 bytes.
func oneLineCapped(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// dropToken forgets the token tok, after an API refused it. A late refusal
// of an older token does not drop a newer one.
func (p *Plugin) dropToken(tok string) {
	p.oauth.mu.Lock()
	defer p.oauth.mu.Unlock()
	if p.oauth.token == tok {
		p.oauth.token = ""
	}
}
