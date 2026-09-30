package runtime

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// SideMessages returns answered side messages for this opted-in pending wait.
// A nil slice means capture is disabled. An enabled empty capture is non-nil.
func (r *Runtime) SideMessages(ctx context.Context, runID string, caller map[string]any) ([]string, error) {
	run, records, enabled, err := r.sideCapture(ctx, runID, caller)
	if err != nil || !enabled {
		return nil, err
	}
	return sideMessages(records, run.Pending)
}

func (r *Runtime) sideCapture(ctx context.Context, runID string, caller map[string]any) (journal.Run, []journal.Record, bool, error) {
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return run, nil, false, err
	}
	if run.Status != journal.StatusWaitingForUser {
		return run, nil, false, nil
	}
	records, err := r.Store.Records(ctx, runID)
	if err != nil {
		return run, nil, false, err
	}
	enabled := false
	for _, rec := range records {
		if rec.Site == run.Pending && rec.Kind == journal.KindSleep {
			m, err := decodeMap(rec.Value)
			if err != nil {
				return run, nil, false, err
			}
			enabled, _ = m["includeSideMessages"].(bool)
		}
	}
	if !enabled {
		return run, nil, false, nil
	}
	if run.Generation != r.Program.Generation() {
		return run, nil, false, ErrGeneration
	}
	scopes, err := decodeMap(run.Scopes)
	if err != nil {
		return run, nil, false, err
	}
	have, err := journal.Encode(scopes["caller"])
	if err != nil {
		return run, nil, false, err
	}
	want, err := journal.Encode(normalizeMap(caller))
	if err != nil {
		return run, nil, false, err
	}
	if !bytes.Equal(have, want) {
		return run, nil, false, fmt.Errorf("side messages: caller mismatch")
	}
	return run, records, true, nil
}

// CaptureSideMessage records a user message only after its side question was
// answered. It does not resume the wait, change its deadline or grant consent.
func (r *Runtime) CaptureSideMessage(ctx context.Context, runID, pending string, caller map[string]any, text string) error {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	run, records, enabled, err := r.sideCapture(ctx, runID, caller)
	if err != nil {
		return err
	}
	if !enabled || run.Pending != pending {
		return ErrNotWaiting
	}
	messages, err := sideMessages(records, pending)
	if err != nil {
		return err
	}
	size := len(text)
	for _, s := range messages {
		size += len(s)
	}
	if len(messages) >= 8 || size > 32*1024 {
		return fmt.Errorf("side messages exceed 8 messages or 32 KiB")
	}
	v, err := journal.Encode(map[string]any{"message": text})
	if err != nil {
		return err
	}
	var seq int
	for _, rec := range records {
		if rec.Seq > seq {
			seq = rec.Seq
		}
	}
	_, err = r.Store.Append(ctx, runID, journal.Record{Site: fmt.Sprintf("%s/side/%d", pending, seq+1), Kind: journal.KindResult, Value: v, At: r.Clock()})
	return err
}

func sideMessages(records []journal.Record, site string) ([]string, error) {
	selected := []journal.Record{}
	for _, rec := range records {
		if rec.Kind == journal.KindResult && strings.HasPrefix(rec.Site, site+"/side/") {
			selected = append(selected, rec)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Seq < selected[j].Seq })
	messages := []string{}
	for _, rec := range selected {
		m, err := decodeMap(rec.Value)
		if err != nil {
			return nil, err
		}
		text, ok := m["message"].(string)
		if !ok {
			return nil, fmt.Errorf("malformed side message at %s", rec.Site)
		}
		messages = append(messages, text)
	}
	return messages, nil
}

func (e *execution) waitWithSideMessages(f *frame, s *step, site string) (any, error) {
	out, err := e.wait(f, s, site)
	if err != nil || !s.includeSideMessages {
		return out, err
	}
	records := make([]journal.Record, 0, len(e.recs))
	for _, rec := range e.recs {
		records = append(records, rec)
	}
	messages, err := sideMessages(records, site)
	if err != nil {
		return nil, err
	}
	out.(map[string]any)["sideMessages"] = anyList(messages)
	return out, nil
}

// NoteAside records that a message at the pending question was about
// something else and got its own answer. Asides counts them: after the
// first, the host stops asking the question again (agency-001: a user who
// said twice they would send their data later heard the question each
// time). The run still waits; a later answer still reaches it.
func (r *Runtime) NoteAside(ctx context.Context, runID, pending string) error {
	r.defaults()
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	unlock := r.locks.lock(run.Conversation)
	defer unlock()
	records, err := r.Store.Records(ctx, runID)
	if err != nil {
		return err
	}
	var seq int
	for _, rec := range records {
		if rec.Seq > seq {
			seq = rec.Seq
		}
	}
	v, err := journal.Encode(map[string]any{})
	if err != nil {
		return err
	}
	_, err = r.Store.Append(ctx, runID, journal.Record{Site: fmt.Sprintf("%s/aside/%d", pending, seq+1), Kind: journal.KindResult, Value: v, At: r.Clock()})
	return err
}

// Asides is how many messages at the pending question got their own answer.
func (r *Runtime) Asides(ctx context.Context, runID, pending string) (int, error) {
	records, err := r.Store.Records(ctx, runID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range records {
		if rec.Kind == journal.KindResult && strings.HasPrefix(rec.Site, pending+"/aside/") {
			n++
		}
	}
	return n, nil
}

// Relays says whether a run waits on a relay wait (wait.relay): every reply
// goes to it as written, without side-question classification.
func (r *Runtime) Relays(ctx context.Context, run journal.Run) (bool, error) {
	if run.Status != journal.StatusWaitingForUser {
		return false, nil
	}
	records, err := r.Store.Records(ctx, run.ID)
	if err != nil {
		return false, err
	}
	for _, rec := range records {
		if rec.Site == run.Pending && rec.Kind == journal.KindSleep {
			m, err := decodeMap(rec.Value)
			if err != nil {
				return false, err
			}
			relay, _ := m["relay"].(bool)
			return relay, nil
		}
	}
	return false, nil
}

// PendingFormat is the format (email, phone, cpf) of the input a run's
// pending question asks for, or "". A reply holding that pattern is the
// answer, whatever else it asks (agency-003: "fulana@x.com. E o
// pagamento?" was taken as a side question and the e-mail asked again).
func (r *Runtime) PendingFormat(run journal.Run) string {
	if run.Status != journal.StatusWaitingForUser {
		return ""
	}
	fn := r.Program.functions[run.Namespace+"/"+run.Function]
	site := run.Pending
	if fn == nil || strings.Contains(site, "steps.") {
		return "" // a nested Function's question: not looked up
	}
	name, ok := strings.CutPrefix(site, "inputs.")
	if !ok {
		return ""
	}
	name, _, _ = strings.Cut(name, "/")
	for _, in := range fn.inputs {
		if in.name == name {
			return in.format
		}
	}
	return ""
}
