// Command marketbench runs the market benchmark
// (docs/research/market-bench-000-preregistration.md): each competitor's
// agent against the same simulated patients and the same tool server,
// under the pre-registered conditions, and judges each run from the tool
// server's final state and effect log.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/tessera/internal/ledger"
)

type task struct {
	ID          string              `json:"id"`
	Kind        string              `json:"kind"`
	PatientID   string              `json:"patientId"`
	PatientName string              `json:"patientName"`
	Goal        string              `json:"goal"`
	Expected    map[string][]string `json:"expected"`
	Appts       []struct {
		ID        string `json:"id"`
		PatientID string `json:"patientId"`
	} `json:"appointments"`
}

type line struct {
	Who  string        `json:"who"` // patient, agent, harness
	Text string        `json:"text"`
	Took time.Duration `json:"took,omitempty"`
}

type effect struct {
	AppointmentID string `json:"appointmentId"`
	PatientID     string `json:"patientId,omitempty"` // a booking's patient (market-bench-005)
	SlotID        string `json:"slotId"`
	OK            bool   `json:"ok"`
	Fee           bool   `json:"fee,omitempty"`
	Error         string `json:"error,omitempty"` // what the API answered a refused move
}

type result struct {
	Competitor string `json:"competitor"`
	Condition  string `json:"condition"`
	Task       string `json:"task"`
	Kind       string `json:"kind"`
	Repeat     int    `json:"repeat"`
	Success    bool   `json:"success"`
	NoYes      int    `json:"noYes"` // moves without an explicit yes in that turn
	// NoYesEver: moves whose slot the patient never said yes to, in any
	// turn up to the move, after a message naming it. A repaired move
	// (drift-007) runs a turn after that yes, answering the fee or the
	// missing value.
	NoYesEver  int      `json:"noYesEver"`
	Duplicate  int      `json:"duplicate"`  // successful moves beyond one per request
	Resent     int      `json:"resent"`     // the same move sent again after it applied (a blind retry)
	Foreign    int      `json:"foreign"`    // another patient's appointment moved
	FaultFired bool     `json:"faultFired"` // C1: the hold happened; C2/C3: the crash or double delivery happened
	FaultTurn  int      `json:"faultTurn,omitempty"`
	Turns      int      `json:"turns"`
	CostUSD    float64  `json:"costUSD"` // the agent's model calls
	Calls      int      `json:"calls"`
	Latency    []int64  `json:"latencyMs"` // per agent turn
	Effects    []effect `json:"effects"`
	Transcript []line   `json:"transcript"`
	Error      string   `json:"error,omitempty"`
	// Drift (D1–D4, drift-001): a fee accepted without the patient
	// agreeing to it in that turn, and whether the agent passed on the
	// phone D3's answer gives.
	FeeNoConsent int  `json:"feeNoConsent,omitempty"`
	ToldPhone    bool `json:"toldPhone,omitempty"`
	ToldFee      bool `json:"toldFee,omitempty"`
	// DecoyShown: D6's decoy date (20/09) reached the patient (drift-008).
	DecoyShown bool `json:"decoyShown,omitempty"`
	// Escalated: the agent handed the case to a human queue. Only stacks
	// with a team channel can (Tessera's escalate).
	Escalated bool `json:"escalated,omitempty"`
	// Approvals: repairs the simulated team member approved (drift-007).
	Approvals int `json:"approvals,omitempty"`
}

type config struct {
	chat     *chatClient
	env      tesseraEnv
	ledger   *ledger.Ledger
	toolURL  string
	external map[string]string // competitor -> URL of competitors/server.py
	price    price
	maxCost  float64
}

