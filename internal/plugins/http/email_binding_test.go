package http

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

// backoffice-005 (E8): the documented e-mail bindings. Outlook: Graph
// sendMail as the agent's mailbox (202, no body), with a client-credentials
// token. Gmail: messages.send with the adapter's raw message, with a
// refresh-token token.
func TestEmailBindings(t *testing.T) {
	type seen struct{ path, auth, body string }
	var got []seen
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			_, _ = io.WriteString(w, `{"access_token":"tok-`+r.PostForm.Get("grant_type")+`","expires_in":3600}`)
			return
		}
		var b any
		_ = json.NewDecoder(r.Body).Decode(&b)
		enc, _ := json.Marshal(b)
		got = append(got, seen{r.URL.EscapedPath(), r.Header.Get("Authorization"), string(enc)})
		if r.URL.Path == "/v1.0/users/agente@clinica.example/sendMail" {
			w.WriteHeader(202)
			return
		}
		_, _ = io.WriteString(w, `{"id":"18c1","threadId":"18c1","labelIds":["SENT"]}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	in := map[string]string{"to": "string", "subject": "string", "text": "string", "link": "string", "raw": "string"}
	outlook := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "email", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress: &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL + "/v1.0", OAuth2: &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "GRAPH_CLIENT_ID", ClientSecret: "GRAPH_CLIENT_SECRET",
			Scope: "https://graph.microsoft.com/.default"}},
		Actions: map[string]plugins.ActionSpec{"send-task": {Effect: plugins.EffectExternalMessage, Input: in, HTTP: &plugins.HTTPCall{Method: "POST", Path: "/users/agente@clinica.example/sendMail", Body: map[string]any{
			"message":         "${ {'subject': inputs.subject, 'body': {'contentType': 'Text', 'content': inputs.text}, 'toRecipients': [{'emailAddress': {'address': inputs.to}}]} }",
			"saveToSentItems": false,
		}}}},
	}}
	gmail := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "email", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress: &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL + "/gmail/v1", OAuth2: &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "GMAIL_CLIENT_ID", ClientSecret: "GMAIL_CLIENT_SECRET",
			RefreshToken: "GMAIL_REFRESH_TOKEN"}},
		Actions: map[string]plugins.ActionSpec{"send-task": {Effect: plugins.EffectExternalMessage, Input: in, HTTP: &plugins.HTTPCall{Method: "POST", Path: "/users/me/messages/send", Body: map[string]any{
			"raw": "${ inputs.raw }",
		}}}},
	}}
	sec := secrets{"GRAPH_CLIENT_ID": "g", "GRAPH_CLIENT_SECRET": "gs", "GMAIL_CLIENT_ID": "c", "GMAIL_CLIENT_SECRET": "cs", "GMAIL_REFRESH_TOKEN": "1//r"}
	args := map[string]any{"to": "rita@clinica.example", "subject": "Aprovação pendente: Desconto", "text": "Olá, Rita.\n\nlink", "link": "https://x/tasks/t", "raw": "VG86IHJpdGE="}
	for _, m := range []*plugins.PluginManifest{outlook, gmail} {
		p, err := NewWithOptions(m, sec, srv.Client(), local)
		if err != nil {
			t.Fatal(err)
		}
		r := act.NewRegistry()
		p.Register(r)
		if _, err := r.Call(context.Background(), act.Call{Action: "email/send-task", IdempotencyKey: "k", Args: args}); err != nil {
			t.Fatalf("%s: %v", m.Spec.Connection.BaseURL, err)
		}
	}
	want := []seen{
		{"/v1.0/users/agente@clinica.example/sendMail", "Bearer tok-client_credentials", `{"message":{"body":{"content":"Olá, Rita.\n\nlink","contentType":"Text"},"subject":"Aprovação pendente: Desconto","toRecipients":[{"emailAddress":{"address":"rita@clinica.example"}}]},"saveToSentItems":false}`},
		{"/gmail/v1/users/me/messages/send", "Bearer tok-refresh_token", `{"raw":"VG86IHJpdGE="}`},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("%d:\n%+v\nwant\n%+v", i, got[i], w)
		}
	}
}
