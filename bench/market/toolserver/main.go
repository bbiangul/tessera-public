// Command toolserver is the neutral clinic API every competitor of the
// market benchmark calls (docs/research/market-bench-000-preregistration.md):
// in-memory state loaded from a task, an effect log, and fault injection.
// It has no idempotency and no authorization, like many real systems: the
// agent is responsible for the policy.
//
// Drift (RFC-0005, drift-001): a session may start with the API changed
// under the agents, with no change to any agent's definitions:
//   - D1: reschedule requires a new field, reason;
//   - D2: a rescheduling fee, accepted with acceptFee: true;
//   - D3: online rescheduling is suspended, patients must call;
//   - D4: the appointment list renames start and label;
//   - D5: like D2, but the fee is R$ 500 (drift-007: the patient refuses it);
//   - D6: like D4, plus decoy fields with another date, createdAt and
//     createdDescription (drift-008: a careless rename shows the wrong
//     date).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type slot struct {
	ID        string `json:"id"`
	DentistID string `json:"dentistId"`
	Start     string `json:"start"`
	Label     string `json:"label"`
}

type appointment struct {
	ID        string `json:"id"`
	PatientID string `json:"patientId"`
	DentistID string `json:"dentistId"`
	SlotID    string `json:"slotId"`
	Start     string `json:"start"`
	Label     string `json:"label,omitempty"`
}

type task struct {
	ID           string        `json:"id"`
	Appointments []appointment `json:"appointments"`
	Slots        []slot        `json:"slots"`
}

type effect struct {
	At            time.Time `json:"at"`
	AppointmentID string    `json:"appointmentId"`
	SlotID        string    `json:"slotId"`
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
	Fee           bool      `json:"fee,omitempty"`       // D2: the fee was accepted; booking: the deposit charged
	PatientID     string    `json:"patientId,omitempty"` // booking: for whom
}

// The drift's phone and fee, which only the API's answers carry.
const (
	driftPhone = "(11) 3030-4040"
	driftFee   = "R$ 50"
)

// session is one run's world: a task's state, its effect log, its fault.
type session struct {
	appts     map[string]*appointment
	slots     map[string]slot
	taken     map[string]bool
	log       []effect
	fault     string
	faultUsed bool
	drift     string
	booked    int
}

// server holds a session per X-Bench-Session header, so runs go in
// parallel without seeing each other.
type server struct {
	mu       sync.Mutex
	tasks    map[string]task
	sessions map[string]*session
	hold     time.Duration
}

// sess is the request's session; nil (404) when it was never reset.
func (s *server) sess(w http.ResponseWriter, r *http.Request) *session {
	x := s.sessions[r.Header.Get("X-Bench-Session")]
	if x == nil {
		http.Error(w, "unknown session: POST /admin/reset first", http.StatusNotFound)
	}
	return x
}

func (s *server) reset(w http.ResponseWriter, r *http.Request) {
	var in struct{ Task, Fault, Drift string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[in.Task]
	if !ok {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	x := &session{appts: map[string]*appointment{}, slots: map[string]slot{}, taken: map[string]bool{}, fault: in.Fault, drift: in.Drift}
	for _, a := range t.Appointments {
		a := a
		x.appts[a.ID] = &a
	}
	for _, sl := range t.Slots {
		x.slots[sl.ID] = sl
	}
	s.sessions[r.Header.Get("X-Bench-Session")] = x
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.sess(w, r)
	if x == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"appointments": x.appts, "log": x.log, "faultFired": x.faultUsed})
}

