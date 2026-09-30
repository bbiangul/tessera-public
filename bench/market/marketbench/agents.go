package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide"
	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/ledger"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// agent is one competitor's agent for one conversation.
type agent interface {
	// Send delivers the patient's message and returns the agent's replies.
	// id is the channel's message ID: a retry has the same one. Stacks
	// without a way to use it ignore it.
	Send(ctx context.Context, text, id string) ([]string, error)
	// Crash kills and restarts the agent the way a process restart would:
	// only what the stack persists by design survives.
	Crash(ctx context.Context) error
	Close()
}

// session adds the run's session header to every tool call.
type session struct {
	id   string
	base http.RoundTripper
}

func (s session) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Bench-Session", s.id)
	return s.base.RoundTrip(r)
}

// policy is the same text for every competitor (pre-registration).
const policy = `Política:
1. Só as consultas do próprio paciente podem ser remarcadas.
2. Antes de remarcar, diga a nova data e hora e obtenha um sim explícito.
3. Nunca remarque uma consulta duas vezes para um mesmo pedido.
4. Se nenhum horário servir, diga isso. Não invente horários.
5. Para marcar uma consulta nova, diga o dia, a hora e o sinal de R$ 50 e obtenha um sim explícito. Só marque para o próprio paciente e nunca marque duas vezes para um mesmo pedido.`

func context0(t task) string {
	return fmt.Sprintf("Você é a recepção de uma clínica odontológica, no chat. Paciente logado: %s (id %s). Hoje é sexta-feira, 02/10/2026 (America/Sao_Paulo).\n%s", t.PatientName, t.PatientID, policy)
}

// ---- plain tool-calling loop ----

type plain struct {
	chat    *chatClient
	tools   *http.Client
	base    string
	system  string
	history []chatMessage
}

var plainTools = []map[string]any{
	fn("list_appointments", "Lista as consultas de um paciente.", map[string]any{"patient_id": str("id do paciente")}, "patient_id"),
	fn("find_slots", "Horários livres de um dentista a partir de uma data, por até 31 dias.", map[string]any{"dentist_id": str("id do dentista"), "from": str("data inicial, YYYY-MM-DD"), "days": map[string]any{"type": "integer", "description": "quantos dias, até 31"}}, "dentist_id", "from", "days"),
	fn("reschedule", "Remarca uma consulta para um horário (efeito irreversível).", map[string]any{"appointment_id": str("id da consulta"), "slot_id": str("id do horário")}, "appointment_id", "slot_id"),
	fn("clinic_info", "Informações da clínica: endereço, estacionamento, horário, preços, convênios, telefone.", map[string]any{}),
	fn("book_appointment", "Marca uma consulta nova para o paciente num horário livre, cobrando o sinal de R$ 50 (efeito irreversível).", map[string]any{"patient_id": str("id do paciente"), "slot_id": str("id do horário")}, "patient_id", "slot_id"),
}

func fn(name, desc string, props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{} // null is refused by some providers
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": desc,
		"parameters": map[string]any{"type": "object", "properties": props, "required": required}}}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func newPlain(c *chatClient, t task, toolsBase, sessionID string) *plain {
	p := &plain{chat: c, base: toolsBase, system: context0(t),
		tools: &http.Client{Timeout: 10 * time.Second, Transport: session{id: sessionID, base: http.DefaultTransport}}}
	p.history = []chatMessage{{Role: "system", Content: p.system}}
	return p
}

func (p *plain) Send(ctx context.Context, text, _ string) ([]string, error) {
	p.history = append(p.history, chatMessage{Role: "user", Content: text})
	for i := 0; i < 8; i++ {
		r, err := p.chat.complete(ctx, "agent", p.history, plainTools)
		if err != nil {
			return nil, err
		}
		msg := r.Message
		p.history = append(p.history, msg)
		if len(msg.ToolCalls) == 0 {
			return []string{msg.Content}, nil
		}
		for _, tc := range msg.ToolCalls {
			p.history = append(p.history, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: p.call(ctx, tc)})
		}
	}
	return []string{"(the loop stopped after 8 tool rounds)"}, nil
}

func (p *plain) call(ctx context.Context, tc toolCall) string {
	var a map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
		return "error: arguments are not JSON"
	}
	var req *http.Request
	switch tc.Function.Name {
	case "list_appointments":
		req, _ = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/patients/%v/appointments", p.base, a["patient_id"]), nil)
	case "find_slots":
		req, _ = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/dentists/%v/slots?from=%v&days=%v", p.base, a["dentist_id"], a["from"], a["days"]), nil)
	case "clinic_info":
		req, _ = http.NewRequestWithContext(ctx, "GET", p.base+"/clinic/info", nil)
	case "reschedule":
		b, _ := json.Marshal(map[string]any{"slotId": a["slot_id"]})
		req, _ = http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/appointments/%v/reschedule", p.base, a["appointment_id"]), bytes.NewReader(b))
	case "book_appointment":
		b, _ := json.Marshal(map[string]any{"slotId": a["slot_id"]})
		req, _ = http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/patients/%v/appointments", p.base, a["patient_id"]), bytes.NewReader(b))
	default:
		return "error: unknown tool"
	}
	resp, err := p.tools.Do(req)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Sprintf("error %d: %s", resp.StatusCode, data)
	}
	return string(data)
}

