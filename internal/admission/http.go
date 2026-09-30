package admission

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/expr"
)

// credentialName reports header, query and body names that carry
// credentials. A literal (or constant) value under one is a secret written
// into a definition.
func credentialName(name string) bool {
	n := strings.ToLower(name)
	if n == "key" || n == "cookie" {
		return true
	}
	for _, w := range []string{"token", "secret", "password", "passwd", "auth", "api-key", "api_key", "apikey", "credential", "signature"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return strings.HasSuffix(n, "-key") || strings.HasSuffix(n, "_key")
}

// reservedHeaders are set by the platform; a manifest cannot replace them.
var reservedHeaders = map[string]bool{"idempotency-key": true, "content-type": true, "accept": true, "host": true, "content-length": true}

// literalSecrets walks a query or body and reports credential-named keys
// whose value is fixed in the definition: a string, a list of strings, or
// a "${ }" expression that reads no scope (a constant).
func (c *Checker) literalSecrets(v any, path string, add func(path, code, msg, fix string)) {
	switch v := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(v) {
			if credentialName(k) && c.constant(v[k]) {
				add(path+"."+k, "LITERAL_SECRET", fmt.Sprintf("%q looks like a credential written into the definition", k), "use connection.auth with a secret name")
			}
			c.literalSecrets(v[k], path+"."+k, add)
		}
	case []any:
		for i, x := range v {
			c.literalSecrets(x, fmt.Sprintf("%s[%d]", path, i), add)
		}
	}
}

// constant reports a value that does not depend on any input.
func (c *Checker) constant(v any) bool {
	switch v := v.(type) {
	case string:
		inner, ok := strings.CutPrefix(strings.TrimSpace(v), "${")
		if !ok {
			return v != ""
		}
		x, err := c.expr.Compile(strings.TrimSuffix(strings.TrimSpace(inner), "}"), expr.Any)
		return err == nil && len(x.References()) == 0
	case []any:
		for _, x := range v {
			if c.constant(x) {
				return true
			}
		}
	}
	return false
}

// checkHTTP checks the built-in http plugin's parts of a manifest.
func (c *Checker) checkHTTP(d Doc) []Issue {
	var issues []Issue
	p := d.Plugin
	add := func(path, code, msg, fix string) {
		issues = append(issues, Issue{Severity: SeverityError, Code: code, File: d.File, Resource: d.ID(), Path: path, Message: msg, Fix: fix})
	}
	hosts := map[string]bool{}
	if p.Spec.Egress != nil {
		for _, h := range p.Spec.Egress.Hosts {
			hosts[strings.ToLower(h)] = true
		}
	}
	usesHTTP := false
	for _, name := range sortedKeys(p.Spec.Actions) {
		a := p.Spec.Actions[name]
		if a.HTTP == nil {
			continue
		}
		usesHTTP = true
		c.checkHTTPCall(a, "spec.actions."+name+".http", add)
	}
	conn := p.Spec.Connection
	switch {
	case usesHTTP && conn == nil:
		add("spec.connection", "MISSING_FIELD", "actions use http but the plugin has no connection", "add connection.baseUrl")
	case conn != nil:
		u, err := url.Parse(conn.BaseURL)
		switch {
		case err != nil || u.Scheme != "https" || u.Host == "":
			add("spec.connection.baseUrl", "INVALID_FIELD", "baseUrl must be an https URL", "")
		case u.RawQuery != "" || u.User != nil:
			add("spec.connection.baseUrl", "INVALID_FIELD", "baseUrl cannot carry a query or credentials", "use connection.auth with a secret")
		case !hosts[strings.ToLower(u.Hostname())]:
			add("spec.connection.baseUrl", "EGRESS_NOT_ALLOWED", fmt.Sprintf("host %s is not in egress.hosts", u.Hostname()), "add it to spec.egress.hosts")
		case internalIP(u.Hostname()):
			add("spec.connection.baseUrl", "EGRESS_NOT_ALLOWED", fmt.Sprintf("host %s is a private, loopback or link-local address", u.Hostname()), "use a hostname; self-hosted installs allow private networks explicitly at runtime")
		}
		if au := conn.Auth; au != nil {
			if reservedHeaders[strings.ToLower(au.Header)] {
				add("spec.connection.auth.header", "INVALID_FIELD", fmt.Sprintf("%s is set by the platform", au.Header), "")
			}
			if au.Prefix != "" && !validPrefix(au.Prefix) {
				add("spec.connection.auth.prefix", "LITERAL_SECRET", "a prefix is a scheme such as \"Bearer \", never part of a credential", "put the whole credential in the secret")
			}
		}
		if au := conn.Auth; au != nil && (au.Header == "") == (au.Query == "") {
			add("spec.connection.auth", "ONE_STRATEGY", "auth sets exactly one of header and query", "")
		}
		if o := conn.OAuth2; o != nil {
			if conn.Auth != nil {
				add("spec.connection.oauth2", "ONE_STRATEGY", "a connection authenticates with auth or oauth2, not both", "")
			}
			t, err := url.Parse(o.TokenURL)
			switch {
			case err != nil || t.Scheme != "https" || t.Host == "":
				add("spec.connection.oauth2.tokenUrl", "INVALID_FIELD", "tokenUrl must be an https URL", "")
			case t.User != nil:
				add("spec.connection.oauth2.tokenUrl", "INVALID_FIELD", "tokenUrl cannot carry credentials", "clientId and clientSecret name secrets")
			case !hosts[strings.ToLower(t.Hostname())]:
				add("spec.connection.oauth2.tokenUrl", "EGRESS_NOT_ALLOWED", fmt.Sprintf("host %s is not in egress.hosts", t.Hostname()), "add it to spec.egress.hosts")
			case internalIP(t.Hostname()):
				// The client secret is posted there: never to a metadata or
				// private address.
				add("spec.connection.oauth2.tokenUrl", "EGRESS_NOT_ALLOWED", fmt.Sprintf("host %s is a private, loopback or link-local address", t.Hostname()), "use the provider's token endpoint by hostname")
			}
		}
	}
	return issues
}

