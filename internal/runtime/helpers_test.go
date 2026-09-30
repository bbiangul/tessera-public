package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/admission"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Test fixtures: the clinica example from examples/, a fake Agenda and a
// Decider that answers from the reply text, like a user would be read.

var testNow = time.Date(2026, 9, 23, 9, 0, 0, 0, time.FixedZone("-03", -3*3600))

func admit(t *testing.T, paths ...string) *Program {
	t.Helper()
	l, err := admission.NewLoader()
	if err != nil {
		t.Fatal(err)
	}
	docs, issues := l.LoadPaths(paths)
	c, err := admission.NewChecker()
	if err != nil {
		t.Fatal(err)
	}
	issues = append(issues, c.Check(docs)...)
	for _, is := range issues {
		if is.Severity == admission.SeverityError {
			t.Fatalf("fixture must pass admission: %s", is)
		}
	}
	p, err := Build(docs)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// clinic is a fake Agenda with two upcoming appointments.
type clinic struct {
	mu           sync.Mutex
	appointments []map[string]any
	reschedules  int // calls that changed something
	calls        map[string]int
	// failReschedule makes the next reschedule call fail with this error;
	// performFirst applies the change before failing (a timeout after the
	// external system acted).
	failReschedule error
	performFirst   bool
	// slotDays is how many days after "from" the offered slots are.
	slotDays int
}

func newClinic() *clinic {
	return &clinic{
		slotDays: 1,
		calls:    map[string]int{},
		appointments: []map[string]any{
			{"id": "a1", "dentistId": "d1", "slotId": "s0", "label": "Seg 21/09 09:00"},
			{"id": "a2", "dentistId": "d2", "slotId": "s0", "label": "Ter 22/09 14:00"},
		},
	}
}

func (c *clinic) actor() *act.Registry {
	r := act.NewRegistry()
	r.Register("agenda/list-upcoming-appointments", func(_ context.Context, call act.Call) (any, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls["list"]++
		out := []any{}
		for _, a := range c.appointments {
			cp := map[string]any{}
			for k, v := range a {
				cp[k] = v
			}
			out = append(out, cp)
		}
		return out, nil
	})
	r.Register("agenda/find-slots", func(_ context.Context, call act.Call) (any, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls["slots"]++
		from, ok := call.Args["from"].(time.Time)
		if !ok {
			return nil, &act.DefiniteError{Code: "INVALID", Message: fmt.Sprintf("from is %T", call.Args["from"])}
		}
		// The manifest says int and the definition writes 7: a float here
		// means literals lost their type on the way (found by tessera test).
		if _, ok := call.Args["days"].(int64); !ok {
			return nil, &act.DefiniteError{Code: "INVALID", Message: fmt.Sprintf("days is %T", call.Args["days"])}
		}
		day := from.AddDate(0, 0, c.slotDays)
		return []any{
			map[string]any{"id": "s1", "label": "Qui 24/09 10:00", "start": time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, from.Location())},
			map[string]any{"id": "s2", "label": "Qui 24/09 16:00", "start": time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, from.Location())},
		}, nil
	})
	r.Register("agenda/reschedule", func(_ context.Context, call act.Call) (any, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls["reschedule"]++
		if call.IdempotencyKey == "" {
			return nil, &act.DefiniteError{Code: "INVALID", Message: "no idempotency key"}
		}
		apply := func() map[string]any {
			for _, a := range c.appointments {
				if a["id"] == call.Args["appointmentId"] {
					if a["slotId"] != call.Args["slotId"] {
						a["slotId"] = call.Args["slotId"]
						c.reschedules++
					}
					return a
				}
			}
			return nil
		}
		if err := c.failReschedule; err != nil {
			c.failReschedule = nil
			if c.performFirst {
				apply()
			}
			return nil, err
		}
		a := apply()
		if a == nil {
			return nil, &act.DefiniteError{Code: "NOT_FOUND", Message: "no such appointment"}
		}
		return map[string]any{"id": a["id"], "slotId": a["slotId"]}, nil
	})
	return r
}

// readerDecider answers "pick" questions from the reply and "yes"
// questions from sim/não, like a well-behaved model would.
type readerDecider struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (d *readerDecider) Name() string { return "reader" }

