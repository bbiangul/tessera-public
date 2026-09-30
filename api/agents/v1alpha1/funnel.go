package v1alpha1

// Funnel follows the same subjects from one milestone to the next
// (RFC-0001, "Funis e tempo por etapa"): how many get from each stage to
// the next, how long it takes, and who is stuck. Conversion is per cohort
// of subjects, never the ratio of two counts in a window.
type Funnel struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Funnel.
	Kind string `json:"kind" jsonschema:"enum=Funnel"`
	// Metadata names the funnel.
	Metadata ObjectMeta `json:"metadata"`
	// Spec lists the stages.
	Spec FunnelSpec `json:"spec"`
}

// FunnelSpec describes a funnel.
type FunnelSpec struct {
	// Description says what the funnel measures.
	Description string `json:"description,omitempty"`
	// Subject is the Subject kind followed.
	Subject string `json:"subject" jsonschema:"minLength=1"`
	// Stages in order; the first is the entry. A subject reaches a stage
	// with the first milestone of that name after it reached the previous.
	Stages []FunnelStage `json:"stages" jsonschema:"minItems=2"`
	// Cohort groups subjects by when they entered: day, week or month.
	Cohort string `json:"cohort,omitempty" jsonschema:"enum=day,enum=week,enum=month"`
	// Dimensions are Subject attributes the funnel can be cut by.
	Dimensions []string `json:"dimensions,omitempty"`
	// OnStuck starts a Function for each subject waiting for a stage past
	// 1.5 times its expected time, once per subject and stage (checked
	// every few minutes by the scheduler). It receives event.payload
	// {funnel, stage, subject, since, days}; typically it escalates to the
	// team.
	OnStuck *OnStuck `json:"onStuck,omitempty"`
}

// OnStuck names what runs for a stuck subject.
type OnStuck struct {
	// Function to start, in the same namespace.
	Function string `json:"function" jsonschema:"minLength=1"`
}

// FunnelStage is one stage.
type FunnelStage struct {
	// Name of the stage.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Milestone that marks it.
	Milestone string `json:"milestone" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Within is how long after the previous stage it still counts (e.g.
	// 14d, 36h). Until the window closes the subject is pending, not lost;
	// without a window nobody is ever lost.
	Within string `json:"within,omitempty"`
	// Expected is how long this stage should take (e.g. 30d). Subjects
	// waiting for it past 1.5 times this are reported as stuck.
	Expected string `json:"expected,omitempty"`
}
