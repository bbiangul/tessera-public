package server

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/tasklink"
)

// LinkTTL is how long a task link works (it also stops once the task is
// answered).
const LinkTTL = 7 * 24 * time.Hour

// TaskLink is the signed page of a task, for notifications; "" without a
// public URL.
func (s *Server) TaskLink(t runtime.Task) string {
	if s.PublicURL == "" || len(s.LinkSecret) == 0 {
		return ""
	}
	tok := tasklink.Sign(s.LinkSecret, tasklink.Claims{RunID: t.RunID, Hash: t.Hash, Exp: time.Now().Add(LinkTTL).Unix()})
	return strings.TrimRight(s.PublicURL, "/") + "/tasks/" + tok
}

// MemberTaskLink is TaskLink bound to one member: the page offers only
// them, and a code goes only to them (a forwarded e-mail approves
// nothing).
func (s *Server) MemberTaskLink(t runtime.Task, member string) string {
	if s.PublicURL == "" || len(s.LinkSecret) == 0 {
		return ""
	}
	tok := tasklink.Sign(s.LinkSecret, tasklink.Claims{RunID: t.RunID, Hash: t.Hash, Exp: time.Now().Add(LinkTTL).Unix(), Member: member})
	return strings.TrimRight(s.PublicURL, "/") + "/tasks/" + tok
}

func (s *Server) linkRoutes(mux *http.ServeMux) {
	// Opening the page changes nothing (link previews in chat apps open
	// links); answering is a POST from its form.
	mux.HandleFunc("GET /tasks/{token}", s.taskPage)
	mux.HandleFunc("POST /tasks/{token}", s.answerTask)
}

func (s *Server) claims(r *http.Request) (tasklink.Claims, runtime.Task, error) {
	c, err := tasklink.Verify(s.LinkSecret, r.PathValue("token"), time.Now())
	if err != nil || len(s.LinkSecret) == 0 {
		return c, runtime.Task{}, tasklink.ErrInvalid
	}
	t, err := s.Host.Runtime.PendingTask(r.Context(), c.RunID)
	if err != nil || t.Hash != c.Hash {
		return c, t, runtime.ErrTaskChanged
	}
	return c, t, nil
}

func (s *Server) taskPage(w http.ResponseWriter, r *http.Request) {
	c, t, err := s.claims(r)
	if err != nil {
		page(w, http.StatusGone, "This task is no longer open", "It was answered already, it changed, or the link expired.")
		return
	}
	if members, open, err := s.Host.Runtime.Answerers(r.Context(), c.RunID); err == nil && (!open || c.Member != "") {
		s.memberPage(w, t, only(members, c.Member)) // the queue has roles: who answers is verified
		return
	}
	var form strings.Builder
	form.WriteString(`<form method="post"><p><label>Your name <input name="by" required></label></p>`)
	if t.Kind == "approve" {
		form.WriteString(`<button name="decision" value="approve">Approve</button> <button name="decision" value="deny">Deny</button>`)
	} else {
		form.WriteString(`<p><textarea name="answer" rows="4" cols="50" required></textarea></p><button>Send</button>`)
	}
	form.WriteString(`</form>`)
	page(w, http.StatusOK, t.Kind+" · "+t.Queue, "<p>"+html.EscapeString(t.Text)+"</p>"+form.String())
}

func (s *Server) answerTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		page(w, http.StatusBadRequest, "Invalid form", "")
		return
	}
	c, t, err := s.claims(r)
	if err != nil {
		page(w, http.StatusGone, "This task is no longer open", "It was answered already, it changed, or the link expired.")
		return
	}
	if members, open, err := s.Host.Runtime.Answerers(r.Context(), c.RunID); err == nil && (!open || c.Member != "") {
		s.memberAnswer(w, r, c, t, only(members, c.Member))
		return
	}
	by := strings.TrimSpace(r.PostFormValue("by"))
	if by == "" {
		page(w, http.StatusBadRequest, "Say who you are", "Your name is recorded with the answer.")
		return
	}
	rep := runtime.Reply{From: "human", TaskHash: c.Hash, By: "link: " + by}
	if t.Kind == "approve" {
		switch r.PostFormValue("decision") {
		case "approve":
			rep.Value, rep.Text = true, "approved"
		case "deny":
			rep.Value, rep.Text = false, "denied"
		default:
			page(w, http.StatusBadRequest, "Approve or deny", "")
			return
		}
	} else {
		rep.Text = r.PostFormValue("answer")
		var v any
		if json.Unmarshal([]byte(rep.Text), &v) == nil {
			rep.Value = v
		}
	}
	if _, err := s.Host.Runtime.Deliver(r.Context(), c.RunID, rep); err != nil {
		if errors.Is(err, runtime.ErrTaskChanged) || errors.Is(err, runtime.ErrNotWaiting) {
			page(w, http.StatusGone, "This task is no longer open", "Someone answered it first, or it changed.")
			return
		}
		page(w, http.StatusInternalServerError, "Not recorded", "Try again in a moment.")
		return
	}
	page(w, http.StatusOK, "Recorded", "Thank you: the agent continues with your answer.")
}

// only keeps the link's member, when the link names one.
func only(members []agents.Member, id string) []agents.Member {
	if id == "" {
		return members
	}
	for _, m := range members {
		if m.ID == id {
			return []agents.Member{m}
		}
	}
	return nil // no longer among those who may answer
}

func page(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>` +
		html.EscapeString(title) + `</title><style>body{font:16px/1.5 system-ui,sans-serif;max-width:560px;margin:40px auto;padding:0 16px}button{font:inherit;padding:6px 14px}</style></head><body><h1>` +
		html.EscapeString(title) + `</h1>` + body + `</body></html>`))
}
