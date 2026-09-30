package v1alpha1

// Team is who works the agent's queues (RFC-0006): members, their roles,
// and how each is reached to prove who they are. Who may answer a task
// comes from here and from the queue's roles, never from a model.
type Team struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Team.
	Kind string `json:"kind" jsonschema:"enum=Team"`
	// Metadata names the team.
	Metadata ObjectMeta `json:"metadata"`
	// Spec lists the members.
	Spec TeamSpec `json:"spec"`
}

// TeamSpec lists the members and how they prove who they are.
type TeamSpec struct {
	// Members of the team.
	Members []Member `json:"members" jsonschema:"minItems=1"`
	// Verify sends a member a one-time code, so an answer on a task page
	// is theirs (a forwarded link approves nothing). Its arguments read
	// inputs.member (the id), inputs.name, inputs.contact (the member's
	// contact for the channel), inputs.code and inputs.task (the task's
	// text), and inputs.raw: when the contact is an e-mail, the code as an
	// RFC 822 message base64url-encoded, for Gmail. An external-message
	// action: WhatsApp, e-mail, SMS.
	Verify *Verify `json:"verify,omitempty"`
}

// Member is one person.
type Member struct {
	// ID is stable (an answer is recorded with it).
	ID string `json:"id" jsonschema:"pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`
	// Name is shown on task pages.
	Name string `json:"name" jsonschema:"minLength=1"`
	// Roles are the queue roles the member holds.
	Roles []string `json:"roles" jsonschema:"minItems=1"`
	// Contact is where the member's one-time codes go (a phone, an
	// e-mail), as the verify action expects it.
	Contact string `json:"contact,omitempty"`
	// WhatsApp is the member's WhatsApp number, E.164 digits without +
	// (5511900000001): tasks reach it on queues with channels: [whatsapp],
	// and a button tap from it is this member's answer.
	WhatsApp string `json:"whatsapp,omitempty" jsonschema:"pattern=^[1-9][0-9]+$"`
	// Slack is the member's Slack user ID (U0123ABCD): tasks of queues
	// with channels: [slack] reach its DM, and a click Slack signs from it
	// is this member's answer.
	Slack string `json:"slack,omitempty" jsonschema:"pattern=^[UW][A-Z0-9]+$"`
	// Teams is the member's Microsoft Entra object ID (a GUID): tasks of
	// queues with channels: [teams] reach their personal chat with the
	// bot, and a click the Bot Framework signs from it, in the configured
	// tenant, is this member's answer.
	Teams string `json:"teams,omitempty"`
	// Email is the member's e-mail: tasks of queues with channels: [email]
	// reach it with a link bound to this member. The e-mail is never read
	// back; only the link, with a one-time code, answers.
	Email string `json:"email,omitempty"`
	// Attributes are the member's facts a Function may use, such as
	// dentistId: D1. A message from the member's attested identity carries
	// them as caller.attributes, never at the caller's top level.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Verify is the action that sends a one-time code.
type Verify struct {
	// Act is plugin/action.
	Act string `json:"act" jsonschema:"pattern=^[a-z0-9-]+/[a-z0-9-]+$"`
	// With gives its arguments over inputs (member, name, contact, code, task).
	With map[string]any `json:"with,omitempty"`
}
