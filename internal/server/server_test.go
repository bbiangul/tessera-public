package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/decide/decidetest"
	"github.com/bbiangul/tessera/internal/host"
	"github.com/bbiangul/tessera/internal/runtime"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const shop = `apiVersion: agents.tessera.dev/v1alpha1
kind: Subject
metadata: { name: loja, namespace: shop }
spec: { id: "event.payload.unidade" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: hours, namespace: shop }
spec:
  description: Opening hours.
  selector: { intent: hours }
  output: { template: "Abrimos às 9h." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: obra, namespace: shop }
spec:
  description: A construction system reports a finished work.
  steps:
    - id: done
      respond: { template: "Obra da loja {{ event.payload.unidade }} concluída." }
      milestone: { name: obra-concluida, subject: loja }
  output: { template: "ok" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: obra-concluida, namespace: shop }
spec:
  function: obra
  webhook: { secret: OBRAS_HOOK }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: lembrete, namespace: shop }
spec:
  description: Reminds the team after an hour.
  steps:
    - { id: espera, wait: { for: 1h } }
    - { id: aviso, respond: { template: "Lembrete: conferir o estoque.", to: "queue:loja" } }
  output: { value: "steps.espera.output" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Trigger
metadata: { name: manha, namespace: shop }
spec:
  function: lembrete
  schedule: { cron: "0 8 * * *", timezone: America/Sao_Paulo }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: basics, namespace: shop }
spec:
  now: "2026-09-23T09:00:00-03:00"
  scenarios:
    - id: hours
      simulator: scripted
      turns:
        - user: "que horas abre?"
          expect: { routed: hours, says: ["9h"] }
    - id: wrong-on-purpose
      simulator: scripted
      turns:
        - user: "que horas abre?"
          expect: { says: ["meia-noite"] }
`

type secrets map[string]string

func (s secrets) Secret(name string) (string, error) { return s[name], nil }

func setup(t *testing.T) (*httptest.Server, journal.Store) {
	return setupWith(t, nil)
}

