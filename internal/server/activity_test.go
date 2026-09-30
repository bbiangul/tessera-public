package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A coding agent follows what the agent does by polling with the cursor:
// each answer has only what is new, explained.
func TestActivityPolling(t *testing.T) {
	ts, _ := setup(t)
	_, first := call(t, ts, "GET", "/v1/activity", "tok", nil)
	if items := first["items"].([]any); len(items) != 0 {
		t.Fatalf("nothing yet: %v", first)
	}
	call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c1", "text": "que horas abre?"})
	_, page := call(t, ts, "GET", "/v1/activity", "tok", nil)
	items := page["items"].([]any)
	if len(items) == 0 {
		t.Fatal("the run's records are listed")
	}
	var kinds []string
	for _, it := range items {
		m := it.(map[string]any)
		if m["function"] != "hours" || m["conversation"] != "c1" {
			t.Errorf("item %v", m)
		}
		kinds = append(kinds, fmt.Sprint(m["entry"].(map[string]any)["kind"]))
	}
	if !strings.Contains(strings.Join(kinds, " "), "send") {
		t.Errorf("the reply is among them: %v", kinds)
	}
	next := fmt.Sprint(page["next"])
	_, again := call(t, ts, "GET", "/v1/activity?after="+next, "tok", nil)
	if len(again["items"].([]any)) != 0 {
		t.Errorf("after the cursor, nothing new: %v", again)
	}
	call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c2", "text": "que horas abre?"})
	_, later := call(t, ts, "GET", "/v1/activity?after="+next, "tok", nil)
	for _, it := range later["items"].([]any) {
		if it.(map[string]any)["conversation"] != "c2" {
			t.Errorf("only the new run's records: %v", it)
		}
	}
	if code, _ := call(t, ts, "GET", "/v1/activity?after=x", "tok", nil); code != http.StatusBadRequest {
		t.Errorf("bad cursor: %d", code)
	}
	if code, _ := call(t, ts, "GET", "/v1/activity", "", nil); code != http.StatusUnauthorized {
		t.Errorf("the stream needs the token: %d", code)
	}
}

// The stream sends records as they are written.
func TestActivityStream(t *testing.T) {
	ts, _ := setup(t)
	req, _ := http.NewRequest("GET", ts.URL+"/v1/activity/stream", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c9", "text": "que horas abre?"})
	events := make(chan map[string]any)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var m map[string]any
				_ = json.Unmarshal([]byte(data), &m)
				events <- m
			}
		}
		close(events)
	}()
	seen := map[float64]bool{}
	deadline := time.After(3 * StreamPoll)
	for done := false; !done; {
		select {
		case m := <-events:
			c, _ := m["cursor"].(float64)
			if m["conversation"] != "c9" || c == 0 {
				t.Fatalf("event %v", m)
			}
			if seen[c] {
				t.Fatalf("record %v sent twice", c)
			}
			seen[c] = true
		case <-deadline:
			done = true
		}
	}
	if len(seen) == 0 {
		t.Fatal("no event")
	}
}
