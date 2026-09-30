package v1alpha1

// DecideBatchStep groups independent questions without changing their dependencies.
// Later steps read output.answers.<name>.value and probability. A low-confidence
// answer has value null when onLowConfidence is continue. output.value is the map
// of values only when every question meets the threshold; otherwise it is null.
// Never treat a null answer as permission or as an absent user restriction.
type DecideBatchStep struct {
	// State is shared by every question and masked before leaving the process.
	State map[string]any `json:"state" jsonschema:"minProperties=1"`
	// Questions names independent questions; they cannot read each other's answers.
	Questions map[string]BatchQuestion `json:"questions" jsonschema:"minProperties=1"`
	// MinConfidence is the minimum winning probability for each answer.
	MinConfidence float64 `json:"minConfidence" jsonschema:"exclusiveMinimum=0,maximum=1"`
	// OnLowConfidence is fail (run onError) or continue (null only uncertain answers).
	// Provider errors or missing answers always run onError, even with continue.
	OnLowConfidence string `json:"onLowConfidence" jsonschema:"enum=fail,enum=continue"`
}

// BatchQuestion asks exactly one choice, yesNo or score question.
type BatchQuestion struct {
	// Instructions say what to decide from the shared state.
	Instructions string `json:"instructions" jsonschema:"minLength=1"`
	// Choice maps option keys to descriptions.
	Choice map[string]string `json:"choice,omitempty" jsonschema:"minProperties=2,maxProperties=255"`
	// YesNo asks a yes/no question.
	YesNo *YesNo `json:"yesNo,omitempty"`
	// Score lists ordered levels, lowest first.
	Score []string `json:"score,omitempty" jsonschema:"minItems=2,maxItems=10"`
}
