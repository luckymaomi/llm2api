package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

type chatRequest struct {
	Model             string           `json:"model"`
	Messages          []message        `json:"messages"`
	Tools             []tool           `json:"tools,omitempty"`
	ToolChoice        json.RawMessage  `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool            `json:"parallel_tool_calls,omitempty"`
	Stream            bool             `json:"stream,omitempty"`
	MaxTokens         *int64           `json:"max_tokens,omitempty"`
	MaxOutputTokens   *int64           `json:"max_completion_tokens,omitempty"`
	N                 *int64           `json:"n,omitempty"`
	Temperature       *float64         `json:"temperature,omitempty"`
	TopP              *float64         `json:"top_p,omitempty"`
	TopK              *float64         `json:"top_k,omitempty"`
	PresencePenalty   *float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty  *float64         `json:"frequency_penalty,omitempty"`
	ThinkingBudget    *int64           `json:"thinking_budget,omitempty"`
	Stop              json.RawMessage  `json:"stop,omitempty"`
	ResponseFormat    *responseFormat  `json:"response_format,omitempty"`
	ReasoningEffort   string           `json:"reasoning_effort,omitempty"`
	Thinking          *thinkingRequest `json:"thinking,omitempty"`
	PromptCacheKey    string           `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier  string           `json:"safety_identifier,omitempty"`
}

type message struct {
	Role             string          `json:"role"`
	Name             string          `json:"name,omitempty"`
	Partial          bool            `json:"partial,omitempty"`
	Content          json.RawMessage `json:"content"`
	ToolCalls        []toolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      *bool           `json:"strict,omitempty"`
}

type toolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolFunctionCall `json:"function"`
}

type toolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

type thinkingRequest struct {
	Type          string `json:"type"`
	ClearThinking *bool  `json:"clear_thinking,omitempty"`
	Keep          string `json:"keep,omitempty"`
}

