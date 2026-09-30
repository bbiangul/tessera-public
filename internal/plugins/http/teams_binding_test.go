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

// TeamsBindings are the documented bindings of the teams plugin
// (docs/guides/authoring.md), with the Bot Framework at base.
func teamsBindings(base string) map[string]plugins.ActionSpec {
	return map[string]plugins.ActionSpec{
		"open-dm": {Effect: plugins.EffectIdempotentWrite, Input: map[string]string{"user": "string", "tenant": "string", "bot": "string"}, Output: "map",
			HTTP: &plugins.HTTPCall{Method: "POST", Path: "/v3/conversations", Body: map[string]any{
				"bot": "${ {'id': inputs.bot} }", "members": "${ [{'id': inputs.user}] }", "isGroup": false,
				"channelData": "${ {'tenant': {'id': inputs.tenant}} }",
			}}},
		"post-task": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"conversation": "string", "text": "string", "buttons": "list"}, Output: "map",
			HTTP: &plugins.HTTPCall{Method: "POST", Path: "/v3/conversations/{{ inputs.conversation }}/activities", Body: map[string]any{
				"type": "message",
				"attachments": "${ [{'contentType': 'application/vnd.microsoft.card.adaptive', 'content': {'type': 'AdaptiveCard', '$schema': 'http://adaptivecards.io/schemas/adaptive-card.json', 'version': '1.4', " +
					"'body': [{'type': 'TextBlock', 'text': inputs.text, 'wrap': true}], " +
					"'actions': inputs.buttons.map(b, {'type': 'Action.Submit', 'title': b.title, 'style': b.style, 'data': {'tessera': b.id}})}}] }",
			}}},
		"update-task": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"conversation": "string", "activity": "string", "text": "string"},
			HTTP: &plugins.HTTPCall{Method: "PUT", Path: "/v3/conversations/{{ inputs.conversation }}/activities/{{ inputs.activity }}", Body: map[string]any{
				"type": "message", "text": "${ inputs.text }",
			}}},
	}
}

// backoffice-004: the documented Bot Framework bindings send what the
// Connector API expects, with the token from connection.oauth2.
func TestTeamsTaskBindings(t *testing.T) {
	type seen struct {
		method, path, auth string
		body               map[string]any
	}
	var got []seen
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"bf-token","expires_in":3600}`)
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		got = append(got, seen{r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), b})
		_, _ = io.WriteString(w, `{"id":"a:1qhNLq"}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "teams", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress: &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL, OAuth2: &plugins.OAuth2{TokenURL: srv.URL + "/token", ClientID: "TEAMS_APP_ID", ClientSecret: "TEAMS_APP_PASSWORD",
			Scope: "https://api.botframework.com/.default"}},
		Actions: teamsBindings(srv.URL),
	}}
	p, err := NewWithOptions(m, secrets{"TEAMS_APP_ID": "app", "TEAMS_APP_PASSWORD": "pw"}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	ctx := context.Background()
	out, err := r.Call(ctx, act.Call{Action: "teams/open-dm", IdempotencyKey: "dm", Args: map[string]any{"user": "0a0a-rita", "tenant": "t1", "bot": "28:app"}})
	if err != nil || out.(map[string]any)["id"] != "a:1qhNLq" {
		t.Fatalf("%v %v", out, err)
	}
	buttons := []any{map[string]any{"id": "task:h:approve", "title": "Aprovar", "style": "positive"}, map[string]any{"id": "task:h:deny", "title": "Recusar", "style": "destructive"}}
	if _, err := r.Call(ctx, act.Call{Action: "teams/post-task", IdempotencyKey: "p", Args: map[string]any{"conversation": "a:1qhNLq", "text": "Desconto de 10%?", "buttons": buttons}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Call(ctx, act.Call{Action: "teams/update-task", IdempotencyKey: "u", Args: map[string]any{"conversation": "a:1qhNLq", "activity": "1:abc", "text": "✅ Aprovado por Rita."}}); err != nil {
		t.Fatal(err)
	}
	enc := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	want := []struct{ method, path, body string }{
		{"POST", "/v3/conversations", `{"bot":{"id":"28:app"},"channelData":{"tenant":{"id":"t1"}},"isGroup":false,"members":[{"id":"0a0a-rita"}]}`},
		{"POST", "/v3/conversations/a:1qhNLq/activities", `{"attachments":[{"content":{"$schema":"http://adaptivecards.io/schemas/adaptive-card.json","actions":[{"data":{"tessera":"task:h:approve"},"style":"positive","title":"Aprovar","type":"Action.Submit"},{"data":{"tessera":"task:h:deny"},"style":"destructive","title":"Recusar","type":"Action.Submit"}],"body":[{"text":"Desconto de 10%?","type":"TextBlock","wrap":true}],"type":"AdaptiveCard","version":"1.4"},"contentType":"application/vnd.microsoft.card.adaptive"}],"type":"message"}`},
		{"PUT", "/v3/conversations/a:1qhNLq/activities/1:abc", `{"text":"✅ Aprovado por Rita.","type":"message"}`},
	}
	for i, w := range want {
		if got[i].method != w.method || got[i].path != w.path || enc(got[i].body) != w.body || got[i].auth != "Bearer bf-token" {
			t.Errorf("%d: %s %s %s %s\nwant %s %s %s", i, got[i].method, got[i].path, got[i].auth, enc(got[i].body), w.method, w.path, w.body)
		}
	}
}
