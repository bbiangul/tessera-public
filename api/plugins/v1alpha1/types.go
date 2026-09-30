// Package v1alpha1 holds the plugin manifest type (RFC-0001,
// "Extensibilidade"). A plugin publishes what it can do and what each action
// does to the world; admission uses it to check every Function that calls
// the plugin. The doc comments are the reference documentation.
package v1alpha1

// APIVersion of the resources in this package.
const APIVersion = "plugins.tessera.dev/v1alpha1"

// Effect classes, in increasing order of risk. external-message is a kind of
// irreversible effect with extra channel rules.
const (
	EffectRead            = "read"
	EffectIdempotentWrite = "idempotent-write"
	EffectIrreversible    = "irreversible"
	EffectExternalMessage = "external-message"
)

// Verification modes: how the platform can tell whether an effect happened.
const (
	VerifyNative = "native" // the external system accepts an idempotency key
	VerifyProbe  = "probe"  // a read action can check whether it happened
	VerifyNone   = "none"   // nobody can tell; an ambiguous failure goes to a human
)

// PluginManifest declares a plugin's actions.
type PluginManifest struct {
	// APIVersion must be plugins.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=plugins.tessera.dev/v1alpha1"`
	// Kind must be PluginManifest.
	Kind string `json:"kind" jsonschema:"enum=PluginManifest"`
	// Metadata names and versions the plugin.
	Metadata PluginMeta `json:"metadata"`
	// Spec lists what the plugin needs and offers.
	Spec PluginSpec `json:"spec"`
}

// PluginMeta identifies a plugin.
type PluginMeta struct {
	// Name of the plugin, used as the prefix in plugin/action.
	Name string `json:"name" jsonschema:"pattern=^[a-z0-9-]+$"`
	// Version in semver.
	Version string `json:"version" jsonschema:"pattern=^[0-9]+\\.[0-9]+\\.[0-9]+"`
}

// PluginSpec describes a plugin.
type PluginSpec struct {
	// Egress lists the hosts the plugin may contact.
	Egress *Egress `json:"egress,omitempty"`
	// Connection says how the built-in http plugin reaches the system, for
	// actions that declare http. Credentials are always secret references.
	Connection *Connection `json:"connection,omitempty"`
	// SQL gives the plugin its own database, for actions that declare sql:
	// the agent's state between turns (profiles, last search results).
	SQL *SQLDatabase `json:"sql,omitempty"`
	// Channel declares the rules of a messaging channel (WhatsApp, SMS),
	// which the platform applies to every message it carries: the free-text
	// window and consent. An Agent whose spec.channel names this plugin
	// gets them on its messages to the user too.
	Channel *Channel `json:"channel,omitempty"`
	// Actions by name.
	Actions map[string]ActionSpec `json:"actions" jsonschema:"minProperties=1"`
	// Types declare the plugin's objects (Appointment, Slot): their
	// fields and field types. An action whose output names a declared
	// type has its reads checked against it, so a changed API is noticed
	// ("CONTRACT") and, with improvise.repairReads, repaired by renaming
	// fields (drift-008). Undeclared plugin types are any object.
	Types map[string]TypeSpec `json:"types,omitempty"`
}

// TypeSpec is an object's fields.
type TypeSpec struct {
	// Fields by name, with their types: a basic type (string, int, float,
	// bool, date, timestamp, duration, map, any), another plugin.Type or
	// list<T>. A trailing "?" makes a field optional.
	Fields map[string]string `json:"fields" jsonschema:"minProperties=1"`
}

// Channel holds a messaging channel's rules.
type Channel struct {
	// Window is how long after the user's last message free text may be
	// sent (Go duration; WhatsApp: 24h). Outside it, only template actions
	// (external-message without freeText) may reach the user. A step that
	// would send free text outside the window fails with the definite
	// error WINDOW_CLOSED, which its onError can answer with a template.
	Window string `json:"window,omitempty"`
	// Consent is a boolean expression on caller and event that must be
	// true before any external-message action of this plugin runs (LGPD
	// opt-in, do-not-contact); otherwise the action fails with the
	// definite error CONSENT_MISSING and nothing is sent.
	Consent string `json:"consent,omitempty"`
}

