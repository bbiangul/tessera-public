package admission

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSideCaptureRequiresReplyWait(t *testing.T) {
	b, err := os.ReadFile("../../testdata/dias-uteis/search-request.function.json")
	if err != nil {
		t.Fatal(err)
	}
	var resource map[string]any
	if err := json.Unmarshal(b, &resource); err != nil {
		t.Fatal(err)
	}
	steps := resource["spec"].(map[string]any)["steps"].([]any)
	wait := steps[2].(map[string]any)["wait"].(map[string]any)
	wait["includeSideMessages"], wait["orReply"] = true, false
	b, err = json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	got := codesAt(errorsOnly(validateDir(t, map[string]string{"f.json": string(b)})))
	want := "INVALID_FIELD@spec.steps[2].wait.includeSideMessages"
	if !strings.Contains(got, want) {
		t.Fatalf("want %s, got %s", want, got)
	}
}
