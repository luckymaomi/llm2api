package providers

import (
	"fmt"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

// ModelProfile is the verified, code-owned capability contract for one
// upstream model identifier. It is deliberately exact rather than inferred
// from a client-facing model-name prefix.
type ModelProfile struct {
	UpstreamName  string
	Capabilities  Capabilities
	ContextTokens int64
	OutputTokens  int64
}

func validateModelProfiles(profiles []ModelProfile) error {
	if len(profiles) == 0 {
		return fmt.Errorf("at least one model profile is required")
	}
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if profile.UpstreamName == "" || !profile.Capabilities.Chat || !profile.Capabilities.Models {
			return fmt.Errorf("model name and chat/models capabilities are required")
		}
		if profile.ContextTokens < 0 || profile.OutputTokens < 0 ||
			(profile.ContextTokens > 0 && profile.OutputTokens > profile.ContextTokens) {
			return fmt.Errorf("model token limits are invalid")
		}
		if _, found := seen[profile.UpstreamName]; found {
			return fmt.Errorf("duplicate model profile %q", profile.UpstreamName)
		}
		seen[profile.UpstreamName] = struct{}{}
	}
	return nil
}

func cloneModelProfiles(profiles []ModelProfile) []ModelProfile {
	result := make([]ModelProfile, len(profiles))
	copy(result, profiles)
	for index := range result {
		result[index].Capabilities = cloneCapabilities(result[index].Capabilities)
	}
	return result
}

func cloneModelProfile(profile ModelProfile) ModelProfile {
	profile.Capabilities = cloneCapabilities(profile.Capabilities)
	return profile
}

func cloneCapabilities(value Capabilities) Capabilities {
	if value.AllowedReasoningEfforts != nil {
		value.AllowedReasoningEfforts = append([]canonical.ReasoningEffort(nil), value.AllowedReasoningEfforts...)
	}
	if value.ToolChoiceModesWithReasoning != nil {
		value.ToolChoiceModesWithReasoning = append([]canonical.ToolChoiceMode(nil), value.ToolChoiceModesWithReasoning...)
	}
	value.Parameters = cloneParameterCapabilities(value.Parameters)
	return value
}

func cloneParameterCapabilities(value ParameterCapabilities) ParameterCapabilities {
	value.MaxOutputTokens = cloneIntegerParameterLimit(value.MaxOutputTokens)
	value.Temperature = cloneNumberParameterLimit(value.Temperature)
	value.TopP = cloneNumberParameterLimit(value.TopP)
	value.PresencePenalty = cloneNumberParameterLimit(value.PresencePenalty)
	value.FrequencyPenalty = cloneNumberParameterLimit(value.FrequencyPenalty)
	value.N = cloneIntegerParameterLimit(value.N)
	value.TopK = cloneNumberParameterLimit(value.TopK)
	value.ThinkingBudget = cloneIntegerParameterLimit(value.ThinkingBudget)
	if value.SamplingConditions != nil {
		value.SamplingConditions = make([]SamplingCondition, len(value.SamplingConditions))
		for index, condition := range value.SamplingConditions {
			value.SamplingConditions[index] = cloneSamplingCondition(condition)
		}
	}
	return value
}

// CloneParameterCapabilities is used at the registry boundary so public and
// persisted projections never retain mutable slices or pointers from catalog
// definitions.
func CloneParameterCapabilities(value ParameterCapabilities) ParameterCapabilities {
	return cloneParameterCapabilities(value)
}

func cloneIntegerParameterLimit(value IntegerParameterLimit) IntegerParameterLimit {
	if value.Minimum != nil {
		minimum := *value.Minimum
		value.Minimum = &minimum
	}
	if value.Maximum != nil {
		maximum := *value.Maximum
		value.Maximum = &maximum
	}
	value.ExactValues = append([]int64(nil), value.ExactValues...)
	return value
}

