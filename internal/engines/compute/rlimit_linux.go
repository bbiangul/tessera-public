package compute

import (
	"os"
	"syscall"
)

// limitData bounds the process's data segment, which counts the Go heap's
// mappings: an allocation past it fails and the runtime exits.
func limitData(n uint64) error {
	return syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: n, Max: n})
}

// offerToOOMKiller makes the worker the first process the kernel kills
// when the container runs out of memory, so the engine is spared.
func offerToOOMKiller() {
	_ = os.WriteFile("/proc/self/oom_score_adj", []byte("1000"), 0)
}
