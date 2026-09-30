// Package v1alpha1 holds the agent resource types (RFC-0001, "Modelo de
// recursos"). The doc comments on these types are the reference
// documentation: they are copied into the JSON Schema and shown by
// `tessera explain`. Keep them accurate and user-facing.
//
// v1alpha1 may change incompatibly.
package v1alpha1

import "github.com/invopop/jsonschema"

// APIVersion of the resources in this package.
const APIVersion = "agents.tessera.dev/v1alpha1"

// Expr is a CEL expression, written without delimiters. The fields that take
// one are always expressions. Expressions read only the scopes inputs,
// sources, steps, event, caller, conversation, agent and env; there is no
// now(), time comes from event.time.
type Expr string

// JSONSchema marks the field as an expression for editors and explain.
func (Expr) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Extras: map[string]any{"x-tessera-kind": "expression"}}
}

// Template is text for people with {{ expr }} interpolations. Each value is
// escaped for the channel it is sent to; the surrounding text is not.
type Template string

// JSONSchema marks the field as a template for editors and explain.
func (Template) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Extras: map[string]any{"x-tessera-kind": "template"}}
}

// ObjectMeta identifies a resource.
type ObjectMeta struct {
	// Name of the resource, unique per kind within its namespace. Lowercase
	// letters, digits and hyphens, starting and ending with a letter or digit.
	Name string `json:"name" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$,maxLength=63"`
	// Namespace the resource belongs to, usually one per customer.
	Namespace string `json:"namespace" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$,maxLength=63"`
	// Labels are free-form key/value pairs for selecting and grouping.
	Labels map[string]string `json:"labels,omitempty"`
}

// Function is one capability of an agent: the inputs it needs, who may run
// it, what must be true before it acts, the action it takes and what it
// tells the user.
type Function struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Function.
	Kind string `json:"kind" jsonschema:"enum=Function"`
	// Metadata names the function.
	Metadata ObjectMeta `json:"metadata"`
	// Spec is what the author wants the function to do.
	Spec FunctionSpec `json:"spec"`
	// InputOrder is the order the inputs were written in, set by the
	// loader. Inputs are resolved after the inputs they read, and otherwise
	// in this order, so the user is asked in the order the author chose.
	InputOrder []string `json:"-"`
}

// FunctionSpec describes a Function.
type FunctionSpec struct {
	// Description says in one sentence what the function does. It is shown
	// to people and used by the router.
	Description string `json:"description" jsonschema:"minLength=1"`
	// Selector says which events can start this function. A function
	// without a selector can only be called by other functions.
	Selector *Selector `json:"selector,omitempty"`
	// Authorize is a boolean expression on caller (and inputs) that must be
	// true for the function to run. It is checked with the verified identity
	// of the caller, never with anything a model decided.
	Authorize Expr `json:"authorize,omitempty"`
	// Inputs the function needs, by name. Each is resolved from its sources
	// in order until one produces a value.
	Inputs map[string]Input `json:"inputs,omitempty"`
	// Preconditions must all be true, after inputs are resolved, before the
	// action runs.
	Preconditions []Precondition `json:"preconditions,omitempty"`
	// Steps run in order after the preconditions and before the action.
	// Each step's result is readable by later steps, the action and the
	// output as steps.<id>.status and steps.<id>.output.
	Steps []Step `json:"steps,omitempty"`
	// Action is the effect the function has on the world.
	Action *Action `json:"action,omitempty"`
	// Output is what the user is told when the function succeeds.
	Output *Output `json:"output,omitempty"`
	// OnError is the chain of strategies tried, in order, when the function
	// fails. Each item runs only if the previous one failed.
	OnError []ErrorStrategy `json:"onError,omitempty"`
}

// Selector matches events to a function.
type Selector struct {
	// Intent is the name of the Intent this function handles.
	Intent string `json:"intent" jsonschema:"minLength=1"`
	// Description is what the router reads to choose this function; the
	// function's own description when empty. Write it in the users'
	// language, the way they ask for it: measured with Jev, a Portuguese
	// routing text moved "quero viajar" from 0.09 to 0.91
	// (docs/research/router-001.md).
	Description string `json:"description,omitempty"`
	// After makes this a follow-up: it is routable only when the
	// conversation's last run was one of these Functions ("e o da LATAM?"
	// only means something after a flight search). Measured: wording alone
	// did not stop general questions from reaching follow-ups
	// (docs/research/router-002.md).
	After []string `json:"after,omitempty"`
	// Who is a bool expression over caller: the router offers this
	// Function only to callers it matches, so a patient is never offered
	// a team Function (RFC-0006). authorize still decides.
	Who Expr `json:"who,omitempty"`
}

