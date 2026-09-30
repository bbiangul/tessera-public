package host

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Errors of Webhook, for the server to map to HTTP statuses.
var (
	ErrUnknownHook  = errors.New("no webhook trigger with this name")
	ErrBadSignature = errors.New("signature does not match")
	ErrBadPayload   = errors.New("payload is not a JSON object")
)

// WebhookTolerance is how far a webhook's signed timestamp may be from
// now: a captured delivery cannot be replayed after it.
const WebhookTolerance = 5 * time.Minute

// SignWebhook is the X-Tessera-Signature of a delivery: the HMAC-SHA256,
// with the trigger's secret, of timestamp "." idempotency key "." body.
// The key is signed so a captured delivery cannot start new runs under
// new keys; the timestamp (unix seconds, X-Tessera-Timestamp) so it
// cannot be replayed later.
func SignWebhook(secret string, timestamp int64, key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s.", timestamp, key)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// validKey bounds an Idempotency-Key: it becomes part of a run ID.
func validKey(k string) bool {
	if len(k) > 128 {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:", r)) {
			return false
		}
	}
	return true
}

type webhook struct {
	name, function, secret string
	conversation           *expr.Expr // on event.payload; nil: its own
	key                    *expr.Expr // the run's key from the payload; nil: the Idempotency-Key
	event                  string     // delivered to waiting runs, when set
	eventKey               *expr.Expr
}

func webhooks(docs []admission.Doc, namespace string) (map[string]webhook, error) {
	e, err := expr.New()
	if err != nil {
		return nil, err
	}
	out := map[string]webhook{}
	for _, d := range docs {
		t := d.Trigger
		if t == nil || t.Metadata.Namespace != namespace || t.Spec.Webhook == nil {
			continue
		}
		w := webhook{name: t.Metadata.Name, function: t.Spec.Function, secret: t.Spec.Webhook.Secret}
		if src := string(t.Spec.Webhook.Conversation); src != "" {
			if w.conversation, err = e.Compile(src, expr.Any); err != nil {
				return nil, fmt.Errorf("trigger %s: conversation: %w", t.Metadata.Name, err)
			}
		}
		if src := string(t.Spec.Webhook.Key); src != "" {
			if w.key, err = e.Compile(src, expr.Any); err != nil {
				return nil, fmt.Errorf("trigger %s: key: %w", t.Metadata.Name, err)
			}
		}
		if ev := t.Spec.Event; ev != nil {
			w.event = ev.Name
			if w.eventKey, err = e.Compile(string(ev.Key), expr.Any); err != nil {
				return nil, fmt.Errorf("trigger %s: event.key: %w", t.Metadata.Name, err)
			}
		}
		out[t.Metadata.Name] = w
	}
	return out, nil
}

// Webhook starts the Function of a webhook trigger. signature is the
// X-Tessera-Signature header (SignWebhook), timestamp the
// X-Tessera-Timestamp header, key the Idempotency-Key header, or empty to
// key on the body's hash. A repeated delivery starts nothing and returns
// the run it started the first time. An unknown trigger and a bad
// signature are the same error, so names cannot be probed.
func (h *Host) Webhook(ctx context.Context, name string, body []byte, signature, timestamp, key string, now time.Time) (runtime.Result, error) {
	w, ok := h.hooks[name]
	if !ok {
		return runtime.Result{}, ErrBadSignature
	}
	if h.opts.Secrets == nil {
		return runtime.Result{}, errors.New("webhooks need a secret store")
	}
	secret, err := h.opts.Secrets.Secret(w.secret)
	if err != nil {
		return runtime.Result{}, err
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return runtime.Result{}, fmt.Errorf("%w: X-Tessera-Timestamp is unix seconds", ErrBadSignature)
	}
	if d := now.Sub(time.Unix(ts, 0)); d > WebhookTolerance || d < -WebhookTolerance {
		return runtime.Result{}, fmt.Errorf("%w: the timestamp is more than %s from now", ErrBadSignature, WebhookTolerance)
	}
	if !validKey(key) {
		return runtime.Result{}, fmt.Errorf("%w: Idempotency-Key is up to 128 of letters, digits and - _ . : (colon)", ErrBadPayload)
	}
	if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(SignWebhook(secret, ts, key, body))) {
		return runtime.Result{}, ErrBadSignature
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil || payload == nil {
		return runtime.Result{}, ErrBadPayload
	}
	norm, err := journal.Normalize(payload)
	if err != nil {
		return runtime.Result{}, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	if key == "" {
		sum := sha256.Sum256(body)
		key = hex.EncodeToString(sum[:16])
	}
	return h.StartHook(ctx, name, norm.(map[string]any), key)
}

// StartHook runs a webhook trigger with a payload. A trigger with an event
// delivers it to the runs waiting for it with the same key and returns the
// first of them; when none waits, it starts its Function, if it has one
// (otherwise the result is empty). A trigger's Function starts once per
// key: the trigger's key expression when it has one, else key. Webhook
// calls it after checking the signature; EvalSuites call it directly.
func (h *Host) StartHook(ctx context.Context, name string, payload map[string]any, key string) (runtime.Result, error) {
	w, ok := h.hooks[name]
	if !ok {
		return runtime.Result{}, fmt.Errorf("%w: %s", ErrUnknownHook, name)
	}
	act := expr.Activation{"event": map[string]any{"payload": payload}}
	if w.event != "" {
		v, err := w.eventKey.Eval(ctx, act)
		if err != nil || v == nil || fmt.Sprint(v) == "" {
			return runtime.Result{}, fmt.Errorf("%w: the event's key gave nothing", ErrBadPayload)
		}
		delivered, err := h.Runtime.DeliverEvent(ctx, h.opts.Namespace, w.event, fmt.Sprint(v), payload)
		if len(delivered) > 0 || err != nil {
			var first runtime.Result
			if len(delivered) > 0 {
				first = delivered[0]
			}
			return first, err
		}
		if w.function == "" {
			return runtime.Result{}, nil // nobody was waiting
		}
	}
	if w.key != nil {
		v, err := w.key.Eval(ctx, act)
		if err != nil || v == nil || fmt.Sprint(v) == "" || !validKey(fmt.Sprint(v)) {
			return runtime.Result{}, fmt.Errorf("%w: the trigger's key gave nothing usable (up to 128 of letters, digits and - _ . : (colon))", ErrBadPayload)
		}
		key = fmt.Sprint(v)
	}
	id := fmt.Sprintf("%s/%s#%s", h.opts.Namespace, name, key)
	conversation := "trigger:" + id
	if w.conversation != nil {
		v, err := w.conversation.Eval(ctx, expr.Activation{"event": map[string]any{"payload": payload}})
		if err != nil || v == nil || fmt.Sprint(v) == "" {
			return runtime.Result{}, fmt.Errorf("%w: the conversation expression gave nothing", ErrBadPayload)
		}
		conversation = fmt.Sprint(v)
	}
	res, err := h.Runtime.Start(ctx, runtime.Start{
		Namespace: h.opts.Namespace, Function: w.function, Conversation: conversation,
		Caller: map[string]any{"trigger": name}, Trigger: name, RunID: id, Payload: payload,
	})
	if errors.Is(err, journal.ErrExists) {
		run, gerr := h.Runtime.Store.GetRun(ctx, id)
		if gerr != nil {
			return runtime.Result{}, gerr
		}
		return runtime.Result{RunID: id, Status: run.Status}, nil
	}
	return res, err
}
