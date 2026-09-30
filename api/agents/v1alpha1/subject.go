package v1alpha1

// Subject is who business numbers are about (RFC-0001, "Métricas de
// negócio"): a lead, a patient, a store. Milestones name a subject;
// funnels follow the same subjects from one milestone to the next.
type Subject struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Subject.
	Kind string `json:"kind" jsonschema:"enum=Subject"`
	// Metadata names the subject kind, e.g. paciente.
	Metadata ObjectMeta `json:"metadata"`
	// Spec says how a subject is identified and described.
	Spec SubjectSpec `json:"spec"`
}

// SubjectSpec describes a subject kind.
type SubjectSpec struct {
	// Description says who the subjects are.
	Description string `json:"description,omitempty"`
	// ID is a string expression that identifies the subject where a
	// milestone is recorded, e.g. caller.patientId. Use a pseudonymous ID,
	// never a phone or an e-mail.
	ID Expr `json:"id" jsonschema:"minLength=1"`
	// Attributes are the dimensions numbers can be cut by. They are typed
	// and closed: free text could carry personal data.
	Attributes map[string]SubjectAttribute `json:"attributes,omitempty"`
}

// SubjectAttribute is one dimension.
type SubjectAttribute struct {
	// Type is string, int or bool.
	Type string `json:"type" jsonschema:"enum=string,enum=int,enum=bool"`
	// Enum lists the allowed values of a string attribute; required for
	// strings, so no free text is stored.
	Enum []string `json:"enum,omitempty"`
}

// MilestoneRef records a business milestone when the step or the action
// it is on succeeds. For an effect that means confirmed: a cancelled or
// unverified effect records nothing. It is recorded once, however often
// the run is resumed.
type MilestoneRef struct {
	// Name of the milestone, e.g. agendou.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Subject is the Subject kind, in the same namespace.
	Subject string `json:"subject" jsonschema:"minLength=1"`
	// ID overrides the Subject's id expression here.
	ID Expr `json:"id,omitempty"`
	// Attributes are expressions for the Subject's declared attributes.
	Attributes map[string]Expr `json:"attributes,omitempty"`
}
