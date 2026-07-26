package requestflow

import (
	"testing"

	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
)

// This protects the public result that a client learns exactly why a selected
// model rejects a declared parameter before the gateway sends upstream.
func TestModelParameterContractExplainsUnsupportedAndConditionalRequests(t *testing.T) {
	t.Parallel()

	model := Model{
		PublicName: "kimi-k2.6", ProviderSlug: "kimi",
		Capabilities: registry.ModelCapabilities{
			Chat: true, Reasoning: true, ReasoningMode: registry.ReasoningToggle, ReasoningDefaultEnabled: true,
			Parameters: providers.ParameterCapabilities{
				Temperature: providers.NumberParameterLimit{Supported: true, ExactValues: []float64{0.6, 1}},
			},
		},
	}
	thinkingBudget := int64(512)
	unsupported := validateCapabilities(model, canonical.ChatRequest{
		Model: "kimi-k2.6", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}}, ThinkingBudget: &thinkingBudget,
	})
	if unsupported == nil || unsupported.Kind != canonical.ErrorUnsupportedCapability || unsupported.Model != "kimi-k2.6" || unsupported.Provider != "kimi" || unsupported.Capability != "parameters.thinking_budget" {
		t.Fatalf("unsupported parameter error = %#v", unsupported)
	}

	nonThinkingTemperature := 0.6
	thinkingTemperature := 1.0
	enabled := true
	model.Capabilities.Parameters.SamplingConditions = []providers.SamplingCondition{{ThinkingEnabled: &enabled, TemperatureExact: float64Pointer(thinkingTemperature)}}
	conditional := validateCapabilities(model, canonical.ChatRequest{
		Model: "kimi-k2.6", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}}, Temperature: &nonThinkingTemperature,
	})
	if conditional == nil || conditional.Kind != canonical.ErrorInvalidRequest || conditional.Model != "kimi-k2.6" || conditional.Provider != "kimi" || conditional.Capability != "parameters.temperature" {
		t.Fatalf("conditional parameter error = %#v", conditional)
	}
}

// This protects the client-visible result that a model cannot accept a
// message-name field which its declared Provider contract cannot carry.
func TestModelCapabilityRejectsUnsupportedMessageNamesBeforeSending(t *testing.T) {
	t.Parallel()

	model := Model{
		PublicName: "glm-free", ProviderSlug: "zhipu",
		Capabilities: registry.ModelCapabilities{Chat: true},
	}
	err := validateCapabilities(model, canonical.ChatRequest{
		Model: "glm-free", Messages: []canonical.Message{{Role: canonical.RoleUser, Name: "member", Content: canonical.TextContent("Reply")}},
	})
	if err == nil || err.Kind != canonical.ErrorUnsupportedCapability || err.Capability != "messages.name" || err.Provider != "zhipu" || err.Model != "glm-free" {
		t.Fatalf("message-name capability error = %#v", err)
	}
}

func TestModelCapabilityRejectsUnsupportedStreamUsageBeforeSending(t *testing.T) {
	t.Parallel()

	includeUsage := true
	model := Model{
		PublicName: "model-without-stream-usage", ProviderSlug: "internal-provider",
		Capabilities: registry.ModelCapabilities{Chat: true, Streaming: true},
	}
	err := validateCapabilities(model, canonical.ChatRequest{
		Model: "model-without-stream-usage", Stream: true, StreamUsage: &includeUsage,
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
	})
	if err == nil || err.Kind != canonical.ErrorUnsupportedCapability || err.Capability != "usage.stream" || err.Parameter != "stream_options.include_usage" {
		t.Fatalf("stream-usage capability error = %#v", err)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