func (s *server) appointments(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.sess(w, r)
	if x == nil {
		return
	}
	out := []appointment{}
	for _, a := range x.appts {
		if a.PatientID == r.PathValue("patient") {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if x.drift == "D4" || x.drift == "D6" { // the fields were renamed
		renamed := []map[string]string{}
		for _, a := range out {
			m := map[string]string{"id": a.ID, "patientId": a.PatientID, "dentistId": a.DentistID, "slotId": a.SlotID, "startsAt": a.Start, "description": a.Label}
			if x.drift == "D6" {
				m["createdAt"], m["createdDescription"] = "2026-09-20T10:00:00-03:00", "domingo 20/09 às 10h"
			}
			renamed = append(renamed, m)
		}
		writeJSON(w, http.StatusOK, renamed)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) freeSlots(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 31 {
		days = 7
	}
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		http.Error(w, "from is YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	end := start.AddDate(0, 0, days)
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.sess(w, r)
	if x == nil {
		return
	}
	out := []slot{}
	for _, sl := range x.slots {
		d, _ := time.Parse("2006-01-02", sl.Start[:10])
		if sl.DentistID == r.PathValue("dentist") && !x.taken[sl.ID] && !d.Before(start) && d.Before(end) {
			out = append(out, sl)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	writeJSON(w, http.StatusOK, out)
}

func (s *server) reschedule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SlotID    string `json:"slotId"`
		Reason    string `json:"reason"`
		AcceptFee bool   `json:"acceptFee"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	x := s.sess(w, r)
	if x == nil {
		s.mu.Unlock()
		return
	}
	a, ok := x.appts[r.PathValue("appointment")]
	e := effect{At: time.Now(), AppointmentID: r.PathValue("appointment"), SlotID: in.SlotID}
	var status int
	var body any
	sl, slotOK := x.slots[in.SlotID]
	switch {
	case x.drift == "D3":
		status, e.Error = http.StatusForbidden, "online rescheduling is suspended: patients must call the clinic at "+driftPhone+" to reschedule"
	case x.drift == "D1" && strings.TrimSpace(in.Reason) == "":
		status, e.Error = http.StatusUnprocessableEntity, "field reason is required: why the patient is rescheduling"
	case x.drift == "D2" && !in.AcceptFee:
		status, e.Error = http.StatusPaymentRequired, "a rescheduling fee of "+driftFee+" now applies: resend with acceptFee: true once the patient agrees to pay it"
	case x.drift == "D5" && !in.AcceptFee:
		status, e.Error = http.StatusPaymentRequired, "a rescheduling fee of R$ 500 now applies: resend with acceptFee: true once the patient agrees to pay it"
	case !ok:
		status, e.Error = http.StatusNotFound, "no such appointment"
	case !slotOK || x.taken[in.SlotID]:
		status, e.Error = http.StatusConflict, "slot not available"
	case sl.DentistID != a.DentistID:
		status, e.Error = http.StatusConflict, "slot is another dentist's"
	default:
		if a.SlotID != "" {
			delete(x.taken, a.SlotID)
		}
		a.SlotID, a.Start = sl.ID, sl.Start
		x.taken[sl.ID] = true
		e.Fee = x.drift == "D2" || x.drift == "D5"
		e.OK, status, body = true, http.StatusOK, *a
	}
	x.log = append(x.log, e)
	hold := e.OK && x.fault == "act-then-timeout" && !x.faultUsed
	if hold {
		x.faultUsed = true
	}
	s.mu.Unlock()
	if hold {
		time.Sleep(s.hold) // done, but the answer arrives after the client gave up
	}
	if body == nil {
		writeJSON(w, status, map[string]string{"error": e.Error})
		return
	}
	writeJSON(w, status, body)
}

// book creates an appointment in a free slot and charges the R$ 50
// deposit (market-bench-005). No idempotency, like the rest of the
// server: a retry after act-then-timeout books, and charges, twice.
func (s *server) book(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SlotID string `json:"slotId"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	x := s.sess(w, r)
	if x == nil {
		s.mu.Unlock()
		return
	}
	patient := r.PathValue("patient")
	e := effect{At: time.Now(), AppointmentID: "new:" + patient, SlotID: in.SlotID, PatientID: patient}
	var status int
	var body any
	sl, slotOK := x.slots[in.SlotID]
	switch {
	case !slotOK || x.taken[in.SlotID]:
		status, e.Error = http.StatusConflict, "slot not available"
	default:
		x.booked++
		a := &appointment{ID: "N" + strconv.Itoa(x.booked), PatientID: patient, DentistID: sl.DentistID, SlotID: sl.ID, Start: sl.Start, Label: sl.Label}
		x.appts[a.ID] = a
		x.taken[sl.ID] = true
		e.OK, e.Fee, status = true, true, http.StatusCreated
		body = map[string]any{"appointment": *a, "deposit": "R$ 50 charged"}
	}
	x.log = append(x.log, e)
	hold := e.OK && x.fault == "act-then-timeout" && !x.faultUsed
	if hold {
		x.faultUsed = true
	}
	s.mu.Unlock()
	if hold {
		time.Sleep(s.hold)
	}
	if body == nil {
		writeJSON(w, status, map[string]string{"error": e.Error})
		return
	}
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8900", "listen address")
	tasksPath := flag.String("tasks", "bench/market/tasks.json", "tasks file")
	hold := flag.Duration("hold", 25*time.Second, "how long act-then-timeout holds the answer")
	tlsAddr := flag.String("tls-addr", "127.0.0.1:8901", "HTTPS listen address (a self-signed certificate for clinic.bench)")
	certOut := flag.String("cert-out", "", "write the HTTPS certificate (PEM) here, for clients to trust")
	flag.Parse()
	raw, err := os.ReadFile(*tasksPath)
	if err != nil {
		log.Fatal(err)
	}
	var file struct {
		Tasks []task
		Facts map[string]any
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		log.Fatal(err)
	}
	s := &server{tasks: map[string]task{}, sessions: map[string]*session{}, hold: *hold}
	for _, t := range file.Tasks {
		s.tasks[t.ID] = t
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/reset", s.reset)
	mux.HandleFunc("GET /admin/state", s.state)
	mux.HandleFunc("GET /patients/{patient}/appointments", s.appointments)
	mux.HandleFunc("GET /dentists/{dentist}/slots", s.freeSlots)
	mux.HandleFunc("POST /appointments/{appointment}/reschedule", s.reschedule)
	mux.HandleFunc("POST /patients/{patient}/appointments", s.book)
	// The clinic's facts (address, parking, prices...): the same for every
	// session; empty for task files without them.
	mux.HandleFunc("GET /clinic/info", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, file.Facts) })
	cert, pemBytes, err := selfSigned("clinic.bench")
	if err != nil {
		log.Fatal(err)
	}
	if *certOut != "" {
		if err := os.WriteFile(*certOut, pemBytes, 0o600); err != nil {
			log.Fatal(err)
		}
	}
	go func() {
		srv := &http.Server{Addr: *tlsAddr, Handler: mux, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
		log.Fatal(srv.ListenAndServeTLS("", ""))
	}()
	log.Printf("toolserver on http://%s and https://%s (clinic.bench), %d tasks", *addr, *tlsAddr, len(s.tasks))
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// selfSigned makes a certificate for host, and its PEM for clients.
func selfSigned(host string) (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}
