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

func float64Pointer(value float64) *float64 {
	return &value
}
