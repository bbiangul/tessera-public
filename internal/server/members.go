package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	agents "github.com/bbiangul/tessera/api/agents/v1alpha1"
	"github.com/bbiangul/tessera/internal/engines/act"
	"github.com/bbiangul/tessera/internal/expr"
	"github.com/bbiangul/tessera/internal/mailmsg"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/tasklink"
)

// Answering a task whose queue has roles (RFC-0006, backoffice-001): the
// person says which member they are, the platform sends that member a
// one-time code through the Team's verify action, and only the code
// answers. A forwarded link approves nothing: the code goes to the member.

const (
	codeTTL   = 10 * time.Minute
	codeTries = 5
	// A link holder could ask for codes without end, guessing 5 times
	// each, and flood the member's phone: codes are rationed per member
	// and per task, in a sliding hour. Per task, not per link: a task has
	// several links (each notification, each member's e-mail), and each
	// must not bring a new allowance (adversarial review).
	codesPerMember = 3
	codesPerTask   = 10
	codeWindow     = time.Hour
)

type oneTimeCode struct {
	code  string
	exp   time.Time
	tries int
}

// codes are the open one-time codes, by task and member. They live in the
// server's memory: a restart only means asking for a new code.
type codes struct {
	mu     sync.Mutex
	m      map[string]*oneTimeCode
	issued map[string][]time.Time // by task+member and by task
	swept  time.Time
}

// sweep forgets expired codes and allowances, at most once a minute.
func (c *codes) sweep(now time.Time) {
	if now.Sub(c.swept) < time.Minute {
		return
	}
	c.swept = now
	for k, x := range c.m {
		if now.After(x.exp) {
			delete(c.m, k)
		}
	}
	for k, ts := range c.issued {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= codeWindow {
			delete(c.issued, k)
		}
	}
}

// allow counts a code request against the member's and the task's
// allowance, and says whether it may be sent.
func (c *codes) allow(task, member string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.issued == nil {
		c.issued = map[string][]time.Time{}
	}
	now := time.Now()
	c.sweep(now)
	link := task // the per-task allowance
	recent := func(k string) []time.Time {
		var keep []time.Time
		for _, t := range c.issued[k] {
			if now.Sub(t) < codeWindow {
				keep = append(keep, t)
			}
		}
		c.issued[k] = keep
		return keep
	}
	if len(recent(link+"|"+member)) >= codesPerMember || len(recent(link)) >= codesPerTask {
		return false
	}
	c.issued[link+"|"+member] = append(c.issued[link+"|"+member], now)
	c.issued[link] = append(c.issued[link], now)
	return true
}

func (c *codes) put(key, code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]*oneTimeCode{}
	}
	c.m[key] = &oneTimeCode{code: code, exp: time.Now().Add(codeTTL)}
}

// check consumes the code when it matches; a wrong code counts a try.
func (c *codes) check(key, code string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	x := c.m[key]
	if x == nil || time.Now().After(x.exp) || x.tries >= codeTries {
		delete(c.m, key)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(x.code), []byte(strings.TrimSpace(code))) != 1 {
		x.tries++
		return false
	}
	delete(c.m, key) // one use
	return true
}

func sixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *Server) memberPage(w http.ResponseWriter, t runtime.Task, members []agents.Member) {
	if s.Host.Runtime.TeamVerify(s.Host.Namespace()) == nil {
		page(w, http.StatusForbidden, "Answer from the console", "This queue is answered by named members, and no way to send them a code is configured (Team spec.verify).")
		return
	}
	if len(members) == 0 {
		page(w, http.StatusForbidden, "Not yours to answer", "This link's member can no longer answer this task: it went to another queue.")
		return
	}
	var b strings.Builder
	b.WriteString(`<form method="post"><input type="hidden" name="step" value="code"><p><label>Who are you? <select name="member">`)
	for _, m := range members {
		fmt.Fprintf(&b, `<option value="%s">%s</option>`, html.EscapeString(m.ID), html.EscapeString(m.Name))
	}
	b.WriteString(`</select></label></p><button>Send me a code</button></form>`)
	page(w, http.StatusOK, t.Kind+" · "+t.Queue, "<p>"+html.EscapeString(t.Text)+"</p>"+b.String())
}