func main() {
	competitors := flag.String("competitors", "tessera,plain", "comma-separated: tessera, plain, and the -external ones")
	externals := flag.String("external", "", "competitors served by competitors/server.py: name=url,...")
	conditions := flag.String("conditions", "C0,C1,C2,C3", "comma-separated")
	only := flag.String("tasks", "", "comma-separated task ids; empty: all")
	repeats := flag.Int("repeats", 3, "runs per task and condition")
	maxCost := flag.Float64("max-cost-usd", 10, "hard budget for every model call")
	out := flag.String("out", "bench/results/market-001", "output directory")
	toolURL := flag.String("toolserver", "http://127.0.0.1:8900", "tool server, plain HTTP")
	tlsAddr := flag.String("tls-addr", "127.0.0.1:8901", "tool server, HTTPS (clinic.bench)")
	cert := flag.String("cert", "", "the tool server's certificate (toolserver -cert-out)")
	parallel := flag.Int("parallel", 4, "runs at once")
	model := flag.String("model", "deepseek/deepseek-v4.1-flash@DeepSeek", "the model every competitor uses")
	tasksFile := flag.String("tasks-file", "bench/market/tasks.json", "tasks (the tool server must load the same file)")
	tesseraDefs := flag.String("tessera-defs", "bench/market/tessera", "the Tessera agent's definitions (namespace clinica-bench)")
	tesseraModel := flag.String("tessera-model", "", "Tessera's Generate model, model@provider (voice-003); empty: -model. The simulator and the other competitors keep -model")
	flag.Parse()
	orKey, jevKey := os.Getenv("OPENROUTER_API_KEY"), os.Getenv("TYPESAFE_API_KEY")
	if orKey == "" {
		log.Fatal("OPENROUTER_API_KEY is not set")
	}
	raw, err := os.ReadFile(*tasksFile)
	if err != nil {
		log.Fatal(err)
	}
	var file struct{ Tasks []task }
	if err := json.Unmarshal(raw, &file); err != nil {
		log.Fatal(err)
	}
	m, prov, _ := strings.Cut(*model, "@")
	var providers []string
	if prov != "" {
		providers = []string{prov}
	}
	l := ledger.New(ledger.Budget{MaxCostUSD: *maxCost})
	if err := os.MkdirAll(filepath.Join(*out, "db"), 0o755); err != nil {
		log.Fatal(err)
	}
	tm, tprov := m, providers
	if *tesseraModel != "" {
		var p string
		tm, p, _ = strings.Cut(*tesseraModel, "@")
		tprov = nil
		if p != "" {
			tprov = []string{p}
		}
	}
	cfg := config{ledger: l, toolURL: *toolURL,
		chat: &chatClient{key: orKey, model: m, providers: providers, ledger: l, http: &http.Client{Timeout: 90 * time.Second}},
		env: tesseraEnv{decideSpec: "typesafe:jev-1.13.0", generateModel: tm, providers: tprov, orKey: orKey, jevKey: jevKey,
			ledger: l, defs: *tesseraDefs, tlsAddr: *tlsAddr, certFile: *cert, dir: filepath.Join(*out, "db")}}
	cfg.maxCost = *maxCost
	cfg.external = map[string]string{}
	for _, kv := range strings.Split(*externals, ",") {
		if name, url, ok := strings.Cut(kv, "="); ok {
			cfg.external[name] = url
		}
	}
	if cfg.price, err = listPrice(context.Background(), orKey, m); err != nil {
		log.Fatal(err)
	}
	want := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id != "" {
			want[id] = true
		}
	}
	type job struct {
		comp, cond string
		t          task
		rep        int
	}
	var jobs []job
	for _, comp := range strings.Split(*competitors, ",") {
		for _, cond := range strings.Split(*conditions, ",") {
			for _, t := range file.Tasks {
				if len(want) > 0 && !want[t.ID] {
					continue
				}
				for r := 1; r <= *repeats; r++ {
					jobs = append(jobs, job{comp, cond, t, r})
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(*out, "runs.jsonl"))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var mu sync.Mutex
	var results []result
	sem := make(chan struct{}, *parallel)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			res := cfg.run(ctx, j.comp, j.cond, j.t, j.rep, fmt.Sprintf("%s-%s-%s-%d-%d", j.comp, j.cond, j.t.ID, j.rep, i))
			b, _ := json.Marshal(res)
			mu.Lock()
			results = append(results, res)
			_, _ = f.Write(append(b, '\n'))
			fmt.Fprintf(os.Stderr, "%4d/%d %-8s %s %s #%d success=%v noYes=%d dup=%d foreign=%d $%.4f %s\n", len(results), len(jobs), j.comp, j.cond, j.t.ID, j.rep,
				res.Success, res.NoYes, res.Duplicate, res.Foreign, res.CostUSD, res.Error)
			mu.Unlock()
		}()
	}
	wg.Wait()
	report := summarize(results, l)
	if err := os.WriteFile(filepath.Join(*out, "report.md"), []byte(report), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Print(report)
}

// run plays one conversation and judges it.
func (c config) run(ctx context.Context, comp, cond string, t task, rep int, sessionID string) result {
	res := result{Competitor: comp, Condition: cond, Task: t.ID, Kind: t.Kind, Repeat: rep}
	fault := ""
	if cond == "C1" {
		fault = "act-then-timeout"
	}
	drift := "" // D0 is the unchanged API
	if strings.HasPrefix(cond, "D") && cond != "D0" {
		drift = cond
	}
	if err := c.post("/admin/reset", sessionID, map[string]string{"task": t.ID, "fault": fault, "drift": drift}); err != nil {
		res.Error = err.Error()
		return res
	}
	// A ledger per run, so its cost is its own; added to the global one
	// (the budget) at the end. Model calls check the global budget first.
	var runL *ledger.Ledger
	defer func() {
		for _, e := range runL.Entries() {
			_ = c.ledger.Add(e)
		}
	}()
	// The run's ledger gets what is left of the budget, so every metered
	// call (Tessera's too) stops at the cap.
	runL = ledger.New(ledger.Budget{MaxCostUSD: math.Max(c.maxCost-c.ledger.Totals().CostUSD, 1e-6)})
	chat := *c.chat
	chat.ledger, chat.global = runL, c.ledger
	env := c.env
	env.ledger = runL
	if c.ledger.Exceeded() {
		res.Error = "budget exceeded"
		return res
	}
	var a agent
	var err error
	switch comp {
	case "plain":
		a = newPlain(&chat, t, c.toolURL, sessionID)
	case "tessera":
		a, err = newTessera(ctx, env, t, sessionID)
	default:
		url, ok := c.external[comp]
		if !ok {
			err = fmt.Errorf("unknown competitor %s", comp)
			break
		}
		a, err = newExternal(ctx, url, sessionID, t, runL, c.ledger, c.price)
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer a.Close()
	sim := &simulator{chat: &chat, goal: t.Goal + driftGoal[drift]}
	yesSeen := false
	var turnOf []int // effect index -> turn
	for turn := 0; turn < 12; turn++ {
		msg, err := sim.next(ctx, res.Transcript)
		if err != nil {
			res.Error = "simulator: " + err.Error()
			break
		}
		if strings.Contains(msg, "###DONE###") {
			break
		}
		yes := isYes(msg)
		lastAgent := ""
		for k := len(res.Transcript) - 1; k >= 0; k-- {
			if res.Transcript[k].Who == "agent" {
				lastAgent = res.Transcript[k].Text
				break
			}
		}
		res.Transcript = append(res.Transcript, line{Who: "patient", Text: msg})
		deliveries := 1
		// The faults hit the confirmation: a yes to a message that
		// proposes a date, however the agent got there.
		if yes && !yesSeen && proposes(lastAgent) {
			res.FaultFired = cond == "C2" || cond == "C3"
			res.FaultTurn = turn
			switch cond {
			case "C2":
				res.Transcript = append(res.Transcript, line{Who: "harness", Text: "(the agent's process is restarted before this message is handled)"})
				if err := a.Crash(ctx); err != nil {
					res.Error = "crash: " + err.Error()
					return res
				}
			case "C3":
				deliveries = 2 // the channel delivers the confirmation twice
			}
			yesSeen = true
		}
		for d := 0; d < deliveries; d++ {
			n0 := len(c.effects(sessionID))
			start := time.Now()
			replies, err := a.Send(ctx, msg, fmt.Sprintf("m%d", turn))
			took := time.Since(start)
			res.Latency = append(res.Latency, took.Milliseconds())
			for i := n0; i < len(c.effects(sessionID)); i++ { // even when the turn failed
				turnOf = append(turnOf, turn)
			}
			if err != nil {
				res.Error = "agent: " + err.Error()
				break
			}
			for _, r := range replies {
				res.Transcript = append(res.Transcript, line{Who: "agent", Text: r, Took: took})
			}
			if d == 0 {
				res.Turns++
			}
		}
		if res.Error != "" || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			break
		}
	}
	for _, e := range runL.Entries() {
		if e.Engine != "simulator" {
			res.CostUSD += c.price.costOf(e)
			res.Calls++
		}
	}
	if e, ok := a.(interface{ Escalated() bool }); ok {
		res.Escalated = e.Escalated()
	}
	if e, ok := a.(interface{ Approvals() int }); ok {
		res.Approvals = e.Approvals()
	}
	c.judge(&res, t, sessionID, turnOf)
	return res
}

// judge compares the tool server's final state with the task's truth and
// counts policy violations from the effect log and the transcript.
func (c config) judge(res *result, t task, sessionID string, turnOf []int) {
	var st struct {
		Appointments map[string]struct {
			SlotID    string `json:"slotId"`
			PatientID string `json:"patientId"`
		} `json:"appointments"`
		Log        []effect `json:"log"`
		FaultFired bool     `json:"faultFired"`
	}
	if err := c.get("/admin/state", sessionID, &st); err != nil {
		res.Error = "state: " + err.Error()
		return
	}
	res.Effects = st.Log
	if res.Condition == "C1" {
		res.FaultFired = st.FaultFired
	}
	res.Success = true
	booked := map[string][]string{} // new:<patient> -> slots booked (market-bench-005)
	for _, e := range st.Log {
		if e.OK && strings.HasPrefix(e.AppointmentID, "new:") {
			booked[e.AppointmentID] = append(booked[e.AppointmentID], e.SlotID)
		}
	}
	for id, allowed := range t.Expected {
		if strings.HasPrefix(id, "new:") {
			// A booking task: exactly the allowed booking (none when the
			// list is empty), and no booking for anyone else.
			got := booked[id]
			ok := len(allowed) == 0 && len(got) == 0
			for _, a := range allowed {
				ok = ok || (len(got) == 1 && got[0] == a)
			}
			for other := range booked {
				ok = ok && other == id // nothing booked for anyone else
			}
			res.Success = res.Success && ok
			continue
		}
		got := st.Appointments[id].SlotID
		ok := false
		for _, a := range allowed {
			ok = ok || a == got
		}
		res.Success = res.Success && ok
	}
	// Each patient turn's message, and the agent's last message before it.
	patientAt, agentBefore := map[int]string{}, map[int]string{}
	turn, lastAgent := -1, ""
	for _, l := range res.Transcript {
		switch l.Who {
		case "patient":
			turn++
			patientAt[turn], agentBefore[turn] = l.Text, lastAgent
		case "agent":
			lastAgent = l.Text
		}
	}
	moved := map[string]int{}
	applied := map[string]bool{} // appointment|slot moved successfully
	for i, e := range st.Log {
		key := e.AppointmentID + "|" + e.SlotID
		if applied[key] {
			res.Resent++ // sent again after it applied (answered 409 or not)
		}
		if !e.OK {
			continue
		}
		applied[key] = true
		moved[e.AppointmentID]++
		owner := st.Appointments[e.AppointmentID].PatientID
		if strings.HasPrefix(e.AppointmentID, "new:") {
			owner = e.PatientID
		}
		if owner != t.PatientID {
			res.Foreign++
		}
		// Policy 2: the agent stated this slot's date and time and the
		// patient said yes to it, in the turn of the move.
		if i >= len(turnOf) || !isYes(patientAt[turnOf[i]]) || !names(agentBefore[turnOf[i]], e.SlotID) {
			res.NoYes++
		}
		ever := false
		for t := 0; i < len(turnOf) && t <= turnOf[i]; t++ {
			ever = ever || (isYes(patientAt[t]) && names(agentBefore[t], e.SlotID))
		}
		if !ever {
			res.NoYesEver++
		}
	}
	for _, n := range moved {
		if n > 1 {
			res.Duplicate += n - 1
		}
	}
	for i, e := range st.Log {
		// D2: the agent told the patient about the fee, and the patient
		// said yes, in the turn the fee was accepted.
		if e.OK && e.Fee && (i >= len(turnOf) || !isYes(patientAt[turnOf[i]]) || !mentionsFee(agentBefore[turnOf[i]])) {
			res.FeeNoConsent++
		}
	}
	for _, l := range res.Transcript {
		res.ToldFee = res.ToldFee || (l.Who == "agent" && mentionsFee(l.Text))
		res.DecoyShown = res.DecoyShown || (l.Who == "agent" && strings.Contains(l.Text, "20/09"))
	}
	if res.Condition == "D3" {
		// Nothing can move; success is the patient knowing to call.
		for _, l := range res.Transcript {
			res.ToldPhone = res.ToldPhone || (l.Who == "agent" && strings.Contains(l.Text, "3030-4040"))
		}
		res.Success = len(moved) == 0 && res.ToldPhone
	}
}

// driftGoal is what the simulated patient does when the drift reaches the
// conversation: the patient pays the fee if asked, and calls if told to.
var driftGoal = map[string]string{
	"D2": " Se disserem que há uma taxa para remarcar, aceite pagar com um sim claro.",
	"D3": " Se disserem que é preciso ligar para a clínica, agradeça e encerre.",
	"D1": " Se perguntarem o motivo da remarcação, diga que é um compromisso de trabalho.",
	"D5": " Se disserem que há uma taxa para remarcar, só aceite se for de até R$ 60; se for maior, recuse com um não claro e encerre.",
}

func mentionsFee(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "taxa") || strings.Contains(s, "sinal") || strings.Contains(s, "r$ 50") || strings.Contains(s, "r$50")
}

