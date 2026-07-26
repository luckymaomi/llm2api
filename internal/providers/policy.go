package providers

import (
	"encoding/json"
	"net/http"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

type reasoningWire string

const (
	reasoningWireStandard reasoningWire = "standard"
	reasoningWireZhipu    reasoningWire = "zhipu"
	reasoningWireAgnes    reasoningWire = "agnes"
	reasoningWireKimi     reasoningWire = "kimi"
)

type outputTokenWire string

const (
	outputTokenWireMaxTokens           outputTokenWire = "max_tokens"
	outputTokenWireMaxCompletionTokens outputTokenWire = "max_completion_tokens"
)

type wirePolicy struct {
	kind                        Kind
	capabilities                Capabilities
	chatPath                    string
	modelsPath                  string
	statusPath                  string
	statusKind                  StatusProbeKind
	reasoning                   reasoningWire
	outputTokens                outputTokenWire
	includeStreamUsage          bool
	sendToolStream              bool
	responseRequestIDBody       bool
	streamRequestIDBody         bool
	responseRequestIDHeader     string
	maxTools                    int
	maxStops                    int
	rejectSamplingWithReasoning bool
	allowedReasoningEfforts     map[canonical.ReasoningEffort]bool
	finishReasons               map[string]canonical.FinishReason
	finishReasonErrors          map[string]canonical.ErrorKind
	transformToolSchema         func(json.RawMessage) (json.RawMessage, error)
	classify                    func(int, *wireError) canonical.ErrorKind
	retryAfter                  func(http.Header, *wireError) *canonical.RetryAfter
	replaySafe                  func(int, *wireError) bool
}

func siliconFlowPolicy(capabilities Capabilities, requestIDHeader string) wirePolicy {
	return wirePolicy{
		kind:                    KindSiliconFlow,
		capabilities:            capabilities,
		chatPath:                "chat/completions",
		modelsPath:              "models",
		reasoning:               reasoningWireStandard,
		includeStreamUsage:      capabilities.StreamUsage,
		responseRequestIDHeader: requestIDHeader,
		maxTools:                128,
		maxStops:                4,
		classify:                classifyHTTPError,
		retryAfter:              standardRetryAfter,
	}
}
