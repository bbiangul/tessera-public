package admission

import (
	"strings"
	"testing"
)

func TestRunReadsValidateAgainstExample(t *testing.T) {
	base := read(t, "../../testdata/contexto/agent.yaml")
	if got := codesAt(errorsOnly(validate(t, map[string]string{"agent.yaml": base}))); got != "" {
		t.Fatalf("valid example: %s", got)
	}
	for _, tc := range []struct{ ref, code string }{
		{"absent/slots", "UNKNOWN_PLUGIN"},
		{"context-agenda/absent", "UNKNOWN_ACTION"},
		{"context-agenda/reschedule", "EFFECT_NOT_ALLOWED"},
	} {
		docs := strings.Replace(base, "runReads: [context-agenda/slots]", "runReads: ["+tc.ref+"]", 1)
		got := codesAt(errorsOnly(validate(t, map[string]string{"agent.yaml": docs})))
		if !strings.Contains(got, tc.code+"@spec.freeMode.runReads[0]") {
			t.Errorf("%s: %s", tc.ref, got)
		}
	}
}
