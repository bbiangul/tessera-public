package admission

import (
	"strings"
	"testing"
)

// B1: tokenUrl skips the private/link-local address check baseUrl has.
func TestAdvOAuth2TokenURLInternalIP(t *testing.T) {
	base := strings.Replace(httpPlugin, "hosts: [booking-com15.p.rapidapi.com]", "hosts: [booking-com15.p.rapidapi.com, 169.254.169.254]", 1)
	base = strings.Replace(base, "    auth: { header: x-rapidapi-key, secret: BOOKING_RAPIDAPI_KEY }\n",
		"    oauth2: { tokenUrl: \"https://169.254.169.254/latest/token\", clientId: APP_ID, clientSecret: APP_PASSWORD }\n", 1)
	got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": base})))
	t.Logf("tokenUrl on 169.254.169.254: errors=%q", got)
	if !strings.Contains(got, "EGRESS_NOT_ALLOWED@spec.connection.oauth2.tokenUrl") {
		t.Errorf("a metadata-address tokenUrl was admitted (baseUrl on the same host is refused)")
	}
}

// B2: a queue with channels: [slack|teams|email] whose role holders have no
// address on that channel is admitted: its tasks reach nobody there.
func TestAdvChannelWithoutAddresses(t *testing.T) {
	docs := `apiVersion: agents.tessera.dev/v1alpha1
kind: Team
metadata: { name: equipe, namespace: ns }
spec:
  members:
    - { id: rita, name: Rita, roles: [recepcao] }
    - { id: gil, name: Gil, roles: [outra], slack: U0GIL }
---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: slack, version: 0.1.0 }
spec:
  actions:
    post-task: { input: { to: string, text: string, buttons: list }, effect: external-message }
    update-task: { input: { channel: string, ts: string, text: string }, effect: external-message }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: ns }
spec: { roles: [recepcao], channels: [slack] }
`
	got := codesAt(errorsOnly(validate(t, map[string]string{"a.yaml": docs})))
	all := codesAt(validate(t, map[string]string{"a.yaml": docs}))
	t.Logf("errors=%q all issues=%q", got, all)
	if all == "" {
		t.Errorf("channels: [slack] admitted with no recepcao member holding a Slack ID")
	}
}
