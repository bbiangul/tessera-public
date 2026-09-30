//go:build unix

package journal

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrLocked means another process runs with the same database.
var ErrLocked = errors.New("another tessera process runs with this database")

// LockRunner takes the database's runner lock (a file next to it) for as
// long as the process lives, or until release. Only one process may
// execute runs of a database: execution is serialized per conversation
// inside a process, not across processes (cluster mode is phase 4).
// Readers (tessera runs, metrics) do not take it.
func LockRunner(path string) (release func(), err error) {
	if path == ":memory:" {
		return func() {}, nil
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w (%s): one process executes runs per database", ErrLocked, path)
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
