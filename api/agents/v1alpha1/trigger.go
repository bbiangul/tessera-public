package v1alpha1

// Trigger starts a Function without a user message (RFC-0001): on a
// schedule in the agent's time zone, or when another system calls a
// webhook, or when a milestone is recorded. Exactly one of schedule,
// webhook and milestone is set. A webhook can also deliver an event to
// the runs waiting for it (event).
type Trigger struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Trigger.
	Kind string `json:"kind" jsonschema:"enum=Trigger"`
	// Metadata names the trigger; it starts Functions of its namespace.
	Metadata ObjectMeta `json:"metadata"`
	// Spec says what starts and when.
	Spec TriggerSpec `json:"spec"`
}

// TriggerSpec describes a trigger.
type TriggerSpec struct {
	// Description says what the trigger is for.
	Description string `json:"description,omitempty"`
	// Function is the Function started, by name, in the same namespace. Its
	// run sees event.time (the scheduled time) and event.trigger (this
	// trigger's name); it has no user message. With event, it starts only
	// when no run was waiting for the event; without event it is required.
	Function string `json:"function,omitempty"`
	// Event delivers the webhook's payload as an event to the runs whose
	// wait listens for it with the same key (webhook triggers only).
	Event *TriggerEvent `json:"event,omitempty"`
	// Schedule starts the Function at the times of a cron expression.
	Schedule *Schedule `json:"schedule,omitempty"`
	// Webhook starts the Function when a system POSTs to
	// /v1/hooks/<trigger name> (tessera serve); the JSON body is
	// event.payload.
	Webhook *Webhook `json:"webhook,omitempty"`
	// Milestone starts the Function once for each milestone of this name
	// recorded about a subject kind (a store opened, a lead booked),
	// in the conversation of the run that recorded it, with the milestone
	// as event.payload {subject, id, name, at, attributes}. Milestones
	// recorded before the trigger existed do not start it.
	Milestone *MilestoneTrigger `json:"milestone,omitempty"`
}

// MilestoneTrigger names the milestone that starts a Function.
type MilestoneTrigger struct {
	// Subject is the Subject kind.
	Subject string `json:"subject" jsonschema:"minLength=1"`
	// Name of the milestone.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
}

// Webhook authenticates calls with an HMAC-SHA256 of timestamp "." key "."
// body (X-Tessera-Timestamp in unix seconds, within 5 minutes; the
// Idempotency-Key, empty without one), sent as X-Tessera-Signature:
// sha256=<hex>. An Idempotency-Key header makes a
// repeated delivery start nothing new (the run's ID is the trigger and the
// key); without it the body's hash is the key.
type Webhook struct {
	// Secret is the name of the signing secret (TESSERA_SECRET_<NAME> when
	// self-hosted); the value never appears in a definition.
	Secret string `json:"secret" jsonschema:"pattern=^[A-Z][A-Z0-9_]*$"`
	// Conversation is a string expression on event.payload giving the
	// conversation the run belongs to (e.g. the lead's channel id), so the
	// user's replies reach it; default: a conversation of its own.
	Conversation Expr `json:"conversation,omitempty"`
	// Key is a string expression on event.payload that identifies the
	// run instead of the Idempotency-Key header (e.g.
	// event.payload.leadId): one run per key, ever, whatever the sender
	// retries or repeats.
	Key Expr `json:"key,omitempty"`
}

// TriggerEvent names the event a webhook delivers.
type TriggerEvent struct {
	// Name of the event, as waits' events name it.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Key is a string expression on event.payload saying whom it is
	// about; it must equal the waiting run's key.
	Key Expr `json:"key"`
}

// Schedule is a cron schedule. Each scheduled time starts one run, once,
// however many schedulers are running (the run's ID is the trigger and the
// time).
type Schedule struct {
	// Cron is minute hour day-of-month month day-of-week, e.g. "0 9 * * 1-5"
	// for 9h on weekdays. Lists, ranges and steps; 0 and 7 are Sunday.
	Cron string `json:"cron" jsonschema:"minLength=9"`
	// Timezone overrides the Agent's time zone for this schedule.
	Timezone string `json:"timezone,omitempty"`
	// MaxDelay is how late a scheduled time may still start, after the
	// platform was down (Go duration, default 1h). Older times are skipped,
	// never run in a burst.
	MaxDelay string `json:"maxDelay,omitempty"`
}
