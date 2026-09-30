package compute

import (
	"context"
	"errors"
	"testing"
)

const clinicSandbox = `
def list_upcoming_appointments(state, patientId):
    return [a for a in state["appointments"] if a["patient"] == patientId]

def reschedule(state, appointmentId, slotId):
    for a in state["appointments"]:
        if a["id"] == appointmentId:
            if state.get("timeout_next"):
                state["timeout_next"] = False
                a["slotId"] = slotId
                ambiguous("timeout after the change")
            a["slotId"] = slotId
            return a
    definite("NOT_FOUND", "no appointment " + appointmentId)
`

func TestSandbox(t *testing.T) {
	s, err := CompileSandbox("clinic.sandbox.star", []byte(clinicSandbox))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := NewSandbox(s, map[string]any{"appointments": []any{
		map[string]any{"id": "a1", "patient": "p1", "slotId": "s0"},
		map[string]any{"id": "a2", "patient": "p2", "slotId": "s0"},
	}, "timeout_next": true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	out, err := sb.Call(ctx, "list-upcoming-appointments", map[string]any{"patientId": "p1"})
	if err != nil || len(out.([]any)) != 1 {
		t.Fatalf("%v %v", out, err)
	}
	var f *SandboxFailure
	if _, err := sb.Call(ctx, "reschedule", map[string]any{"appointmentId": "a1", "slotId": "s9"}); !errors.As(err, &f) || f.Definite || f.Message != "timeout after the change" {
		t.Errorf("ambiguous failure: %v", err)
	}
	st, _ := sb.State()
	if st["appointments"].([]any)[0].(map[string]any)["slotId"] != "s9" {
		t.Error("state changed before an ambiguous failure must stay changed: that is the point")
	}
	if _, err := sb.Call(ctx, "reschedule", map[string]any{"appointmentId": "zz", "slotId": "s1"}); !errors.As(err, &f) || !f.Definite || f.Code != "NOT_FOUND" {
		t.Errorf("definite failure: %v", err)
	}
	if !sb.Has("reschedule") || sb.Has("cancel") {
		t.Error("Has")
	}
	if _, err := sb.Call(ctx, "cancel", nil); !errors.Is(err, ErrCompute) {
		t.Errorf("missing function: %v", err)
	}
}
