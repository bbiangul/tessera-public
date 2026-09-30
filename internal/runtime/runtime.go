// Package runtime executes admitted Functions (RFC-0003).
//
// A run is executed from the top every time something happens to it — it
// starts, the user replies, a human answers, the process restarts — over its
// journal. Every site that already has a record returns it, so model calls
// and effects are never repeated; the first site without a record is new
// work. Suspensions (waiting for the user or a human) end an execution; the
// next event starts a new one.
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/mask"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Runtime runs Functions of one Program.
type Runtime struct {
	Program *Program
	Store   journal.Store
	Actor   act.Actor
	Decider decide.Decider
	// Generator runs extract and generate steps; nil fails those steps.
	Generator generate.Generator
	// Retrieve searches the knowledge bases for a caller (the host serves
	// it; retrieve-001). Nil: retrieve steps fail.
	Retrieve func(ctx context.Context, caller map[string]any, bases []string, query string, k int) ([]any, error)
	// Voice, when set, rewrites every message to users in a persona's
	// words, keeping the facts word for word (voice.go); nil sends the
	// templates' text.
	Voice *Voice
	// ExplainFailures passes on what a system said when an action failed
	// definitely (a new rule, a phone, a fee), after the failure's message
	// (drift-002). Needs a Generator; a Jev check drops an explanation
	// that says the action was done.
	ExplainFailures bool
	// Masker masks the state sent to Decide; nil means mask.Default. There
	// is no way to turn masking off.
	Masker mask.Masker
	// Escaper escapes interpolated values in messages; nil means
	// expr.PlainText. Channels set their own (expr.WhatsApp).
	Escaper expr.Escaper
	// Clock gives event.time at the start of a run and the time of records.
	// It is never read to take a decision after that. Nil means time.Now.
	Clock func() time.Time
	// Sleep waits for retry backoff; tests replace it. Nil means a timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// NewID makes run IDs; nil means 16 random bytes in hex.
	NewID func() string
	// Messages are the platform's own texts, in the agent's language.
	Messages Messages
	// Audiences are the Agent's audiences' voices and messages, by name.
	Audiences map[string]Audience
	// Autonomy is the Agent's (autonomous when empty); see AutonomyFor.
	Autonomy string
	// AudienceOf names the audience a caller belongs to ("" for none); the
	// host sets it. Sees: viewer audience -> seen audience -> masked|full.
	AudienceOf func(caller map[string]any) string
	Sees       map[string]map[string]string
	// Limits bound how long a run keeps asking.
	Limits Limits

	locks    keyedMutex
	defaultz sync.Once
}

// Audience is what one of the Agent's audiences hears differently: its
// voice and messages (RFC-0006). The host names a run's audience in
// caller.audience, journaled with the run, so a resumed run speaks the
// same way.
type Audience struct {
	Voice    *Voice
	Messages Messages
	Autonomy string
}

// audienceOf is the caller's audience, if the runtime knows it.
func (r *Runtime) audienceOf(caller map[string]any) (Audience, bool) {
	name, _ := caller["audience"].(string)
	a, ok := r.Audiences[name]
	return a, ok && name != ""
}

// VoiceFor is the voice a caller hears: their audience's, else the
// Agent's (nil: none).
func (r *Runtime) VoiceFor(caller map[string]any) *Voice {
	if a, ok := r.audienceOf(caller); ok && a.Voice != nil {
		return a.Voice
	}
	return r.Voice
}

// MessagesFor are the messages a caller hears: their audience's, field by
// field over the Agent's.
func (r *Runtime) MessagesFor(caller map[string]any) Messages {
	r.defaults()
	m := r.Messages
	if a, ok := r.audienceOf(caller); ok {
		for _, f := range []struct{ dst, src *string }{
			{&m.NotAllowed, &a.Messages.NotAllowed}, {&m.Cancelled, &a.Messages.Cancelled}, {&m.Denied, &a.Messages.Denied}, {&m.Failed, &a.Messages.Failed},
		} {
			if *f.src != "" {
				*f.dst = *f.src
			}
		}
	}
	return m
}

