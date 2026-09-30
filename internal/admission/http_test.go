package admission

import (
	"strings"
	"testing"
)

const httpPlugin = `apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: booking, version: 1.0.0 }
spec:
  egress: { hosts: [booking-com15.p.rapidapi.com] }
  connection:
    baseUrl: https://booking-com15.p.rapidapi.com/api/v1
    auth: { header: x-rapidapi-key, secret: BOOKING_RAPIDAPI_KEY }
  actions:
    search-flights:
      input: { fromId: string, toId: string, departDate: string }
      output: "list<booking.Offer>"
      effect: read
      http:
        method: GET
        path: /flights/searchFlights
        query: { fromId: "${ inputs.fromId }", toId: "${ inputs.toId }", departDate: "${ inputs.departDate }", sort: BEST }
        headers: { x-rapidapi-host: booking-com15.p.rapidapi.com }
        output: "sources.response.body.data.flightOffers"
`

func TestHTTPBaselineIsClean(t *testing.T) {
	if is := errorsOnly(validate(t, map[string]string{"p.yaml": httpPlugin})); len(is) > 0 {
		t.Fatalf("baseline must be clean: %v", is)
	}
}

func TestHTTPRules(t *testing.T) {
	cases := []struct{ name, old, new, want string }{
		{"host outside egress", "hosts: [booking-com15.p.rapidapi.com]", "hosts: [other.example]", "EGRESS_NOT_ALLOWED@spec.connection.baseUrl"},
		{"plain http", "baseUrl: https://", "baseUrl: http://", "INVALID_FIELD@spec.connection.baseUrl"},
		{"credentials in the URL", "rapidapi.com/api/v1", "rapidapi.com/api/v1?key=abc", "INVALID_FIELD@spec.connection.baseUrl"},
		{"literal secret header", "headers: { x-rapidapi-host: booking-com15.p.rapidapi.com }", "headers: { x-rapidapi-key: 5f3a9c }", "LITERAL_SECRET@spec.actions.search-flights.http.headers.x-rapidapi-key"},
		{"literal secret in query", "sort: BEST }", "sort: BEST, key: AIzaSy123 }", "LITERAL_SECRET@spec.actions.search-flights.http.query.key"},
		{"no connection", "  connection:\n    baseUrl: https://booking-com15.p.rapidapi.com/api/v1\n    auth: { header: x-rapidapi-key, secret: BOOKING_RAPIDAPI_KEY }\n", "", "MISSING_FIELD@spec.connection"},
		{"auth in two places", "auth: { header: x-rapidapi-key,", "auth: { header: x-rapidapi-key, query: key,", "ONE_STRATEGY@spec.connection.auth"},
		{"output reads another source", "sources.response.body", "sources.probe.body", "UNRESOLVED_REF@spec.actions.search-flights.http.output"},
		{"query reads an unknown parameter", "${ inputs.toId }", "${ inputs.to }", "UNRESOLVED_REF@spec.actions.search-flights.http.query"},
		{"body on GET", "        output:", "        body: { a: 1 }\n        output:", "UNUSED_FIELD@spec.actions.search-flights.http.body"},
		{"lowercase secret name", "secret: BOOKING_RAPIDAPI_KEY", "secret: booking", "INVALID_FIELD@"},
		// Review 2026-09-23: bypasses of the literal-secret check.
		{"secret as a constant expression", "sort: BEST }", "sort: BEST, api_key: \"${ 'sk_live_abc' }\" }", "LITERAL_SECRET@spec.actions.search-flights.http.query.api_key"},
		{"secret in a list", "sort: BEST }", "sort: BEST, token: [sk_live_abc] }", "LITERAL_SECRET@spec.actions.search-flights.http.query.token"},
		{"secret in a nested body", "        output:", "        body: { auth: { client_secret: sk_live } }\n        output:", "LITERAL_SECRET@spec.actions.search-flights.http.body.auth.client_secret"},
		{"secret in the auth prefix", "secret: BOOKING_RAPIDAPI_KEY }", "secret: BOOKING_RAPIDAPI_KEY, prefix: \"Bearer sk_live_abc\" }", "LITERAL_SECRET@spec.connection.auth.prefix"},
		{"credential header with another name", "x-rapidapi-host: booking-com15.p.rapidapi.com }", "x-rapidapi-host: booking-com15.p.rapidapi.com, X-Api-Token: abc }", "LITERAL_SECRET@spec.actions.search-flights.http.headers.X-Api-Token"},
		{"header set by the platform", "x-rapidapi-host: booking-com15.p.rapidapi.com }", "x-rapidapi-host: booking-com15.p.rapidapi.com, idempotency-key: fixed }", "INVALID_FIELD@spec.actions.search-flights.http.headers.idempotency-key"},
		{"metadata address", "hosts: [booking-com15.p.rapidapi.com] }\n  connection:\n    baseUrl: https://booking-com15.p.rapidapi.com/api/v1", "hosts: [169.254.169.254] }\n  connection:\n    baseUrl: https://169.254.169.254/latest", "EGRESS_NOT_ALLOWED@spec.connection.baseUrl"},
		{"read that is not a GET", "method: GET", "method: POST", "EFFECT_NOT_ALLOWED@spec.actions.search-flights.http.method"},
		{"safe on a GET", "method: GET", "method: GET\n        safe: true", "INVALID_FIELD@spec.actions.search-flights.http.safe"},
		{"safe on a PUT", "method: GET", "method: PUT\n        safe: true", "INVALID_FIELD@spec.actions.search-flights.http.safe"},
		{"output reads a scope the runtime does not give it", "sources.response.body.data.flightOffers", "caller.id", "SCOPE_NOT_ALLOWED@spec.actions.search-flights.http.output"},
		{"query reads a scope the runtime does not give it", "${ inputs.toId }", "${ env.TO }", "SCOPE_NOT_ALLOWED@spec.actions.search-flights.http.query"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(httpPlugin, c.old) {
				t.Fatalf("fixture does not contain %q", c.old)
			}
			got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": strings.Replace(httpPlugin, c.old, c.new, 1)})))
			if !strings.Contains(got, c.want) {
				t.Errorf("want %s, got %s", c.want, got)
			}
		})
	}
}