func (d *readerDecider) Decide(_ context.Context, req decide.Request) (*decide.Response, error) {
	d.mu.Lock()
	d.calls++
	fail := d.fail
	d.mu.Unlock()
	if fail {
		return nil, errors.New("backend down")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	state, _ := req.State.(map[string]any)
	reply, _ := state["reply"].(string)
	if reply == "" {
		reply, _ = state["message"].(string) // the message that started the run
	}
	resp := &decide.Response{Answers: map[string]decide.Answer{}, Backend: d.Name(), Model: "reader-1"}
	for id, q := range req.Questions {
		var probs map[string]float64
		switch q.Kind {
		case decide.KindChoice:
			probs = map[string]float64{}
			pick := noneKey
			for _, o := range q.Options {
				if o.Key != noneKey && reply != "" && matches(o.Description, reply) {
					pick = o.Key
				}
			}
			for _, o := range q.Options {
				probs[o.Key] = 0.05 / float64(len(q.Options)-1)
			}
			probs[pick] = 0.95
		case decide.KindNoul:
			switch {
			case strings.Contains(q.Instructions, "something different instead"):
				probs = map[string]float64{"true": 0.03, "false": 0.97}
				if strings.Contains(reply, "prefiro") || strings.Contains(reply, "mudei") {
					probs = map[string]float64{"true": 0.97, "false": 0.03}
				}
			case strings.Contains(reply, "sim"):
				probs = map[string]float64{"true": 0.97, "false": 0.03}
			case strings.Contains(reply, "não"):
				probs = map[string]float64{"true": 0.03, "false": 0.97}
			default:
				probs = map[string]float64{"true": 0.5, "false": 0.5}
			}
		}
		a, err := decide.AnswerFor(q, probs, decide.ProbNative)
		if err != nil {
			return nil, err
		}
		resp.Answers[id] = a
	}
	return resp, nil
}

// matches: "a de terça" picks "Ter 22/09 14:00", "às 16h" picks "... 16:00".
func matches(label, reply string) bool {
	switch {
	case strings.Contains(reply, "terça"):
		return strings.HasPrefix(label, "Ter")
	case strings.Contains(reply, "segunda"):
		return strings.HasPrefix(label, "Seg")
	case strings.Contains(reply, "16h"):
		return strings.HasSuffix(label, "16:00")
	case strings.Contains(reply, "10h"):
		return strings.HasSuffix(label, "10:00")
	}
	return false
}

type harness struct {
	t     *testing.T
	rt    *Runtime
	store *journal.Memory
	clin  *clinic
	dec   *readerDecider
	slept []time.Duration
	// user collects every message delivered to the user, in order.
	user   []string
	queues []Message
}

func newHarness(t *testing.T, paths ...string) *harness {
	if len(paths) == 0 {
		paths = []string{"../../testdata/agenda"}
	}
	h := &harness{t: t, store: journal.NewMemory(), clin: newClinic(), dec: &readerDecider{}}
	h.rt = &Runtime{
		Program: admit(t, paths...),
		Store:   h.store,
		Actor:   h.clin.actor(),
		Decider: h.dec,
		Clock:   func() time.Time { return testNow },
		Sleep: func(_ context.Context, d time.Duration) error {
			h.slept = append(h.slept, d)
			return nil
		},
		NewID: func() string { return "run-1" },
	}
	return h
}

var patient = map[string]any{"role": "patient", "patientId": "p1", "appointmentIds": []any{"a1", "a2"}}

func (h *harness) start(msg string) Result {
	h.t.Helper()
	res, err := h.rt.Start(context.Background(), Start{
		Namespace: "agenda-exemplo", Function: "reschedule-appointment", Conversation: "c1", Caller: patient, Message: msg,
	})
	if err != nil {
		h.t.Fatalf("start: %v", err)
	}
	return h.deliver(res)
}

func (h *harness) reply(from, text string, value any) Result {
	h.t.Helper()
	rep := Reply{From: from, Text: text, Value: value}
	if from == "human" { // a person answers the task as it is now
		task, err := h.rt.PendingTask(context.Background(), "run-1")
		if err != nil {
			h.t.Fatalf("task: %v", err)
		}
		rep.TaskHash = task.Hash
	}
	res, err := h.rt.Deliver(context.Background(), "run-1", rep)
	if err != nil {
		h.t.Fatalf("deliver %q: %v", text, err)
	}
	return h.deliver(res)
}

// deliver plays the channel layer: take the outbox, then ack it.
func (h *harness) deliver(res Result) Result {
	h.t.Helper()
	for _, m := range res.Outbox {
		if m.To == "user" {
			h.user = append(h.user, m.Text)
		} else {
			h.queues = append(h.queues, m)
		}
	}
	if n := len(res.Outbox); n > 0 {
		if err := h.rt.Ack(context.Background(), res.RunID, res.Outbox[n-1].Seq); err != nil {
			h.t.Fatal(err)
		}
	}
	return res
}

func expectStatus(t *testing.T, res Result, want journal.Status) {
	t.Helper()
	if res.Status != want {
		t.Fatalf("status %s, want %s (error %q)", res.Status, want, res.Error)
	}
}
