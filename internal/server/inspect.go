package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/evals"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/inspect"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// DevClock shifts the host's clock in development (tessera serve --dev),
// so waits, schedules and deadlines can be tested without waiting.
type DevClock struct{ offset atomic.Int64 }

// Now is the shifted time.
func (c *DevClock) Now() time.Time { return time.Now().Add(time.Duration(c.offset.Load())) }

// Advance moves the clock forward.
func (c *DevClock) Advance(d time.Duration) { c.offset.Add(int64(d)) }

func (s *Server) inspectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/agent", s.auth(func(w http.ResponseWriter, _ *http.Request) { reply(w, http.StatusOK, s.Host.Describe()) }))
	mux.HandleFunc("GET /v1/runs", s.auth(s.listRuns))
	mux.HandleFunc("GET /v1/tasks", s.auth(s.listTasks))
	mux.HandleFunc("GET /v1/runs/{id}", s.auth(s.getRun))
	mux.HandleFunc("GET /v1/conversations/{id}", s.auth(s.getConversation))
	mux.HandleFunc("POST /v1/triggers/{name}/fire", s.auth(s.fire))
	mux.HandleFunc("POST /v1/tick", s.auth(s.tick))
	mux.HandleFunc("POST /v1/dev/clock", s.auth(s.clock))
	mux.HandleFunc("POST /v1/evals/run", s.auth(s.runEvals))
	mux.HandleFunc("GET /v1/metrics/funnels/{name}", s.auth(func(w http.ResponseWriter, r *http.Request) {
		rep, err := s.Host.FunnelReport(r.Context(), r.PathValue("name"))
		if err != nil {
			fail(w, notFound(err))
			return
		}
		reply(w, http.StatusOK, rep)
	}))
	mux.HandleFunc("GET /v1/metrics/experiments/{name}", s.auth(func(w http.ResponseWriter, r *http.Request) {
		rep, err := s.Host.ExperimentReport(r.Context(), r.PathValue("name"))
		if err != nil {
			fail(w, notFound(err))
			return
		}
		reply(w, http.StatusOK, rep)
	}))
}

func (s *Server) explainer() *inspect.Explainer {
	s.once.Do(func() { s.explain = inspect.New(s.Host.Docs()) })
	return s.explain
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := journal.RunFilter{Namespace: s.Host.Namespace(), Conversation: q.Get("conversation"), Function: q.Get("function"), Status: journal.Status(q.Get("status"))}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			reply(w, http.StatusBadRequest, map[string]any{"error": "limit must be a number"})
			return
		}
		f.Limit = n
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			reply(w, http.StatusBadRequest, map[string]any{"error": "before must be RFC 3339"})
			return
		}
		f.Before = t
	}
	runs, err := s.Host.Runtime.Store.Runs(r.Context(), f)
	if err != nil {
		fail(w, err)
		return
	}
	out := []any{}
	for _, run := range runs {
		out = append(out, map[string]any{"id": run.ID, "function": run.Function, "conversation": run.Conversation, "status": run.Status,
			"pending": run.Pending, "createdAt": run.CreatedAt, "updatedAt": run.UpdatedAt, "wakeAt": run.WakeAt, "generation": run.Generation})
	}
	reply(w, http.StatusOK, map[string]any{"runs": out})
}

