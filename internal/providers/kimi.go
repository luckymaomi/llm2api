package providers

import "github.com/luckymaomi/llm2api/internal/canonical"

func NewKimi() Adapter {
	return mustNewAdapter("https://api.moonshot.cn/v1", kimiPolicy(kimiK3Capabilities()))
}

func NewKimiWithBaseURL(baseURL string) (Adapter, error) {
	return NewKimiWithCapabilities(baseURL, kimiK3Capabilities())
}

func NewKimiWithCapabilities(baseURL string, capabilities Capabilities) (Adapter, error) {
	return newAdapter(baseURL, kimiPolicy(capabilities))
}

func kimiK3Capabilities() Capabilities {
	return Capabilities{
		Chat: true, Models: true, Streaming: true, Tools: true, ToolStreaming: true,
		ToolChoiceNone: true, ToolChoiceAuto: true, ToolChoiceRequired: true, ToolChoiceNamed: true,
		StrictTools: true, ImageInput: true, VideoInput: true, PartialMode: true, JSONOutput: true, JSONSchemaOutput: true,
		MessageName: true, PromptCacheKey: true, SafetyIdentifier: true,
		ReasoningAlwaysOn: true, ReasoningContent: true, ReasoningReplay: true, ReasoningEffort: true,
		AllowedReasoningEfforts: []canonical.ReasoningEffort{canonical.ReasoningEffortLow, canonical.ReasoningEffortHigh, canonical.ReasoningEffortMax},
		ResponseUsage:           true, StreamUsage: true,
		Parameters: ParameterCapabilities{
			MaxOutputTokens: integerAtLeast(1), Temperature: numberBetween(0, 1), N: integerAtLeast(1),
		},
	}
}

func kimiK27CodeCapabilities() Capabilities {
	capabilities := kimiK3Capabilities()
	capabilities.ToolChoiceRequired = false
	capabilities.ReasoningEffort = false
	capabilities.AllowedReasoningEfforts = nil
	capabilities.ReasoningConfig = true
	capabilities.ImageInput = false
	capabilities.VideoInput = false
	return capabilities
}

func kimiK26Capabilities() Capabilities {
	capabilities := kimiK3Capabilities()
	capabilities.ToolChoiceRequired = false
	capabilities.ReasoningAlwaysOn = false
	capabilities.ReasoningToggle = true
	capabilities.ReasoningEffort = false
	capabilities.AllowedReasoningEfforts = nil
	capabilities.ReasoningDefaultEnabled = true
	capabilities.ToolChoiceModesWithReasoning = []canonical.ToolChoiceMode{canonical.ToolChoiceAuto, canonical.ToolChoiceNone}
	capabilities.Parameters.Temperature = fixedNumbers(0.6, 1)
	capabilities.Parameters.TopP = fixedNumbers(0.95)
	capabilities.Parameters.N = fixedIntegers(1)
	capabilities.Parameters.PresencePenalty = fixedNumbers(0)
	capabilities.Parameters.FrequencyPenalty = fixedNumbers(0)
	enabled, disabled := true, false
	thinkingTemperature, nonThinkingTemperature := 1.0, 0.6
	capabilities.Parameters.SamplingConditions = []SamplingCondition{
		{ThinkingEnabled: &enabled, TemperatureExact: &thinkingTemperature},
		{ThinkingEnabled: &disabled, TemperatureExact: &nonThinkingTemperature},
	}
	return capabilities
}

func kimiK25Capabilities() Capabilities {
	capabilities := kimiK26Capabilities()
	capabilities.VideoInput = false
	return capabilities
}

func kimiPolicy(capabilities Capabilities) wirePolicy {
	return wirePolicy{
		kind: KindKimi, capabilities: capabilities,
		chatPath: "chat/completions", modelsPath: "models", statusPath: "users/me/balance", statusKind: StatusProbeKimiBalance, reasoning: reasoningWireKimi,
		outputTokens:       outputTokenWireMaxCompletionTokens,
		includeStreamUsage: capabilities.StreamUsage,
		classify:           classifyHTTPError, retryAfter: standardRetryAfter,
	}
}