func setupWith(t *testing.T, dev *DevClock) (*httptest.Server, journal.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shop.yaml"), []byte(shop), 0o600); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemory()
	d := &decidetest.Scripted{Answers: map[string]map[string]float64{"route": {"hours": 0.95, "none": 0.05}}}
	opts := host.Options{Paths: []string{dir}, Namespace: "shop", Store: store, Decider: d, Secrets: secrets{"OBRAS_HOOK": "s3cret"}}
	if dev != nil {
		opts.Configure = func(rt *runtime.Runtime) { rt.Clock = dev.Now }
	}
	h, issues, err := host.Load(opts)
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ts := httptest.NewServer((&Server{Host: h, Token: "tok", Dev: dev}).Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

func call(t *testing.T, ts *httptest.Server, method, path, token string, body any, headers ...string) (int, map[string]any) {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case []byte:
		raw = b
	case nil:
	default:
		raw, _ = json.Marshal(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(raw))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// signed are the headers of a webhook delivery at ts with key.
func signed(body []byte, secret string, ts int64, key string) []string {
	h := []string{"X-Tessera-Signature", host.SignWebhook(secret, ts, key, body), "X-Tessera-Timestamp", strconv.FormatInt(ts, 10)}
	if key != "" {
		h = append(h, "Idempotency-Key", key)
	}
	return h
}

func TestAPIToken(t *testing.T) {
	ts, _ := setup(t)
	for _, tok := range []string{"", "wrong"} {
		if code, _ := call(t, ts, "POST", "/v1/messages", tok, map[string]any{"conversation": "c", "text": "oi"}); code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", tok, code)
		}
	}
	if code, _ := call(t, ts, "GET", "/health", "", nil); code != http.StatusOK {
		t.Errorf("health %d", code)
	}
}

func TestMessageAndOutbox(t *testing.T) {
	ts, _ := setup(t)
	code, out := call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c1", "text": "que horas abre?"})
	if code != http.StatusOK || out["routed"] != "hours" {
		t.Fatalf("%d %v", code, out)
	}
	// The channel delivered nothing yet: the message is in the outbox.
	_, box := call(t, ts, "GET", "/v1/outbox", "tok", nil)
	runs, _ := box["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("outbox %v", box)
	}
	run := runs[0].(map[string]any)
	msgs := run["messages"].([]any)
	seq := msgs[len(msgs)-1].(map[string]any)["seq"]
	if code, _ := call(t, ts, "POST", "/v1/runs/"+run["runId"].(string)+"/ack", "tok", map[string]any{"seq": seq}); code != http.StatusOK {
		t.Fatalf("ack %d", code)
	}
	if _, box := call(t, ts, "GET", "/v1/outbox", "tok", nil); len(box["runs"].([]any)) != 0 {
		t.Errorf("acknowledged messages must leave the outbox: %v", box)
	}
	if code, _ := call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c1", "text": "oi", "extra": 1}); code != http.StatusBadRequest {
		t.Errorf("unknown fields are refused: %d", code)
	}
}

func TestWebhook(t *testing.T) {
	ts, store := setup(t)
	body := []byte(`{"unidade": "12", "etapa": "obra"}`)
	now := time.Now().Unix()
	if code, _ := call(t, ts, "POST", "/v1/hooks/obra-concluida", "", body, signed(body, "wrong", now, "evt-1")...); code != http.StatusUnauthorized {
		t.Fatalf("a bad signature must be refused: %d", code)
	}
	var first string
	for i := 0; i < 2; i++ { // a retried delivery
		code, out := call(t, ts, "POST", "/v1/hooks/obra-concluida", "", body, signed(body, "s3cret", now, "evt-1")...)
		if code != http.StatusAccepted || out["status"] != "Succeeded" {
			t.Fatalf("delivery %d: %d %v", i, code, out)
		}
		if i == 0 {
			first = out["runId"].(string)
		} else if out["runId"] != first {
			t.Errorf("a repeated delivery started another run: %v vs %s", out["runId"], first)
		}
	}
	ms, _ := store.Milestones(context.Background(), "shop", "loja")
	if len(ms) != 1 || ms[0].ID != "12" || ms[0].Name != "obra-concluida" {
		t.Errorf("milestones %+v", ms)
	}
	_, box := call(t, ts, "GET", "/v1/outbox", "tok", nil)
	if runs := box["runs"].([]any); len(runs) != 1 {
		t.Errorf("the webhook run's message waits in the outbox: %v", box)
	}
	// A captured delivery: its signature covers the key and the time.
	replay := signed(body, "s3cret", now, "evt-1")
	replay[len(replay)-1] = "attacker-1"
	if code, _ := call(t, ts, "POST", "/v1/hooks/obra-concluida", "", body, replay...); code != http.StatusUnauthorized {
		t.Errorf("a captured delivery under a new key: %d", code)
	}
	old := now - 6*60
	if code, _ := call(t, ts, "POST", "/v1/hooks/obra-concluida", "", body, signed(body, "s3cret", old, "evt-9")...); code != http.StatusUnauthorized {
		t.Errorf("a delivery signed six minutes ago: %d", code)
	}
	for _, c := range []struct {
		path, key string
		body      []byte
		want      int
	}{
		{"/v1/hooks/nope", "evt-2", body, http.StatusUnauthorized}, // names cannot be probed
		{"/v1/hooks/obra-concluida", "evt-3", []byte(`[1,2]`), http.StatusBadRequest},
		{"/v1/hooks/obra-concluida", "evt/../x", body, http.StatusBadRequest},
		{"/v1/hooks/obra-concluida", strings.Repeat("k", 129), body, http.StatusBadRequest},
	} {
		if code, _ := call(t, ts, "POST", c.path, "", c.body, signed(c.body, "s3cret", now, c.key)...); code != c.want {
			t.Errorf("%s %s key %.10s: %d, want %d", c.path, c.body, c.key, code, c.want)
		}
	}
}

// The inspection API a coding agent uses: what the agent is, its runs,
// one run explained, a conversation's transcript.
func TestInspectionAPI(t *testing.T) {
	ts, _ := setup(t)
	_, agent := call(t, ts, "GET", "/v1/agent", "tok", nil)
	if agent["namespace"] != "shop" || len(agent["routes"].([]any)) != 1 || len(agent["triggers"].([]any)) != 2 {
		t.Fatalf("agent %v", agent)
	}
	if _, ok := agent["actions"].(map[string]any)["metrics/funnel"]; !ok {
		t.Errorf("the built-in actions are listed: %v", agent["actions"])
	}
	_, out := call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "c9", "text": "que horas abre?"})
	runID := out["runId"].(string)
	_, list := call(t, ts, "GET", "/v1/runs?conversation=c9", "tok", nil)
	if runs := list["runs"].([]any); len(runs) != 1 || runs[0].(map[string]any)["id"] != runID {
		t.Fatalf("runs %v", list)
	}
	code, run := call(t, ts, "GET", "/v1/runs/"+runID, "tok", nil)
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, run)
	}
	tl := run["timeline"].([]any)
	first := tl[0].(map[string]any)
	if first["what"] != "router" || !strings.Contains(first["summary"].(string), `routed to "hours"`) {
		t.Errorf("the timeline starts with the router's choice: %v", first)
	}
	_, conv := call(t, ts, "GET", "/v1/conversations/c9", "tok", nil)
	lines := conv["transcript"].([]any)
	if len(lines) != 2 || lines[0].(map[string]any)["text"] != "que horas abre?" || lines[1].(map[string]any)["text"] != "Abrimos às 9h." {
		t.Errorf("transcript %v", conv)
	}
	if code, _ := call(t, ts, "GET", "/v1/runs/nope", "tok", nil); code != http.StatusNotFound {
		t.Errorf("unknown run: %d", code)
	}
	if code, _ := call(t, ts, "GET", "/v1/metrics/funnels/nope", "tok", nil); code != http.StatusNotFound {
		t.Errorf("unknown funnel: %d", code)
	}
}