// listTasks is the inbox: what runs wait for from people, oldest first,
// optionally for one queue. Answer with POST /v1/runs/{id}/reply
// {"from": "human", "taskHash": ..., "by": ...}.
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	queue := r.URL.Query().Get("queue")
	tasks := []runtime.Task{}
	for _, st := range []journal.Status{journal.StatusWaitingForHuman, journal.StatusIndeterminate} {
		runs, err := s.Host.Runtime.Store.Runs(r.Context(), journal.RunFilter{Namespace: s.Host.Namespace(), Status: st, Limit: 500})
		if err != nil {
			fail(w, err)
			return
		}
		for _, run := range runs {
			t, err := s.Host.Runtime.PendingTask(r.Context(), run.ID)
			if err != nil {
				continue // moved on meanwhile
			}
			if queue == "" || t.Queue == queue {
				tasks = append(tasks, t)
			}
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Since.Before(tasks[j].Since) })
	reply(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.Host.Runtime.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	recs, err := s.Host.Runtime.Store.Records(r.Context(), run.ID)
	if err != nil {
		fail(w, err)
		return
	}
	out, err := s.explainer().Explain(run, recs)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, out)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	runs, err := s.Host.Runtime.Store.Runs(r.Context(), journal.RunFilter{Namespace: s.Host.Namespace(), Conversation: r.PathValue("id"), Limit: 500})
	if err != nil {
		fail(w, err)
		return
	}
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 { // oldest first
		runs[i], runs[j] = runs[j], runs[i]
	}
	recs := map[string][]journal.Record{}
	headers := []any{}
	for _, run := range runs {
		rs, err := s.Host.Runtime.Store.Records(r.Context(), run.ID)
		if err != nil {
			fail(w, err)
			return
		}
		recs[run.ID] = rs
		headers = append(headers, map[string]any{"id": run.ID, "function": run.Function, "status": run.Status, "createdAt": run.CreatedAt})
	}
	reply(w, http.StatusOK, map[string]any{"conversation": r.PathValue("id"), "runs": headers, "transcript": inspect.Transcript(runs, recs)})
}

func (s *Server) fire(w http.ResponseWriter, r *http.Request) {
	res, err := s.Host.Fire(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, host.ErrNotSchedule) {
			reply(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, res)
}

func (s *Server) tick(w http.ResponseWriter, r *http.Request) {
	res, err := s.Host.Tick(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"results": res})
}

func (s *Server) clock(w http.ResponseWriter, r *http.Request) {
	if s.Dev == nil {
		reply(w, http.StatusForbidden, map[string]any{"error": "the clock moves only in development: tessera serve --dev"})
		return
	}
	var in struct {
		Advance string `json:"advance"`
	}
	if !decode(w, r, &in) {
		return
	}
	d, err := time.ParseDuration(in.Advance)
	if err != nil || d <= 0 {
		reply(w, http.StatusBadRequest, map[string]any{"error": "advance is a positive Go duration, e.g. 48h"})
		return
	}
	s.Dev.Advance(d)
	res, err := s.Host.Tick(r.Context()) // what came due runs now
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"now": s.Dev.Now(), "results": res})
}

func (s *Server) runEvals(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Only               string  `json:"only"`
		MaxScenarioCostUSD float64 `json:"maxScenarioCostUSD"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.MaxScenarioCostUSD <= 0 || in.MaxScenarioCostUSD > 1 {
		in.MaxScenarioCostUSD = 0.05
	}
	s.evalMu.Lock() // one suite at a time: they share the host's models
	defer s.evalMu.Unlock()
	paths, d, g := s.Host.Engines()
	results, issues, err := evals.Run(r.Context(), evals.Options{Paths: paths, Decider: d, Generator: g, MaxScenarioCostUSD: in.MaxScenarioCostUSD, Only: in.Only})
	if err != nil {
		msgs := []string{}
		for _, is := range issues {
			msgs = append(msgs, is.String())
		}
		reply(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "issues": msgs})
		return
	}
	failed := 0
	cost := 0.0
	for _, res := range results {
		if !res.Pass {
			failed++
		}
		cost += res.Totals.CostUSD
	}
	reply(w, http.StatusOK, map[string]any{"scenarios": len(results), "failed": failed, "costUSD": cost, "results": results})
}

// notFound maps "no such funnel/experiment" to a 404.
func notFound(err error) error {
	var d *act.DefiniteError
	if errors.As(err, &d) && d.Code == "NOT_FOUND" {
		return fmt.Errorf("%w: %s", journal.ErrNotFound, d.Message)
	}
	return err
}