func (c *Checker) checkHTTPCall(a plugins.ActionSpec, path string, add func(path, code, msg, fix string)) {
	h := a.HTTP
	if h.Response == "bytes" && a.Effect != plugins.EffectRead {
		add(path+".response", "EFFECT_NOT_ALLOWED", "response: bytes is for reads (downloading a file)", "declare the action effect: read, or drop response")
	}
	sub := func(p string) func(code, s, msg, fix string) {
		return func(code, s, msg, fix string) { add(path+p+s, code, msg, fix) }
	}
	if t, err := c.expr.CompileTemplate(h.Path); err != nil {
		add(path+".path", "INVALID_TEMPLATE", err.Error(), "")
	} else {
		pluginRefs(t.References(), a, requestScopes, "", "", sub(".path"))
	}
	for field, v := range map[string]map[string]any{".query": h.Query, ".body": h.Body} {
		if v == nil {
			continue
		}
		cv, err := c.expr.CompileValue(v)
		if err != nil {
			add(path+field, "INVALID_EXPR", err.Error(), "")
			continue
		}
		pluginRefs(cv.References(), a, requestScopes, "", "", sub(field))
	}
	c.literalSecrets(h.Query, path+".query", add)
	c.literalSecrets(h.Body, path+".body", add)
	for _, k := range sortedKeys(h.Headers) {
		switch {
		case reservedHeaders[strings.ToLower(k)]:
			add(path+".headers."+k, "INVALID_FIELD", fmt.Sprintf("header %q is set by the platform", k), "remove it")
		case credentialName(k):
			add(path+".headers."+k, "LITERAL_SECRET", fmt.Sprintf("header %q carries credentials; headers are literal", k), "use connection.auth with a secret name")
		}
	}
	if h.Body != nil && (h.Method == "GET" || h.Method == "DELETE") {
		add(path+".body", "UNUSED_FIELD", h.Method+" requests have no body", "use query")
	}
	switch {
	case h.Safe && (h.Method != "POST" || a.Effect != plugins.EffectRead):
		add(path+".safe", "INVALID_FIELD", "safe only declares a POST of a read action that only reads", "remove it")
	case a.Effect == plugins.EffectRead && h.Method != "GET" && !h.Safe:
		add(path+".method", "EFFECT_NOT_ALLOWED", fmt.Sprintf("a read action must use GET; %s may change the world and would be retried", h.Method),
			"use GET, declare the real effect class, or for a query API that reads with a POST body, safe: true")
	}
	if h.FailWhen != "" {
		if x, err := c.expr.Compile(h.FailWhen, expr.Bool); err != nil {
			add(path+".failWhen", "INVALID_EXPR", err.Error(), "")
		} else {
			pluginRefs(x.References(), a, responseScopes, "response", "", sub(".failWhen"))
		}
	}
	if h.Output != "" {
		x, err := c.expr.Compile(h.Output, expr.Any)
		if err != nil {
			add(path+".output", "INVALID_EXPR", err.Error(), "")
			return
		}
		pluginRefs(x.References(), a, responseScopes, "response", "", sub(".output"))
	}
}

// internalIP reports a host written as a private, loopback, link-local or
// unspecified IP address (the cloud metadata service is link-local).
func internalIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

// validPrefix accepts an auth scheme ("Bearer ", "Token "), never data.
func validPrefix(p string) bool {
	word := strings.TrimSuffix(p, " ")
	if word == "" || len(word) > 20 {
		return false
	}
	for _, r := range word {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}
