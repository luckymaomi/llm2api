package providers

import (
	"strconv"
	"unicode/utf8"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func (a *openAIAdapter) encodeReasoning(reasoning *canonical.ReasoningConfig, request *wireChatRequest) error {
	if reasoning == nil {
		if a.policy.reasoning == reasoningWireStandard && a.policy.capabilities.ReasoningToggle {
			disabled := false
			request.EnableThinking = &disabled
		}
		return nil
	}
	if reasoning.Enabled != nil && !a.policy.capabilities.ReasoningToggle && !a.policy.capabilities.ReasoningAlwaysOn {
		return a.unsupported("reasoning.enabled")
	}
	if reasoning.Effort != "" && !a.policy.capabilities.ReasoningEffort {
		return a.unsupported("reasoning.effort")
	}
	if reasoning.Preserve != nil && a.policy.reasoning != reasoningWireZhipu && a.policy.reasoning != reasoningWireKimi {
		return a.unsupported("reasoning.preserve")
	}
	if reasoning.Preserve != nil && reasoning.Enabled == nil && !a.policy.capabilities.ReasoningAlwaysOn {
		return a.requestError(canonical.ErrorInvalidRequest, "preserve_without_thinking", "reasoning preservation requires an explicit thinking mode", "reasoning.preserve")
	}
	if reasoning.Enabled != nil && !*reasoning.Enabled && (reasoning.Effort != "" || reasoning.Preserve != nil) {
		return a.requestError(canonical.ErrorInvalidRequest, "reasoning_options_without_thinking", "reasoning effort and preservation require thinking mode", "reasoning")
	}
	if reasoning.Effort != "" && !a.supportsReasoningEffort(reasoning.Effort) {
		return a.requestError(canonical.ErrorInvalidRequest, "invalid_reasoning_effort", "reasoning effort is invalid", "reasoning.effort")
	}

	switch a.policy.reasoning {
	case reasoningWireStandard:
		if reasoning.Enabled != nil {
			request.EnableThinking = reasoning.Enabled
		} else if a.policy.capabilities.ReasoningToggle {
			disabled := false
			request.EnableThinking = &disabled
		}
		request.ReasoningEffort = string(reasoning.Effort)
	case reasoningWireZhipu:
		if reasoning.Enabled != nil || reasoning.Preserve != nil {
			thinking := &wireThinking{}
			if reasoning.Enabled != nil {
				thinking.Type = enabledType(*reasoning.Enabled)
			}
			if reasoning.Preserve != nil {
				clearThinking := !*reasoning.Preserve
				thinking.ClearThinking = &clearThinking
			}
			request.Thinking = thinking
		}
		request.ReasoningEffort = string(reasoning.Effort)
	case reasoningWireAgnes:
		if reasoning.Enabled != nil {
			request.ChatTemplateKwargs = &wireChatTemplateKwargs{EnableThinking: *reasoning.Enabled}
		}
	case reasoningWireKimi:
		if a.policy.capabilities.ReasoningAlwaysOn {
			if reasoning.Enabled != nil && !*reasoning.Enabled {
				return a.unsupported("reasoning.enabled")
			}
			if reasoning.Preserve != nil && !*reasoning.Preserve {
				return a.unsupported("reasoning.preserve")
			}
			if a.policy.capabilities.ReasoningConfig && (reasoning.Enabled != nil || reasoning.Preserve != nil) {
				thinking := &wireThinking{Type: "enabled"}
				if reasoning.Preserve != nil && *reasoning.Preserve {
					thinking.Keep = "all"
				}
				request.Thinking = thinking
			}
			request.ReasoningEffort = string(reasoning.Effort)
			break
		}
		if reasoning.Enabled != nil || reasoning.Preserve != nil {
			thinking := &wireThinking{}
			if reasoning.Enabled != nil {
				thinking.Type = enabledType(*reasoning.Enabled)
			}
			if reasoning.Preserve != nil && *reasoning.Preserve {
				thinking.Keep = "all"
			}
			request.Thinking = thinking
		}
		request.ReasoningEffort = string(reasoning.Effort)
	}
	return nil
}

func (a *openAIAdapter) supportsReasoningEffort(effort canonical.ReasoningEffort) bool {
	if !validReasoningEffort(effort) {
		return false
	}
	if a.policy.allowedReasoningEfforts != nil {
		return a.policy.allowedReasoningEfforts[effort]
	}
	if len(a.policy.capabilities.AllowedReasoningEfforts) > 0 {
		for _, supported := range a.policy.capabilities.AllowedReasoningEfforts {
			if effort == supported {
				return true
			}
		}
		return false
	}
	return true
}

func (a *openAIAdapter) validateParameters(request canonical.ChatRequest) error {
	if err := a.validateReasoningReplay(request); err != nil {
		return err
	}
	parameters := a.policy.capabilities.Parameters
	if err := a.validateInteger("max_tokens", request.MaxOutputTokens, parameters.MaxOutputTokens); err != nil {
		return err
	}
	if err := a.validateInteger("n", request.N, parameters.N); err != nil {
		return err
	}
	if err := a.validateNumber("temperature", request.Temperature, parameters.Temperature); err != nil {
		return err
	}
	if err := a.validateNumber("top_p", request.TopP, parameters.TopP); err != nil {
		return err
	}
	if err := a.validateNumber("top_k", request.TopK, parameters.TopK); err != nil {
		return err
	}
	if err := a.validateNumber("presence_penalty", request.PresencePenalty, parameters.PresencePenalty); err != nil {
		return err
	}
	if err := a.validateNumber("frequency_penalty", request.FrequencyPenalty, parameters.FrequencyPenalty); err != nil {
		return err
	}
	if err := a.validateInteger("thinking_budget", request.ThinkingBudget, parameters.ThinkingBudget); err != nil {
		return err
	}
	if err := a.validateSamplingConditions(request, parameters.SamplingConditions); err != nil {
		return err
	}
	if a.policy.maxStops > 0 && len(request.Stop) > a.policy.maxStops {
		return a.requestError(canonical.ErrorInvalidRequest, "too_many_stop_sequences", "stop sequence count exceeds provider limit", "stop")
	}
	for _, stop := range request.Stop {
		if stop == "" {
			return a.requestError(canonical.ErrorInvalidRequest, "empty_stop_sequence", "stop sequences must not be empty", "stop")
		}
	}
	if request.ResponseFormat != nil {
		if request.ResponseFormat.Type == canonical.ResponseFormatText {
			// Text is the default Chat Completions response and requires no
			// provider-specific response_format wire field.
		} else if request.ResponseFormat.Type == canonical.ResponseFormatJSONSchema {
			if !a.policy.capabilities.JSONSchemaOutput {
				return a.unsupported("response_format.json_schema")
			}
		} else if !a.policy.capabilities.JSONOutput || (request.ResponseFormat.Type != canonical.ResponseFormatText && request.ResponseFormat.Type != canonical.ResponseFormatJSONObject) {
			return a.unsupported("response_format")
		}
	}
	if request.ParallelToolCalls != nil && !a.policy.capabilities.ParallelToolCalls {
		return a.unsupported("parallel_tool_calls")
	}
	if request.ToolChoice != nil && len(a.policy.capabilities.ToolChoiceModesWithReasoning) > 0 && reasoningEnabledByRequest(a.policy.capabilities, request) && !containsToolChoiceMode(a.policy.capabilities.ToolChoiceModesWithReasoning, request.ToolChoice.Mode) {
		return a.requestError(canonical.ErrorInvalidRequest, "tool_choice_with_thinking", "this model only accepts auto or none tool_choice while thinking is enabled", "tool_choice")
	}
	if request.PromptCacheKey != "" && !a.policy.capabilities.PromptCacheKey {
		return a.unsupported("prompt_cache_key")
	}
	if request.SafetyIdentifier != "" && !a.policy.capabilities.SafetyIdentifier {
		return a.unsupported("safety_identifier")
	}
	if a.policy.responseRequestIDBody && request.RequestID != "" {
		requestIDLength := utf8.RuneCountInString(request.RequestID)
		if requestIDLength < 6 || requestIDLength > 64 {
			return a.requestError(canonical.ErrorInvalidRequest, "invalid_request_id", "request ID must contain 6-64 characters", "request_id")
		}
	}
	if a.policy.rejectSamplingWithReasoning && (request.Reasoning == nil || request.Reasoning.Enabled == nil || *request.Reasoning.Enabled) {
		if request.Temperature != nil || request.TopP != nil || request.PresencePenalty != nil || request.FrequencyPenalty != nil {
			return a.requestError(canonical.ErrorInvalidRequest, "sampling_with_thinking", "sampling parameters are not effective in thinking mode", "reasoning")
		}
	}
	return nil
}

func (a *openAIAdapter) validateReasoningReplay(request canonical.ChatRequest) error {
	replayRequired := false
	switch a.policy.reasoning {
	case reasoningWireZhipu:
		replayRequired = request.Reasoning != nil && request.Reasoning.Enabled != nil && *request.Reasoning.Enabled &&
			request.Reasoning.Preserve != nil && *request.Reasoning.Preserve
	}
	if !replayRequired {
		return nil
	}
	for index, message := range request.Messages {
		if message.Role == canonical.RoleAssistant && len(message.ToolCalls) > 0 && message.Reasoning == nil {
			return a.requestError(canonical.ErrorInvalidRequest, "missing_reasoning_replay", "assistant tool calls require their preserved reasoning content", "messages["+strconv.Itoa(index)+"].reasoning")
		}
	}
	return nil
}

func (a *openAIAdapter) validateNumber(parameter string, value *float64, limit NumberParameterLimit) error {
	if value == nil {
		return nil
	}
	if !limit.Supported {
		return a.unsupported(parameter)
	}
	if (limit.Minimum != nil && *value < *limit.Minimum) || (limit.Maximum != nil && *value > *limit.Maximum) || (len(limit.ExactValues) > 0 && !containsNumber(limit.ExactValues, *value)) {
		return a.requestError(canonical.ErrorInvalidRequest, "parameter_out_of_range", parameter+" is outside the provider range", parameter)
	}
	return nil
}

func (a *openAIAdapter) validateInteger(parameter string, value *int64, limit IntegerParameterLimit) error {
	if value == nil {
		return nil
	}
	if !limit.Supported {
		return a.unsupported(parameter)
	}
	if (limit.Minimum != nil && *value < *limit.Minimum) || (limit.Maximum != nil && *value > *limit.Maximum) || (len(limit.ExactValues) > 0 && !containsInteger(limit.ExactValues, *value)) {
		return a.requestError(canonical.ErrorInvalidRequest, "parameter_out_of_range", parameter+" is outside the provider range", parameter)
	}
	return nil
}

func (a *openAIAdapter) validateSamplingConditions(request canonical.ChatRequest, conditions []SamplingCondition) error {
	for _, condition := range conditions {
		if condition.ThinkingEnabled != nil && *condition.ThinkingEnabled != reasoningEnabledByRequest(a.policy.capabilities, request) {
			continue
		}
		if condition.TemperatureExact != nil && request.Temperature != nil && *request.Temperature != *condition.TemperatureExact {
			return a.requestError(canonical.ErrorInvalidRequest, "parameter_condition_violation", "temperature conflicts with the model's thinking-mode requirement", "temperature")
		}
		if condition.TemperatureAtMost != nil && request.Temperature != nil && *request.Temperature <= *condition.TemperatureAtMost && condition.NMaximum != nil && request.N != nil && *request.N > *condition.NMaximum {
			return a.requestError(canonical.ErrorInvalidRequest, "parameter_condition_violation", "n exceeds the model limit for this temperature", "n")
		}
	}
	return nil
}

func reasoningEnabledByRequest(capabilities Capabilities, request canonical.ChatRequest) bool {
	if request.Reasoning != nil && request.Reasoning.Enabled != nil {
		return *request.Reasoning.Enabled
	}
	return capabilities.ReasoningAlwaysOn || capabilities.ReasoningDefaultEnabled
}

func containsToolChoiceMode(values []canonical.ToolChoiceMode, value canonical.ToolChoiceMode) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func containsNumber(values []float64, value float64) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func containsInteger(values []int64, value int64) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