// Messages are texts the runtime sends on its own.
type Messages struct {
	NotAllowed string // authorize was false
	Cancelled  string // the user said no to a confirmation
	Denied     string // a person denied an approval (escalate task: approve)
	Failed     string // a failure no strategy handled
}

// Limits bound a run.
type Limits struct {
	// MaxAskRounds is how many times an ask asks the user; default 2.
	MaxAskRounds int
	// ConfirmYes and ConfirmNo are the probabilities of "yes" at or above
	// which a confirmation counts as yes, and at or below which as no;
	// defaults 0.9 and 0.1. In between the question is asked again.
	ConfirmYes, ConfirmNo float64
	// MaxCallDepth bounds fromFunction nesting; default 4.
	MaxCallDepth int
	// DefaultQueue receives escalations that do not name a queue.
	DefaultQueue string
	// FailureQueue, when set (the Agent's humanInTheLoop queue), receives a
	// takeover task when a run fails after telling the user that a person
	// will follow up (#52). Empty: the run just fails.
	FailureQueue string
	// Repair lists, comma-separated, the actions whose refused calls may
	// be repaired (Agent improvise.repair; repair.go).
	Repair string
	// RepairReads: reads that break their declared type are repaired by
	// renaming fields (Agent improvise.repairReads; contract.go).
	RepairReads bool
}

func (l Limits) record() map[string]any {
	return map[string]any{"maxAskRounds": int64(l.MaxAskRounds), "confirmYes": l.ConfirmYes, "confirmNo": l.ConfirmNo,
		"maxCallDepth": int64(l.MaxCallDepth), "defaultQueue": l.DefaultQueue, "failureQueue": l.FailureQueue, "repair": l.Repair, "repairReads": l.RepairReads}
}

// limitsFrom reads recorded limits; a missing field keeps the current one.
func limitsFrom(m map[string]any, cur Limits) Limits {
	if v, ok := m["maxAskRounds"].(int64); ok {
		cur.MaxAskRounds = int(v)
	}
	if v, ok := m["confirmYes"].(float64); ok {
		cur.ConfirmYes = v
	}
	if v, ok := m["confirmNo"].(float64); ok {
		cur.ConfirmNo = v
	}
	if v, ok := m["maxCallDepth"].(int64); ok {
		cur.MaxCallDepth = int(v)
	}
	if v, ok := m["defaultQueue"].(string); ok {
		cur.DefaultQueue = v
	}
	// A run recorded before failureQueue existed keeps failing as it
	// would have: a changed setting never changes a run under way.
	cur.FailureQueue, _ = m["failureQueue"].(string)
	cur.Repair, _ = m["repair"].(string)
	cur.RepairReads, _ = m["repairReads"].(bool)
	return cur
}

// DefaultMessages are used for empty fields of Runtime.Messages.
var DefaultMessages = Messages{
	NotAllowed: "Sorry, I can't do that for you.",
	Cancelled:  "Ok, nothing was changed.",
	Denied:     "Sorry, this was not approved.",
	Failed:     "Something went wrong. A person from the team will follow up.",
}

// defaults fills unset fields once; Start, Deliver and Resume may run
// concurrently for different conversations.
func (r *Runtime) defaults() { r.defaultz.Do(r.fillDefaults) }

// Mask masks a value for a model, with the runtime's masker.
func (r *Runtime) Mask(v any) any {
	r.defaults()
	return r.Masker.Mask(v)
}

// MessageState is the masked state for a question about a user's message.
func (r *Runtime) MessageState(text string) map[string]any {
	r.defaults()
	return map[string]any{"message": r.Masker.Mask(text)}
}

func (r *Runtime) fillDefaults() {
	if r.Masker == nil {
		r.Masker = mask.Default
	}
	if r.Escaper == nil {
		r.Escaper = expr.PlainText
	}
	if r.Clock == nil {
		r.Clock = time.Now
	}
	if r.Sleep == nil {
		r.Sleep = sleep
	}
	if r.NewID == nil {
		r.NewID = randomID
	}
	m := &r.Messages
	for _, f := range []struct {
		dst *string
		def string
	}{
		{&m.NotAllowed, DefaultMessages.NotAllowed}, {&m.Cancelled, DefaultMessages.Cancelled}, {&m.Denied, DefaultMessages.Denied}, {&m.Failed, DefaultMessages.Failed},
	} {
		if *f.dst == "" {
			*f.dst = f.def
		}
	}
	l := &r.Limits
	if l.MaxAskRounds <= 0 {
		l.MaxAskRounds = 2
	}
	if l.ConfirmYes == 0 {
		l.ConfirmYes = 0.9
	}
	if l.ConfirmNo == 0 {
		l.ConfirmNo = 0.1
	}
	if l.MaxCallDepth <= 0 {
		l.MaxCallDepth = 4
	}
	if l.DefaultQueue == "" {
		l.DefaultQueue = "default"
	}
}

