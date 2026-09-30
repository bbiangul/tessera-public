package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	plugins "github.com/bbiangul/tessera/api/plugins/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// RunReadFacts returns masked snapshots of explicitly allowed top-level
// fromAction reads in a pending run. It never executes the Function or a
// plugin, and never exposes arguments, arbitrary scopes or nested calls.
// Identity and generation checks precede reading any result.
func (r *Runtime) RunReadFacts(ctx context.Context, runID, namespace, conversation string, caller map[string]any, allowed []string) (map[string]any, error) {
	if len(allowed) == 0 {
		return nil, nil
	}
	run, err := r.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Namespace != namespace || run.Conversation != conversation {
		return nil, fmt.Errorf("run read context: namespace or conversation mismatch")
	}
	if run.Generation != r.Program.Generation() {
		return nil, ErrGeneration
	}
	if run.Status != journal.StatusWaitingForUser && run.Status != journal.StatusConfirming {
		return nil, fmt.Errorf("run read context: the run is not waiting for the user")
	}
	scopes, err := decodeMap(run.Scopes)
	if err != nil {
		return nil, err
	}
	stored, ok := scopes["caller"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("run read context: missing caller")
	}
	want, err := journal.Encode(normalizeMap(caller))
	if err != nil {
		return nil, err
	}
	have, err := journal.Encode(stored)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(have, want) {
		return nil, fmt.Errorf("run read context: caller mismatch")
	}
	fn := r.Program.functions[namespace+"/"+run.Function]
	if fn == nil {
		return nil, ErrUnknownFunction
	}
	permit := make(map[string]bool, len(allowed))
	for _, action := range allowed {
		if spec, ok := r.Program.actions[action]; !ok || spec.Effect != plugins.EffectRead {
			return nil, fmt.Errorf("run read context: %s is not a declared read action", action)
		}
		permit[action] = true
	}
	// Match exact compiled source sites, never a suffix or an arbitrary
	// journal path supplied by the model. Unselected results remain private.
	sites := map[string]string{}
	for _, in := range fn.inputs {
		for _, source := range in.sources {
			if permit[source.fromAction] {
				sites["inputs."+in.name+"/"+source.id] = source.fromAction
			}
		}
	}
	records, err := r.Store.Records(ctx, runID)
	if err != nil {
		return nil, err
	}
	facts := map[string]any{}
	for _, rec := range records {
		action, selected := sites[rec.Site]
		if !selected || rec.Kind != journal.KindResult {
			continue
		}
		out, err := journal.Decode(rec.Value)
		if err != nil {
			return nil, fmt.Errorf("run read context at %s: %w", rec.Site, err)
		}
		facts[rec.Site] = map[string]any{"action": action, "output": r.Mask(out), "readAt": rec.At.Format("2006-01-02T15:04:05Z07:00")}
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 32*1024 {
		return nil, fmt.Errorf("run read context exceeds 32 KiB; expose a smaller read result")
	}
	return facts, nil
}