func (s *Server) memberAnswer(w http.ResponseWriter, r *http.Request, c tasklink.Claims, t runtime.Task, members []agents.Member) {
	id := r.PostFormValue("member")
	var member *agents.Member
	for i := range members {
		if members[i].ID == id {
			member = &members[i]
		}
	}
	if member == nil {
		page(w, http.StatusForbidden, "Not one of this queue's members", "Only members with the queue's roles answer it.")
		return
	}
	task := c.RunID + "|" + c.Hash
	key := task + "|" + member.ID
	if r.PostFormValue("step") == "code" {
		if !s.codes.allow(task, member.ID) {
			page(w, http.StatusTooManyRequests, "Too many codes", "Wait a while before asking for another code.")
			return
		}
		code, err := sixDigits()
		if err == nil {
			err = s.sendCode(r, *member, code, t)
		}
		if err != nil {
			page(w, http.StatusBadGateway, "The code could not be sent", "Try again in a moment.")
			return
		}
		s.codes.put(key, code)
		var b strings.Builder
		fmt.Fprintf(&b, `<form method="post"><input type="hidden" name="step" value="answer"><input type="hidden" name="member" value="%s">`, html.EscapeString(member.ID))
		b.WriteString(`<p><label>The code we sent you <input name="code" inputmode="numeric" autocomplete="one-time-code" required></label></p>`)
		if t.Kind == "approve" {
			b.WriteString(`<button name="decision" value="approve">Approve</button> <button name="decision" value="deny">Deny</button>`)
		} else {
			b.WriteString(`<p><textarea name="answer" rows="4" cols="50" required></textarea></p><button>Send</button>`)
		}
		b.WriteString(`</form>`)
		page(w, http.StatusOK, "Check your messages", "<p>We sent a code to "+html.EscapeString(member.Name)+".</p>"+b.String())
		return
	}
	// The form is checked before the code: a request refused for a missing
	// decision must not use the code up.
	rep := runtime.Reply{From: "human", TaskHash: c.Hash, By: member.ID}
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
	if !s.codes.check(key, r.PostFormValue("code")) {
		page(w, http.StatusForbidden, "Wrong or expired code", "Ask for a new code from the task's link.")
		return
	}
	if _, err := s.Host.Runtime.Deliver(r.Context(), c.RunID, rep); err != nil {
		if errors.Is(err, runtime.ErrTaskChanged) || errors.Is(err, runtime.ErrNotWaiting) {
			page(w, http.StatusGone, "This task is no longer open", "Someone answered it first, or it changed.")
			return
		}
		if errors.Is(err, runtime.ErrNotAllowed) {
			page(w, http.StatusForbidden, "Not allowed", "")
			return
		}
		page(w, http.StatusInternalServerError, "Not recorded", "The code was used: ask for a new one from the task's link and try again in a moment.")
		return
	}
	page(w, http.StatusOK, "Recorded", "Thank you, "+html.EscapeString(member.Name)+": the agent continues with your answer.")
}

// sendCode calls the Team's verify action with the member and the code.
func (s *Server) sendCode(r *http.Request, m agents.Member, code string, t runtime.Task) error {
	v := s.Host.Runtime.TeamVerify(s.Host.Namespace())
	e, err := expr.New()
	if err != nil {
		return err
	}
	with, err := e.CompileValue(v.With)
	if err != nil {
		return err
	}
	in := map[string]any{"member": m.ID, "name": m.Name, "contact": m.Contact, "code": code, "task": t.Text, "raw": ""}
	if strings.Contains(m.Contact, "@") {
		// For an API that takes the whole message (Gmail): the code as an
		// RFC 822 message to the contact, built safely (mailmsg).
		if raw, err := mailmsg.Raw("", m.Contact, "Seu código: "+code, "Seu código para responder a tarefa é "+code+". Vale por 10 minutos.\n"); err == nil {
			in["raw"] = raw
		}
	}
	args, err := with.Resolve(r.Context(), expr.Activation{"inputs": in})
	if err != nil {
		return err
	}
	a, _ := args.(map[string]any)
	_, err = s.Host.Runtime.Actor.Call(r.Context(), act.Call{Action: v.Act, Args: a, IdempotencyKey: t.Hash + "/" + m.ID + "/" + code})
	return err
}