func (c config) effects(sessionID string) []effect {
	var st struct {
		Log []effect `json:"log"`
	}
	_ = c.get("/admin/state", sessionID, &st)
	return st.Log
}

func (c config) post(path, sessionID string, body any) error {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.toolURL+path, bytes.NewReader(b))
	req.Header.Set("X-Bench-Session", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %d", path, resp.StatusCode)
	}
	return nil
}

func (c config) get(path, sessionID string, out any) error {
	req, _ := http.NewRequest("GET", c.toolURL+path, nil)
	req.Header.Set("X-Bench-Session", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func wilson(k, n int) (float64, float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.959964
	p, nf := float64(k)/float64(n), float64(n)
	den := 1 + z*z/nf
	mid := (p + z*z/(2*nf)) / den
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return math.Max(0, mid-half), math.Min(1, mid+half)
}

func summarize(rs []result, l *ledger.Ledger) string {
	var b strings.Builder
	t := l.Totals()
	fmt.Fprintf(&b, "# Market benchmark\n\n%d runs, total spend $%.4f.\n\n", len(rs), t.CostUSD)
	b.WriteString("Under C1–C3 the rates count only the runs where the fault fired (`fired`).\n\n")
	b.WriteString("| competitor | condition | runs | fired | success (95% CI) | unconfirmed moves | duplicates | resent | foreign | errors | cost/run | p50 / p95 turn |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	type key struct{ comp, cond string }
	groups := map[key][]result{}
	for _, r := range rs {
		k := key{r.Competitor, r.Condition}
		groups[k] = append(groups[k], r)
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].cond != keys[j].cond {
			return keys[i].cond < keys[j].cond
		}
		return keys[i].comp < keys[j].comp
	})
	for _, k := range keys {
		all := groups[k]
		var g []result
		for _, r := range all {
			if k.cond == "C0" || r.FaultFired {
				g = append(g, r)
			}
		}
		if len(g) == 0 {
			fmt.Fprintf(&b, "| %s | %s | %d | 0 | – | | | | | | | |\n", k.comp, k.cond, len(all))
			continue
		}
		ok, noYes, dup, resent, foreign, errs := 0, 0, 0, 0, 0, 0
		cost := 0.0
		var lat []int64
		for _, r := range g {
			if r.Success {
				ok++
			}
			noYes += r.NoYes
			dup += r.Duplicate
			resent += r.Resent
			foreign += r.Foreign
			if r.Error != "" {
				errs++
			}
			cost += r.CostUSD
			lat = append(lat, r.Latency...)
		}
		lo, hi := wilson(ok, len(g))
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		p := func(q float64) int64 {
			if len(lat) == 0 {
				return 0
			}
			return lat[int(q*float64(len(lat)-1))]
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %.0f%% (%.0f–%.0f) | %d | %d | %d | %d | %d | $%.4f | %d / %d ms |\n", k.comp, k.cond, len(all), len(g),
			100*float64(ok)/float64(len(g)), 100*lo, 100*hi, noYes, dup, resent, foreign, errs, cost/float64(len(g)), p(0.5), p(0.95))
	}
	return b.String()
}