func cloneNumberParameterLimit(value NumberParameterLimit) NumberParameterLimit {
	if value.Minimum != nil {
		minimum := *value.Minimum
		value.Minimum = &minimum
	}
	if value.Maximum != nil {
		maximum := *value.Maximum
		value.Maximum = &maximum
	}
	value.ExactValues = append([]float64(nil), value.ExactValues...)
	return value
}

func cloneSamplingCondition(value SamplingCondition) SamplingCondition {
	if value.ThinkingEnabled != nil {
		thinkingEnabled := *value.ThinkingEnabled
		value.ThinkingEnabled = &thinkingEnabled
	}
	if value.TemperatureExact != nil {
		temperature := *value.TemperatureExact
		value.TemperatureExact = &temperature
	}
	if value.TemperatureAtMost != nil {
		temperature := *value.TemperatureAtMost
		value.TemperatureAtMost = &temperature
	}
	if value.NMaximum != nil {
		maximum := *value.NMaximum
		value.NMaximum = &maximum
	}
	return value
}

func siliconFlowModel(name string, contextTokens, outputTokens int64, reasoning bool) ModelProfile {
	capabilities := SiliconFlowCapabilities()
	capabilities.ImageInput = false
	capabilities.ReasoningToggle = reasoning
	capabilities.ReasoningEffort = false
	capabilities.ReasoningContent = reasoning
	capabilities.ParallelToolCalls = false
	if reasoning {
		capabilities.Parameters.ThinkingBudget = integerBetween(128, 32_768)
	}
	return ModelProfile{UpstreamName: name, Capabilities: capabilities, ContextTokens: contextTokens, OutputTokens: outputTokens}
}

func zhipuTextModel(name string, contextTokens, outputTokens int64, effort, toolStreaming bool) ModelProfile {
	capabilities := zhipuCapabilities()
	capabilities.ImageInput = false
	capabilities.ToolStreaming = toolStreaming
	capabilities.ReasoningEffort = effort
	if effort {
		capabilities.AllowedReasoningEfforts = []canonical.ReasoningEffort{
			canonical.ReasoningEffortNone, canonical.ReasoningEffortMinimal, canonical.ReasoningEffortLow,
			canonical.ReasoningEffortMedium, canonical.ReasoningEffortHigh, canonical.ReasoningEffortXHigh,
			canonical.ReasoningEffortMax,
		}
	}
	return ModelProfile{UpstreamName: name, Capabilities: capabilities, ContextTokens: contextTokens, OutputTokens: outputTokens}
}

func zhipuVisionModel(name string, contextTokens, outputTokens int64) ModelProfile {
	profile := zhipuTextModel(name, contextTokens, outputTokens, false, false)
	profile.Capabilities.ImageInput = true
	return profile
}

func agnesTextModel(name string, contextTokens, outputTokens int64, advanced bool) ModelProfile {
	capabilities := agnesCapabilities()
	if !advanced {
		capabilities.Tools = false
		capabilities.ToolChoiceAuto = false
		capabilities.ToolChoiceNamed = false
		capabilities.ReasoningToggle = false
	}
	return ModelProfile{UpstreamName: name, Capabilities: capabilities, ContextTokens: contextTokens, OutputTokens: outputTokens}
}

func kimiK3Model() ModelProfile {
	return ModelProfile{UpstreamName: "kimi-k3", Capabilities: kimiK3Capabilities(), ContextTokens: 1_000_000}
}

func kimiK27CodeModel(name string) ModelProfile {
	return ModelProfile{UpstreamName: name, Capabilities: kimiK27CodeCapabilities(), ContextTokens: 256_000}
}

func kimiK26Model() ModelProfile {
	return ModelProfile{UpstreamName: "kimi-k2.6", Capabilities: kimiK26Capabilities(), ContextTokens: 256_000}
}

func kimiK25Model() ModelProfile {
	return ModelProfile{UpstreamName: "kimi-k2.5", Capabilities: kimiK25Capabilities(), ContextTokens: 256_000}
}
