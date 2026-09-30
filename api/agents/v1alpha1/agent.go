package v1alpha1

// Agent holds what is the same for every Function of a namespace (RFC-0001,
// "Humano no loop"): how the platform itself speaks to users, in their
// language, and where work for people goes. At most one per namespace;
// without one, the platform's English defaults and the "default" queue
// apply.
type Agent struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Agent.
	Kind string `json:"kind" jsonschema:"enum=Agent"`
	// Metadata names the agent; its namespace is the one it configures.
	Metadata ObjectMeta `json:"metadata"`
	// Spec is the configuration.
	Spec AgentSpec `json:"spec"`
}

// AgentSpec configures a namespace.
type AgentSpec struct {
	// Description says what the agent is for.
	Description string `json:"description,omitempty"`
	// Timezone is the IANA time zone schedules are read in, e.g.
	// America/Sao_Paulo.
	Timezone string `json:"timezone,omitempty"`
	// Channel names the plugin whose channel rules (the free-text window)
	// apply to messages to the user, e.g. whatsapp.
	Channel string `json:"channel,omitempty"`
	// Messages are what the platform says on its own, when no Function
	// speaks. Write them in the users' language.
	Messages AgentMessages `json:"messages,omitempty"`
	// HumanInTheLoop says where work for people goes.
	HumanInTheLoop *HumanInTheLoop `json:"humanInTheLoop,omitempty"`
	// Autonomy is how much the agent does alone (RFC-0001): autonomous,
	// the default; approve-actions, a person of humanInTheLoop approves
	// every irreversible or external-message effect first; draft, that and
	// every message a run sends the user. An audience's autonomy wins.
	Autonomy string `json:"autonomy,omitempty" jsonschema:"enum=autonomous,enum=approve-actions,enum=draft"`
	// Routing tunes the router for this namespace.
	Routing *Routing `json:"routing,omitempty"`
	// Voice rewrites every message to users in a persona's words
	// (RFC-0004): the facts the templates interpolate (dates, times, what
	// will be done) stay word for word, or the template's text is sent.
	// Needs a Generate backend; each message costs one call.
	Voice *Voice `json:"voice,omitempty"`
	// FreeMode answers what no Function covers (RFC-0004, part B): a
	// message the router cannot place, and a question or request in the
	// middle of a process, after which the pending question is asked
	// again. It reads (the reads' actions must be read-only) and talks; it
	// never acts. Without it, those messages get messages.notUnderstood.
	FreeMode *FreeMode `json:"freeMode,omitempty"`
	// Audiences give each kind of caller (patients, the reception, a
	// dentist) its own menu, voice, free mode and messages (RFC-0006). The
	// first audience whose who holds for the caller is theirs; one without
	// who, last, catches everyone else. A caller no audience matches is
	// offered nothing. Without audiences every caller gets the Agent.
	Audiences []Audience `json:"audiences,omitempty"`
	// ExplainFailures, on by default: when an action fails because the
	// system refused it (a definite failure) and the system said why, the
	// failure's message is rewritten to say it was not done, what the
	// system said and the user's next step. The system's words are masked
	// unless its plugin declares connection.publicErrors; a rewrite that
	// says the action was done, or loses one of the system's numbers, is
	// dropped. It also turns on the free mode's claims guard (it never says
	// it registered or passed something on). Measured in drift-005/006:
	// 0/96 false claims under API drift, every refusal explained.
	// Needs a Generate backend.
	ExplainFailures *bool `json:"explainFailures,omitempty"`
	// Improvise lets the agent go beyond its definitions, with the
	// platform's guarantees (RFC-0005). Off unless set.
	Improvise *Improvise `json:"improvise,omitempty"`
}

// Improvise is what the agent may do beyond its definitions.
type Improvise struct {
	// Repair lists actions (plugin/action, http bindings that send a body)
	// whose refused calls may be repaired. When the system refuses one and
	// says what it now needs (a field, the user's acceptance of a fee), the
	// model proposes only additions to the request body; the platform
	// checks them, asks the user for a missing value or their agreement
	// (naming any amount exactly), and a person on the humanInTheLoop
	// queue approves before the action runs again, with its own
	// idempotency key and the action's verification (drift-007).
	Repair []string `json:"repair,omitempty"`
	// RepairReads: when a read's answer no longer matches the plugin's
	// declared type (a field renamed), the model maps the answer's fields
	// to the declared ones. Only renames: values are never computed or
	// changed, the result must meet the type, and the rename is journaled.
	// Reads change nothing, so no approval is asked (RFC-0005 step 2,
	// drift-008).
	RepairReads bool `json:"repairReads,omitempty"`
}

