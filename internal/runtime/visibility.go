package runtime

import (
	"context"
	"errors"
	"strings"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Context visibility (RFC-0006 section 1, audiences-002): what a member
// sees of the conversation behind a task is declared by their audience
// (sees: [{audience, conversation: masked|full}]), and every view is
// journaled in the task's run, as the audit.

// visibleLines is how much of the conversation a view shows.
const visibleLines = 6

// Visibility is what member m sees of the conversation behind task t:
// "masked", "full", or "" (only the task's text).
func (r *Runtime) Visibility(ctx context.Context, t Task, m agents.Member) string {
	if r.AudienceOf == nil || len(r.Sees) == 0 {
		return ""
	}
	viewer := r.AudienceOf(AsMember(map[string]any{}, m))
	run, err := r.Store.GetRun(ctx, t.RunID)
	if err != nil {
		return ""
	}
	scopes, _ := decodeMap(run.Scopes)
	caller, _ := scopes["caller"].(map[string]any)
	subject, _ := caller["audience"].(string)
	return r.Sees[viewer][subject]
}

// TaskView is task t as member m sees it on channel: its text and, when
// their audience may see it, the conversation's last lines, masked or in
// full. A view with the conversation is journaled in the task's run: who,
// on which channel, how.
func (r *Runtime) TaskView(ctx context.Context, t Task, m agents.Member, channel string) string {
	lines, vis := r.TaskContext(ctx, t, m, channel)
	if vis == "" || len(lines) == 0 {
		return t.Text
	}
	return t.Text + "\n\nConversa:\n" + strings.Join(lines, "\n")
}

// TaskContext is the conversation lines m may see behind t (masked when
// so declared) and the visibility; the view is journaled.
func (r *Runtime) TaskContext(ctx context.Context, t Task, m agents.Member, channel string) ([]string, string) {
	r.defaults()
	vis := r.Visibility(ctx, t, m)
	if vis == "" {
		return nil, ""
	}
	lines, err := r.Conversation(ctx, t.RunID, visibleLines)
	if err != nil || len(lines) == 0 {
		return nil, ""
	}
	if vis == "masked" {
		masked := make([]string, len(lines))
		for i, l := range lines {
			masked[i], _ = r.Mask(l).(string)
		}
		lines = masked
	}
	run, err := r.Store.GetRun(ctx, t.RunID)
	if err != nil {
		return nil, ""
	}
	v, err := journal.Encode(map[string]any{"member": m.ID, "channel": channel, "visibility": vis, "lines": len(lines)})
	if err != nil {
		return nil, ""
	}
	unlock := r.locks.lock(run.Conversation)
	_, err = r.Store.Append(ctx, t.RunID, journal.Record{Site: t.Site + "/viewed/" + m.ID + "/" + channel, Kind: journal.KindResult, Value: v, At: r.Clock()})
	unlock()
	if err != nil && !errors.Is(err, journal.ErrDuplicate) {
		return nil, "" // no audit, no view
	}
	return lines, vis
}
