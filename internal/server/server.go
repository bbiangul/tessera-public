// Package server is the HTTP face of a host (tessera serve): channels post
// users' messages and poll the outbox, people answer tasks, other systems
// call webhook triggers. Every /v1 call needs the API token, except
// webhooks, which are signed.
package server

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/inspect"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

//go:embed openapi.json
var openapi []byte

// console is the web console: a client of the /v1 API, with no assets of
// its own, served at / (the page is public; its calls need the token).
//
//go:embed console.html
var console []byte

// MaxBody bounds every request body.
const MaxBody = 1 << 20

// Server serves one host.
type Server struct {
	Host *host.Host
	// Token is the bearer token of the /v1 API. The channel layer that
	// holds it is trusted to have verified who the caller is.
	Token string
	// Dev, when set, lets POST /v1/dev/clock move the host's clock.
	Dev *DevClock
	// PublicURL and LinkSecret make signed task links (notifications):
	// the base URL people reach this server at, and the signing key.
	PublicURL  string
	LinkSecret []byte

	codes   codes // one-time codes of task pages (members.go)
	once    sync.Once
	explain *inspect.Explainer
	evalMu  sync.Mutex
}

// Handler returns the routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		_, _ = w.Write(console)
	})
	mux.HandleFunc("GET /v1/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(openapi)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { reply(w, http.StatusOK, map[string]any{"ok": true}) })
	mux.HandleFunc("POST /v1/messages", s.auth(s.message))
	mux.HandleFunc("GET /v1/outbox", s.auth(s.outbox))
	mux.HandleFunc("POST /v1/runs/{id}/ack", s.auth(s.ack))
	mux.HandleFunc("POST /v1/runs/{id}/reply", s.auth(s.reply))
	mux.HandleFunc("POST /v1/runs/{id}/forward", s.auth(s.forward))
	mux.HandleFunc("POST /v1/hooks/{trigger}", s.hook) // signed, no token
	s.inspectRoutes(mux)
	s.linkRoutes(mux)
	s.activityRoutes(mux)
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || s.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
			reply(w, http.StatusUnauthorized, map[string]any{"error": "missing or wrong bearer token"})
			return
		}
		next(w, r)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		reply(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, journal.ErrNotFound), errors.Is(err, host.ErrUnknownHook):
		status = http.StatusNotFound
	case errors.Is(err, runtime.ErrNotWaiting), errors.Is(err, runtime.ErrFinished), errors.Is(err, runtime.ErrTaskChanged):
		status = http.StatusConflict
	case errors.Is(err, host.ErrBadSignature):
		status = http.StatusUnauthorized
	case errors.Is(err, runtime.ErrNotAllowed):
		status = http.StatusForbidden
	case errors.Is(err, host.ErrBadPayload):
		status = http.StatusBadRequest
	}
	msg := err.Error()
	if status == http.StatusInternalServerError {
		msg = "internal error" // details stay in the server's log
	}
	reply(w, status, map[string]any{"error": msg})
}

type messageIn struct {
	Conversation string         `json:"conversation"`
	Caller       map[string]any `json:"caller"`
	Text         string         `json:"text"`
	// MessageID is the channel's ID of the message: a retry with the same
	// ID is not handled twice.
	MessageID string `json:"messageId,omitempty"`
}

func (s *Server) message(w http.ResponseWriter, r *http.Request) {
	var in messageIn
	if !decode(w, r, &in) {
		return
	}
	if in.Conversation == "" || strings.TrimSpace(in.Text) == "" {
		reply(w, http.StatusBadRequest, map[string]any{"error": "conversation and text are required"})
		return
	}
	rep, err := s.Host.HandleOnce(r.Context(), in.Conversation, in.Caller, in.Text, in.MessageID)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, rep)
}

type outboxEntry struct {
	RunID        string            `json:"runId"`
	Conversation string            `json:"conversation"`
	Status       journal.Status    `json:"status"`
	Messages     []runtime.Message `json:"messages"`
}

func (s *Server) outbox(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			reply(w, http.StatusBadRequest, map[string]any{"error": "limit is 1 to 500"})
			return
		}
		limit = n
	}
	runs, err := s.Host.Runtime.Store.Unacked(r.Context(), r.URL.Query().Get("conversationPrefix"), limit)
	if err != nil {
		fail(w, err)
		return
	}
	out := []outboxEntry{}
	for _, run := range runs {
		msgs, err := s.Host.Runtime.Outbox(r.Context(), run.ID)
		if err != nil {
			fail(w, err)
			return
		}
		if len(msgs) > 0 {
			out = append(out, outboxEntry{RunID: run.ID, Conversation: run.Conversation, Status: run.Status, Messages: msgs})
		}
	}
	reply(w, http.StatusOK, map[string]any{"runs": out})
}

func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Seq int `json:"seq"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.Host.Runtime.Ack(r.Context(), r.PathValue("id"), in.Seq); err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) reply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From     string `json:"from"`
		Text     string `json:"text"`
		Value    any    `json:"value"`
		TaskHash string `json:"taskHash"`
		By       string `json:"by"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.From != "user" && in.From != "human" {
		reply(w, http.StatusBadRequest, map[string]any{"error": `from is "user" or "human"`})
		return
	}
	v, err := journal.Normalize(in.Value)
	if err != nil {
		reply(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	res, err := s.Host.Runtime.Deliver(r.Context(), r.PathValue("id"), runtime.Reply{From: in.From, Text: in.Text, Value: v, TaskHash: in.TaskHash, By: in.By})
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, res)
}

// forward hands the run's pending task to another queue (RFC-0006): {queue,
// by}, where by is a member of a queue the task reached.
func (s *Server) forward(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Queue string `json:"queue"`
		By    string `json:"by"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Queue == "" {
		reply(w, http.StatusBadRequest, map[string]any{"error": "queue is required"})
		return
	}
	res, err := s.Host.Runtime.Forward(r.Context(), r.PathValue("id"), in.Queue, in.By)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, res)
}

func (s *Server) hook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
	if err != nil {
		reply(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "body too large"})
		return
	}
	res, err := s.Host.Webhook(r.Context(), r.PathValue("trigger"), body, r.Header.Get("X-Tessera-Signature"),
		r.Header.Get("X-Tessera-Timestamp"), r.Header.Get("Idempotency-Key"), time.Now())
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusAccepted, map[string]any{"runId": res.RunID, "status": res.Status})
}