// Egress limits a plugin's network access.
type Egress struct {
	// Hosts the plugin may contact. "*.example.com" allows its subdomains
	// (not example.com itself), e.g. a tenant's *.sharepoint.com.
	Hosts []string `json:"hosts" jsonschema:"minItems=1"`
}

// ActionSpec describes one action.
type ActionSpec struct {
	// Input maps each parameter name to its type. All parameters are required.
	Input map[string]string `json:"input,omitempty"`
	// Output is the type the action returns.
	Output string `json:"output,omitempty"`
	// Effect is what the action does to the world. The manifest is the
	// authority: a Function may treat an action as riskier, never as safer.
	Effect string `json:"effect" jsonschema:"enum=read,enum=idempotent-write,enum=irreversible,enum=external-message"`
	// Verify says how to tell whether the effect happened after an ambiguous
	// failure. Missing means none.
	Verify *Verify `json:"verify,omitempty"`
	// Compensate names the action that undoes this one, if any.
	Compensate string `json:"compensate,omitempty"`
	// Authorize is a boolean expression on inputs and caller checked by the
	// platform before every call: object-level authorization.
	Authorize string `json:"authorize,omitempty"`
	// FreeText marks an external-message action that sends free text (not
	// an approved template): the channel's window applies to it.
	FreeText bool `json:"freeText,omitempty"`
	// HTTP implements the action with the built-in http plugin.
	HTTP *HTTPCall `json:"http,omitempty"`
	// SQL implements the action with the built-in sql plugin.
	SQL *SQLStatement `json:"sql,omitempty"`
}

// SQLDatabase is a plugin's own database (SQLite when self-hosted).
type SQLDatabase struct {
	// Migrations are applied in order, once each, and never edited after
	// they ran: add a new one to change the schema.
	Migrations []string `json:"migrations" jsonschema:"minItems=1"`
}

// SQLStatement is one parameterized statement. The SQL text is literal;
// values only enter as named parameters (:name), so no input can change
// the statement. A SELECT must be a read action; any other statement must
// declare the effect it has.
type SQLStatement struct {
	// Query is the statement, with :name parameters.
	Query string `json:"query" jsonschema:"minLength=1"`
	// Params gives each :name parameter its value. Values are literal
	// unless the whole value is "${ expr }"; expressions read inputs (the
	// action's arguments) and caller.
	Params map[string]any `json:"params,omitempty"`
	// Result shapes the output: rows (a list of objects, the default for a
	// SELECT), row (the first row, or null), or affected (rows changed).
	Result string `json:"result,omitempty" jsonschema:"enum=rows,enum=row,enum=affected"`
	// JSON lists columns holding JSON text, decoded in the output.
	JSON []string `json:"json,omitempty"`
}

