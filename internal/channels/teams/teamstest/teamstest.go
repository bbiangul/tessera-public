// Package teamstest is a stand-in for the Bot Framework's key publication,
// for tests and benchmarks: an HTTPS server with an OpenID configuration
// and keys, and a signer for activity tokens.
package teamstest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/bbiangul/tessera/internal/channels/teams"
)

// Keys are the kids the stand-in publishes: Teams is endorsed for
// msteams, Other only for another channel. Unknown is never published.
const (
	Teams   = "k-teams"
	Other   = "k-other"
	Unknown = "k-unknown"
)

// BotFramework publishes keys and signs tokens.
type BotFramework struct {
	Server *httptest.Server
	// Issuer is the issuer the OpenID configuration names; teams.Issuer
	// unless set (a sovereign cloud's).
	Issuer string
	keys   map[string]*rsa.PrivateKey
}

// New starts the stand-in; Close it after.
func New() *BotFramework {
	b := &BotFramework{keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range []string{Teams, Other, Unknown} {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		b.keys[kid] = k
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openid", func(w http.ResponseWriter, _ *http.Request) {
		iss := b.Issuer
		if iss == "" {
			iss = teams.Issuer
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": iss, "jwks_uri": b.Server.URL + "/keys"})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		var keys []any
		for kid, end := range map[string][]string{Teams: {"msteams", "webchat"}, Other: {"webchat"}} {
			pub := b.keys[kid].PublicKey
			keys = append(keys, map[string]any{"kty": "RSA", "use": "sig", "kid": kid, "endorsements": end,
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})
	b.Server = httptest.NewTLSServer(mux)
	return b
}

// OpenIDURL is the stand-in's OpenID configuration.
func (b *BotFramework) OpenIDURL() string { return b.Server.URL + "/openid" }

// Client trusts the stand-in's certificate.
func (b *BotFramework) Client() *http.Client { return b.Server.Client() }

// Close stops the stand-in.
func (b *BotFramework) Close() { b.Server.Close() }

// Token is what to sign; zero fields get the Bot Framework's values.
type Token struct {
	Kid        string
	Issuer     string
	AppID      string
	ServiceURL string
	Expires    time.Time
}

// Sign returns a Bearer header for t.
func (b *BotFramework) Sign(t Token) string {
	if t.Kid == "" {
		t.Kid = Teams
	}
	if t.Issuer == "" {
		t.Issuer = teams.Issuer
	}
	head, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": t.Kid})
	body, _ := json.Marshal(map[string]any{"iss": t.Issuer, "aud": t.AppID, "serviceUrl": t.ServiceURL,
		"nbf": t.Expires.Add(-time.Hour).Unix(), "exp": t.Expires.Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, b.keys[t.Kid], crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return "Bearer " + unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Click is the activity Teams delivers when user (an Entra object ID) in
// tenant clicks the card cardID's button carrying id.
func Click(serviceURL, tenant, user, cardID, id string) []byte {
	b, _ := json.Marshal(map[string]any{
		"type": "message", "channelId": "msteams", "serviceUrl": serviceURL, "replyToId": cardID,
		"from":         map[string]any{"id": "29:x", "aadObjectId": user},
		"conversation": map[string]any{"id": "a:dm-" + user, "conversationType": "personal", "tenantId": tenant},
		"channelData":  map[string]any{"tenant": map[string]any{"id": tenant}},
		"value":        map[string]any{"tessera": id},
	})
	return b
}