// A read action may POST a query when it says so (a route matrix, a
// GraphQL query); nothing else changes.
func TestSafePostIsARead(t *testing.T) {
	src := strings.Replace(httpPlugin, "method: GET", "method: POST\n        safe: true", 1)
	if got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": src}))); got != "" {
		t.Errorf("a safe POST read must be admitted: %s", got)
	}
}

// backoffice-004: connection.oauth2 (client credentials).
func TestHTTPOAuth2(t *testing.T) {
	base := strings.Replace(httpPlugin, "hosts: [booking-com15.p.rapidapi.com]", "hosts: [booking-com15.p.rapidapi.com, login.microsoftonline.com]", 1)
	base = strings.Replace(base, "    auth: { header: x-rapidapi-key, secret: BOOKING_RAPIDAPI_KEY }\n",
		"    oauth2: { tokenUrl: \"https://login.microsoftonline.com/t1/oauth2/v2.0/token\", clientId: APP_ID, clientSecret: APP_PASSWORD, scope: \"https://api.botframework.com/.default\" }\n", 1)
	if is := errorsOnly(validate(t, map[string]string{"p.yaml": base})); len(is) > 0 {
		t.Fatalf("oauth2 refused: %v", is)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"token host outside egress", "booking-com15.p.rapidapi.com, login.microsoftonline.com]", "booking-com15.p.rapidapi.com]", "EGRESS_NOT_ALLOWED@spec.connection.oauth2.tokenUrl"},
		{"plain http token URL", "tokenUrl: \"https://", "tokenUrl: \"http://", "INVALID_FIELD@spec.connection.oauth2.tokenUrl"},
		{"auth and oauth2", "    oauth2:", "    auth: { header: x-k, secret: K }\n    oauth2:", "ONE_STRATEGY@spec.connection.oauth2"},
		{"a literal client secret", "clientSecret: APP_PASSWORD", "clientSecret: s3cr3t~value", "INVALID_FIELD@spec.connection.oauth2.clientSecret"},
	} {
		got := codesAt(errorsOnly(validate(t, map[string]string{"p.yaml": strings.Replace(base, c.old, c.new, 1)})))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}