// FreeMode is the agent outside its processes.
type FreeMode struct {
	// Instructions: what the agent may talk about and how to point to a
	// person or a phone for the rest.
	Instructions string `json:"instructions" jsonschema:"minLength=1"`
	// WhenUnsure is a Function started when the free mode has no answer it
	// can stand behind (its checks dropped every draft), instead of
	// messages.unsure: asking the team, as a person who does not know
	// would (agency-003). Usually one of functions, with no selector.
	WhenUnsure string `json:"whenUnsure,omitempty"`
	// Reads are read actions whose results are what the free mode knows
	// (e.g. the business's facts). Called on each use, with their with.
	Reads []FreeRead `json:"reads,omitempty"`
	// RunReads lists read-only plugin/action names whose successful input
	// source results may answer a side question during the current Function.
	// Only top-level fromAction sources already recorded in that run are
	// shared, for the same caller, conversation and namespace. Results are
	// always masked; arguments, other inputs, steps and called Functions are
	// not shared. No action is called again. These are snapshots, not fresh
	// availability or proof that an effect happened. Default: share nothing.
	// The combined context must fit in 32 KiB of JSON; larger context is an
	// error, never silently truncated. Expose a smaller read result instead.
	RunReads []string `json:"runReads,omitempty" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// Functions the free mode may start instead of answering, such as
	// asking a person. Before answering a message no Function was routed
	// to, one Decide question, which sees the reads' results, chooses
	// between answering and each of these.
	Functions []FreeFunction `json:"functions,omitempty"`
	// Search lets the free mode search the web for current public
	// information (a country's visa rules, an airline's baggage policy),
	// off unless set. Jev decides whether a message needs it, the Generate
	// engine writes one query from the masked message, a query with
	// anything about a person is never sent, and the results reach the
	// model as third-party pages: cited, never obeyed (websearch-001).
	Search *FreeSearch `json:"search,omitempty"`
	// Tasks gives the free mode the caller's open tasks (backoffice-006):
	// those a Team member may answer, as FACTS.myTasks, masked like any
	// private read. The free mode says how many and which; they are
	// answered with the buttons or the link sent to the member, never by a
	// message in the conversation. For a team audience.
	Tasks bool `json:"tasks,omitempty"`
	// Self gives the free mode FACTS.self (self-001): what the agent does
	// for this caller, built from the definitions (the Functions on the
	// caller's audience menu and the free mode's functions, what each asks
	// for, what a person approves), so the agent can say what it does, what
	// it needs and where to go for the rest. Another audience's Functions
	// never appear. On unless set to false (self-001; default since
	// guard-003 fixed the claims guard that dropped these answers, #60).
	Self *bool `json:"self,omitempty"`
	// Knowledge gives the free mode the business's documents
	// (KnowledgeBase, knowledge-001).
	Knowledge *FreeKnowledge `json:"knowledge,omitempty"`
	// Agent turns on agent mode (RFC-0010, experimental, ADR-0007): with no
	// run pending, a model plans each turn in steps over the audience's
	// Functions, the functions above, the web search and the documents,
	// instead of the router and the free answer. A Function it calls runs
	// as always (its questions, confirmation, approval, idempotency); its
	// reply passes the free mode's checks; each step is journaled.
	Agent *FreeAgent `json:"agent,omitempty"`
}

// FreeAgent is agent mode's configuration.
type FreeAgent struct {
	// MaxSteps bounds the tool calls of one turn; default 5.
	MaxSteps int `json:"maxSteps,omitempty" jsonschema:"minimum=0,maximum=8"`
}

// FreeSearch is the free mode's web search.
type FreeSearch struct {
	// Act is the search action, plugin/action: a read taking {query} and
	// returning a list of {site, title, url, snippet}. tessera add web
	// writes one (TinyFish by default; SearXNG, Tavily, Brave).
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// When says which messages need a search; default: current public
	// information from outside the business (rules, requirements,
	// seasons, schedules, general prices).
	When string `json:"when,omitempty"`
	// MaxResults is how many results the model sees; default 5, at most 10.
	MaxResults int `json:"maxResults,omitempty" jsonschema:"minimum=0,maximum=10"`
}

// FreeFunction is a Function the free mode may start.
type FreeFunction struct {
	// Function is the name of a Function in the Agent's namespace.
	Function string `json:"function" jsonschema:"minLength=1"`
	// When says when to start it rather than answer, for Decide: e.g.
	// "the question needs a decision, an approval or an exception that
	// the checklist gives to the coach".
	When string `json:"when" jsonschema:"minLength=1"`
	// MinConfidence is the lowest probability acted on; below it the free
	// mode answers. Default 0.6.
	MinConfidence float64 `json:"minConfidence,omitempty" jsonschema:"minimum=0,maximum=1"`
}

// FreeRead is one read action and its arguments.
type FreeRead struct {
	// Act is plugin/action; its effect must be read.
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// With gives its arguments; expressions read caller.
	With map[string]any `json:"with,omitempty"`
	// Public says the result is the business's public information (an
	// address, a phone, prices), not personal data: it reaches the model
	// unmasked. Without it the result is masked like the conversation, so
	// a phone number in it becomes a placeholder the free mode cannot
	// repeat.
	Public bool `json:"public,omitempty"`
}

// Voice is how the agent speaks.
type Voice struct {
	// Persona: who speaks, the tone, how formal, how to call the user.
	Persona string `json:"persona" jsonschema:"minLength=1"`
	// Examples of messages in this voice.
	Examples []string `json:"examples,omitempty"`
	// Keep are words a rewrite must keep whenever the template has them,
	// matched case-insensitively as substrings: the action's verb, so
	// "remarcar" never becomes "agendar". A stem ("remarc") covers its
	// forms.
	Keep []string `json:"keep,omitempty"`
	// Greeting is "first" (the default): only the conversation's first
	// message may greet, and a later greeting is removed; or "any": the
	// voice decides, as in voice-001.
	Greeting string `json:"greeting,omitempty" jsonschema:"enum=first,enum=any"`
	// Latitude is "warm" (the default): the facts are locked, and the
	// voice may acknowledge, add transitions and a closing offer, and put
	// facts on lines, as a friendly person would. Warm rewrites receive the
	// approved draft, facts, persona and user name, plus whether the agent
	// already spoke; they do not receive dialogue text that could introduce
	// extra requests or promises. Routing and free mode retain their history.
	// "strict" keeps masked dialogue context and says what the template says
	// and nothing more, for texts that must stay exact.
	Latitude string `json:"latitude,omitempty" jsonschema:"enum=strict,enum=warm"`
	// Dialogue is what the warm voice sees of the conversation. "full"
	// (the default): the masked dialogue, with the rule that only the
	// template authorizes actions and services; the conversation gives
	// tone, not permission (voice-008/009: 0/108 unsupported promises;
	// natural share against the competitors 40.8% against 30.3% over three
	// seeds). "marker" (voice-007): only whether the agent already spoke.
	Dialogue string `json:"dialogue,omitempty" jsonschema:"enum=marker,enum=full"`
}

// Routing tunes the router. Set it from the namespace's own routing
// benchmark (tessera bench route), not by feel.
type Routing struct {
	// MinConfidence is the lowest router probability acted on; below it the
	// user is asked to say it another way. Default 0.6.
	MinConfidence float64 `json:"minConfidence,omitempty" jsonschema:"exclusiveMinimum=0,exclusiveMaximum=1"`
}

// AgentMessages are the platform's own messages to users. An empty one
// keeps the English default.
type AgentMessages struct {
	// NotUnderstood: a message no Function fits, or fits too weakly.
	NotUnderstood string `json:"notUnderstood,omitempty"`
	// Unsure: the free mode had no answer it could stand behind (its checks
	// dropped every draft), so it says it is not sure instead of "not
	// understood" (agency-001). Empty: NotUnderstood.
	Unsure string `json:"unsure,omitempty"`
	// Later: the user will give what a process asks later ("mando amanhã");
	// the process keeps waiting and the question is not asked again
	// (agency-005).
	Later string `json:"later,omitempty"`
	// WithAPerson: the user writes while a person handles the run.
	WithAPerson string `json:"withAPerson,omitempty"`
	// NotAllowed: the caller may not run the Function (authorize).
	NotAllowed string `json:"notAllowed,omitempty"`
	// Cancelled: the user did not confirm an effect.
	Cancelled string `json:"cancelled,omitempty"`
	// Denied: a person denied an approval the Function asked for
	// (escalate task: approve), and the step has no onDeny.
	Denied string `json:"denied,omitempty"`
	// Failed: the run failed and no onError strategy told the user.
	Failed string `json:"failed,omitempty"`
}

// HumanInTheLoop configures escalation.
type HumanInTheLoop struct {
	// Queue receives escalations and questions that do not name a queue.
	Queue string `json:"queue" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`
}

// Audience is one kind of caller and what the agent is for them.
type Audience struct {
	// Name identifies it (journaled with each run as caller.audience).
	Name string `json:"name" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`
	// Who is a bool expression over caller: "'dentista' in caller.roles".
	// Roles come from the Team through an identity a channel attests,
	// never from what the caller writes. Empty: everyone else (last only).
	Who Expr `json:"who,omitempty"`
	// Functions are the router's menu for this audience. selector.who and
	// authorize still apply to each.
	Functions []string `json:"functions,omitempty"`
	// Voice replaces the Agent's voice for this audience.
	Voice *Voice `json:"voice,omitempty"`
	// FreeMode replaces the Agent's free mode for this audience.
	FreeMode *FreeMode `json:"freeMode,omitempty"`
	// Messages replace the Agent's, field by field.
	Messages *AgentMessages `json:"messages,omitempty"`
	// Autonomy replaces the Agent's for this audience's runs.
	Autonomy string `json:"autonomy,omitempty" jsonschema:"enum=autonomous,enum=approve-actions,enum=draft"`
	// Sees declares what this audience's members see of other audiences'
	// conversations behind the tasks they answer (audiences-002). Every
	// view is journaled in the task's run. Without it, only the task's
	// text.
	Sees []AudienceView `json:"sees,omitempty"`
}

// AudienceView is one audience whose conversations another sees.
type AudienceView struct {
	// Audience whose conversations are seen.
	Audience string `json:"audience" jsonschema:"minLength=1"`
	// Conversation: masked (e-mails, phones and documents hidden) or full.
	Conversation string `json:"conversation" jsonschema:"enum=masked,enum=full"`
}
