package compute

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The test binary is its own worker, as cmd/tessera is.
func TestMain(m *testing.M) {
	RunWorkerIfRequested()
	os.Exit(m.Run())
}

func isolated(t *testing.T, iso *Isolation) {
	t.Helper()
	if iso == nil {
		iso = &Isolation{}
	}
	if len(iso.Command) == 0 {
		iso.Command = []string{os.Args[0]}
	}
	Isolate(iso)
	t.Cleanup(func() { Isolate(nil) })
}

func TestIsolatedCallsReturnWhatInProcessCallsReturn(t *testing.T) {
	src := []byte("def f(n, x, when, lit):\n" +
		"    return {'n': n + 1, 'x': x * 2, 'two': 2.0, 'big': 1 << 63, 'when': when + time.parse_duration('1h'),\n" +
		"            'd': time.parse_duration('90s'), 'list': [1, 'a', None, True], 'tuple': (1, 2), 'lit': lit}\n")
	s, err := Compile("f.star", src, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"n": 1, "x": 1.5, "when": time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC), "lit": json.Number("7")}
	want, err := s.Call(context.Background(), "f", args, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	isolated(t, nil)
	got, err := s.Call(context.Background(), "f", args, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("isolated %#v\nin-process %#v", got, want)
	}
}

func TestIsolatedErrorsAreScrubbedTheSameWay(t *testing.T) {
	s, _ := Compile("f.star", []byte("def code():\n    fail('NO_PLACE')\ndef data(x):\n    fail(x)\ndef spin():\n    while True:\n        pass\n"), Limits{})
	cases := []struct {
		fn     string
		args   map[string]any
		limits Limits
		want   string
	}{
		{"code", nil, Limits{}, "fail: NO_PLACE"},
		{"data", map[string]any{"x": "maria.silva@example.com"}, Limits{}, "withheld"},
		{"spin", nil, Limits{Steps: 1000}, "step limit"},
		{"_hidden", nil, Limits{}, "no function"},
	}
	for _, c := range cases {
		_, inProcess := s.Call(context.Background(), c.fn, c.args, c.limits)
		isolated(t, nil)
		_, err := s.Call(context.Background(), c.fn, c.args, c.limits)
		Isolate(nil)
		if err == nil || inProcess == nil || err.Error() != inProcess.Error() || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: isolated %v, in-process %v", c.fn, err, inProcess)
		}
		if err != nil && strings.Contains(err.Error(), "maria") {
			t.Errorf("%s: data leaked: %v", c.fn, err)
		}
	}
}

func TestIsolatedCallsStopWithTheirContext(t *testing.T) {
	s, _ := Compile("f.star", []byte("def spin():\n    while True:\n        pass\n"), Limits{})
	isolated(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.Call(ctx, "spin", nil, Limits{Steps: 1 << 50})
	if err != context.DeadlineExceeded {
		t.Errorf("want the context's error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("the child must be killed with the context")
	}
}

func TestSandboxesStayInProcess(t *testing.T) {
	src := []byte("def f(state, **kw):\n    return 1\n")
	box, _ := CompileSandbox("s.star", src)
	s, _ := Compile("s.star", src, Limits{})
	isolated(t, &Isolation{Command: []string{"/nonexistent/worker"}})
	if _, err := box.Call(context.Background(), "f", map[string]any{"state": map[string]any{}}, Limits{}); err != nil {
		t.Errorf("a sandbox must not need the worker: %v", err)
	}
	if _, err := s.Call(context.Background(), "f", map[string]any{"state": map[string]any{}}, Limits{}); err == nil || !strings.Contains(err.Error(), "process failed") {
		t.Errorf("a script must go through the worker: %v", err)
	}
}

// starmem-001: one instruction allocating ~3 GiB. In-process this kills a
// 2 GiB container; isolated, only the child dies. RLIMIT_DATA is Linux's.
func TestIsolationSurvivesOneHugeAllocation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the hard limit is RLIMIT_DATA, Linux only")
	}
	// 3 GiB fails as "out of memory"; 400 MiB as "cannot allocate memory".
	for _, n := range []string{"200 << 20", "25 << 20"} {
		s, _ := Compile("m.star", []byte("def f():\n    return len([0] * ("+n+"))\n"), Limits{})
		isolated(t, &Isolation{Memory: 256 << 20})
		_, err := s.Call(context.Background(), "f", nil, Limits{})
		if err == nil || !strings.Contains(err.Error(), "memory limit") {
			t.Errorf("%s: want the memory limit, got %v", n, err)
		}
	}
}
