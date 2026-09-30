package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

const tripFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: trip, namespace: travel }
spec:
  description: Plans a trip.
  inputs:
    destination:
      type: string
      description: the city the user wants to fly to
      resolve:
        - { id: said, extract: {} }
        - { id: asked, ask: { text: "Para onde você quer ir?", minConfidence: 0.5, onLowConfidence: ask-user } }
    departDate:
      type: date
      description: the departure day
      resolve:
        - { id: said, extract: { instructions: "a day without a year is its next occurrence" } }
        - { id: asked, ask: { text: "Quando você quer ir?", minConfidence: 0.5, onLowConfidence: ask-user } }
    adults:
      type: int
      description: adults traveling, including the user
      resolve:
        - { id: said, extract: {} }
        - { id: default, value: "1" }
  output:
    template: "{{ inputs.destination }}, {{ inputs.departDate }}, {{ inputs.adults }} adulto(s)"
    value: "{'to': inputs.destination, 'on': inputs.departDate}"
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: caller, namespace: travel }
spec:
  description: Calls trip with a known destination.
  steps:
    - { id: plan, call: trip, with: { destination: "Porto", departDate: null } }
  output: { template: "{{ steps.plan.output.to }} em {{ steps.plan.output.on }}" }
`

// wordsGen extracts by keywords, like a careful model would, and records
// every request.
type wordsGen struct {
	calls []generate.Request
	// badDate makes the next batch extraction return an impossible date.
	badDate bool
}

func (g *wordsGen) Name() string { return "words" }
func (g *wordsGen) Generate(_ context.Context, r generate.Request) (*generate.Response, error) {
	g.calls = append(g.calls, r)
	st := r.State.(map[string]any)
	text, _ := st["message"].(string)
	if reply, ok := st["reply"].(string); ok {
		text = reply
	}
	if earlier, ok := st["earlier"].([]any); ok {
		// Like a careful model: an earlier message fills only the values
		// the instructions mark, and only what the message leaves out.
		for _, l := range earlier {
			if strings.Contains(l.(string), "2 adultos") && strings.Contains(r.Instructions, "adults (int): adults traveling, including the user. May come from EARLIER") && !strings.Contains(text, "adultos") {
				text += " 2 adultos"
			}
			if strings.Contains(l.(string), "Lisboa") && strings.Contains(r.Instructions, "destination (string): the city the user wants to fly to. May come from EARLIER") && !strings.Contains(text, "para ") {
				text += " Lisboa"
			}
		}
	}
	find := func() map[string]any {
		out := map[string]any{"destination": nil, "departDate": nil, "adults": nil}
		if strings.Contains(text, "Lisboa") {
			out["destination"] = "Lisboa"
		}
		if strings.Contains(text, "Porto") {
			out["destination"] = "Porto"
		}
		if strings.Contains(text, "12 de outubro") {
			out["departDate"] = "2026-10-12"
		}
		if strings.Contains(text, "2 adultos") {
			out["adults"] = int64(2)
		}
		return out
	}
	props := r.Schema["properties"].(map[string]any)
	if value, single := props["value"]; single {
		// Answer only what was asked: a date question gets a date.
		f := find()
		key := "destination"
		if _, isDate := value.(map[string]any)["pattern"]; isDate {
			key = "departDate"
		}
		out := map[string]any{"value": f[key]}
		if also, ok := props["also"].(map[string]any); ok {
			// The later inputs the reply also gives (a round trip's
			// return in the answer about the departure).
			found := map[string]any{}
			for k := range also["properties"].(map[string]any) {
				found[k] = f[k]
			}
			out["also"] = found
		}
		return &generate.Response{Value: out, Model: "words"}, nil
	}
	f := find()
	if g.badDate {
		f["departDate"], g.badDate = "2026-02-30", false
	}
	out := map[string]any{}
	for k := range props {
		out[k] = f[k]
	}
	return &generate.Response{Value: out, Model: "words"}, nil
}

func tripHarness(t *testing.T) (*harness, *wordsGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trip.yaml"), []byte(tripFunction), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	g := &wordsGen{}
	h.rt.Generator = g
	return h, g
}

func TestOneExtractionForAWholeRequest(t *testing.T) {
	h, g := tripHarness(t)
	res := h.startFn("trip", "Quero ir para Lisboa dia 12 de outubro com 2 adultos")
	expectStatus(t, res, journal.StatusSucceeded)
	if h.user[0] != "Lisboa, 2026-10-12, 2 adulto(s)" {
		t.Errorf("messages %q", h.user)
	}
	if len(g.calls) != 1 {
		t.Fatalf("%d Generate calls: every extract source must share one", len(g.calls))
	}
	req := g.calls[0]
	if !strings.Contains(req.Instructions, "the city the user wants to fly to") || !strings.Contains(req.Instructions, "next occurrence") {
		t.Errorf("descriptions and instructions must reach the model: %s", req.Instructions)
	}
	if got := req.Schema["required"]; !reflect.DeepEqual(got, []any{"destination", "departDate", "adults"}) { // written order
		t.Errorf("required %v", got)
	}
	if !strings.Contains(req.State.(map[string]any)["now"].(string), "2026-09-23") {
		t.Errorf("the model needs the date for relative days: %v", req.State)
	}
}

func TestAskOnlyForWhatIsMissing(t *testing.T) {
	h, g := tripHarness(t)
	res := h.startFn("trip", "quero viajar")
	expectStatus(t, res, journal.StatusWaitingForUser)
	h.reply("user", "Lisboa", nil)
	h.reply("user", "ainda não sei", nil)
	res = h.reply("user", "dia 12 de outubro", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	want := []string{"Para onde você quer ir?", "Quando você quer ir?", "Quando você quer ir?", "Lisboa, 2026-10-12, 1 adulto(s)"}
	if !reflect.DeepEqual(h.user, want) {
		t.Errorf("messages\n got %q\nwant %q", h.user, want)
	}
	if len(g.calls) != 4 { // one batch, then one per reply
		t.Errorf("%d Generate calls, want 4", len(g.calls))
	}
}

func TestImpossibleExtractedDateIsAsked(t *testing.T) {
	h, g := tripHarness(t)
	g.badDate = true
	res := h.startFn("trip", "Lisboa dia 12 de outubro")
	expectStatus(t, res, journal.StatusWaitingForUser)
	if h.user[0] != "Quando você quer ir?" {
		t.Errorf("an impossible date from the model must be treated as not said: %q", h.user)
	}
}

func TestNullArgumentsAreResolvedByTheCallee(t *testing.T) {
	h, _ := tripHarness(t)
	res := h.startFn("caller", "")
	expectStatus(t, res, journal.StatusWaitingForUser) // destination given, the date asked
	if h.user[0] != "Quando você quer ir?" {
		t.Fatalf("messages %q", h.user)
	}
	res = h.reply("user", "12 de outubro", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Porto em 2026-10-12" {
		t.Errorf("output.value must reach the caller: %q", last)
	}
}

// A free-text answer can be relative to what was said before ("fico 2
// noites" to the check-out question): the model sees the inputs already
// resolved and the input's extract instructions. Found by the hotels
// EvalSuite, where the check-out was asked forever.
func TestFreeAskSeesWhatIsKnown(t *testing.T) {
	h, g := tripHarness(t)
	res := h.startFn("trip", "quero ir para Lisboa")
	expectStatus(t, res, journal.StatusWaitingForUser)
	h.reply("user", "dia 12 de outubro", nil)
	ask := g.calls[len(g.calls)-1]
	known, _ := ask.State.(map[string]any)["known"].(map[string]any)
	if known["destination"] != "Lisboa" {
		t.Errorf("the answer to a question must be read knowing what was said: %v", ask.State)
	}
	if _, self := known["departDate"]; self {
		t.Errorf("the asked input is not known yet: %v", known)
	}
	if !strings.Contains(ask.Instructions, "next occurrence") {
		t.Errorf("the input's extract instructions apply to its answer too: %s", ask.Instructions)
	}
}

// Natal, 2026-09-28: "15 de novembro, volto dia 22" answered the departure
// question and the return was lost. A reply now also fills the later
// inputs it gives, in the same call, so they are not asked again.
func TestAReplyAlsoGivesLaterInputs(t *testing.T) {
	h, g := tripHarness(t)
	res := h.startFn("trip", "quero viajar")
	expectStatus(t, res, journal.StatusWaitingForUser)
	res = h.reply("user", "Lisboa, dia 12 de outubro, 2 adultos", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Lisboa, 2026-10-12, 2 adulto(s)" {
		t.Errorf("the date and the adults came with the destination: %q", h.user)
	}
	if len(g.calls) != 2 {
		t.Errorf("one call for the message and one for the reply, got %d", len(g.calls))
	}
	also := g.calls[1].Schema["properties"].(map[string]any)["also"].(map[string]any)["properties"].(map[string]any)
	if _, ok := also["destination"]; ok || len(also) != 2 {
		t.Errorf("also asks the later inputs only, got %v", also)
	}
}

// What the first message said is not overwritten by a reply.
func TestAReplyDoesNotOverrideTheMessage(t *testing.T) {
	h, g := tripHarness(t)
	res := h.startFn("trip", "quero ir dia 12 de outubro")
	expectStatus(t, res, journal.StatusWaitingForUser)
	res = h.reply("user", "Lisboa", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Lisboa, 2026-10-12, 1 adulto(s)" {
		t.Errorf("messages %q", h.user)
	}
	if also := g.calls[1].Schema["properties"].(map[string]any)["also"].(map[string]any)["properties"].(map[string]any); len(also) != 1 {
		t.Errorf("the date the message gave is not asked again: %v", also)
	}
}

// agency-001: a list of travelers ({name, birthDate}) got the schema
// {"type": null} for its items, the model's answer never validated, and
// the booking failed at "nome e nascimento".
func TestListSchemas(t *testing.T) {
	for typ, want := range map[string]any{"list<map>": "object", "list<int>": "integer", "list<string>": "string"} {
		items := typeSchema(typ)["items"].(map[string]any)
		if items["type"] != want {
			t.Errorf("%s: items type %v, want %v", typ, items["type"], want)
		}
	}
	if items := typeSchema("list<any>")["items"].(map[string]any); len(items) != 0 {
		t.Errorf("list<any> items are any value: %v", items)
	}
}

const contactFunction = `apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: contact, namespace: travel }
spec:
  description: Takes a booking's e-mail.
  inputs:
    destination:
      type: string
      description: the city the user wants to fly to
      resolve:
        - { id: said, extract: {} }
        - { id: asked, ask: { text: "Para onde você quer ir?", minConfidence: 0.5, onLowConfidence: ask-user } }
    email:
      type: string
      format: email
      description: the e-mail for the booking
      resolve:
        - { id: said, extract: {} }
        - { id: asked, ask: { text: "Qual e-mail?", minConfidence: 0.5, onLowConfidence: ask-user } }
  output:
    template: "{{ inputs.destination }} → {{ inputs.email }}"