// Input is one value the function needs.
type Input struct {
	// Type of the value: a basic type (string, int, float, bool, date,
	// timestamp, duration, map, any), a type published by a plugin
	// (agenda.Slot), or list<T> of either. A date is a "YYYY-MM-DD"
	// string.
	Type string `json:"type" jsonschema:"minLength=1"`
	// Description says what the value is. Extraction and free-text
	// questions show it to the model, so write it for a reader: "the city
	// the user flies from".
	Description string `json:"description,omitempty"`
	// Resolve lists where the value can come from, tried in order. A source
	// may read the outputs of earlier sources of the same input as
	// sources.<id>.output.
	Resolve []Source `json:"resolve" jsonschema:"minItems=1"`
	// OnUnresolved runs when no source produced a value.
	OnUnresolved []ErrorStrategy `json:"onUnresolved,omitempty"`
	// Format marks personal data of a known shape: email, phone or cpf.
	// Its extract source and its free-text question read it from the
	// user's own words by pattern, in code: no model sees it, since models
	// only get masked text, where it is a placeholder (agency-001). The
	// type must be string.
	Format string `json:"format,omitempty" jsonschema:"enum=email,enum=phone,enum=cpf"`
}

// Source is one way to get an input's value. Exactly one of fromAction,
// fromFunction, value, extract and ask must be set.
type Source struct {
	// ID names the source so later sources can read its output.
	ID string `json:"id" jsonschema:"pattern=^[a-zA-Z][a-zA-Z0-9_]*$"`
	// When is a boolean expression; the source is skipped when it is false.
	When Expr `json:"when,omitempty"`
	// FromAction calls a plugin action, written plugin/action. Its effect
	// class must be read.
	FromAction string `json:"fromAction,omitempty"`
	// FromFunction calls another Function in the same namespace.
	FromFunction string `json:"fromFunction,omitempty"`
	// With gives the arguments of fromAction or fromFunction. Values are
	// literal unless the whole value is "${ expr }".
	With map[string]any `json:"with,omitempty"`
	// Value computes the input directly with an expression.
	Value Expr `json:"value,omitempty"`
	// Extract takes the value from the user's message with the Generate
	// engine; null when the message does not say. All extract sources of a
	// Function are answered by one model call.
	Extract *Extract `json:"extract,omitempty"`
	// Ask asks the user and waits for the answer.
	Ask *Ask `json:"ask,omitempty"`
}

// Extract reads an input from the conversation.
type Extract struct {
	// Instructions add to the input's description, e.g. how to resolve
	// relative dates or which default not to assume.
	Instructions string `json:"instructions,omitempty"`
	// Future moves a date the model read as more than a week past to the
	// same day a year later: "10 de julho" said in September is next July.
	// Models often got the year wrong (agency-002); "ontem" stays past.
	Future bool `json:"future,omitempty"`
	// Conversation lets the value come from what the user said earlier in
	// the conversation when the message leaves it out ("eu e minha esposa"
	// said before asking for a hotel). The message always wins. Leave it off
	// for values that belong to one request only, such as a new trip's
	// dates (agency-002).
	Conversation bool `json:"conversation,omitempty"`
}

// Ask asks the user for a value and interprets the reply with the Decide
// engine.
type Ask struct {
	// Text is the question sent to the user.
	Text Template `json:"text"`
	// Choices is an expression producing the list the user picks from. When
	// set, the reply is matched to one element; otherwise it is extracted as
	// the input's type.
	Choices Expr `json:"choices,omitempty"`
	// MinConfidence is the lowest confidence at which the interpretation is
	// accepted, between 0 and 1. Every decision declares one.
	MinConfidence float64 `json:"minConfidence" jsonschema:"exclusiveMinimum=0,maximum=1"`
	// OnLowConfidence says what happens below minConfidence. Every decision
	// declares it.
	OnLowConfidence string `json:"onLowConfidence" jsonschema:"enum=ask-user,enum=escalate-model,enum=human"`
}

// Precondition is a condition checked before the action.
type Precondition struct {
	// CEL is the boolean expression that must be true.
	CEL Expr `json:"cel"`
	// Message tells the user why the function cannot proceed. It is a
	// template: naming what was understood ("Entendi a ida como
	// {{ inputs.departDate }}") lets the user correct a misreading instead of
	// hearing it as a fact (agency-002).
	Message string `json:"message" jsonschema:"minLength=1"`
}

// Action is a call to a plugin action with an effect on the world.
type Action struct {
	// Act is the plugin action, written plugin/action.
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// With gives the action's arguments. Values are literal unless the whole
	// value is "${ expr }".
	With map[string]any `json:"with,omitempty"`
	// Confirm is the question asked to the user before an irreversible
	// action. The action runs only after a clear yes.
	Confirm Template `json:"confirm,omitempty"`
	// IdempotencyKey is a string expression identifying the effect, so it is
	// not repeated. Required for irreversible actions.
	IdempotencyKey Expr `json:"idempotencyKey,omitempty"`
	// Milestone is recorded when the effect is confirmed.
	Milestone *MilestoneRef `json:"milestone,omitempty"`
}