func ParseChatRequest(body io.Reader, requestID string) (canonical.ChatRequest, *canonical.Error) {
	decoder := json.NewDecoder(io.LimitReader(body, 8<<20))
	decoder.DisallowUnknownFields()
	var wire chatRequest
	if err := decoder.Decode(&wire); err != nil {
		return canonical.ChatRequest{}, invalid("invalid_json", "request body must be one valid JSON object", "", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return canonical.ChatRequest{}, invalid("invalid_json", "request body must contain exactly one JSON object", "", err)
	}
	if wire.Model == "" {
		return canonical.ChatRequest{}, invalid("missing_model", "model is required", "model", nil)
	}
	if len(wire.Messages) == 0 {
		return canonical.ChatRequest{}, invalid("missing_messages", "messages must not be empty", "messages", nil)
	}

	request := canonical.ChatRequest{
		RequestID: requestID, Model: wire.Model, Stream: wire.Stream,
		N: wire.N, Temperature: wire.Temperature, TopP: wire.TopP, TopK: wire.TopK,
		PresencePenalty: wire.PresencePenalty, FrequencyPenalty: wire.FrequencyPenalty, ThinkingBudget: wire.ThinkingBudget,
		ParallelToolCalls: wire.ParallelToolCalls,
		PromptCacheKey:    wire.PromptCacheKey,
		SafetyIdentifier:  wire.SafetyIdentifier,
	}
	request.MaxOutputTokens = wire.MaxOutputTokens
	if request.MaxOutputTokens == nil {
		request.MaxOutputTokens = wire.MaxTokens
	}
	if wire.MaxTokens != nil && wire.MaxOutputTokens != nil && *wire.MaxTokens != *wire.MaxOutputTokens {
		return canonical.ChatRequest{}, invalid("conflicting_max_tokens", "max_tokens and max_completion_tokens disagree", "max_completion_tokens", nil)
	}
	if err := validateSampling(request); err != nil {
		return canonical.ChatRequest{}, err
	}

	for index, item := range wire.Messages {
		parsed, parseError := parseMessage(item)
		if parseError != nil {
			parseError.Parameter = fmt.Sprintf("messages.%d.%s", index, parseError.Parameter)
			return canonical.ChatRequest{}, parseError
		}
		request.Messages = append(request.Messages, parsed)
	}
	for index, item := range wire.Tools {
		parsed, parseError := parseTool(item)
		if parseError != nil {
			parseError.Parameter = fmt.Sprintf("tools.%d.%s", index, parseError.Parameter)
			return canonical.ChatRequest{}, parseError
		}
		request.Tools = append(request.Tools, parsed)
	}
	choice, parseError := parseToolChoice(wire.ToolChoice)
	if parseError != nil {
		return canonical.ChatRequest{}, parseError
	}
	request.ToolChoice = choice
	request.Stop, parseError = parseStop(wire.Stop)
	if parseError != nil {
		return canonical.ChatRequest{}, parseError
	}
	if wire.ResponseFormat != nil {
		switch wire.ResponseFormat.Type {
		case string(canonical.ResponseFormatText):
			request.ResponseFormat = &canonical.ResponseFormat{Type: canonical.ResponseFormatText}
		case string(canonical.ResponseFormatJSONObject):
			request.ResponseFormat = &canonical.ResponseFormat{Type: canonical.ResponseFormatJSONObject}
		case string(canonical.ResponseFormatJSONSchema):
			if wire.ResponseFormat.JSONSchema == nil || wire.ResponseFormat.JSONSchema.Name == "" || !validJSONObject(wire.ResponseFormat.JSONSchema.Schema) {
				return canonical.ChatRequest{}, invalid("invalid_json_schema", "response_format.json_schema requires a name and one JSON object schema", "response_format.json_schema", nil)
			}
			request.ResponseFormat = &canonical.ResponseFormat{Type: canonical.ResponseFormatJSONSchema, JSONSchema: &canonical.JSONSchema{
				Name: wire.ResponseFormat.JSONSchema.Name, Description: wire.ResponseFormat.JSONSchema.Description,
				Schema: append(json.RawMessage(nil), wire.ResponseFormat.JSONSchema.Schema...), Strict: wire.ResponseFormat.JSONSchema.Strict,
			}}
		default:
			return canonical.ChatRequest{}, invalid("unsupported_response_format", "response_format.type is not supported", "response_format.type", nil)
		}
	}
	if wire.ReasoningEffort != "" || wire.Thinking != nil {
		reasoning := &canonical.ReasoningConfig{Effort: canonical.ReasoningEffort(wire.ReasoningEffort)}
		if wire.Thinking != nil {
			switch wire.Thinking.Type {
			case "enabled":
				enabled := true
				reasoning.Enabled = &enabled
			case "disabled":
				enabled := false
				reasoning.Enabled = &enabled
			default:
				return canonical.ChatRequest{}, invalid("invalid_thinking", "thinking.type must be enabled or disabled", "thinking.type", nil)
			}
			if wire.Thinking.ClearThinking != nil {
				if wire.Thinking.Keep != "" {
					return canonical.ChatRequest{}, invalid("ambiguous_thinking_preserve", "thinking.keep and thinking.clear_thinking cannot be combined", "thinking", nil)
				}
				preserve := !*wire.Thinking.ClearThinking
				reasoning.Preserve = &preserve
			}
			if wire.Thinking.Keep != "" {
				if wire.Thinking.Keep != "all" {
					return canonical.ChatRequest{}, invalid("invalid_thinking_keep", "thinking.keep must be all", "thinking.keep", nil)
				}
				preserve := true
				reasoning.Preserve = &preserve
			}
		}
		request.Reasoning = reasoning
	}
	return request, nil
}

func parseMessage(item message) (canonical.Message, *canonical.Error) {
	parsed := canonical.Message{Role: canonical.Role(item.Role), Name: item.Name, Partial: item.Partial, ToolCallID: item.ToolCallID}
	switch parsed.Role {
	case canonical.RoleSystem, canonical.RoleDeveloper, canonical.RoleUser, canonical.RoleAssistant, canonical.RoleTool:
	default:
		return canonical.Message{}, invalid("invalid_role", "message role is invalid", "role", nil)
	}
	if len(item.Content) > 0 && !bytes.Equal(bytes.TrimSpace(item.Content), []byte("null")) {
		content, contentError := parseContent(item.Content)
		if contentError != nil {
			return canonical.Message{}, contentError
		}
		parsed.Content = content
	}
	for _, call := range item.ToolCalls {
		if call.ID == "" || call.Type != "function" || call.Function.Name == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return canonical.Message{}, invalid("invalid_tool_call", "assistant tool call is invalid", "tool_calls", nil)
		}
		parsed.ToolCalls = append(parsed.ToolCalls, canonical.ToolCall{ID: call.ID, Type: call.Type, Function: canonical.ToolFunctionCall{Name: call.Function.Name, Arguments: call.Function.Arguments}})
	}
	if item.ReasoningContent != "" {
		parsed.Reasoning = &canonical.ReasoningContent{Text: item.ReasoningContent}
	}
	if parsed.Role == canonical.RoleTool && parsed.ToolCallID == "" {
		return canonical.Message{}, invalid("missing_tool_call_id", "tool message requires tool_call_id", "tool_call_id", nil)
	}
	if parsed.Partial && parsed.Role != canonical.RoleAssistant {
		return canonical.Message{}, invalid("invalid_partial_message", "partial is only valid on an assistant message", "partial", nil)
	}
	if len(parsed.Content) == 0 && len(parsed.ToolCalls) == 0 && parsed.Reasoning == nil {
		return canonical.Message{}, invalid("empty_message", "message must contain text, reasoning, or tool calls", "content", nil)
	}
	return parsed, nil
}