`

func contactHarness(t *testing.T) (*harness, *wordsGen) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact.yaml"), []byte(contactFunction), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	g := &wordsGen{}
	h.rt.Generator = g
	return h, g
}

// agency-001: the e-mail asked for a booking reached the model masked,
// and "[EMAIL]" became the booking's e-mail. An input with a format is read
// by pattern from the raw text, and no model sees it.
func TestFormatReadsPersonalDataByCode(t *testing.T) {
	h, g := contactHarness(t)
	res := h.startFn("contact", "Quero ir para Lisboa, meu e-mail é ana.souza@example.com")
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Lisboa → ana.souza@example.com" {
		t.Errorf("messages %q", h.user)
	}
	for _, c := range g.calls {
		if _, asked := c.Schema["properties"].(map[string]any)["email"]; asked {
			t.Error("the model was asked for the e-mail")
		}
	}
}

func TestFormatAskedIsReadByCode(t *testing.T) {
	h, g := contactHarness(t)
	res := h.startFn("contact", "Quero ir para Lisboa")
	expectStatus(t, res, journal.StatusWaitingForUser)
	calls := len(g.calls)
	res = h.reply("user", "é esse: ana.souza@example.com", nil)
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Lisboa → ana.souza@example.com" {
		t.Errorf("messages %q", h.user)
	}
	if len(g.calls) != calls {
		t.Error("the reply with the e-mail went to a model")
	}
}

// agency-002 (#70): "eu e minha esposa" said before a request is not lost:
// an input whose extract reads the conversation takes it from the user's
// earlier messages; the others do not, and the message always wins.
func TestExtractReadsTheConversation(t *testing.T) {
	ids := func() func() string {
		n := 0
		return func() string { n++; return fmt.Sprintf("run-%d", n) }
	}
	h, g := tripHarness(t)
	h.rt.NewID = ids()
	h.startFn("trip", "vamos 2 adultos, e quero ir para Lisboa dia 12 de outubro")
	res := h.startFn("trip", "quero ir para Porto dia 12 de outubro")
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Porto, 2026-10-12, 1 adulto(s)" {
		t.Errorf("without conversation: the message alone: %q", last)
	}
	if _, ok := g.calls[len(g.calls)-1].State.(map[string]any)["earlier"]; ok {
		t.Error("no input reads the conversation: EARLIER is not sent")
	}

	dir := t.TempDir()
	withConv := strings.Replace(tripFunction, "        - { id: said, extract: {} }\n        - { id: default, value: \"1\" }", "        - { id: said, extract: { conversation: true } }\n        - { id: default, value: \"1\" }", 1)
	if withConv == tripFunction {
		t.Fatal("test setup: adults source not found")
	}
	if err := os.WriteFile(filepath.Join(dir, "trip.yaml"), []byte(withConv), 0o600); err != nil {
		t.Fatal(err)
	}
	h2 := newHarness(t, dir)
	h2.rt.NewID = ids()
	g2 := &wordsGen{}
	h2.rt.Generator = g2
	h2.startFn("trip", "vamos 2 adultos, e quero ir para Lisboa dia 12 de outubro")
	res = h2.startFn("trip", "quero ir para Porto dia 12 de outubro")
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h2.user[len(h2.user)-1]; last != "Porto, 2026-10-12, 2 adulto(s)" {
		t.Errorf("the adults said before are read; the destination is the message's: %q", last)
	}
}

// An e-mail given earlier in the conversation is read by code when the
// input reads the conversation.
func TestFormatFromTheConversation(t *testing.T) {
	dir := t.TempDir()
	fn := strings.Replace(contactFunction, "      format: email\n      description: the e-mail for the booking\n      resolve:\n        - { id: said, extract: {} }", "      format: email\n      description: the e-mail for the booking\n      resolve:\n        - { id: said, extract: { conversation: true } }", 1)
	if fn == contactFunction {
		t.Fatal("test setup")
	}
	if err := os.WriteFile(filepath.Join(dir, "contact.yaml"), []byte(fn), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	n := 0
	h.rt.NewID = func() string { n++; return fmt.Sprintf("run-%d", n) }
	h.rt.Generator = &wordsGen{}
	h.startFn("contact", "Lisboa, meu e-mail é ana.souza@example.com")
	res := h.startFn("contact", "agora quero ir para Porto")
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Porto → ana.souza@example.com" {
		t.Errorf("messages %q", h.user)
	}
}

// agency-002: a precondition says what was understood, so a misreading is
// corrected instead of heard as a fact.
func TestPreconditionMessageIsATemplate(t *testing.T) {
	dir := t.TempDir()
	fn := strings.Replace(tripFunction, "  output:\n    template: \"{{ inputs.destination }}, {{ inputs.departDate }}, {{ inputs.adults }} adulto(s)\"",
		"  preconditions:\n    - cel: \"inputs.destination != 'Lisboa'\"\n      message: \"Entendi {{ inputs.destination }} em {{ inputs.departDate }}: não voamos para lá.\"\n  output:\n    template: \"{{ inputs.destination }}, {{ inputs.departDate }}, {{ inputs.adults }} adulto(s)\"", 1)
	if fn == tripFunction {
		t.Fatal("test setup")
	}
	if err := os.WriteFile(filepath.Join(dir, "trip.yaml"), []byte(fn), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, dir)
	h.rt.Generator = &wordsGen{}
	res := h.startFn("trip", "Quero ir para Lisboa dia 12 de outubro")
	expectStatus(t, res, journal.StatusFailed)
	if last := h.user[len(h.user)-1]; last != "Entendi Lisboa em 2026-10-12: não voamos para lá." {
		t.Errorf("messages %q", h.user)
	}
}

// agency-002: "10 de julho", said in September, came back as this July.
// With extract.future a date more than a week past moves a year on;
// yesterday stays yesterday.
func TestExtractFuture(t *testing.T) {
	h, _ := tripHarness(t)
	h.rt.Clock = func() time.Time { return time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC) }
	e := &execution{scopes: map[string]any{"event": map[string]any{"time": h.rt.Clock()}}}
	for in, want := range map[string]string{"2026-07-10": "2027-07-10", "2026-10-19": "2026-10-19", "2026-10-12": "2027-10-12", "2026-11-02": "2026-11-02"} {
		if got := e.nextOccurrence(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

// RFC-0010: agent mode starts a Function with the inputs it knows; the
// rest resolve as usual, and a wrong or unknown input is refused.
func TestStartWithGivenInputs(t *testing.T) {
	h, g := tripHarness(t)
	res, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: "trip", Conversation: "c1", Message: "dia 12 de outubro", Inputs: map[string]any{"destination": "Porto"}})
	if err != nil {
		t.Fatal(err)
	}
	res = h.deliver(res)
	expectStatus(t, res, journal.StatusSucceeded)
	if last := h.user[len(h.user)-1]; last != "Porto, 2026-10-12, 1 adulto(s)" {
		t.Errorf("messages %q", h.user)
	}
	if props := g.calls[0].Schema["properties"].(map[string]any); props["destination"] != nil {
		t.Error("a given input is not extracted")
	}
	if _, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: "trip", Conversation: "c2", Inputs: map[string]any{"adults": "two"}}); err == nil {
		t.Error("a given input must conform to its type")
	}
	if _, err := h.rt.Start(context.Background(), Start{Namespace: "travel", Function: "trip", Conversation: "c3", Inputs: map[string]any{"cabin": "x"}}); err == nil {
		t.Error("an unknown input is refused")
	}
}