// Output is what the function produces on success. At least one of
// template and value must be set.
type Output struct {
	// Template is the message sent to the user; a function called by
	// another sends nothing.
	Template Template `json:"template,omitempty"`
	// Value is what a caller receives (call, fromFunction); by default the
	// action's output, or the rendered template.
	Value Expr `json:"value,omitempty"`
	// Guardrails checked before the message is sent, by name.
	Guardrails []string `json:"guardrails,omitempty"`
}

// ErrorStrategy is one step of an onError chain. Exactly one field must be set.
type ErrorStrategy struct {
	// When is a boolean expression: the strategy runs only when it is
	// true. It reads the failure as sources.error (code, message, definite)
	// besides inputs, steps and caller, e.g. "sources.error.code ==
	// 'CONFLICT'". It only narrows the chain: an irreversible action whose
	// outcome is unknown is still verified first and never repeated or
	// re-routed blind (#30).
	When Expr `json:"when,omitempty"`
	// Probe asks the external system whether the failed effect happened
	// anyway. The action's plugin must declare verify mode probe or native.
	Probe *Probe `json:"probe,omitempty"`
	// Retry tries again. After an irreversible action it is allowed only
	// when the plugin verifies natively or a probe proved nothing happened.
	Retry *Retry `json:"retry,omitempty"`
	// Fallback takes another path: another function, another action, or a
	// fixed value.
	Fallback *Fallback `json:"fallback,omitempty"`
	// Escalate creates a human task and waits for it.
	Escalate *Escalate `json:"escalate,omitempty"`
	// Compensate undoes earlier effects of the same run, in reverse order.
	Compensate *Compensate `json:"compensate,omitempty"`
	// Respond sends a message to the user and ends the run.
	Respond *Respond `json:"respond,omitempty"`
	// Fail ends the run as failed.
	Fail *Fail `json:"fail,omitempty"`
}

// Probe has no settings; the verification action comes from the plugin.
type Probe struct{}

// Retry settings.
type Retry struct {
	// Attempts is the number of extra attempts.
	Attempts int `json:"attempts" jsonschema:"minimum=1,maximum=10"`
	// Backoff is the wait before the first retry, as a duration like 2s.
	Backoff string `json:"backoff,omitempty" jsonschema:"pattern=^[0-9]+(ms|s|m)$"`
}

// Fallback settings. Exactly one of function, act and useNull must be set.
// A fallback action is held to the same effect rules as the main action:
// inside an input's onUnresolved it must be read-only, and when it is
// irreversible it needs its own idempotencyKey.
type Fallback struct {
	// Function to call instead.
	Function string `json:"function,omitempty"`
	// Act is another plugin action to call instead, written plugin/action.
	Act string `json:"act,omitempty"`
	// With gives the arguments of function or act. Values are literal unless
	// the whole value is "${ expr }".
	With map[string]any `json:"with,omitempty"`
	// IdempotencyKey is required when act is irreversible.
	IdempotencyKey Expr `json:"idempotencyKey,omitempty"`
	// UseNull continues without the value; the input becomes null.
	UseNull bool `json:"useNull,omitempty"`
}

// Escalate creates a HumanTask.
type Escalate struct {
	// Queue is the HumanQueue that receives the task.
	Queue string `json:"queue" jsonschema:"minLength=1"`
	// Task is the kind of human task.
	Task string `json:"task" jsonschema:"enum=input,enum=answer,enum=approve,enum=takeover"`
	// Message tells the human what is needed.
	Message Template `json:"message,omitempty"`
	// WhileWaiting is sent to the user while the human task is open.
	WhileWaiting *Respond `json:"whileWaiting,omitempty"`
	// OnDeny is what the user hears when a person denies an approval
	// (task: approve); default: the Agent's messages.denied. A denied (or
	// unclear) approval ends the Function: no later step or action runs.
	OnDeny *Respond `json:"onDeny,omitempty"`
	// Route sends the task to another queue by a declared rule: the first
	// rule whose when holds decides, else queue ("a discount above 50% goes
	// to the owner"). A rule, never a model's choice (RFC-0006).
	Route []EscalateRoute `json:"route,omitempty"`
}

// EscalateRoute is one routing rule.
type EscalateRoute struct {
	// When is a bool expression over inputs, steps and caller.
	When Expr `json:"when"`
	// Queue receives the task when it holds.
	Queue string `json:"queue" jsonschema:"minLength=1"`
}

// Compensate has no settings; each action's compensation comes from its plugin.
type Compensate struct{}

// Respond sends a message.
type Respond struct {
	// Template is the message.
	Template Template `json:"template"`
	// To is "user" (the default) or "queue:<name>": a note to a human
	// queue that asks nothing and waits for nothing (a report, an alert).
	To string `json:"to,omitempty" jsonschema:"pattern=^(user|queue:[a-z0-9]([-a-z0-9]*[a-z0-9])?)$"`
}

// Fail ends the run as failed.
type Fail struct {
	// Message is recorded with the failure.
	Message string `json:"message,omitempty"`
}
