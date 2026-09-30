package teams

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// The Bot Framework signs every activity it delivers with a JWT (RS256).
// Its keys are published at the OpenID configuration's jwks_uri, each
// endorsed for the channels that may use it. See "Authentication with the
// Bot Connector API": the issuer, the audience (the bot's app ID), the
// validity, the serviceUrl claim and the key's endorsement are all checked.

// DefaultOpenIDURL is the Bot Framework's OpenID configuration.
const DefaultOpenIDURL = "https://login.botframework.com/v1/.well-known/openidconfiguration"

// Issuer is the Bot Framework's token issuer in the public cloud; the
// OpenID configuration's issuer is used when it names one (sovereign
// clouds: https://api.botframework.us).
const Issuer = "https://api.botframework.com"

// skew is the clock difference tolerated on exp and nbf (the Bot
// Framework's own libraries allow 5 minutes).
const skew = 5 * time.Minute

type jwk struct {
	Kty          string   `json:"kty"`
	Kid          string   `json:"kid"`
	N            string   `json:"n"`
	E            string   `json:"e"`
	Endorsements []string `json:"endorsements"`
}

type key struct {
	pub          *rsa.PublicKey
	endorsements []string
}

// keySet fetches and keeps the Bot Framework's signing keys, and the
// issuer its OpenID configuration names.
type keySet struct {
	openID string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	keys     map[string]key
	issuer   string
	fetched  time.Time // last successful fetch
	tried    time.Time // last attempt, successful or not
	fetching chan struct{}
}

// minRefetch spaces fetches: an unknown kid (anyone can send one) or an
// unreachable endpoint makes at most one fetch per interval, never one per
// request.
const minRefetch = time.Minute

// get returns the key kid. Keys are refreshed when a day old, or when kid
// is unknown; either at most once per minRefetch. The fetch runs outside
// the lock, one at a time: a request with a known key never waits for it.
func (ks *keySet) get(ctx context.Context, kid string) (key, string, error) {
	ks.mu.Lock()
	k, ok := ks.keys[kid]
	issuer := ks.issuer
	stale := ks.keys == nil || !ok || ks.now().Sub(ks.fetched) >= 24*time.Hour
	if !stale || ks.now().Sub(ks.tried) < minRefetch {
		ks.mu.Unlock()
		if !ok {
			return key{}, "", errors.New("unknown signing key")
		}
		return k, issuer, nil
	}
	if ok && ks.fetching != nil {
		ks.mu.Unlock()
		return k, issuer, nil // known key, a refresh is under way
	}
	wait := ks.fetching
	if wait == nil {
		ks.fetching = make(chan struct{})
		ks.tried = ks.now()
		done := ks.fetching
		ks.mu.Unlock()
		keys, iss, err := ks.fetch(ctx)
		ks.mu.Lock()
		if err == nil {
			ks.keys, ks.issuer, ks.fetched = keys, iss, ks.now()
		}
		ks.fetching = nil
		close(done)
	} else {
		ks.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return key{}, "", ctx.Err()
		}
		ks.mu.Lock()
	}
	k, ok = ks.keys[kid]
	issuer = ks.issuer
	ks.mu.Unlock()
	if !ok {
		return key{}, "", errors.New("unknown signing key")
	}
	return k, issuer, nil
}

func (ks *keySet) fetch(ctx context.Context) (map[string]key, string, error) {
	var conf struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := ks.getJSON(ctx, ks.openID, &conf); err != nil {
		return nil, "", fmt.Errorf("openid configuration: %w", err)
	}
	if !strings.HasPrefix(conf.JWKSURI, "https://") {
		return nil, "", errors.New("openid configuration: jwks_uri is not https")
	}
	if conf.Issuer == "" {
		conf.Issuer = Issuer
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := ks.getJSON(ctx, conf.JWKSURI, &set); err != nil {
		return nil, "", fmt.Errorf("keys: %w", err)
	}
	keys := map[string]key{}
	for _, j := range set.Keys {
		if j.Kty != "RSA" || j.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(j.N)
		e, err2 := base64.RawURLEncoding.DecodeString(j.E)
		if err1 != nil || err2 != nil || len(e) > 4 {
			continue
		}
		keys[j.Kid] = key{pub: &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, endorsements: j.Endorsements}
	}
	if len(keys) == 0 {
		return nil, "", errors.New("keys: none usable")
	}
	return keys, conf.Issuer, nil
}

func (ks *keySet) getJSON(ctx context.Context, url string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := ks.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// claims are the token's claims checked here.
type claims struct {
	Iss        string  `json:"iss"`
	Aud        any     `json:"aud"`
	Exp        float64 `json:"exp"`
	Nbf        float64 `json:"nbf"`
	ServiceURL string  `json:"serviceurl"`
}

// verify checks a Bearer token for an activity on channel with
// serviceURL, for the bot appID.
func (ks *keySet) verify(ctx context.Context, header, appID, channel, serviceURL string) error {
	tok, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return errors.New("no bearer token")
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return errors.New("not a JWT")
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &h); err != nil {
		return err
	}
	if h.Alg != "RS256" {
		return fmt.Errorf("algorithm %q", h.Alg)
	}
	k, issuer, err := ks.get(ctx, h.Kid)
	if err != nil {
		return err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("bad signature encoding")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(k.pub, crypto.SHA256, sum[:], sig); err != nil {
		return errors.New("bad signature")
	}
	if !slices.Contains(k.endorsements, channel) {
		return fmt.Errorf("the key is not endorsed for %q", channel)
	}
	// Claim names are compared case-insensitively ("serviceUrl" in the
	// Bot Framework's tokens), as its libraries do.
	var raw map[string]any
	if err := decodeSegment(parts[1], &raw); err != nil {
		return err
	}
	lower := map[string]any{}
	for name, v := range raw {
		lower[strings.ToLower(name)] = v
	}
	b, _ := json.Marshal(lower)
	var c claims
	if err := json.Unmarshal(b, &c); err != nil {
		return errors.New("bad claims")
	}
	now := ks.now()
	switch {
	case c.Iss != issuer:
		return fmt.Errorf("issuer %q", c.Iss)
	case !audience(c.Aud, appID):
		return errors.New("the token is for another bot")
	case c.Exp == 0 || now.After(time.Unix(int64(c.Exp), 0).Add(skew)):
		return errors.New("expired")
	case c.Nbf != 0 && now.Before(time.Unix(int64(c.Nbf), 0).Add(-skew)):
		return errors.New("not valid yet")
	case strings.TrimSuffix(c.ServiceURL, "/") != strings.TrimSuffix(serviceURL, "/") || serviceURL == "":
		return errors.New("the serviceUrl claim is not the activity's")
	}
	return nil
}

func audience(aud any, appID string) bool {
	switch a := aud.(type) {
	case string:
		return a == appID && appID != ""
	case []any:
		for _, x := range a {
			if s, _ := x.(string); s == appID && appID != "" {
				return true
			}
		}
	}
	return false
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return errors.New("bad token encoding")
	}
	if err := json.Unmarshal(b, v); err != nil {
		return errors.New("bad token encoding")
	}
	return nil
}