// Start describes the event that starts a run.
type Start struct {
	Namespace    string
	Function     string
	Conversation string
	// Caller is the verified identity of who sent the event.
	Caller map[string]any
	// Message is the user's message that triggered the run, if any.
	Message string
	// Trigger names the Trigger that started the run, if any
	// (event.trigger).
	Trigger string
	// RunID, when set, is the run's ID: starting the same ID twice fails
	// with journal.ErrExists, which makes a scheduled start happen once.
	RunID string
	// Time, when set, is event.time (a scheduled time); otherwise the clock.
	Time time.Time
	// Payload is event.payload: what a webhook received.
	Payload map[string]any
	// Route is how the router chose this Function (choice, probabilities),
	// kept in the journal for inspection; runs never read it.
	Route map[string]any
	// Inputs are values given to the Function's inputs, as a call's with
	// gives them (agent mode, RFC-0010): a given input is not resolved from
	// its sources, and must conform to its type.
	Inputs map[string]any
}

// Reply answers a waiting run.
type Reply struct {
	// From is "user" or "human" and must match what the run waits for.
	From string
	// Text is what the person wrote.
	Text string
	// Value is a structured answer from a human (a chosen option key, the
	// output of an action done by hand, {"happened": bool}).
	Value any
	// TaskHash is the pending task's hash (PendingTask) this answer is
	// about. Required to answer an approval: an approval of something
	// that changed since it was read is refused.
	TaskHash string
	// By is who answered, recorded for audit (a person's name or ID).
	By string
}

// Message is something to deliver. The runtime never sends; the channel
// layer delivers messages after the records that produced them are durable.
type Message struct {
	// ID is unique and stable (run ID and site), so a channel can drop a
	// message it already delivered.
	ID string `json:"id"`
	// Seq orders messages within a run; pass it to Ack after delivery.
	Seq int `json:"seq"`
	// To is "user" or "queue:<name>".
	To   string `json:"to"`
	Text string `json:"text"`
	// Task is the HumanTask type for queue messages: input, answer,
	// approve, takeover or indeterminate.
	Task string `json:"task,omitempty"`
	// Options are the choices offered to a human, by key.
	Options map[string]string `json:"options,omitempty"`
}

// Result is the outcome of one execution.
type Result struct {
	RunID  string         `json:"runId"`
	Status journal.Status `json:"status"`
	// Outbox holds every message of the run not acknowledged yet, in order,
	// including messages recorded by an execution that failed afterwards.
	// Deliver them, then call Ack with the last Seq.
	Outbox []Message `json:"outbox,omitempty"`
	// WaitingFor is "user" or "human" when the run is suspended.
	WaitingFor string `json:"waitingFor,omitempty"`
	// Error explains a Failed run, without personal data.
	Error string `json:"error,omitempty"`
	// WakeAt is when a run WaitingForTime wakes.
	WakeAt time.Time `json:"wakeAt,omitzero"`
	// Restart is set when the user answered a confirmation with a
	// correction ("no, Tuesday at 10 instead"): the run ended without
	// acting, and the host starts the same Function again with this
	// message, which resolves the new values.
	Restart string `json:"restart,omitempty"`
}

// Errors returned to the caller. Run failures are not errors: they are a
// Result with status Failed.
var (
	ErrUnknownFunction = errors.New("unknown function")
	ErrFinished        = errors.New("run already finished")
	ErrNotWaiting      = errors.New("run is not waiting for this reply")
	// ErrGeneration means the run started with other definitions or scripts
	// than this Runtime has. It must be resumed by a Runtime with its
	// generation; replaying it here could take a different path.
	ErrGeneration = errors.New("run belongs to another generation")
)