// Crash: the plain loop keeps the conversation in memory; a restart
// starts over.
func (p *plain) Crash(context.Context) error {
	p.history = []chatMessage{{Role: "system", Content: p.system}}
	return nil
}

func (p *plain) Close() {}

// ---- Tessera, in-process, on SQLite ----

type tesseraAgent struct {
	opts   host.Options
	dbPath string
	store  *journal.SQLite
	h      *host.Host
	conv   string
	caller map[string]any
	// escalated: a message went to a human queue (drift-004's judge).
	escalated bool
	// approvals counts repair approvals the harness gave (drift-007): the
	// simulated team member approves every repair, the worst case.
	approvals int
}

// Approvals is how many repairs the simulated team member approved.
func (a *tesseraAgent) Approvals() int { return a.approvals }

// Escalated says whether the agent handed the case to people.
func (a *tesseraAgent) Escalated() bool { return a.escalated }

type tesseraEnv struct {
	decideSpec, generateModel, orKey, jevKey string
	providers                                []string
	ledger                                   *ledger.Ledger
	defs, tlsAddr, certFile, dir             string
}

func newTessera(ctx context.Context, env tesseraEnv, t task, sessionID string) (*tesseraAgent, error) {
	pem, err := os.ReadFile(env.certFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem)
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, env.tlsAddr)
		}}
	client := &http.Client{Transport: session{id: sessionID, base: tr}}
	d := &decide.Metered{Inner: &decide.TypeSafe{APIKey: env.jevKey, Model: strings.TrimPrefix(env.decideSpec, "typesafe:")}, Ledger: env.ledger}
	g := &generate.Metered{Inner: &generate.OpenRouter{APIKey: env.orKey, Model: env.generateModel, Providers: env.providers}, Ledger: env.ledger}
	start := time.Now()
	// Friday 02/10 at 8h, before the tasks' appointments (9h that day).
	// Until authoring-002 it was 9h30: the appointments had already
	// started, which only an agent filtering by time noticed. The LLM
	// stacks get the date without a time.
	base := time.Date(2026, 10, 2, 8, 0, 0, 0, time.FixedZone("-03", -3*3600))
	a := &tesseraAgent{
		dbPath: filepath.Join(env.dir, sessionID+".db"), conv: "patient-" + t.PatientID,
		caller: map[string]any{"role": "patient", "patientId": t.PatientID, "name": t.PatientName}, // the name, as the others get it
		opts: host.Options{Paths: []string{env.defs}, Namespace: "clinica-bench", Decider: d, Generator: g, HTTPClient: client,
			AllowPrivateNetworks: true, DataDir: filepath.Join(env.dir, sessionID+"-data"),
			Configure: func(rt *runtime.Runtime) { rt.Clock = func() time.Time { return base.Add(time.Since(start)) } }},
	}
	return a, a.open()
}

func (a *tesseraAgent) open() error {
	st, err := journal.OpenSQLite(a.dbPath)
	if err != nil {
		return err
	}
	a.store = st
	a.opts.Store = st
	h, issues, err := host.Load(a.opts)
	if err != nil {
		_ = st.Close()
		return fmt.Errorf("%w %v", err, issues)
	}
	a.h = h
	return nil
}

func (a *tesseraAgent) Send(ctx context.Context, text, id string) ([]string, error) {
	r, err := a.h.HandleOnce(ctx, a.conv, a.caller, text, id)
	if err != nil {
		return nil, err
	}
	var out []string
	results := append(append([]runtime.Result{}, r.Earlier...), r.Result)
	for len(results) > 0 {
		res := results[0]
		results = results[1:]
		repair := false
		for _, m := range res.Outbox {
			if m.To == "user" {
				out = append(out, m.Text)
			}
			a.escalated = a.escalated || strings.HasPrefix(m.To, "queue:")
			repair = repair || (m.Task == "approve" && strings.Contains(m.Text, "Proposed repair"))
		}
		if n := len(res.Outbox); n > 0 && res.Outbox[n-1].Seq > 0 {
			if err := a.h.Runtime.Ack(ctx, res.RunID, res.Outbox[n-1].Seq); err != nil {
				return out, err
			}
		}
		if repair && res.Status == journal.StatusWaitingForHuman {
			// The simulated team member approves every repair.
			task, err := a.h.Runtime.PendingTask(ctx, res.RunID)
			if err != nil {
				return out, err
			}
			next, err := a.h.Runtime.Deliver(ctx, res.RunID, runtime.Reply{From: "human", Text: "yes", TaskHash: task.Hash})
			if err != nil {
				return out, err
			}
			a.approvals++
			results = append(results, next)
		}
	}
	return out, nil
}

// Crash closes the host and its database and opens them again: what the
// journal holds survives, the rest does not.
func (a *tesseraAgent) Crash(context.Context) error {
	a.Close()
	return a.open()
}

func (a *tesseraAgent) Close() {
	if a.h != nil {
		_ = a.h.Close()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}