// Testing time without waiting: fire a schedule by hand, then move the
// development clock past the wait.
func TestFireAndDevClock(t *testing.T) {
	ts, _ := setup(t)
	if code, _ := call(t, ts, "POST", "/v1/dev/clock", "tok", map[string]any{"advance": "2h"}); code != http.StatusForbidden {
		t.Errorf("outside --dev the clock does not move: %d", code)
	}
	if code, _ := call(t, ts, "POST", "/v1/triggers/obra-concluida/fire", "tok", nil); code != http.StatusBadRequest {
		t.Errorf("a webhook trigger is not fired by hand: %d", code)
	}
	dev := &DevClock{}
	ts, _ = setupWith(t, dev)
	code, fired := call(t, ts, "POST", "/v1/triggers/manha/fire", "tok", nil)
	if code != http.StatusOK || fired["status"] != "WaitingForTime" {
		t.Fatalf("fire: %d %v", code, fired)
	}
	_, moved := call(t, ts, "POST", "/v1/dev/clock", "tok", map[string]any{"advance": "61m"})
	res, _ := moved["results"].([]any)
	// Advancing the wall clock can also cross the daily cron time. Find the
	// manually fired run; unrelated scheduled runs may legitimately be present.
	var manual map[string]any
	for _, result := range res {
		run := result.(map[string]any)
		if run["runId"] == fired["runId"] {
			manual = run
		}
	}
	if manual == nil || manual["status"] != "Succeeded" {
		t.Fatalf("after the clock moved the run wakes: %v", moved)
	}
	msgs := manual["outbox"].([]any)
	if m := msgs[len(msgs)-1].(map[string]any); m["to"] != "queue:loja" || m["text"] != "Lembrete: conferir o estoque." {
		t.Errorf("message %v", m)
	}
}

// Every route the server has is in the OpenAPI document, and nothing
// else: coding agents read it to find their way.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(openapi, &spec); err != nil {
		t.Fatal(err)
	}
	ts, _ := setup(t)
	for path, methods := range spec.Paths {
		for method := range methods {
			url := strings.NewReplacer("{id}", "x", "{name}", "x", "{trigger}", "x").Replace(path)
			req, _ := http.NewRequest(strings.ToUpper(method), ts.URL+url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusMethodNotAllowed || (resp.StatusCode == http.StatusNotFound && !strings.Contains(path, "{")) {
				t.Errorf("%s %s is documented but not served (%d)", method, path, resp.StatusCode)
			}
		}
	}
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	more, _ := os.ReadFile("inspect.go")
	links, _ := os.ReadFile("links.go")
	activity, _ := os.ReadFile("activity.go")
	for _, line := range strings.Split(string(src)+string(more)+string(links)+string(activity), "\n") {
		i := strings.Index(line, `HandleFunc("`)
		if i < 0 {
			continue
		}
		route := strings.SplitN(line[i+len(`HandleFunc("`):], `"`, 2)[0]
		method, path, ok := strings.Cut(route, " ")
		if !ok || path == "/{$}" {
			continue
		}
		if _, ok := spec.Paths[path][strings.ToLower(method)]; !ok {
			t.Errorf("%s is served but not documented in openapi.json", route)
		}
	}
}

