package schemas

import (
	"bytes"
	"testing"
)

// TestSchemasAreUpToDate fails when the API types or their doc comments
// changed but the committed schemas were not regenerated with `make schema`.
func TestSchemasAreUpToDate(t *testing.T) {
	t.Chdir("../..")
	fresh, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	for kind, name := range Kinds {
		committed, err := Get(kind)
		if err != nil {
			t.Fatalf("%s: %v (run make schema)", kind, err)
		}
		if !bytes.Equal(committed, fresh[name]) {
			t.Errorf("%s is out of date: run make schema", name)
		}
	}
}
