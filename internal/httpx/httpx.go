// Package httpx is a small JSON-over-HTTP client with bounded retries.
//
// Only transient statuses are retried (429, 502, 503, 504, 529) and network
// errors where the request was never sent (DNS, dial). A request that was
// sent and got no answer (a timeout, a connection lost) is not repeated:
// model providers bill a processed request even when the answer is lost,
// and OpenRouter takes no idempotency key (#7). Retries honour Retry-After
// when present and otherwise back off exponentially with full jitter.
//
// Every call reports how many of its attempts may have been billed without
// a usage report (Stats.MaybeBilled), so the ledger can charge them.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Stats describe a call's attempts.
type Stats struct {
	Attempts int
	// MaybeBilled counts the attempts that may have been processed and
	// billed by the provider without a usage report: sent and not answered,
	// answered 408/500/502/504, or answered 2xx with a body that could not
	// be read or decoded. Refused requests (429, 503, 529, other 4xx) and
	// requests never sent are not counted.
	MaybeBilled int
}

// Error is a failed call with its Stats.
type Error struct {
	Err   error
	Stats Stats
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// MaybeBilled is how many attempts of a failed call may have been billed,
// and whether err came from this package at all.
func MaybeBilled(err error) (int, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Stats.MaybeBilled, true
	}
	return 0, false
}

// NotSent says whether err happened before the request left: a DNS failure
// or a failed dial. Such a request cannot have been processed.
func NotSent(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// maybeProcessed says whether a status means the provider may have done the
// work: a gateway or server failure, or its own timeout.
func maybeProcessed(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// Client posts JSON and decodes JSON.
type Client struct {
	HTTP        *http.Client
	MaxAttempts int           // total attempts, including the first; default 3
	BaseBackoff time.Duration // default 500ms
	MaxBackoff  time.Duration // default 8s
	// sleep is replaceable in tests.
	sleep func(context.Context, time.Duration) error
}

// StatusError is returned for a non-2xx response that was not retried or
// exhausted its retries. Error() never includes the body: providers often
// echo the request back, and the request carries customer data. The body is
// kept in Body for local debugging only.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("http %d %s", e.Status, http.StatusText(e.Status))
}

// ErrTooLarge is returned when a response body exceeds MaxResponseBytes.
var ErrTooLarge = errors.New("response too large")

// ErrRetryAfterTooLong is returned when a server asks to wait longer than
// MaxBackoff; waiting would stall the caller for no good reason.
var ErrRetryAfterTooLong = errors.New("server asked to retry later than the backoff limit")

// MaxResponseBytes bounds a response body.
const MaxResponseBytes = 16 << 20

func (c *Client) defaults() (int, time.Duration, time.Duration, *http.Client) {
	attempts, base, maxB, hc := c.MaxAttempts, c.BaseBackoff, c.MaxBackoff, c.HTTP
	if attempts <= 0 {
		attempts = 3
	}
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if maxB <= 0 {
		maxB = 8 * time.Second
	}
	if hc == nil {
		hc = &http.Client{
			Timeout: 60 * time.Second,
			// Model APIs never need redirects, and following one could send
			// the request body (customer data) or credentials elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return attempts, base, maxB, hc
}

func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout, 529:
		return true
	}
	return false
}

// PostJSON sends body as JSON to url with the given headers and decodes the
// response into out. A failure is an *Error carrying the Stats; on success
// the Stats are returned too, since earlier attempts may have been billed.
func (c *Client) PostJSON(ctx context.Context, url string, headers map[string]string, body, out any) (Stats, error) {
	var st Stats
	fail := func(err error) (Stats, error) { return st, &Error{Err: err, Stats: st} }
	payload, err := json.Marshal(body)
	if err != nil {
		return fail(fmt.Errorf("marshal request: %w", err))
	}
	attempts, base, maxB, hc := c.defaults()
	sleep := c.sleep
	if sleep == nil {
		sleep = sleepCtx
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		st.Attempts = attempt
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return fail(err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := hc.Do(req)
		var wait time.Duration
		switch {
		case err != nil:
			if !NotSent(err) {
				// Sent, and no answer: it may have been processed and
				// billed. Repeating it could bill it twice (#7).
				st.MaybeBilled++
				if ctx.Err() != nil {
					return fail(ctx.Err())
				}
				return fail(err)
			}
			if ctx.Err() != nil {
				return fail(ctx.Err())
			}
			lastErr = err
		default:
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
			resp.Body.Close()
			if readErr != nil {
				if resp.StatusCode/100 == 2 {
					st.MaybeBilled++ // answered, and the answer was lost
					return fail(readErr)
				}
				lastErr = readErr
				break
			}
			if len(data) > MaxResponseBytes {
				if resp.StatusCode/100 == 2 {
					st.MaybeBilled++
				}
				return fail(fmt.Errorf("%w: over %d bytes", ErrTooLarge, MaxResponseBytes))
			}
			if resp.StatusCode/100 == 2 {
				if err := json.Unmarshal(data, out); err != nil {
					st.MaybeBilled++
					return fail(fmt.Errorf("decode response: %w", err))
				}
				return st, nil
			}
			if maybeProcessed(resp.StatusCode) {
				st.MaybeBilled++
			}
			lastErr = &StatusError{Status: resp.StatusCode, Body: truncate(string(data), 500)}
			if !retryable(resp.StatusCode) {
				return fail(lastErr)
			}
			wait = retryAfter(resp.Header.Get("Retry-After"))
			if wait > maxB {
				return fail(fmt.Errorf("%w (%v): %w", ErrRetryAfterTooLong, wait, lastErr))
			}
		}
		if attempt == attempts {
			break
		}
		if wait == 0 {
			wait = backoff(base, maxB, attempt)
		}
		if err := sleep(ctx, wait); err != nil {
			return fail(err)
		}
	}
	return fail(fmt.Errorf("after %d attempts: %w", attempts, lastErr))
}

// backoff is exponential with full jitter: uniform in [0, min(max, base*2^(n-1))].
func backoff(base, maxB time.Duration, attempt int) time.Duration {
	d := base << (attempt - 1)
	if d <= 0 || d > maxB {
		d = maxB
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}

func retryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		if secs > 24*3600 {
			secs = 24 * 3600 // avoid overflow; the caller rejects waits this long anyway
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// IsStatus reports whether err is a StatusError with the given status.
func IsStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == status
}
