package providers

func NewAgnes() Adapter {
	return mustNewAdapter("https://apihub.agnes-ai.com/v1", agnesPolicy(agnesCapabilities()))
}

func NewAgnesWithBaseURL(baseURL string) (Adapter, error) {
	return NewAgnesWithCapabilities(baseURL, agnesCapabilities())
}

func NewAgnesWithCapabilities(baseURL string, capabilities Capabilities) (Adapter, error) {
	return newAdapter(baseURL, agnesPolicy(capabilities))
}

func agnesCapabilities() Capabilities {
	return Capabilities{
		Chat: true, Models: true, Streaming: true, Tools: true,
		ToolChoiceAuto: true, ToolChoiceNamed: true, ImageInput: true, ReasoningToggle: true,
		Parameters: ParameterCapabilities{
			MaxOutputTokens: integerAtLeast(1), Temperature: numberBetween(0, 2), TopP: numberBetween(0, 1),
			PresencePenalty: numberBetween(-2, 2), FrequencyPenalty: numberBetween(-2, 2),
		},
	}
}

func agnesPolicy(capabilities Capabilities) wirePolicy {
	return wirePolicy{
		kind:         KindAgnes,
		capabilities: capabilities,
		chatPath:     "chat/completions", modelsPath: "models", reasoning: reasoningWireAgnes,
		maxStops: 4,
		classify: classifyHTTPError, retryAfter: standardRetryAfter,
	}
}
