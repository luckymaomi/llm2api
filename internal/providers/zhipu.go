package providers

import "github.com/luckymaomi/llm2api/internal/canonical"

func NewZhipu() Adapter {
	return mustNewAdapter("https://open.bigmodel.cn/api/paas/v4", zhipuPolicy(zhipuCapabilities()))
}

func NewZhipuWithBaseURL(baseURL string) (Adapter, error) {
	return NewZhipuWithCapabilities(baseURL, zhipuCapabilities())
}

func NewZhipuWithCapabilities(baseURL string, capabilities Capabilities) (Adapter, error) {
	return newAdapter(baseURL, zhipuPolicy(capabilities))
}

func zhipuCapabilities() Capabilities {
	return Capabilities{
		Chat: true, Models: true, Streaming: true, Tools: true, ToolStreaming: true, ToolChoiceAuto: true,
		JSONOutput: true, ReasoningToggle: true, ReasoningEffort: true, ReasoningContent: true,
		ReasoningReplay: true, ResponseUsage: true, ResponseRequestID: true,
		Parameters: ParameterCapabilities{
			MaxOutputTokens: integerBetween(1, 131_072), Temperature: numberBetween(0, 1), TopP: numberBetween(0.01, 1),
		},
	}
}

func zhipuPolicy(capabilities Capabilities) wirePolicy {
	return wirePolicy{
		kind:         KindZhipu,
		capabilities: capabilities,
		chatPath:     "chat/completions", modelsPath: "models", reasoning: reasoningWireZhipu,
		sendToolStream: true, responseRequestIDBody: true, maxStops: 4,
		finishReasons: map[string]canonical.FinishReason{"sensitive": canonical.FinishReasonContentFilter},
		finishReasonErrors: map[string]canonical.ErrorKind{
			"network_error": canonical.ErrorProviderTemporary, "model_context_window_exceeded": canonical.ErrorInvalidRequest,
		},
		classify: classifyZhipuError, retryAfter: standardRetryAfter,
	}
}

func classifyZhipuError(statusCode int, providerError *wireError) canonical.ErrorKind {
	code := ""
	if providerError != nil {
		code = string(providerError.Code)
	}
	switch code {
	case "1000", "1001", "1002", "1003", "1004", "1110", "1111", "1112":
		return canonical.ErrorAuthentication
	case "1113", "1304", "1308", "1309", "1310":
		return canonical.ErrorQuota
	case "1210", "1213", "1214", "1215", "1261":
		return canonical.ErrorInvalidRequest
	case "1211", "1221", "1222":
		return canonical.ErrorProviderConfiguration
	case "1212":
		return canonical.ErrorUnsupportedCapability
	case "1220", "1301", "1311":
		return canonical.ErrorPermission
	case "1302":
		return canonical.ErrorRateLimit
	case "500", "1120", "1230", "1234", "1305":
		return canonical.ErrorProviderTemporary
	case "1121", "1231", "1300":
		return canonical.ErrorProviderPermanent
	default:
		return classifyHTTPError(statusCode, providerError)
	}
}
