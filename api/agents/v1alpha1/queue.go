package v1alpha1

// HumanQueue is where work for people goes (RFC-0001, "Humano no
// circuito"): who answers is outside Tessera (the console's inbox, a
// channel); what Tessera owns is the deadline. A task waits for its queue;
// past the SLA it reminds, escalates, and at the end does what onTimeout
// says, so no run waits for a person forever.
type HumanQueue struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be HumanQueue.
	Kind string `json:"kind" jsonschema:"enum=HumanQueue"`
	// Metadata names the queue (escalate.queue refers to it).
	Metadata ObjectMeta `json:"metadata"`
	// Spec is the queue's service level.
	Spec HumanQueueSpec `json:"spec"`
}

// HumanQueueSpec describes a queue.
type HumanQueueSpec struct {
	// Description says who works this queue.
	Description string `json:"description,omitempty"`
	// Roles are the Team roles that may answer this queue's tasks (and,
	// after an escalation or a forward, the tasks that reached it). With
	// roles, every answer is a member's: on the task page after a one-time
	// code, on the API by naming the member. Without, anyone the channel
	// trusts answers, as before (RFC-0006).
	Roles []string `json:"roles,omitempty"`
	// Channels are where the queue's tasks reach its members besides the
	// console and links: whatsapp sends each task to the WhatsApp of every
	// member who may answer it, with buttons; a tap from the member's own
	// number (attested by Meta) answers it. Needs roles, and the Agent's
	// channel plugin with a send-buttons action (backoffice-002). slack
	// posts it to the Slack DM of every member who may answer it; a click
	// in a request Slack signs answers it. Needs roles and the slack
	// plugin with post-task and update-task (backoffice-003). teams sends
	// it to the member's personal chat with the bot as an Adaptive Card; a
	// click the Bot Framework signs answers it. Needs roles and the teams
	// plugin with open-dm, post-task and update-task (backoffice-004).
	// email sends it to the member's e-mail with a link bound to them; the
	// link and a one-time code (the Team's verify) answer it, never a reply
	// to the e-mail. Needs roles, the Team's verify, and the email plugin
	// with send-task (backoffice-005).
	Channels []string `json:"channels,omitempty" jsonschema:"uniqueItems=true"`
	// SLA is how long a task may wait, and what happens then.
	SLA *SLA `json:"sla,omitempty"`
	// Notify tells people about the queue's tasks through plugin actions
	// (Slack, WhatsApp, e-mail, any webhook): each is called once when a
	// task is created, reminded or escalated to this queue. The console's
	// inbox needs nothing here.
	Notify []Notify `json:"notify,omitempty"`
}

// Notify is one plugin action called for each task. Its arguments read
// the task as inputs: runId, queue, kind (approve, answer, input,
// takeover, indeterminate), text, stage (created, remind, escalate), link
// (a signed page where the task is answered, when tessera serve has a
// public URL) and since.
type Notify struct {
	// Act is the plugin action, plugin/action.
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// With gives its arguments. Values are literal unless the whole value
	// is "${ expr }" over inputs (the task).
	With map[string]any `json:"with,omitempty"`
}

// SLA is a task's deadlines, counted from when it was created. Each
// happens once; durations are Go durations (4h, 90m), counted in the
// Agent's business hours when businessHours is set.
type SLA struct {
	// Remind sends the task to the queue again after this long.
	Remind string `json:"remind,omitempty"`
	// Escalate sends the task to escalateTo after this long.
	Escalate string `json:"escalate,omitempty"`
	// EscalateTo is the next queue.
	EscalateTo string `json:"escalateTo,omitempty"`
	// Timeout ends the wait after this long, with onTimeout.
	Timeout string `json:"timeout,omitempty"`
	// OnTimeout says what the run does when nobody answered in time.
	OnTimeout *OnTimeout `json:"onTimeout,omitempty"`
	// BusinessHours counts the durations only in working time, in the
	// Agent's time zone.
	BusinessHours *BusinessHours `json:"businessHours,omitempty"`
}

// OnTimeout ends an unanswered task. Exactly one of answer and fail.
type OnTimeout struct {
	// Answer is used as the person's answer (a safe default: false for an
	// approval, a value for an input), recorded as given by the timeout.
	Answer any `json:"answer,omitempty"`
	// Fail ends the run as failed.
	Fail bool `json:"fail,omitempty"`
}

// BusinessHours are working days and hours.
type BusinessHours struct {
	// Days are the working weekdays, 0 (Sunday) to 6; default Monday to
	// Friday.
	Days []int `json:"days,omitempty"`
	// From and To bound the working day, HH:MM.
	From string `json:"from" jsonschema:"pattern=^[0-2][0-9]:[0-5][0-9]$"`
	To   string `json:"to" jsonschema:"pattern=^[0-2][0-9]:[0-5][0-9]$"`
}
