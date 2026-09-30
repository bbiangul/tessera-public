// Package journal stores what happened in a run (RFC-0003). The journal is
// the source of truth: a run is re-executed from the top over its journal,
// and every site that already has a record returns it instead of running
// again, so a resumed run never repeats a model call or an effect.
package journal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind is the type of a record.
type Kind string

const (
	// KindResult is the value a site produced.
	KindResult Kind = "result"
	// KindError is a classified failure of a site.
	KindError Kind = "error"
	// KindIntent is written before an effect starts. An intent with no
	// result means the outcome is unknown.
	KindIntent Kind = "intent"
	// KindSuspend records that the run waits for someone, and what was sent.
	KindSuspend Kind = "suspend"
	// KindResume is the answer that ended a suspension.
	KindResume Kind = "resume"
	// KindSend is a message for the channel layer to deliver.
	KindSend Kind = "send"
	// KindSleep records that a wait step sleeps until a time; it is not a
	// message.
	KindSleep Kind = "sleep"
	// KindMilestone is a business milestone of a subject (RFC-0001,
	// "Métricas de negócio"); stores also index it across runs.
	KindMilestone Kind = "milestone"
)

// Milestone is a business fact about a subject: a lead that booked, a
// store that opened. Its value in the journal is
// {namespace, subject, id, name, at, attributes}.
type Milestone struct {
	Namespace  string            `json:"namespace"`
	Subject    string            `json:"subject"` // the Subject kind, e.g. paciente
	ID         string            `json:"id"`      // the subject's pseudonymous ID
	Name       string            `json:"name"`
	At         time.Time         `json:"at"`
	Attributes map[string]string `json:"attributes,omitempty"`
	RunID      string            `json:"runId"`
	Site       string            `json:"site"`
	// Seq orders milestones by when they were recorded (not by At, which
	// a step may set in the past); MilestonesAfter pages by it.
	Seq int64 `json:"seq,omitempty"`
}

// MilestoneOf decodes a milestone record.
func MilestoneOf(runID string, r Record) (Milestone, error) {
	v, err := Decode(r.Value)
	if err != nil {
		return Milestone{}, err
	}
	m, _ := v.(map[string]any)
	out := Milestone{RunID: runID, Site: r.Site, Attributes: map[string]string{}}
	out.Namespace, _ = m["namespace"].(string)
	out.Subject, _ = m["subject"].(string)
	out.ID, _ = m["id"].(string)
	out.Name, _ = m["name"].(string)
	out.At, _ = m["at"].(time.Time)
	if a, ok := m["attributes"].(map[string]any); ok {
		for k, x := range a {
			out.Attributes[k] = fmt.Sprint(x)
		}
	}
	if out.Namespace == "" || out.Subject == "" || out.ID == "" || out.Name == "" || out.At.IsZero() {
		return Milestone{}, fmt.Errorf("milestone at %s: namespace, subject, id, name and at are required", r.Site)
	}
	return out, nil
}

// Record is one entry. Value holds the Encode()d value.
type Record struct {
	Seq   int       `json:"seq"`
	Site  string    `json:"site"`
	Kind  Kind      `json:"kind"`
	Value []byte    `json:"value,omitempty"`
	At    time.Time `json:"at"`
}

// Status is where a run is (RFC-0001, "Ciclo de vida de um Run").
type Status string

const (
	StatusResolving       Status = "Resolving"
	StatusWaitingForUser  Status = "WaitingForUser"
	StatusWaitingForHuman Status = "WaitingForHuman"
	// StatusWaitingForTime: a wait step sleeps until Run.WakeAt. Such a
	// run is not the conversation's active run: the user's messages start
	// other runs while it sleeps.
	StatusWaitingForTime Status = "WaitingForTime"
	StatusConfirming     Status = "Confirming"
	StatusActing         Status = "Acting"
	StatusIndeterminate  Status = "Indeterminate"
	StatusSucceeded      Status = "Succeeded"
	StatusFailed         Status = "Failed"
)

// Final reports whether no event can change the run any more.
func (s Status) Final() bool { return s == StatusSucceeded || s == StatusFailed }

