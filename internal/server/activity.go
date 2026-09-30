package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bbiangul/tessera/internal/inspect"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// Activity is one record of any run, explained: what a live view shows.
type Activity struct {
	Cursor       int64          `json:"cursor"`
	RunID        string         `json:"runId"`
	Function     string         `json:"function"`
	Conversation string         `json:"conversation"`
	Status       journal.Status `json:"status"` // the run's status now
	Entry        inspect.Entry  `json:"entry"`
}

// StreamPoll is how often the stream looks for new records.
const StreamPoll = 500 * time.Millisecond

func (s *Server) activityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/activity", s.auth(s.activityPage))
	mux.HandleFunc("GET /v1/activity/stream", s.auth(s.activityStream))
}

// activity reads the records after cursor, explained.
func (s *Server) activity(ctx context.Context, after int64, limit int) ([]Activity, error) {
	changes, err := s.Host.Runtime.Store.Changes(ctx, s.Host.Namespace(), after, limit)
	if err != nil {
		return nil, err
	}
	runs := map[string]journal.Run{}
	out := make([]Activity, 0, len(changes))
	for _, c := range changes {
		run, ok := runs[c.RunID]
		if !ok {
			if run, err = s.Host.Runtime.Store.GetRun(ctx, c.RunID); err != nil {
				return nil, err
			}
			runs[c.RunID] = run
		}
		ex, err := s.explainer().Explain(run, []journal.Record{c.Record})
		if err != nil {
			return nil, err
		}
		out = append(out, Activity{Cursor: c.Cursor, RunID: run.ID, Function: run.Function, Conversation: run.Conversation, Status: run.Status, Entry: ex.Timeline[0]})
	}
	return out, nil
}

func cursorParam(r *http.Request) (int64, int, error) {
	q := r.URL.Query()
	after, limit := int64(0), 100
	v := q.Get("after")
	if v == "" {
		v = r.Header.Get("Last-Event-ID") // a reconnecting EventSource
	}
	if v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, fmt.Errorf("after is a cursor from an earlier answer")
		}
		after = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			return 0, 0, fmt.Errorf("limit is 1 to 500")
		}
		limit = n
	}
	return after, limit, nil
}

// activityPage answers the records after a cursor; poll it with next.
func (s *Server) activityPage(w http.ResponseWriter, r *http.Request) {
	after, limit, err := cursorParam(r)
	if err != nil {
		reply(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	items, err := s.activity(r.Context(), after, limit)
	if err != nil {
		fail(w, err)
		return
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Cursor
	}
	reply(w, http.StatusOK, map[string]any{"items": items, "next": next})
}

// activityStream sends the records as Server-Sent Events as they are
// appended (event "record", id = the cursor), with a comment every 15s so
// proxies keep the connection.
func (s *Server) activityStream(w http.ResponseWriter, r *http.Request) {
	after, _, err := cursorParam(r)
	if err != nil {
		reply(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()
	tick := time.NewTicker(StreamPoll)
	defer tick.Stop()
	quiet := time.Now()
	for {
		// The server's write timeout is for requests; a stream extends it.
		_ = rc.SetWriteDeadline(time.Now().Add(time.Minute))
		items, err := s.activity(r.Context(), after, 100)
		if err != nil {
			return
		}
		for _, it := range items {
			b, _ := json.Marshal(it)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: record\ndata: %s\n\n", it.Cursor, b); err != nil {
				return
			}
			after = it.Cursor
		}
		if len(items) > 0 {
			quiet = time.Now()
		} else if time.Since(quiet) > 15*time.Second {
			if _, err := fmt.Fprint(w, ": still here\n\n"); err != nil {
				return
			}
			quiet = time.Now()
		}
		if err := rc.Flush(); err != nil {
			return
		}
		if len(items) == 100 {
			continue // more are waiting
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}