func TestConsolePage(t *testing.T) {
	ts, _ := setup(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || resp.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("console: %d %v", resp.StatusCode, resp.Header)
	}
	if strings.Contains(string(console), "http://") || strings.Contains(string(console), "https://") {
		t.Error("the console loads nothing from outside: it works offline and leaks nothing")
	}
}

// A coding agent runs the suites through the API: an edited definition is
// tested without restarting, and real runs are untouched.
func TestEvalsAPI(t *testing.T) {
	ts, store := setup(t)
	code, out := call(t, ts, "POST", "/v1/evals/run", "tok", map[string]any{"only": "basics/hours"})
	if code != http.StatusOK || out["scenarios"] != float64(1) || out["failed"] != float64(0) {
		t.Fatalf("%d %v", code, out)
	}
	_, out = call(t, ts, "POST", "/v1/evals/run", "tok", map[string]any{})
	if out["scenarios"] != float64(2) || out["failed"] != float64(1) {
		t.Fatalf("all: %v", out)
	}
	for _, r := range out["results"].([]any) {
		m := r.(map[string]any)
		if m["scenario"] == "wrong-on-purpose" && !strings.Contains(fmt.Sprint(m["failures"]), "meia-noite") {
			t.Errorf("a failure says why: %v", m["failures"])
		}
	}
	if runs, _ := store.Runs(context.Background(), journal.RunFilter{}); len(runs) != 0 {
		t.Errorf("evals must not write to the served journal: %d runs", len(runs))
	}
}

// The inbox: a task waiting for a person, answered with its hash; the
// same answer without the hash is refused with 409.
func TestTasksAPI(t *testing.T) {
	dir := t.TempDir()
	src := `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: desconto, namespace: shop }
spec:
  description: A discount.
  selector: { intent: desconto }
  steps:
    - { id: ok, escalate: { queue: gerencia, task: approve, message: "Aprovar 10% para a mesa 4?" } }
  output: { template: "Aprovado: {{ string(steps.ok.output) }}" }
`
	if err := os.WriteFile(filepath.Join(dir, "d.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &decidetest.Scripted{Answers: map[string]map[string]float64{"route": {"desconto": 0.95, "none": 0.05}}}
	h, issues, err := host.Load(host.Options{Paths: []string{dir}, Namespace: "shop", Store: journal.NewMemory(), Decider: d})
	if err != nil {
		t.Fatalf("%v %v", err, issues)
	}
	ts := httptest.NewServer((&Server{Host: h, Token: "tok"}).Handler())
	defer ts.Close()
	_, out := call(t, ts, "POST", "/v1/messages", "tok", map[string]any{"conversation": "m4", "text": "quero desconto"})
	runID := out["runId"].(string)
	_, box := call(t, ts, "GET", "/v1/tasks?queue=gerencia", "tok", nil)
	tasks := box["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasks %v", box)
	}
	task := tasks[0].(map[string]any)
	if task["kind"] != "approve" || task["hash"] == "" || task["runId"] != runID {
		t.Fatalf("task %v", task)
	}
	if code, _ := call(t, ts, "POST", "/v1/runs/"+runID+"/reply", "tok", map[string]any{"from": "human", "value": true}); code != http.StatusConflict {
		t.Errorf("an approval without the hash: %d", code)
	}
	code, res := call(t, ts, "POST", "/v1/runs/"+runID+"/reply", "tok", map[string]any{"from": "human", "value": true, "taskHash": task["hash"], "by": "gerente"})
	if code != http.StatusOK || res["status"] != "Succeeded" {
		t.Errorf("approval: %d %v", code, res)
	}
	if _, box := call(t, ts, "GET", "/v1/tasks", "tok", nil); len(box["tasks"].([]any)) != 0 {
		t.Errorf("an answered task leaves the inbox: %v", box)
	}
}