// Run is a run's header. Records are stored separately.
type Run struct {
	ID           string `json:"id"`
	Namespace    string `json:"namespace"`
	Function     string `json:"function"`
	Generation   string `json:"generation"`
	Conversation string `json:"conversation"`
	// Scopes holds the encoded caller, conversation and event scopes, fixed
	// at creation.
	Scopes []byte `json:"scopes"`
	Status Status `json:"status"`
	// Pending is the site the run waits on, when it waits.
	Pending string `json:"pending,omitempty"`
	// Acked is the highest record seq whose message the channel layer has
	// delivered; messages after it are delivered again.
	Acked int `json:"acked"`
	// WakeAt is when a run WaitingForTime is due; zero otherwise.
	WakeAt    time.Time `json:"wakeAt,omitzero"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store persists runs and their records.
type Store interface {
	CreateRun(ctx context.Context, r Run) error
	GetRun(ctx context.Context, id string) (Run, error)
	// UpdateRun changes status and pending site, and clears WakeAt.
	UpdateRun(ctx context.Context, id string, status Status, pending string) error
	// Sleep puts a run in status (WaitingForTime, or WaitingForUser for a
	// wait that also ends on a reply) at the pending site until wake.
	Sleep(ctx context.Context, id string, status Status, pending string, wake time.Time) error
	// DueRuns returns up to limit runs with a WakeAt not after now, still
	// waiting (for time, or for a reply that did not come), earliest first.
	// Only runs of generation, when it is not empty: a process runs only
	// its own definitions, and other generations' runs must not fill its
	// page.
	DueRuns(ctx context.Context, now time.Time, generation string, limit int) ([]Run, error)
	// Append adds a record. Seq is assigned by the store and returned. A
	// second record with the same (site, kind) is refused with ErrDuplicate:
	// sites are unique by construction, so a duplicate is a bug.
	Append(ctx context.Context, runID string, r Record) (int, error)
	Records(ctx context.Context, runID string) ([]Record, error)
	// Ack records that messages up to seq were delivered. It never moves
	// backwards.
	Ack(ctx context.Context, runID string, seq int) error
	// ActiveRun returns the most recent run of a conversation that is not
	// final nor sleeping (WaitingForTime), if any.
	ActiveRun(ctx context.Context, conversation string) (Run, bool, error)
	// Runs lists runs, newest first, for inspection (the console, tessera
	// runs). Empty filter fields match everything.
	Runs(ctx context.Context, f RunFilter) ([]Run, error)
	// LastRun returns the most recent run of a conversation, final or not.
	LastRun(ctx context.Context, conversation string) (Run, bool, error)
	// Unacked returns up to limit runs that have messages (send or
	// suspend records) after their Acked seq, oldest first: the outbox a
	// channel layer polls. A prefix keeps only conversations starting with
	// it (a channel's own, e.g. "wa:"), so other channels' unread runs
	// never fill its page.
	Unacked(ctx context.Context, prefix string, limit int) ([]Run, error)
	// Watch records that a run waits at site for an event with a key
	// (idempotent). Watchers returns the watches of an event and key;
	// Unwatch forgets a run's watches. A watch can outlive its wait (a
	// timer or a reply ended it first): whoever delivers checks that the
	// run still waits at that site.
	Watch(ctx context.Context, w Watch) error
	Watchers(ctx context.Context, namespace, event, key string) ([]Watch, error)
	Unwatch(ctx context.Context, runID string) error
	// MilestonesAfter returns up to limit milestones of a namespace
	// recorded after seq, in the order they were recorded; LastMilestone
	// is the seq of the latest (0 for none).
	MilestonesAfter(ctx context.Context, namespace string, after int64, limit int) ([]Milestone, error)
	LastMilestone(ctx context.Context, namespace string) (int64, error)
	// Cursor and SetCursor keep a named position (a milestone trigger's
	// last seq handled) across restarts.
	Cursor(ctx context.Context, name string) (int64, bool, error)
	// Changes returns up to limit records of a namespace's runs appended
	// after cursor, in the order they were appended: what a live view
	// follows.
	Changes(ctx context.Context, namespace string, after int64, limit int) ([]Change, error)
	// FirstDelivery records a user message's ID in a conversation and
	// reports whether this is its first delivery: channels retry, and a
	// retried message must not be handled twice.
	FirstDelivery(ctx context.Context, conversation, messageID string) (bool, error)
	SetCursor(ctx context.Context, name string, v int64) error
	// Milestones returns the milestones of a subject kind in a namespace,
	// oldest first.
	Milestones(ctx context.Context, namespace, subject string) ([]Milestone, error)
}

// RunFilter selects runs to list.
type RunFilter struct {
	Namespace    string
	Conversation string
	Function     string
	Status       Status
	// Before lists runs created before this time (paging); zero: now.
	Before time.Time
	Limit  int // default 50, at most 500
}

func (f RunFilter) limit() int {
	switch {
	case f.Limit <= 0:
		return 50
	case f.Limit > 500:
		return 500
	}
	return f.Limit
}

func (f RunFilter) match(r Run) bool {
	return (f.Namespace == "" || r.Namespace == f.Namespace) && (f.Conversation == "" || r.Conversation == f.Conversation) &&
		(f.Function == "" || r.Function == f.Function) && (f.Status == "" || r.Status == f.Status) &&
		(f.Before.IsZero() || r.CreatedAt.Before(f.Before))
}

var (
	ErrNotFound  = errors.New("run not found")
	ErrExists    = errors.New("run already exists")
	ErrDuplicate = errors.New("record already exists for this site")
)

// Memory is an in-memory Store for tests and the dev loop.
type Memory struct {
	mu      sync.Mutex
	runs    map[string]Run
	records map[string][]Record
	order   map[string]int // insertion order, the tiebreak for equal times
	watches []Watch
	cursors map[string]int64
	appends []Change // every record, in append order (Cursor = index + 1)
	seenMsg map[string]bool

	milestones []Milestone
	// FailAppendAfter, when > 0, makes the n-th Append from now fail, for
	// crash-injection tests. The failing record is not stored.
	FailAppendAfter int
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory {
	return &Memory{runs: map[string]Run{}, records: map[string][]Record{}, order: map[string]int{}}
}

// ErrInjected is returned by Memory when FailAppendAfter triggers.
var ErrInjected = errors.New("injected failure")

func (m *Memory) CreateRun(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[r.ID]; ok {
		return fmt.Errorf("%w: %s", ErrExists, r.ID)
	}
	m.runs[r.ID] = r
	m.order[r.ID] = len(m.order)
	return nil
}

// newer reports whether a was created after b: by time, then by insertion
// (a fixed clock, or two runs in the same tick, give equal times).
func (m *Memory) newer(a, b Run) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return m.order[a.ID] > m.order[b.ID]
}

func (m *Memory) GetRun(_ context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return r, nil
}

func (m *Memory) UpdateRun(_ context.Context, id string, status Status, pending string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	r.Status, r.Pending, r.UpdatedAt, r.WakeAt = status, pending, time.Now().UTC(), time.Time{}
	m.runs[id] = r
	return nil
}

func (m *Memory) Sleep(_ context.Context, id string, status Status, pending string, wake time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	r.Status, r.Pending, r.UpdatedAt, r.WakeAt = status, pending, time.Now().UTC(), wake.UTC()
	m.runs[id] = r
	return nil
}

func (m *Memory) DueRuns(_ context.Context, now time.Time, generation string, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, r := range m.runs {
		if generation != "" && r.Generation != generation {
			continue
		}
		if (r.Status == StatusWaitingForTime || r.Status == StatusWaitingForUser || r.Status == StatusWaitingForHuman || r.Status == StatusIndeterminate) && !r.WakeAt.IsZero() && !r.WakeAt.After(now) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].WakeAt.Equal(out[j].WakeAt) {
			return out[i].WakeAt.Before(out[j].WakeAt)
		}
		return m.order[out[i].ID] < m.order[out[j].ID]
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) Append(_ context.Context, runID string, r Record) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[runID]; !ok {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	if m.FailAppendAfter > 0 {
		m.FailAppendAfter--
		if m.FailAppendAfter == 0 {
			return 0, ErrInjected
		}
	}
	for _, x := range m.records[runID] {
		if x.Site == r.Site && x.Kind == r.Kind {
			return 0, fmt.Errorf("%w: %s %s", ErrDuplicate, r.Kind, r.Site)
		}
	}
	if r.Kind == KindMilestone {
		ms, err := MilestoneOf(runID, r)
		if err != nil {
			return 0, err
		}
		ms.Seq = int64(len(m.milestones) + 1)
		m.milestones = append(m.milestones, ms)
	}
	r.Seq = len(m.records[runID]) + 1
	m.records[runID] = append(m.records[runID], r)
	m.appends = append(m.appends, Change{Cursor: int64(len(m.appends) + 1), RunID: runID, Record: r})
	return r.Seq, nil
}

func (m *Memory) Runs(_ context.Context, f RunFilter) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, r := range m.runs {
		if f.match(r) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.newer(out[i], out[j]) })
	if len(out) > f.limit() {
		out = out[:f.limit()]
	}
	return out, nil
}

func (m *Memory) Unacked(_ context.Context, prefix string, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for id, r := range m.runs {
		if !strings.HasPrefix(r.Conversation, prefix) {
			continue
		}
		for _, rec := range m.records[id] {
			if rec.Seq > r.Acked && (rec.Kind == KindSend || rec.Kind == KindSuspend) {
				out = append(out, r)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return m.order[out[i].ID] < m.order[out[j].ID] })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) Milestones(_ context.Context, namespace, subject string) ([]Milestone, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Milestone
	for _, x := range m.milestones {
		if x.Namespace == namespace && x.Subject == subject {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func (m *Memory) Records(_ context.Context, runID string) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[runID]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	return append([]Record(nil), m.records[runID]...), nil
}

func (m *Memory) Ack(_ context.Context, runID string, seq int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[runID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, runID)
	}
	if seq > r.Acked {
		r.Acked = seq
		m.runs[runID] = r
	}
	return nil
}

func (m *Memory) ActiveRun(_ context.Context, conversation string) (Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best Run
	found := false
	for _, r := range m.runs {
		if r.Conversation != conversation || r.Status.Final() || r.Status == StatusWaitingForTime {
			continue
		}
		if !found || m.newer(r, best) {
			best, found = r, true
		}
	}
	return best, found, nil
}

func (m *Memory) LastRun(_ context.Context, conversation string) (Run, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best Run
	found := false
	for _, r := range m.runs {
		if r.Conversation != conversation {
			continue
		}
		if !found || m.newer(r, best) {
			best, found = r, true
		}
	}
	return best, found, nil
}

// Watch is a run waiting at a site for an event about a key.
type Watch struct {
	RunID     string
	Namespace string
	Site      string
	Event     string
	Key       string
}

func (m *Memory) Watch(_ context.Context, w Watch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.watches {
		if x == w {
			return nil
		}
	}
	m.watches = append(m.watches, w)
	return nil
}

func (m *Memory) Watchers(_ context.Context, namespace, event, key string) ([]Watch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Watch
	for _, w := range m.watches {
		if w.Namespace == namespace && w.Event == event && w.Key == key {
			out = append(out, w)
		}
	}
	return out, nil
}

func (m *Memory) Unwatch(_ context.Context, runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.watches[:0]
	for _, w := range m.watches {
		if w.RunID != runID {
			kept = append(kept, w)
		}
	}
	m.watches = kept
	return nil
}

func (m *Memory) MilestonesAfter(_ context.Context, namespace string, after int64, limit int) ([]Milestone, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Milestone
	for _, x := range m.milestones {
		if x.Namespace == namespace && x.Seq > after && len(out) < limit {
			out = append(out, x)
		}
	}
	return out, nil
}

func (m *Memory) LastMilestone(_ context.Context, namespace string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last int64
	for _, x := range m.milestones {
		if x.Namespace == namespace {
			last = x.Seq
		}
	}
	return last, nil
}

func (m *Memory) Cursor(_ context.Context, name string) (int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.cursors[name]
	return v, ok, nil
}

func (m *Memory) SetCursor(_ context.Context, name string, v int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cursors == nil {
		m.cursors = map[string]int64{}
	}
	m.cursors[name] = v
	return nil
}

// Change is a record as a live view sees it: its position in the
// namespace's stream and its run.
type Change struct {
	Cursor int64
	RunID  string
	Record Record
}

func (m *Memory) Changes(_ context.Context, namespace string, after int64, limit int) ([]Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Change
	for _, c := range m.appends {
		if c.Cursor > after && m.runs[c.RunID].Namespace == namespace && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *Memory) FirstDelivery(_ context.Context, conversation, messageID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seenMsg == nil {
		m.seenMsg = map[string]bool{}
	}
	k := conversation + "\x00" + messageID
	if m.seenMsg[k] {
		return false, nil
	}
	m.seenMsg[k] = true
	return true, nil
}
