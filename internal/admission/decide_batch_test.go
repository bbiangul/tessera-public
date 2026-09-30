package admission

import (
	"os"
	"strings"
	"testing"
)

func TestBatchDecisionAdmission(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/decide-batch/function.yaml")
	if err != nil {
		t.Fatal(err)
	}
	base := string(raw)
	if issues := errorsOnly(validateDir(t, map[string]string{"fn.yaml": base})); len(issues) > 0 {
		t.Fatal(issues)
	}
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"self-dependency", "${ event.message }", "${ steps.preferences.output }", "UNRESOLVED_REF@spec.steps[0].decideBatch.state"},
		{"two-kinds", "            yesNo:", "            score: [low, high]\n            yesNo:", "STEP_KIND@spec.steps[0].decideBatch.questions.weekend"},
		{"empty-name", "          weekend:", "          '':", "INVALID_FIELD@spec.steps[0].decideBatch.questions."},
		{"two-step-kinds", "      decideBatch:", "      respond: { template: x }\n      decideBatch:", "STEP_KIND@spec.steps[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(base, tc.old) {
				t.Fatal("bad mutation")
			}
			changed := strings.Replace(base, tc.old, tc.replacement, 1)
			got := codesAt(errorsOnly(validateDir(t, map[string]string{"fn.yaml": changed})))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}