// Connection is the endpoint and credentials of the system a plugin calls.
type Connection struct {
	// BaseURL is https://host[:port][/prefix]; its host must be in egress.
	BaseURL string `json:"baseUrl" jsonschema:"pattern=^https://"`
	// Auth sends a secret with every request.
	Auth *Auth `json:"auth,omitempty"`
	// OAuth2 gets a bearer token by the client-credentials grant and sends
	// it with every request (the Bot Framework, Microsoft Graph). Not with
	// auth.
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`
	// PublicErrors says the system's error texts are the business's own
	// words (a new rule, a phone to call, a fee), not personal data. When
	// an action fails definitely, the platform then passes what the system
	// said on to the user unmasked; otherwise it is masked like personal
	// data (drift-002).
	PublicErrors bool `json:"publicErrors,omitempty"`
}

// OAuth2 is the client-credentials grant, or the refresh-token grant with
// refreshToken. The token is fetched through
// the plugin's egress (the token URL's host must be in egress.hosts) and
// kept until a minute before it expires, or until an API answers 401.
type OAuth2 struct {
	// TokenURL is the token endpoint, e.g.
	// https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token.
	TokenURL string `json:"tokenUrl" jsonschema:"pattern=^https://"`
	// ClientID names the secret holding the client (app) ID.
	ClientID string `json:"clientId" jsonschema:"pattern=^[A-Z][A-Z0-9_]*$"`
	// ClientSecret names the secret holding the client secret.
	ClientSecret string `json:"clientSecret" jsonschema:"pattern=^[A-Z][A-Z0-9_]*$"`
	// Scope is requested with the token, e.g.
	// https://api.botframework.com/.default.
	Scope string `json:"scope,omitempty"`
	// RefreshToken names the secret holding a refresh token: the grant is
	// then refresh_token, for an API that acts as a user who consented
	// once (Gmail).
	RefreshToken string `json:"refreshToken,omitempty" jsonschema:"pattern=^[A-Z][A-Z0-9_]*$"`
}

// Auth puts a secret in a header or a query parameter. Exactly one of header
// and query must be set.
type Auth struct {
	// Header names the header, e.g. Authorization or x-api-key.
	Header string `json:"header,omitempty"`
	// Query names the query parameter, e.g. key.
	Query string `json:"query,omitempty"`
	// Secret is the name of the secret; the value never appears in a
	// definition. Self-hosted, it is read from TESSERA_SECRET_<NAME>.
	Secret string `json:"secret" jsonschema:"pattern=^[A-Z][A-Z0-9_]*$"`
	// Prefix goes before the value, e.g. "Bearer ".
	Prefix string `json:"prefix,omitempty"`
}

// HTTPCall describes the request an action makes and how its output is read.
type HTTPCall struct {
	// Method is the HTTP method.
	Method string `json:"method" jsonschema:"enum=GET,enum=POST,enum=PUT,enum=PATCH,enum=DELETE"`
	// Path is appended to the connection's base URL. It is a template:
	// {{ inputs.id }} is escaped as one path segment.
	Path string `json:"path" jsonschema:"pattern=^/"`
	// Query parameters. Values are literal unless the whole value is
	// "${ expr }"; a null value is left out, a list repeats the parameter.
	Query map[string]any `json:"query,omitempty"`
	// Headers are literal. Credentials go in connection.auth, never here.
	Headers map[string]string `json:"headers,omitempty"`
	// Body is sent as JSON. Values are literal unless the whole value is
	// "${ expr }".
	Body map[string]any `json:"body,omitempty"`
	// Output is an expression on sources.response.body (the decoded JSON)
	// and sources.response.status; default: the whole body.
	Output string `json:"output,omitempty"`
	// FailWhen is a boolean expression on sources.response: when true, the
	// server refused the request although it answered 2xx, as APIs that
	// report errors in the body do (Slack: "!sources.response.body.ok").
	// The call fails definitely (nothing happened), with the body's start
	// in the message; for an irreversible action it is unclear instead, and
	// verification decides. Match only answers that mean nothing was done.
	FailWhen string `json:"failWhen,omitempty"`
	// Safe declares that this POST only reads: a query API that takes its
	// question as a body (GraphQL queries, route matrices). Only then may a
	// read action use POST; it is retried like a GET, so a POST that
	// changes anything must not say so.
	Safe bool `json:"safe,omitempty"`
	// Response is json (the default) or bytes: a file, returned as
	// {base64, contentType, size} (plugin-001). Only for reads; redirects
	// are followed (at most 3), each to a host in egress.hosts, without
	// credentials when the host changes. SharePoint's /content answers
	// with a redirect.
	Response string `json:"response,omitempty" jsonschema:"enum=json,enum=bytes"`
}

// Verify describes verification.
type Verify struct {
	// Mode is native, probe or none.
	Mode string `json:"mode" jsonschema:"enum=native,enum=probe,enum=none"`
	// With names a read action in this PluginManifest that checks the effect
	// (mode probe). Use its local name, e.g. "lookup", not "plugin/lookup".
	With string `json:"with,omitempty"`
	// Args are the probe action's arguments. Values are literal unless the
	// whole value is "${ expr }"; expressions read inputs (this action's
	// arguments) and caller.
	Args map[string]any `json:"args,omitempty"`
	// Happened is a boolean expression that says whether the effect
	// happened, reading the probe's result as sources.probe.output and this
	// action's arguments as inputs. Required for mode probe. Example:
	// "sources.probe.output.exists(a, a.id == inputs.appointmentId && a.slotId == inputs.slotId)"
	Happened string `json:"happened,omitempty"`
	// Output optionally computes this action's output from the probe's
	// result when the effect happened; without it the output is null.
	Output string `json:"output,omitempty"`
}
