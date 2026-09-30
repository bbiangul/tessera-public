//go:build unix

package journal

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOneRunnerPerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	release, err := LockRunner(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockRunner(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("a second runner must be refused: %v", err)
	}
	release()
	again, err := LockRunner(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}
