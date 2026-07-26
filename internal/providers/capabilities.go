package providers

import "github.com/luckymaomi/llm2api/internal/canonical"

// ParameterCapabilities is part of a model's public capability contract. A
// missing upper bound means the Provider has not published one, not infinity.
type ParameterCapabilities struct {
	MaxOutputTokens    IntegerParameterLimit `json:"max_completion_tokens"`
	Temperature        NumberParameterLimit  `json:"temperature"`
	TopP               NumberParameterLimit  `json:"top_p"`
	PresencePenalty    NumberParameterLimit  `json:"presence_penalty"`
	FrequencyPenalty   NumberParameterLimit  `json:"frequency_penalty"`
	N                  IntegerParameterLimit `json:"n"`
	TopK               NumberParameterLimit  `json:"top_k"`
	ThinkingBudget     IntegerParameterLimit `json:"thinking_budget"`
	SamplingConditions []SamplingCondition   `json:"sampling_conditions,omitempty"`
}

type IntegerParameterLimit struct {
	Supported   bool    `json:"supported"`
	Minimum     *int64  `json:"minimum,omitempty"`
	Maximum     *int64  `json:"maximum,omitempty"`
	ExactValues []int64 `json:"exact_values,omitempty"`
}

type NumberParameterLimit struct {
	Supported   bool      `json:"supported"`
	Minimum     *float64  `json:"minimum,omitempty"`
	Maximum     *float64  `json:"maximum,omitempty"`
	ExactValues []float64 `json:"exact_values,omitempty"`
}

// SamplingCondition captures an upstream rule that only applies in a
// particular reasoning mode. It keeps model-specific constraints out of
// generic model-name inference.
type SamplingCondition struct {
	ThinkingEnabled   *bool    `json:"thinking_enabled,omitempty"`
	TemperatureExact  *float64 `json:"temperature_exact,omitempty"`
	TemperatureAtMost *float64 `json:"temperature_at_most,omitempty"`
	NMaximum          *int64   `json:"n_maximum,omitempty"`
}

type Capabilities struct {
	Chat                         bool
	Models                       bool
	Responses                    bool
	Streaming                    bool
	Tools                        bool
	ToolStreaming                bool
	ToolChoiceNone               bool
	ToolChoiceAuto               bool
	ToolChoiceRequired           bool
	ToolChoiceNamed              bool
	ParallelToolCalls            bool
	StrictTools                  bool
	ImageInput                   bool
	VideoInput                   bool
	PartialMode                  bool
	JSONOutput                   bool
	JSONSchemaOutput             bool
	MessageName                  bool
	PromptCacheKey               bool
	SafetyIdentifier             bool
	ReasoningToggle              bool
	ReasoningAlwaysOn            bool
	ReasoningDefaultEnabled      bool
	ReasoningConfig              bool
	ReasoningEffort              bool
	ReasoningContent             bool
	ReasoningReplay              bool
	AllowedReasoningEfforts      []canonical.ReasoningEffort
	ToolChoiceModesWithReasoning []canonical.ToolChoiceMode
	Parameters                   ParameterCapabilities
	ResponseUsage                bool
	StreamUsage                  bool
	ResponseRequestID            bool
}

func SiliconFlowCapabilities() Capabilities {
	return Capabilities{
		Chat:              true,
		Models:            true,
		Streaming:         true,
		Tools:             true,
		ToolStreaming:     false,
		ParallelToolCalls: false,
		StrictTools:       false,
		JSONOutput:        true,
		JSONSchemaOutput:  true,
		MessageName:       true,
		ReasoningEffort:   false,
		ResponseUsage:     true,
		StreamUsage:       true,
		Parameters: ParameterCapabilities{
			MaxOutputTokens:  integerAtLeast(1),
			Temperature:      numberBetween(0, 2),
			TopP:             numberBetween(0.1, 1),
			PresencePenalty:  numberBetween(-2, 2),
			FrequencyPenalty: numberBetween(-2, 2),
			N:                integerAtLeast(1),
			TopK:             numberAtMost(100),
		},
	}
}

func integerAtLeast(minimum int64) IntegerParameterLimit {
	return IntegerParameterLimit{Supported: true, Minimum: &minimum}
}

func integerBetween(minimum, maximum int64) IntegerParameterLimit {
	return IntegerParameterLimit{Supported: true, Minimum: &minimum, Maximum: &maximum}
}

func fixedIntegers(values ...int64) IntegerParameterLimit {
	return IntegerParameterLimit{Supported: true, ExactValues: append([]int64(nil), values...)}
}

func numberBetween(minimum, maximum float64) NumberParameterLimit {
	return NumberParameterLimit{Supported: true, Minimum: &minimum, Maximum: &maximum}
}

func numberAtMost(maximum float64) NumberParameterLimit {
	return NumberParameterLimit{Supported: true, Maximum: &maximum}
}

func fixedNumbers(values ...float64) NumberParameterLimit {
	return NumberParameterLimit{Supported: true, ExactValues: append([]float64(nil), values...)}
}

func ModelDiscoveryCapabilities() Capabilities {
	return Capabilities{Chat: true, Models: true}
}