func parseContent(raw json.RawMessage) ([]canonical.ContentPart, *canonical.Error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return canonical.TextContent(text), nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL    string `json:"url"`
			Detail string `json:"detail,omitempty"`
		} `json:"image_url,omitempty"`
		VideoURL *struct {
			URL string `json:"url"`
		} `json:"video_url,omitempty"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) == 0 {
		return nil, invalid("unsupported_content", "content must be text or supported content parts", "content", err)
	}
	result := make([]canonical.ContentPart, 0, len(parts))
	for index, part := range parts {
		switch canonical.ContentPartType(part.Type) {
		case canonical.ContentPartText:
			result = append(result, canonical.ContentPart{Type: canonical.ContentPartText, Text: part.Text})
		case canonical.ContentPartImageURL:
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				return nil, invalid("invalid_image_url", "image_url content requires a URL", fmt.Sprintf("content.%d.image_url.url", index), nil)
			}
			result = append(result, canonical.ContentPart{Type: canonical.ContentPartImageURL, ImageURL: &canonical.ImageURL{URL: part.ImageURL.URL, Detail: part.ImageURL.Detail}})
		case canonical.ContentPartVideoURL:
			if part.VideoURL == nil || part.VideoURL.URL == "" {
				return nil, invalid("invalid_video_url", "video_url content requires a URL", fmt.Sprintf("content.%d.video_url.url", index), nil)
			}
			result = append(result, canonical.ContentPart{Type: canonical.ContentPartVideoURL, VideoURL: &canonical.VideoURL{URL: part.VideoURL.URL}})
		default:
			return nil, invalid("unsupported_content_part", "content part type is not supported", fmt.Sprintf("content.%d.type", index), nil)
		}
	}
	return result, nil
}

func parseTool(item tool) (canonical.ToolDefinition, *canonical.Error) {
	if item.Type != "function" || item.Function.Name == "" || !json.Valid(item.Function.Parameters) {
		return canonical.ToolDefinition{}, invalid("invalid_tool", "tool definition is invalid", "function", nil)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(item.Function.Parameters, &object); err != nil || object == nil {
		return canonical.ToolDefinition{}, invalid("invalid_tool_schema", "tool parameters must be a JSON object", "function.parameters", err)
	}
	return canonical.ToolDefinition{Name: item.Function.Name, Description: item.Function.Description, Parameters: append(json.RawMessage(nil), item.Function.Parameters...), Strict: item.Function.Strict}, nil
}

func validJSONObject(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice, *canonical.Error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var mode string
	if json.Unmarshal(raw, &mode) == nil {
		switch canonical.ToolChoiceMode(mode) {
		case canonical.ToolChoiceNone, canonical.ToolChoiceAuto, canonical.ToolChoiceRequired:
			return &canonical.ToolChoice{Mode: canonical.ToolChoiceMode(mode)}, nil
		default:
			return nil, invalid("invalid_tool_choice", "tool_choice mode is invalid", "tool_choice", nil)
		}
	}
	var named struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &named); err != nil || named.Type != "function" || named.Function.Name == "" {
		return nil, invalid("invalid_tool_choice", "named tool_choice is invalid", "tool_choice", err)
	}
	return &canonical.ToolChoice{Mode: canonical.ToolChoiceFunction, FunctionName: named.Function.Name}, nil
}

func parseStop(raw json.RawMessage) ([]string, *canonical.Error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil || len(many) > 4 {
		return nil, invalid("invalid_stop", "stop must be a string or at most four strings", "stop", err)
	}
	return many, nil
}

func validateSampling(request canonical.ChatRequest) *canonical.Error {
	if request.MaxOutputTokens != nil && *request.MaxOutputTokens < 1 {
		return invalid("invalid_max_tokens", "max output tokens must be positive", "max_completion_tokens", nil)
	}
	if request.N != nil && *request.N < 1 {
		return invalid("invalid_n", "n must be positive", "n", nil)
	}
	if request.TopK != nil && *request.TopK < 0 {
		return invalid("invalid_top_k", "top_k must not be negative", "top_k", nil)
	}
	if request.ThinkingBudget != nil && *request.ThinkingBudget < 1 {
		return invalid("invalid_thinking_budget", "thinking_budget must be positive", "thinking_budget", nil)
	}
	if request.Temperature != nil && (*request.Temperature < 0 || *request.Temperature > 2) {
		return invalid("invalid_temperature", "temperature must be between 0 and 2", "temperature", nil)
	}
	if request.TopP != nil && (*request.TopP <= 0 || *request.TopP > 1) {
		return invalid("invalid_top_p", "top_p must be greater than 0 and at most 1", "top_p", nil)
	}
	for parameter, value := range map[string]*float64{"presence_penalty": request.PresencePenalty, "frequency_penalty": request.FrequencyPenalty} {
		if value != nil && (*value < -2 || *value > 2) {
			return invalid("invalid_penalty", parameter+" must be between -2 and 2", parameter, nil)
		}
	}
	return nil
}

func invalid(code, message, parameter string, cause error) *canonical.Error {
	return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: code, Message: message, Parameter: parameter, HTTPStatus: 400, Cause: cause}
}

func unixSeconds(value time.Time) int64 { return value.Unix() }
