package v1alpha1

// Step is one step of a Function (RFC-0001, "Tipos de passo"). Exactly one
// of compute, act, call, decide, decideBatch, extract, generate, ask, respond, escalate,
// branch and wait must be set. parallel, foreach, emit, remember and
// delegate come later.
type Step struct {
	// ID names the step; later steps read its result as steps.<id>.
	ID string `json:"id" jsonschema:"pattern=^[a-zA-Z][a-zA-Z0-9_]*$"`
	// When is a boolean expression; the step is skipped when it is false,
	// and its status is then "skipped".
	When Expr `json:"when,omitempty"`

	// Compute runs a CEL expression or a Starlark function.
	Compute *Compute `json:"compute,omitempty"`
	// Act calls a plugin action, written plugin/action, with the guarantees
	// of its effect class.
	Act string `json:"act,omitempty" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// Call runs another Function of the same namespace; with gives its
	// inputs.
	Call string `json:"call,omitempty"`
	// With gives the arguments of act or call. Values are literal unless
	// the whole value is "${ expr }".
	With map[string]any `json:"with,omitempty"`
	// IdempotencyKey is a string expression; required when act is
	// irreversible.
	IdempotencyKey Expr `json:"idempotencyKey,omitempty"`
	// Confirm is asked before an act step runs; the step runs only on a
	// clear yes.
	Confirm Template `json:"confirm,omitempty"`
	// Decide asks one typed question.
	Decide *DecideStep `json:"decide,omitempty"`
	// DecideBatch asks independent named questions about one shared state in one request.
	DecideBatch *DecideBatchStep `json:"decideBatch,omitempty"`
	// Extract fills a JSON Schema from a state with the Generate engine; the
	// output is the validated object.
	Extract *GenerateStep `json:"extract,omitempty"`
	// Generate writes text, or an object when a schema is given.
	Generate *GenerateStep `json:"generate,omitempty"`
	// Ask asks the user to choose from a list and waits for the answer.
	Ask *Ask `json:"ask,omitempty"`
	// Respond sends a message to the user and continues.
	Respond *Respond `json:"respond,omitempty"`
	// Escalate creates a HumanTask and waits; the person's answer is the
	// step's output.
	Escalate *Escalate `json:"escalate,omitempty"`
	// Branch runs the first case whose condition is true. It must end with
	// an else case, so every situation is covered.
	Branch []BranchCase `json:"branch,omitempty"`
	// Experiment assigns the subject a variant of an Experiment, records
	// that the subject saw it, and outputs the variant's name; later steps
	// branch on it. The same subject always gets the same variant.
	Experiment string `json:"experiment,omitempty"`
	// Wait sleeps durably: the run waits for a time, not for a message, and
	// survives restarts. Its output is the time it woke.
	Wait *Wait `json:"wait,omitempty"`
	// Retrieve searches the namespace's KnowledgeBases (retrieve-001). Its
	// output is the list of passages found, {source, section, text}, as
	// the free mode finds them: only bases the caller's audience may read,
	// no expired or suspended lesson, only passages relevant to the query.
	// Recorded, so a resumed run replays the same passages.
	Retrieve *RetrieveStep `json:"retrieve,omitempty"`

	// Milestone is recorded when the step succeeds (for act: when the
	// effect is confirmed).
	Milestone *MilestoneRef `json:"milestone,omitempty"`

	// OnError is tried, in order, when the step fails.
	OnError []ErrorStrategy `json:"onError,omitempty"`
}

// Wait is a durable timer. Exactly one of for and until must be set.
type Wait struct {
	// For is a duration from when the step is first reached, in Go syntax
	// (90m, 36h). Days are business decisions; write them with until.
	For string `json:"for,omitempty"`
	// Until is a timestamp expression, e.g. the next business day at 10h:
	// "atTime(addBusinessDays(event.time, 1, 'America/Sao_Paulo'), '10:00', 'America/Sao_Paulo')".
	Until Expr `json:"until,omitempty"`
	// OrReply ends the wait early when the user replies: while it waits
	// the run is the conversation's active run, so the reply comes here,
	// not to the router. The output is then {reply, timedOut}: the reply's
	// text and false, or null and true when the time came first.
	OrReply bool `json:"orReply,omitempty"`
	// IncludeSideMessages opts this reply wait into preference capture. The host
	// uses a Jev batch to distinguish current factual questions and withdrawal;
	// answered side user messages are journaled without resuming the wait.
	// Requires orReply. Output.sideMessages is chronological, scoped to this
	// wait and caller, limited to 8 messages/32 KiB (overflow is an error).
	// Combine it explicitly with the current reply for preference extraction.
	// This context is never confirmation or permission to perform an effect.
	IncludeSideMessages bool `json:"includeSideMessages,omitempty"`
	// Relay makes every reply this wait's own, as written: no side-question
	// classification. For a wait that relays a conversation with a person
	// ("Confirma que quer remarcar?"), where a question back is for that
	// person, not for the free mode (agency-003). Requires orReply.
	Relay bool `json:"relay,omitempty"`
	// Events end the wait early when another system reports one of them
	// for this run's key: a Trigger with event (e.g. the CRM says the
	// lead's meeting was booked). The output is then {reply: null,
	// timedOut: false, event: <name>, payload: <the event's payload>};
	// with events, the output always has that shape.
	Events []WaitEvent `json:"events,omitempty"`
}

// WaitEvent is an event a wait listens for.
type WaitEvent struct {
	// Name of the event, as a Trigger's event.name delivers it.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Key is a string expression identifying whom it is about, evaluated
	// when the wait starts (e.g. event.payload.leadId): only an event with
	// the same key ends this wait.
	Key Expr `json:"key"`
}

// Compute is a deterministic computation. Exactly one of cel and starlark
// must be set.
type Compute struct {
	// CEL is an expression; its value is the step's output.
	CEL Expr `json:"cel,omitempty"`
	// Starlark calls a function of a script file.
	Starlark *StarlarkCall `json:"starlark,omitempty"`
}

// StarlarkCall calls a top-level function of a Starlark script.
type StarlarkCall struct {
	// Script is the path of the .star file, relative to the file that
	// declares the Function.
	Script string `json:"script" jsonschema:"pattern=\\.star$"`
	// Function is the name of a top-level function of the script.
	Function string `json:"function" jsonschema:"pattern=^[a-zA-Z][a-zA-Z0-9_]*$"`
	// With gives the function's keyword arguments. Values are literal unless
	// the whole value is "${ expr }".
	With map[string]any `json:"with,omitempty"`
}

// DecideStep asks the Decide engine one question about a state. Exactly one
// of choice, yesNo and score must be set. The step's output is
// {value, probability, probabilities}: value is the chosen key (choice),
// true or false (yesNo), or the most probable level (score).
type DecideStep struct {
	// State is what the question is about. It is masked for personal data
	// before it leaves the process.
	State map[string]any `json:"state" jsonschema:"minProperties=1"`
	// Instructions say what to decide.
	Instructions string `json:"instructions" jsonschema:"minLength=1"`
	// Choice maps each option key to its description.
	Choice map[string]string `json:"choice,omitempty" jsonschema:"minProperties=2,maxProperties=255"`
	// YesNo asks a yes/no question.
	YesNo *YesNo `json:"yesNo,omitempty"`
	// Score lists ordered levels, lowest first.
	Score []string `json:"score,omitempty" jsonschema:"minItems=2,maxItems=10"`
	// MinConfidence is the lowest probability of the answer that is acted on.
	MinConfidence float64 `json:"minConfidence" jsonschema:"exclusiveMinimum=0,maximum=1"`
	// OnLowConfidence says what happens below minConfidence: human asks a
	// person (an answer task with the options), fail fails the step, and
	// continue sets output.value to null so a later branch can handle it.
	OnLowConfidence string `json:"onLowConfidence" jsonschema:"enum=human,enum=fail,enum=continue"`
}

// YesNo describes a yes/no question.
type YesNo struct {
	// IfTrue says what yes means.
	IfTrue string `json:"ifTrue,omitempty"`
	// IfFalse says what no means.
	IfFalse string `json:"ifFalse,omitempty"`
}

// BranchCase is one case of a branch. Exactly one of when and else must be
// set, and exactly one of call, respond and escalate.
type BranchCase struct {
	// When is a boolean expression.
	When Expr `json:"when,omitempty"`
	// Else marks the last case, taken when no other matched.
	Else bool `json:"else,omitempty"`
	// Call runs another Function; with gives its inputs.
	Call string `json:"call,omitempty"`
	// With gives the arguments of call.
	With map[string]any `json:"with,omitempty"`
	// Respond sends a message.
	Respond *Respond `json:"respond,omitempty"`
	// Escalate hands over to a person.
	Escalate *Escalate `json:"escalate,omitempty"`
}

// GenerateStep asks the Generate engine for text or an object. The reply is
// validated against the schema locally, repaired once if needed, and
// refused otherwise; the model is never trusted to follow the schema.
type GenerateStep struct {
	// State is the input. It is masked for personal data before it leaves
	// the process.
	State map[string]any `json:"state" jsonschema:"minProperties=1"`
	// Instructions say what to produce.
	Instructions string `json:"instructions" jsonschema:"minLength=1"`
	// Schema is a JSON Schema the output must match. Required for extract.
	Schema map[string]any `json:"schema,omitempty"`
}

// RetrieveStep searches knowledge bases.
type RetrieveStep struct {
	// Bases are KnowledgeBase names of this namespace.
	Bases []string `json:"bases" jsonschema:"minItems=1"`
	// Query is a string expression: what to search for, e.g.
	// "event.message".
	Query Expr `json:"query" jsonschema:"minLength=1"`
	// K is how many passages at most, 1 to 10; default 4.
	K int `json:"k,omitempty" jsonschema:"minimum=0,maximum=10"`
}
