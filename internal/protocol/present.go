package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

const PublicProvider = "llm2api"

type ErrorEnvelope struct {
	Error struct {
		Message    string `json:"message"`
		Type       string `json:"type"`
		Param      string `json:"param,omitempty"`
		Code       string `json:"code"`
		Model      string `json:"model,omitempty"`
		Provider   string `json:"provider,omitempty"`
		Capability string `json:"capability,omitempty"`
	} `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

func PresentChatResponse(response canonical.ChatResponse) map[string]any {
	choices := make([]map[string]any, 0, len(response.Choices))
	for _, choice := range response.Choices {
		choices = append(choices, map[string]any{"index": choice.Index, "message": presentMessage(choice.Message), "finish_reason": choice.FinishReason})
	}
	result := map[string]any{"id": response.ID, "object": "chat.completion", "created": unixSeconds(response.CreatedAt), "model": response.Model, "choices": choices}
	if response.Usage != nil {
		result["usage"] = presentUsage(*response.Usage)
	}
	return result
}

func PresentStreamEvent(event canonical.StreamEvent) map[string]any {
	delta := map[string]any{}
	switch event.Type {
	case canonical.StreamMessageStart:
		delta["role"] = event.Role
	case canonical.StreamContentDelta:
		delta["content"] = event.ContentDelta
	case canonical.StreamReasoningDelta:
		delta["reasoning_content"] = event.ReasoningDelta
	case canonical.StreamToolCallDelta:
		if event.ToolCallDelta != nil {
			call := map[string]any{"index": event.ToolCallDelta.Index, "id": event.ToolCallDelta.ID, "type": event.ToolCallDelta.Type, "function": map[string]any{"name": event.ToolCallDelta.FunctionName, "arguments": event.ToolCallDelta.ArgumentsFragment}}
			delta["tool_calls"] = []map[string]any{call}
		}
	}
	choice := map[string]any{"index": event.ChoiceIndex, "delta": delta, "finish_reason": nil}
	if event.Type == canonical.StreamFinish {
		choice["finish_reason"] = event.FinishReason
	}
	result := map[string]any{"id": event.CompletionID, "object": "chat.completion.chunk", "created": time.Now().UTC().Unix(), "model": event.Model, "choices": []map[string]any{choice}}
	if event.Type == canonical.StreamUsage && event.Usage != nil {
		result["choices"] = []map[string]any{}
		result["usage"] = presentUsage(*event.Usage)
	}
	return result
}

func WriteSSE(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "data: %s\n\n", encoded)
	return err
}

func WriteNamedSSE(writer io.Writer, event string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\n", event); err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "data: %s\n\n", encoded)
	return err
}

func WriteSSEDone(writer io.Writer) error {
	_, err := io.WriteString(writer, "data: [DONE]\n\n")
	return err
}

func WriteError(w http.ResponseWriter, requestID string, providerError *canonical.Error) {
	status := providerError.HTTPStatus
	if status < 400 || status > 599 {
		status = statusForError(providerError.Kind)
	}
	envelope := PresentError(requestID, providerError)
	w.Header().Set("Content-Type", "application/json")
	if providerError.RetryAfter != nil {
		if providerError.RetryAfter.DelaySeconds != nil {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", *providerError.RetryAfter.DelaySeconds))
		} else if providerError.RetryAfter.At != nil {
			w.Header().Set("Retry-After", providerError.RetryAfter.At.UTC().Format(http.TimeFormat))
		}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

func PresentError(requestID string, providerError *canonical.Error) ErrorEnvelope {
	envelope := ErrorEnvelope{RequestID: requestID}
	envelope.Error.Message = providerError.Message
	envelope.Error.Type = string(providerError.Kind)
	envelope.Error.Param = providerError.Parameter
	envelope.Error.Code = providerError.Code
	envelope.Error.Model = providerError.Model
	envelope.Error.Provider = PublicProvider
	envelope.Error.Capability = providerError.Capability
	if providerError.Provider != "" && providerError.Capability == "" {
		envelope.Error.Code, envelope.Error.Message = publicUpstreamError(providerError.Kind)
	}
	return envelope
}

func PresentErrorDetail(providerError *canonical.Error) map[string]any {
	envelope := PresentError("", providerError)
	return map[string]any{
		"code": envelope.Error.Code, "message": envelope.Error.Message,
		"type": envelope.Error.Type, "param": envelope.Error.Param,
		"model": envelope.Error.Model, "provider": envelope.Error.Provider,
		"capability": envelope.Error.Capability,
	}
}

func publicUpstreamError(kind canonical.ErrorKind) (string, string) {
	switch kind {
	case canonical.ErrorAuthentication:
		return "model_authentication_failed", "the selected model is temporarily unavailable"
	case canonical.ErrorPermission:
		return "model_access_denied", "the selected model is not currently accessible"
	case canonical.ErrorQuota:
		return "model_capacity_exhausted", "the selected model has no available upstream capacity"
	case canonical.ErrorRateLimit:
		return "model_rate_limited", "the selected model is temporarily rate limited"
	case canonical.ErrorInvalidRequest:
		return "model_request_rejected", "the selected model rejected the request"
	case canonical.ErrorUnsupportedCapability:
		return "model_capability_unsupported", "the selected model cannot represent the requested capability"
	case canonical.ErrorProviderConfiguration:
		return "model_service_unavailable", "the selected model is temporarily unavailable"
	case canonical.ErrorProviderTemporary:
		return "model_service_temporarily_unavailable", "the selected model service is temporarily unavailable"
	case canonical.ErrorProviderPermanent:
		return "model_request_failed", "the selected model could not complete the request"
	case canonical.ErrorStreamInterrupted:
		return "model_stream_interrupted", "the model stream was interrupted"
	case canonical.ErrorUncertain:
		return "model_outcome_uncertain", "the model request outcome is uncertain"
	default:
		return "model_request_failed", "the selected model request failed"
	}
}

func presentMessage(message canonical.Message) map[string]any {
	result := map[string]any{"role": message.Role, "content": messageText(message.Content)}
	if message.Reasoning != nil {
		result["reasoning_content"] = message.Reasoning.Text
	}
	if len(message.ToolCalls) > 0 {
		calls := make([]map[string]any, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			presented := map[string]any{"id": call.ID, "type": call.Type, "function": map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments}}
			calls = append(calls, presented)
		}
		result["tool_calls"] = calls
	}
	return result
}

func messageText(parts []canonical.ContentPart) string {
	result := ""
	for _, part := range parts {
		if part.Type == canonical.ContentPartText {
			result += part.Text
		}
	}
	return result
}

func presentUsage(usage canonical.Usage) map[string]any {
	result := map[string]any{}
	if usage.InputTokens != nil {
		result["prompt_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		result["completion_tokens"] = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		result["total_tokens"] = *usage.TotalTokens
	}
	if usage.CachedInputTokens != nil {
		result["prompt_tokens_details"] = map[string]any{"cached_tokens": *usage.CachedInputTokens}
	}
	if usage.ReasoningTokens != nil {
		result["completion_tokens_details"] = map[string]any{"reasoning_tokens": *usage.ReasoningTokens}
	}
	return result
}

func statusForError(kind canonical.ErrorKind) int {
	switch kind {
	case canonical.ErrorInvalidRequest, canonical.ErrorUnsupportedCapability:
		return http.StatusBadRequest
	case canonical.ErrorAuthentication:
		return http.StatusUnauthorized
	case canonical.ErrorPermission:
		return http.StatusForbidden
	case canonical.ErrorQuota:
		return http.StatusPaymentRequired
	case canonical.ErrorAdmissionTimeout, canonical.ErrorRateLimit:
		return http.StatusTooManyRequests
	case canonical.ErrorProviderTemporary, canonical.ErrorStorageUnavailable:
		return http.StatusServiceUnavailable
	case canonical.ErrorProviderConfiguration, canonical.ErrorProviderPermanent:
		return http.StatusBadGateway
	case canonical.ErrorUncertain:
		return http.StatusConflict
	case canonical.ErrorStreamInterrupted:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
