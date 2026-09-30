package compute

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// Isolation runs script calls in a child process (starmem-001, #6). The
// in-process watchdog stops a script at its next instruction, but one
// instruction such as [0] * (200 << 20) allocates gigabytes before any
// check runs, and the out-of-memory kill takes the whole engine with it.
// In a child, only the child dies; the call fails with "the memory limit
// was reached". On Linux the child also bounds itself with RLIMIT_DATA and
// offers itself first to the OOM killer; elsewhere it is only separated.
//
// The child is the same binary: the parent sets WorkerEnv, and the
// program's main must call RunWorkerIfRequested first thing (cmd/tessera
// does). Sandboxes (test fakes) always run in-process.
type Isolation struct {
	// Command starts the worker; default the running executable, with no
	// arguments.
	Command []string
	// Memory is the child's hard data limit; default four times the
	// call's Limits.Memory (1 GiB with the defaults), which leaves room for
	// the Go runtime and for the watchdog to act first on slow growth.
	Memory uint64
}

// WorkerEnv marks a process started as a compute worker.
const WorkerEnv = "TESSERA_COMPUTE_WORKER"

var isolation atomic.Pointer[Isolation]

// Isolate makes every later call run in a child process; nil turns it off.
func Isolate(iso *Isolation) { isolation.Store(iso) }

type workerRequest struct {
	Name, Fn string
	Src      []byte
	Args     map[string]any
	Limits   Limits
	Memory   uint64
}

type workerResponse struct {
	Out any
	Err string // the scrubbed message, without the ErrCompute prefix
}

func init() {
	for _, v := range []any{[]any{}, map[string]any{}, time.Time{}, time.Duration(0), json.Number(""), int(0), int64(0), uint64(0), float64(0), false, ""} {
		gob.Register(v)
	}
}

func (s *Script) callIsolated(ctx context.Context, iso *Isolation, fn string, args map[string]any, limits Limits) (any, error) {
	req := workerRequest{Name: s.name, Fn: fn, Src: s.src, Args: args, Limits: limits, Memory: iso.Memory}
	if req.Memory == 0 {
		req.Memory = 4 * limits.Memory
	}
	var in bytes.Buffer
	if err := gob.NewEncoder(&in).Encode(&req); err != nil {
		return nil, fmt.Errorf("%w: arguments: %v", ErrCompute, err)
	}
	command := iso.Command
	if len(command) == 0 {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("%w: no worker: %v", ErrCompute, err)
		}
		command = []string{exe}
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = append(os.Environ(), WorkerEnv+"=1")
	cmd.Stdin = &in
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &limitedWriter{w: &stderr, n: 4096}
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var resp workerResponse
	if runErr != nil || gob.NewDecoder(&out).Decode(&resp) != nil {
		// The child died. Its stderr may hold script data, so only the
		// kind of death is kept.
		if outOfMemory(stderr.String()) || killed(runErr) {
			return nil, fmt.Errorf("%w: the memory limit was reached (the script's process ended)", ErrCompute)
		}
		return nil, fmt.Errorf("%w: the script's process failed", ErrCompute)
	}
	if resp.Err != "" {
		return nil, fmt.Errorf("%w: %s", ErrCompute, resp.Err)
	}
	return resp.Out, nil
}

// outOfMemory reads the Go runtime's fatal error: "out of memory" when the
// heap cannot grow, "cannot allocate memory" when RLIMIT_DATA refuses a
// mapping (starmem-001, list-1g).
func outOfMemory(stderr string) bool {
	return strings.Contains(stderr, "out of memory") || strings.Contains(stderr, "cannot allocate memory")
}

// killed reports a death by signal, which is how the OOM killer ends a
// process.
func killed(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == -1
}

// RunWorkerIfRequested runs one isolated call and exits when this process
// was started as a compute worker; otherwise it returns at once.
func RunWorkerIfRequested() {
	if os.Getenv(WorkerEnv) != "1" {
		return
	}
	os.Exit(worker(os.Stdin, os.Stdout))
}

func worker(r io.Reader, w io.Writer) int {
	var req workerRequest
	if err := gob.NewDecoder(r).Decode(&req); err != nil {
		return 2
	}
	offerToOOMKiller()
	_ = limitData(req.Memory) // not available everywhere: then only separated
	resp := workerResponse{}
	s, err := Compile(req.Name, req.Src, req.Limits)
	if err == nil {
		if _, ok := s.Signature(req.Fn); !ok {
			err = fmt.Errorf("%w: %s has no function %q", ErrCompute, req.Name, req.Fn)
		} else {
			resp.Out, err = s.call(context.Background(), req.Fn, req.Args, req.Limits.withDefaults())
		}
	}
	if err != nil {
		resp.Err = strings.TrimPrefix(err.Error(), ErrCompute.Error()+": ")
	}
	if gob.NewEncoder(w).Encode(&resp) != nil {
		return 2
	}
	return 0
}

// limitedWriter keeps the first n bytes.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
