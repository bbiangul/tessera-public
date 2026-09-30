package runtime

import (
	"context"
	"fmt"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// channelRules are a channel plugin's rules (RFC-0001, "Plugins de canal").
type channelRules struct {
	window  time.Duration
	consent *expr.Expr
}

// compileChannels reads the channel plugins and which one each namespace's
// Agent uses for its messages to the user.
func (p *Program) compileChannels(e *expr.Engine, manifests map[string]*plugins.PluginManifest, agentDocs []*agents.Agent) error {
	p.channels = map[string]*channelRules{}
	p.agentChannel = map[string]string{}
	for name, m := range manifests {
		ch := m.Spec.Channel
		if ch == nil {
			continue
		}
		r := &channelRules{}
		if ch.Window != "" {
			d, err := time.ParseDuration(ch.Window)
			if err != nil || d <= 0 {
				return fmt.Errorf("plugin %s: channel.window %q is not a positive duration", name, ch.Window)
			}
			r.window = d
		}
		if ch.Consent != "" {
			x, err := e.Compile(ch.Consent, expr.Bool)
			if err != nil {
				return fmt.Errorf("plugin %s: channel.consent: %w", name, err)
			}
			r.consent = x
		}
		p.channels[name] = r
	}
	for _, a := range agentDocs {
		if a.Spec.Channel != "" {
			p.agentChannel[a.Metadata.Namespace] = a.Spec.Channel
		}
	}
	return nil
}

// windowClosed means free text would reach the user outside the channel's
// window.
type windowClosed struct{ reason string }

func (w *windowClosed) Error() string { return w.reason }

// userWindow checks, once per site, whether free text may reach the user
// now: the time since the user last wrote is within the channel's window.
// The answer is recorded, so a resumed run agrees with the first one.
func (e *execution) userWindow(site string, window time.Duration) error {
	if window <= 0 {
		return nil
	}
	v, err := e.once(site+"/window", func() (any, error) {
		last, err := e.lastInbound()
		if err != nil {
			return nil, err
		}
		now := e.rt.Clock()
		return map[string]any{"open": !last.IsZero() && now.Sub(last) <= window, "lastInbound": last, "at": now}, nil
	})
	if err != nil {
		return err
	}
	m, _ := v.(map[string]any)
	if open, _ := m["open"].(bool); !open {
		return &windowClosed{reason: fmt.Sprintf("WINDOW_CLOSED: the user last wrote more than %s ago (or never): free text cannot reach them now; send a template", window)}
	}
	return nil
}

// replyWindow is the window of the channel the namespace's Agent uses.
func (e *execution) replyWindow() time.Duration {
	if r := e.rt.Program.channels[e.rt.Program.agentChannel[e.run.Namespace]]; r != nil {
		return r.window
	}
	return 0
}

// lastInbound is when the user last wrote in this conversation: a message
// that started a run, or a reply to a question sent to the user.
func (e *execution) lastInbound() (time.Time, error) {
	var last time.Time
	runs, err := e.rt.Store.Runs(e.ctx, journal.RunFilter{Namespace: e.run.Namespace, Conversation: e.run.Conversation, Limit: 50})
	if err != nil {
		return last, err
	}
	for _, run := range runs {
		if s, err := journal.Decode(run.Scopes); err == nil {
			if ev, ok := s.(map[string]any)["event"].(map[string]any); ok {
				if msg, _ := ev["message"].(string); msg != "" {
					if t, _ := ev["time"].(time.Time); t.After(last) {
						last = t
					}
				}
			}
		}
		recs, err := e.rt.Store.Records(e.ctx, run.ID)
		if err != nil {
			return last, err
		}
		toUser := map[string]bool{}
		for _, r := range recs {
			if r.Kind != journal.KindSuspend {
				continue
			}
			if v, err := journal.Decode(r.Value); err == nil {
				if m, _ := v.(map[string]any); m["to"] == "user" {
					toUser[r.Site] = true
				}
			}
		}
		for _, r := range recs {
			if r.Kind == journal.KindResume && (toUser[r.Site] || isReplyWait(r)) && r.At.After(last) {
				last = r.At
			}
		}
	}
	return last, nil
}

// isReplyWait is a user's reply to a wait with orReply (its resume has
// text; the timer's has none).
func isReplyWait(r journal.Record) bool {
	v, err := journal.Decode(r.Value)
	if err != nil {
		return false
	}
	m, _ := v.(map[string]any)
	timer, _ := m["timer"].(bool)
	text, _ := m["text"].(string)
	return !timer && text != "" && m["from"] == "user"
}

// channelChecks runs a channel plugin's rules before an external-message
// action: consent always, the window for free text. A refusal is a
// definite failure: nothing was sent, and onError can take over.
func (e *execution) channelChecks(ctx context.Context, action string, spec plugins.ActionSpec, args map[string]any) error {
	if spec.Effect != plugins.EffectExternalMessage {
		return nil
	}
	plugin, _, _ := cutAction(action)
	r := e.rt.Program.channels[plugin]
	if r == nil {
		return nil
	}
	if r.consent != nil {
		ok, err := r.consent.EvalBool(ctx, expr.Activation{"caller": e.scopes["caller"], "event": e.scopes["event"], "inputs": args, "conversation": e.scopes["conversation"]})
		if err != nil || !ok {
			return &act.DefiniteError{Code: "CONSENT_MISSING", Message: action + ": no consent to message this person (channel.consent is false)"}
		}
	}
	if spec.FreeText && r.window > 0 {
		// The call's outcome is recorded once, so this is decided once too.
		last, err := e.lastInbound()
		if err != nil {
			return err
		}
		if last.IsZero() || e.rt.Clock().Sub(last) > r.window {
			return &act.DefiniteError{Code: "WINDOW_CLOSED", Message: fmt.Sprintf("%s: the user last wrote more than %s ago (or never): send a template", action, r.window)}
		}
	}
	return nil
}

func cutAction(a string) (plugin, name string, ok bool) {
	for i := 0; i < len(a); i++ {
		if a[i] == '/' {
			return a[:i], a[i+1:], true
		}
	}
	return a, "", false
}
