package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Regression tests for the adversarial review of 2026-09-23.

func TestRedirectsAreNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		_, _ = io.ReadAll(r.Body) // would receive the customer data
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	var out any
	_, err := (&Client{sleep: noSleep}).PostJSON(context.Background(), srv.URL, map[string]string{"Authorization": "Bearer SECRET"}, map[string]string{"cpf": "123"}, &out)
	if err == nil || !IsStatus(err, http.StatusTemporaryRedirect) {
		t.Errorf("want the redirect status as an error, got %v", err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the request body was sent to the redirect target")
	}
}

func TestRetryAfterTooLongGivesUp(t *testing.T) {
	srv, n := server(t, []int{429}, "86400")
	start := time.Now()
	var out any
	_, err := (&Client{}).PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if !errors.Is(err, ErrRetryAfterTooLong) || n.Load() != 1 {
		t.Errorf("err=%v calls=%d", err, n.Load())
	}
	if time.Since(start) > time.Second {
		t.Error("must not wait")
	}
}

func TestResponseTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`"` + strings.Repeat("x", MaxResponseBytes) + `"`))
	}))
	t.Cleanup(srv.Close)
	var out any
	if _, err := (&Client{}).PostJSON(context.Background(), srv.URL, nil, 1, &out); !errors.Is(err, ErrTooLarge) {
		t.Errorf("want ErrTooLarge, got %v", err)
	}
}

func TestStatusErrorHidesTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"echo":{"cpf":"123.456.789-00"}}`))
	}))
	t.Cleanup(srv.Close)
	var out any
	_, err := (&Client{}).PostJSON(context.Background(), srv.URL, nil, 1, &out)
	if err == nil || strings.Contains(err.Error(), "123.456") {
		t.Errorf("the error must not carry the body: %v", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || !strings.Contains(se.Body, "cpf") {
		t.Error("the body stays available in StatusError.Body for local debugging")
	}
}

func TestRetryAfterParsing(t *testing.T) {
	if d := retryAfter("99999999999"); d > 24*time.Hour {
		t.Errorf("huge values must not overflow: %v", d)
	}
	if d := retryAfter(time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)); d <= 0 || d > 3*time.Second {
		t.Errorf("HTTP-date: %v", d)
	}
	if retryAfter("soon") != 0 || retryAfter("-1") != 0 {
		t.Error("garbage must be ignored")
	}
}

func TestSleepHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Error(err)
	}
}