// Start creates a run and executes it until it finishes or waits.
// earlierLines is how much of the conversation a run keeps from before it
// started: the voice's greeting rule and extraction's conversation read it.
const earlierLines = 16

func (r *Runtime) Start(ctx context.Context, s Start) (Result, error) {
	r.defaults()
	key := s.Namespace + "/" + s.Function
	if _, ok := r.Program.functions[key]; !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownFunction, key)
	}
	now := r.Clock()
	eventTime := now
	if !s.Time.IsZero() {
		eventTime = s.Time
	}
	event := map[string]any{"time": eventTime, "message": s.Message}
	if s.Trigger != "" {
		event["trigger"] = s.Trigger
	}
	if s.Payload != nil {
		event["payload"] = normalizeMap(s.Payload)
	}
	conv := map[string]any{"id": s.Conversation}
	if s.Conversation != "" {
		// What was said in the conversation's previous run: the voice
		// greets only in a conversation's first message (freemode-002).
		if last, ok, err := r.Store.LastRun(ctx, s.Conversation); err == nil && ok {
			if lines, err := r.Conversation(ctx, last.ID, earlierLines); err == nil && len(lines) > 0 {
				conv["earlier"] = anyList(lines)
			}
		}
	}
	sc := map[string]any{
		"caller":       normalizeMap(s.Caller),
		"conversation": conv,
		"event":        event,
		"limits":       r.Limits.record(),
	}
	if len(s.Inputs) > 0 {
		given, err := journal.Normalize(s.Inputs)
		if err != nil {
			return Result{}, fmt.Errorf("inputs: %w", err)
		}
		fn := r.Program.functions[key]
		for k, v := range given.(map[string]any) {
			known := false
			for _, in := range fn.inputs {
				if in.name == k {
					known = true
					if v != nil && !conforms(in.typ, v) {
						return Result{}, fmt.Errorf("input %s gets %s, the input is %s", k, describe(v), in.typ)
					}
				}
			}
			if !known {
				return Result{}, fmt.Errorf("%s has no input %s", key, k)
			}
		}
		sc["given"] = given
	}
	scopes, err := journal.Encode(sc)
	if err != nil {
		return Result{}, fmt.Errorf("caller: %w", err)
	}
	id := s.RunID
	if id == "" {
		id = r.NewID()
	}
	run := journal.Run{
		ID: id, Namespace: s.Namespace, Function: s.Function, Generation: r.Program.Generation(),
		Conversation: s.Conversation, Scopes: scopes, Status: journal.StatusResolving,
		CreatedAt: now, UpdatedAt: now,
	}
	unlock := r.locks.lock(s.Conversation)
	defer unlock()
	if err := r.Store.CreateRun(ctx, run); err != nil {
		return Result{}, err
	}
	if s.Route != nil {
		v, err := journal.Encode(s.Route)
		if err != nil {
			return Result{}, err
		}
		if _, err := r.Store.Append(ctx, run.ID, journal.Record{Site: "router", Kind: journal.KindResult, Value: v, At: now}); err != nil {
			return Result{}, err
		}
	}
	return r.execute(ctx, run)
}

