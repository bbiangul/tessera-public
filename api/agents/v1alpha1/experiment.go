package v1alpha1

// Experiment is an A/B test (RFC-0001, "Experimentos"): each subject gets
// one variant, always the same, and the variants are compared on a Funnel
// stage with a sequential test, so looking at the results every day does
// not produce false winners. Promoting a winner is a person's decision: it
// is a change to the definitions, never automatic.
type Experiment struct {
	// APIVersion must be agents.tessera.dev/v1alpha1.
	APIVersion string `json:"apiVersion" jsonschema:"enum=agents.tessera.dev/v1alpha1"`
	// Kind must be Experiment.
	Kind string `json:"kind" jsonschema:"enum=Experiment"`
	// Metadata names the experiment; the name seeds the assignment.
	Metadata ObjectMeta `json:"metadata"`
	// Spec describes the variants and how they are judged.
	Spec ExperimentSpec `json:"spec"`
}

// ExperimentSpec describes an experiment.
type ExperimentSpec struct {
	// Description says what is tested.
	Description string `json:"description,omitempty"`
	// Subject is the Subject kind assigned (by its id).
	Subject string `json:"subject" jsonschema:"minLength=1"`
	// Variants; the first is the control. Weights are relative.
	Variants []ExperimentVariant `json:"variants" jsonschema:"minItems=2"`
	// Primary is the Funnel stage that decides: conversion to it, among
	// the subjects exposed to each variant.
	Primary PrimaryMetric `json:"primary"`
	// MinSamplePerVariant is how many decided subjects each variant needs
	// before any result is declared. Default 100.
	MinSamplePerVariant int `json:"minSamplePerVariant,omitempty" jsonschema:"minimum=1"`
	// Alpha is the false positive rate accepted. Default 0.05.
	Alpha float64 `json:"alpha,omitempty" jsonschema:"exclusiveMinimum=0,maximum=0.2"`
	// Guardrails are outcomes that must not get worse (opt-outs,
	// complaints, escalations): when a variant is significantly worse than
	// the control by more than maxIncrease, the platform stops the
	// experiment (every subject exposed after that gets the control) and
	// starts onBreach once. Checked every few minutes by the scheduler.
	Guardrails []Guardrail `json:"guardrails,omitempty"`
	// OnBreach starts a Function when a guardrail stops the experiment,
	// with event.payload {experiment, guardrail, variant, controlRate,
	// variantRate, difference}; typically it escalates to the team.
	// Required with guardrails.
	OnBreach *OnStuck `json:"onBreach,omitempty"`
}

// Guardrail is an outcome that must not rise: the share of exposed
// subjects with a milestone within a window after their exposure.
type Guardrail struct {
	// Name of the guardrail, e.g. opt-out.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Milestone of the experiment's Subject that counts, e.g. opt-out.
	Milestone string `json:"milestone" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Within is how long after the exposure it counts (e.g. 14d, 72h).
	// Until then the subject is pending.
	Within string `json:"within"`
	// MaxIncrease is how much worse than the control a variant may be,
	// in rate points (0.02: two points, e.g. 3% to 5%).
	MaxIncrease float64 `json:"maxIncrease" jsonschema:"exclusiveMinimum=0,maximum=1"`
}

// ExperimentVariant is one arm.
type ExperimentVariant struct {
	// Name of the variant, read by steps.<experiment step>.output.
	Name string `json:"name" jsonschema:"pattern=^[a-z][a-z0-9-]*$"`
	// Weight is the share of subjects, relative to the others.
	Weight int `json:"weight" jsonschema:"minimum=1"`
}

// PrimaryMetric is a Funnel stage.
type PrimaryMetric struct {
	// Funnel name.
	Funnel string `json:"funnel" jsonschema:"minLength=1"`
	// Stage of the funnel that counts as conversion.
	Stage string `json:"stage" jsonschema:"minLength=1"`
}
