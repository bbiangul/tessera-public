package v1alpha1

// EvalSuite tests an agent with whole conversations (RFC-0002, level 4). A
// scenario plays a user against the agent's Functions, with plugins
// replaced by sandboxes that keep state, and checks what happened: the
// effects, the messages, the invariants every agent must keep, and the cost.
type EvalSuite struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be EvalSuite.
	Kind string `json:"kind" jsonschema:"enum=EvalSuite"`
	// Metadata names the suite; its namespace is the agent under test.
	Metadata ObjectMeta `json:"metadata"`
	// Spec lists the scenarios.
	Spec EvalSuiteSpec `json:"spec"`
}

// EvalSuiteSpec describes a suite.
type EvalSuiteSpec struct {
	// Description says what the suite covers.
	Description string `json:"description,omitempty"`
	// Now is the clock of every scenario (RFC 3339 with offset), so dates
	// in the conversation mean the same thing on every run. A scenario can
	// set its own.
	Now string `json:"now" jsonschema:"format=date-time"`
	// Sandboxes replace plugins with stateful fakes written in Starlark.
	// Each scenario starts from the sandbox's initial state.
	Sandboxes []Sandbox `json:"sandboxes,omitempty"`
	// Scenarios are the conversations.
	Scenarios []Scenario `json:"scenarios" jsonschema:"minItems=1"`
	// Team plays the business's team in llm scenarios, for a task the
	// scenario's humans have no answer for (team-001). A model answers
	// only from Facts, and says so when they do not settle the task. A
	// task asking for a value gets one, in the task's format.
	Team *SimTeam `json:"team,omitempty"`
}

// Sandbox fakes a plugin. The script has one function per action, named
// after it with "-" as "_" (list-upcoming-appointments becomes
// list_upcoming_appointments), taking state and the action's arguments as
// keyword arguments. It may change state and returns the output; it calls
// definite(code, message) or ambiguous(message) to fail like a real system.
type Sandbox struct {
	// Plugin is the PluginManifest name this sandbox replaces.
	Plugin string `json:"plugin" jsonschema:"pattern=^[a-z0-9-]+$"`
	// Script is the .star file, relative to this file.
	Script string `json:"script" jsonschema:"pattern=\\.star$"`
	// State is the initial state, a map the functions read and change.
	State map[string]any `json:"state,omitempty"`
}

// Scenario is one conversation.
type Scenario struct {
	// ID names the scenario in reports.
	ID string `json:"id" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`
	// Description says what it checks.
	Description string `json:"description,omitempty"`
	// Now overrides the suite's clock.
	Now string `json:"now,omitempty" jsonschema:"format=date-time"`
	// Caller is the verified identity of the user.
	Caller map[string]any `json:"caller,omitempty"`
	// State changes the sandboxes' state for this scenario, per plugin:
	// its top-level keys replace the suite's (an outage, an empty agenda).
	State map[string]map[string]any `json:"state,omitempty"`
	// Simulator plays the user. scripted replays fixed turns; llm has a
	// model play the persona until its goal is met (RFC-0002), after the
	// turns, when there are any.
	Simulator string `json:"simulator" jsonschema:"enum=scripted,enum=llm"`
	// Turns are played in order. A scripted scenario needs at least one.
	Turns []Turn `json:"turns,omitempty"`
	// Persona is the user the llm simulator plays.
	Persona *Persona `json:"persona,omitempty"`
	// MaxTurns bounds the llm simulator's messages (default 12).
	MaxTurns int `json:"maxTurns,omitempty" jsonschema:"minimum=1,maximum=40"`
	// Humans answer, in order, the tasks the agent hands to a person
	// during an llm scenario: a value such as true, "o2" or {"ok": true}.
	// A question for the team (an answer task) takes the next text, a task
	// asking for a value (input) the next non-text. When none fits, the
	// scenario ends at that task.
	Humans []any `json:"humans,omitempty"`
	// Expect is checked after the last turn.
	Expect *Expectation `json:"expect,omitempty"`
}

// Persona is a simulated user: what they want, how they write and what
// they know. The simulator shares what it knows only when it is natural
// or asked, and never invents facts outside it.
type Persona struct {
	// Goal is what the user wants from the conversation.
	Goal string `json:"goal" jsonschema:"minLength=1"`
	// Style is how they write (short WhatsApp messages, typos, formal).
	Style string `json:"style,omitempty"`
	// Knows holds the facts the user can tell: dates, names, preferences.
	Knows map[string]any `json:"knows,omitempty"`
	// Language of the user's messages (default: Brazilian Portuguese).
	Language string `json:"language,omitempty"`
}

