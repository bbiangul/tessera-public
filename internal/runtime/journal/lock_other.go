//go:build !unix

package journal

import "errors"

// ErrLocked means another process runs with the same database.
var ErrLocked = errors.New("another tessera process runs with this database")

// LockRunner is not enforced on this platform: run one process per
// database.
func LockRunner(path string) (release func(), err error) { return func() {}, nil }
