package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// routingHistory supplies only dialogue from the same caller and namespace.
// It gives routing the completion preceding an intervening free answer,
// without sharing Function inputs or results.
func (h *Host) routingHistory(ctx context.Context, conversation string, caller map[string]any) ([]string, error) {
	last, ok, err := h.opts.Store.LastRun(ctx, conversation)
	if err != nil || !ok {
		return nil, err
	}
	if last.Namespace != h.opts.Namespace {
		return nil, fmt.Errorf("routing history: namespace mismatch")
	}
	scopes, err := journal.Decode(last.Scopes)
	if err != nil {
		return nil, err
	}
	m, ok := scopes.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("routing history: invalid scopes")
	}
	if caller == nil {
		caller = map[string]any{}
	}
	want, err := json.Marshal(caller)
	if err != nil {
		return nil, err
	}
	have, err := json.Marshal(m["caller"])
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(want, have) {
		return nil, fmt.Errorf("routing history: caller mismatch")
	}
	return h.Runtime.Conversation(ctx, last.ID, 8)
}
