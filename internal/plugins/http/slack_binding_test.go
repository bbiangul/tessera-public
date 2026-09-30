package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
)

// backoffice-003: the documented Slack bindings. post-task sends
// chat.postMessage to the member's user ID (their DM with the app) with a
// section and two buttons whose action_ids are the task ids; update-task
// replaces the buttons with the outcome (chat.update). Slack reports
// failures as {"ok": false} with status 200: failWhen makes them fail.
func TestSlackTaskBindings(t *testing.T) {
	var bodies []map[string]any
	var paths []string
	srv := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies, paths = append(bodies, b), append(paths, r.URL.Path)
		if b["channel"] == "U0GONE" {
			_, _ = io.WriteString(w, `{"ok":false,"error":"user_not_found"}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"channel":"D0DM","ts":"1.5"}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	m := &plugins.PluginManifest{Metadata: plugins.PluginMeta{Name: "slack", Version: "0.1.0"}, Spec: plugins.PluginSpec{
		Egress:     &plugins.Egress{Hosts: []string{u.Hostname()}},
		Connection: &plugins.Connection{BaseURL: srv.URL},
		Actions: map[string]plugins.ActionSpec{
			"post-task": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"to": "string", "text": "string", "buttons": "list"},
				HTTP: &plugins.HTTPCall{Method: "POST", Path: "/chat.postMessage", FailWhen: "!sources.response.body.ok", Body: map[string]any{
					"channel": "${ inputs.to }",
					"text":    "${ inputs.text }",
					"blocks": "${ [{'type': 'section', 'text': {'type': 'mrkdwn', 'text': inputs.text}}, " +
						"{'type': 'actions', 'elements': inputs.buttons.map(b, {'type': 'button', 'action_id': b.id, 'style': b.style, 'text': {'type': 'plain_text', 'text': b.title}})}] }",
				}}},
			"update-task": {Effect: plugins.EffectExternalMessage, Input: map[string]string{"channel": "string", "ts": "string", "text": "string"},
				HTTP: &plugins.HTTPCall{Method: "POST", Path: "/chat.update", FailWhen: "!sources.response.body.ok", Body: map[string]any{
					"channel": "${ inputs.channel }", "ts": "${ inputs.ts }", "text": "${ inputs.text }", "blocks": "${ [] }",
				}}},
		},
	}}
	p, err := NewWithOptions(m, secrets{}, srv.Client(), local)
	if err != nil {
		t.Fatal(err)
	}
	r := act.NewRegistry()
	p.Register(r)
	ctx := context.Background()
	buttons := []any{map[string]any{"id": "task:h:approve", "title": "Aprovar", "style": "primary"}, map[string]any{"id": "task:h:deny", "title": "Recusar", "style": "danger"}}
	if _, err := r.Call(ctx, act.Call{Action: "slack/post-task", IdempotencyKey: "k1", Args: map[string]any{"to": "U0RITA", "text": "Desconto de 10%?", "buttons": buttons}}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(bodies[0])
	want := `{"blocks":[{"text":{"text":"Desconto de 10%?","type":"mrkdwn"},"type":"section"},{"elements":[{"action_id":"task:h:approve","style":"primary","text":{"text":"Aprovar","type":"plain_text"},"type":"button"},{"action_id":"task:h:deny","style":"danger","text":{"text":"Recusar","type":"plain_text"},"type":"button"}],"type":"actions"}],"channel":"U0RITA","text":"Desconto de 10%?"}`
	if paths[0] != "/chat.postMessage" || string(got) != want {
		t.Errorf("%s\n%s\nwant\n%s", paths[0], got, want)
	}
	if _, err := r.Call(ctx, act.Call{Action: "slack/update-task", IdempotencyKey: "k2", Args: map[string]any{"channel": "D0DM", "ts": "1.5", "text": "✅ Aprovado por Rita."}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := json.Marshal(bodies[1]); paths[1] != "/chat.update" || string(got) != `{"blocks":[],"channel":"D0DM","text":"✅ Aprovado por Rita.","ts":"1.5"}` {
		t.Errorf("%s %s", paths[1], got)
	}
	_, err = r.Call(ctx, act.Call{Action: "slack/post-task", IdempotencyKey: "k3", Args: map[string]any{"to": "U0GONE", "text": "x", "buttons": buttons}})
	var definite *act.DefiniteError
	if !errors.As(err, &definite) {
		t.Errorf("ok:false must fail definitely: %v", err)
	}
}