// Turn is one event. Exactly one of user, human, advance and trigger must
// be set.
type Turn struct {
	// User is what the user writes.
	User string `json:"user,omitempty"`
	// Advance moves the scenario's clock forward (Go duration, e.g. 48h)
	// and runs the scheduler: waits that come due wake, schedules start,
	// stuck subjects escalate. The turn's expectations read what they sent.
	Advance string `json:"advance,omitempty"`
	// Trigger fires a Trigger as if it came due (a schedule) or was called
	// (a webhook, with this payload, unsigned in tests). A run it starts in
	// another conversation becomes the scenario's conversation, so later
	// user turns reply to it.
	Trigger *TriggerTurn `json:"trigger,omitempty"`
	// Human answers the human task the run waits for: a value such as true,
	// "o2" or {"happened": true}.
	Human any `json:"human,omitempty"`
	// Expect is checked right after this turn.
	Expect *TurnExpect `json:"expect,omitempty"`
}

// TriggerTurn fires a Trigger in a scenario.
type TriggerTurn struct {
	// Name of the Trigger.
	Name string `json:"name" jsonschema:"minLength=1"`
	// Payload is event.payload for a webhook trigger.
	Payload map[string]any `json:"payload,omitempty"`
}

// TurnExpect checks one turn.
type TurnExpect struct {
	// Status is the run's status after the turn.
	Status string `json:"status,omitempty" jsonschema:"enum=WaitingForUser,enum=WaitingForHuman,enum=Confirming,enum=Indeterminate,enum=Succeeded,enum=Failed"`
	// Routed is the Function this turn started through the router.
	Routed string `json:"routed,omitempty"`
	// Handoff is the free mode's decision this turn: "answer", or the
	// Function of freeMode.functions it started (such as asking a person).
	Handoff string `json:"handoff,omitempty"`
	// Says lists texts that must each appear (ignoring case) in a message
	// sent to the user this turn.
	Says []string `json:"says,omitempty"`
	// NotSays lists texts no message to the user may contain this turn.
	NotSays []string `json:"notSays,omitempty"`
	// Queue is a human queue that must receive a task this turn.
	Queue string `json:"queue,omitempty"`
}

// Expectation checks a whole scenario.
type Expectation struct {
	// Status is the final status of the last run.
	Status string `json:"status,omitempty" jsonschema:"enum=WaitingForUser,enum=WaitingForHuman,enum=Confirming,enum=Indeterminate,enum=Succeeded,enum=Failed"`
	// Effects counts successful calls per action.
	Effects []EffectCount `json:"effects,omitempty"`
	// Milestones counts recorded milestones per subject kind and name.
	Milestones []MilestoneCount `json:"milestones,omitempty"`
	// Never lists actions that must not be called at all.
	Never []string `json:"never,omitempty"`
	// Says lists texts that must appear in some message to the user.
	Says []string `json:"says,omitempty"`
	// NotSays lists texts no message to the user may contain.
	NotSays []string `json:"notSays,omitempty"`
	// Invariants to skip; every invariant runs unless listed here, with a
	// reason in the scenario's description.
	SkipInvariants []string `json:"skipInvariants,omitempty" jsonschema:"enum=confirm-before-irreversible,enum=no-duplicate-effects,enum=no-lost-messages"`
	// Budget fails the scenario when exceeded.
	Budget *ScenarioBudget `json:"budget,omitempty"`
}

// EffectCount is how many successful calls an action must have.
type EffectCount struct {
	// Action is plugin/action.
	Action string `json:"action" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// Count of successful calls.
	Count int `json:"count" jsonschema:"minimum=0"`
	// Where counts only the calls whose arguments meet this CEL condition,
	// read as inputs, as in the plugin's manifest: what was sent, not only
	// that something was ("inputs.children == '5,8'").
	Where string `json:"where,omitempty"`
}

// MilestoneCount expects a number of milestones.
type MilestoneCount struct {
	// Subject is the Subject kind.
	Subject string `json:"subject"`
	// Name is the milestone.
	Name string `json:"name"`
	// Count of milestones recorded in the scenario.
	Count int `json:"count" jsonschema:"minimum=0"`
}

// ScenarioBudget bounds what a scenario may consume.
type ScenarioBudget struct {
	// CostUSD of model calls.
	CostUSD float64 `json:"costUSD,omitempty" jsonschema:"exclusiveMinimum=0"`
	// DecideCalls counts Decide calls.
	DecideCalls int `json:"decideCalls,omitempty" jsonschema:"minimum=1"`
	// GenerateCalls counts Generate calls.
	GenerateCalls int `json:"generateCalls,omitempty" jsonschema:"minimum=1"`
}

// SimTeam is the simulated team of an EvalSuite.
type SimTeam struct {
	// Facts is a Markdown file, relative to the suite, holding everything
	// the team knows about the business and its world.
	Facts string `json:"facts" jsonschema:"pattern=\\.md$"`
}