// Deliver records a reply to a waiting run and continues it.
func (r *Runtime) Deliver(ctx context.Context, runID string, rep Reply) (Result, error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	if run, err = r.Store.GetRun(ctx, runID); err != nil { // may have moved while we waited
		return Result{}, err
	}
	if run.Status.Final() {
		return Result{}, fmt.Errorf("%w: %s is %s", ErrFinished, runID, run.Status)
	}
	if run.Pending == "" || waitingFor(run.Status) != rep.From {
		return Result{}, fmt.Errorf("%w: status %s, reply from %q", ErrNotWaiting, run.Status, rep.From)
	}
	if rep.From == "human" {
		t, err := r.PendingTask(ctx, runID)
		if err != nil {
			return Result{}, err
		}
		// Every answer names the task it answers: a retried or late answer
		// must not become the answer to the next task of the same run.
		if rep.TaskHash != t.Hash {
			return Result{}, ErrTaskChanged
		}
		// With roles on the queue, the answer is a member's (RFC-0006).
		if err := r.mayAnswer(ctx, runID, rep.By); err != nil {
			return Result{}, err
		}
	}
	answer := map[string]any{"text": rep.Text, "value": rep.Value, "from": rep.From}
	if rep.By != "" {
		answer["by"] = rep.By
	}
	v, err := journal.Encode(answer)
	if err != nil {
		return Result{}, fmt.Errorf("reply: %w", err)
	}
	_, err = r.Store.Append(ctx, runID, journal.Record{Site: run.Pending, Kind: journal.KindResume, Value: v, At: r.Clock()})
	if errors.Is(err, journal.ErrDuplicate) {
		// A reply was recorded but the execution after it failed. The same
		// reply delivered again (a channel retry) just finishes that
		// execution; a different message is not a second answer to the
		// same question and is refused.
		same, cerr := r.sameReply(ctx, run, rep)
		if cerr != nil {
			return Result{}, cerr
		}
		res, xerr := r.execute(ctx, run)
		if xerr != nil {
			return Result{}, xerr
		}
		if !same {
			return res, fmt.Errorf("%w: the previous reply to this question was still being processed", ErrNotWaiting)
		}
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	return r.execute(ctx, run)
}

// sameReply reports whether the reply recorded at the run's pending site
// is rep.
func (r *Runtime) sameReply(ctx context.Context, run journal.Run, rep Reply) (bool, error) {
	recs, err := r.Store.Records(ctx, run.ID)
	if err != nil {
		return false, err
	}
	for _, rec := range recs {
		if rec.Site == run.Pending && rec.Kind == journal.KindResume {
			v, err := journal.Decode(rec.Value)
			if err != nil {
				return false, err
			}
			m, _ := v.(map[string]any)
			text, _ := m["text"].(string)
			by, _ := m["by"].(string)
			if text != rep.Text || by != rep.By {
				// Another person's identical answer is not a retry of the
				// first: it must not be shown as theirs.
				return false, nil
			}
			// Two approvals with the same (empty) text but opposite values
			// are not the same reply.
			enc, err := journal.Encode(rep.Value)
			if err != nil {
				return false, err
			}
			want, err := journal.Decode(enc)
			if err != nil {
				return false, err
			}
			return reflect.DeepEqual(m["value"], want), nil
		}
	}
	return false, nil
}

// LastUserMessage returns the text of the latest message a run sent to the
// user, "" when none. Routers use it as context for the next message.
func (r *Runtime) LastUserMessage(ctx context.Context, runID string) (string, error) {
	recs, err := r.Store.Records(ctx, runID)
	if err != nil {
		return "", err
	}
	text := ""
	for _, rec := range recs {
		if rec.Kind != journal.KindSend && rec.Kind != journal.KindSuspend {
			continue
		}
		v, err := journal.Decode(rec.Value)
		if err != nil {
			return "", err
		}
		m, _ := v.(map[string]any)
		if m["to"] == "user" {
			text, _ = m["text"].(string)
		}
	}
	return text, nil
}

// Ack marks the run's messages up to seq as delivered.
func (r *Runtime) Ack(ctx context.Context, runID string, seq int) error {
	return r.Store.Ack(ctx, runID, seq)
}

// Resume executes a run again from its journal, e.g. after a restart. It
// never repeats recorded work.
func (r *Runtime) Resume(ctx context.Context, runID string) (Result, error) {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	if run, err = r.Store.GetRun(ctx, runID); err != nil {
		return Result{}, err
	}
	if run.Status.Final() {
		// Nothing to execute, but messages the channel never acknowledged
		// must still come back (transactional outbox).
		outbox, err := r.pendingMessages(ctx, run)
		return Result{RunID: runID, Status: run.Status, Outbox: outbox}, err
	}
	return r.execute(ctx, run)
}

func waitingFor(s journal.Status) string {
	switch s {
	case journal.StatusWaitingForUser, journal.StatusConfirming:
		return "user"
	case journal.StatusWaitingForHuman, journal.StatusIndeterminate:
		return "human"
	}
	return ""
}

// keyedMutex serializes runs of the same conversation (RFC-0001: one event
// per conversation at a time).
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		k.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b[:])
}

func normalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
