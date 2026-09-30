package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func noSleep(context.Context, time.Duration) error { return nil }

func server(t *testing.T, statuses []int, retryAfter string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		st := statuses[min(i, len(statuses)-1)]
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(st)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestRetriesTransientThenSucceeds(t *testing.T) {
	srv, n := server(t, []int{529, 503, 200}, "")
	c := &Client{sleep: noSleep}
	var out map[string]bool
	st, err := c.PostJSON(context.Background(), srv.URL, nil, map[string]int{}, &out)
	if err != nil || !out["ok"] || st.Attempts != 3 || n.Load() != 3 {
		t.Fatalf("err=%v out=%v attempts=%d calls=%d", err, out, st.Attempts, n.Load())
	}
	if st.MaybeBilled != 0 {
		t.Errorf("529 and 503 are refusals, not billed work: %+v", st)
	}
}

func TestDoesNotRetryClientErrors(t *testing.T) {
	srv, n := server(t, []int{422}, "")
	c := &Client{sleep: noSleep}
	var out any
	_, err := c.PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if !IsStatus(err, 422) || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	srv, n := server(t, []int{429}, "")
	c := &Client{MaxAttempts: 4, sleep: noSleep}
	var out any
	_, err := c.PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if !IsStatus(err, 429) || n.Load() != 4 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
}

func TestHonoursRetryAfter(t *testing.T) {
	srv, _ := server(t, []int{429, 200}, "7")
	var waited time.Duration
	c := &Client{sleep: func(_ context.Context, d time.Duration) error { waited = d; return nil }}
	var out any
	if _, err := c.PostJSON(context.Background(), srv.URL, nil, 1, &out); err != nil {
		t.Fatal(err)
	}
	if waited != 7*time.Second {
		t.Fatalf("waited %v, want 7s", waited)
	}
}

func TestBackoffBounds(t *testing.T) {
	for attempt := 1; attempt < 10; attempt++ {
		d := backoff(100*time.Millisecond, time.Second, attempt)
		if d < 0 || d > time.Second {
			t.Fatalf("attempt %d: %v out of bounds", attempt, d)
		}
	}
}

func TestContextCancelStopsRetries(t *testing.T) {
	srv, _ := server(t, []int{503}, "")
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{sleep: func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }}
	var out any
	if _, err := c.PostJSON(ctx, srv.URL, nil, 1, &out); err == nil {
		t.Fatal("want error after cancel")
	}
}

// #7: a request that was sent and got no answer is not repeated, and it
// is reported as possibly billed; gateway failures are retried but counted.
func TestBillingAwareRetries(t *testing.T) {
	// A server that accepts the request and never answers in time.
	var calls atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(200 * time.Millisecond)
	}))
	t.Cleanup(slow.Close)
	c := &Client{sleep: noSleep, HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	var out any
	_, err := c.PostJSON(context.Background(), slow.URL, nil, 1, &out)
	if n, ok := MaybeBilled(err); err == nil || !ok || n != 1 || calls.Load() != 1 {
		t.Fatalf("timeout after send: err=%v maybeBilled=%d/%v calls=%d", err, n, ok, calls.Load())
	}

	// 502 then success: retried, and the 502 counted as possibly billed.
	srv, n := server(t, []int{502, 200}, "")
	st, err := (&Client{sleep: noSleep}).PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if err != nil || st.Attempts != 2 || st.MaybeBilled != 1 || n.Load() != 2 {
		t.Fatalf("502 then 200: %+v %v", st, err)
	}

	// A plain refusal (404) is not billed.
	srv, _ = server(t, []int{404}, "")
	_, err = (&Client{sleep: noSleep}).PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if n, ok := MaybeBilled(err); !ok || n != 0 {
		t.Fatalf("404: maybeBilled=%d/%v", n, ok)
	}

	// Never sent (nothing listens): retried, never billed.
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	_, err = (&Client{sleep: noSleep}).PostJSON(context.Background(), url, nil, 1, &out)
	if n, ok := MaybeBilled(err); !ok || n != 0 || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("not sent: maybeBilled=%d/%v err=%v", n, ok, err)
	}
}
