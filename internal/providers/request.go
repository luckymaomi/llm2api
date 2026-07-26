package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type openAIAdapter struct {
	baseURL *url.URL
	policy  wirePolicy
}

type wireChatRequest struct {
	Model               string                  `json:"model"`
	Messages            []wireMessage           `json:"messages"`
	Stream              bool                    `json:"stream"`
	MaxTokens           *int64                  `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int64                  `json:"max_completion_tokens,omitempty"`
	N                   *int64                  `json:"n,omitempty"`
	Temperature         *float64                `json:"temperature,omitempty"`
	TopP                *float64                `json:"top_p,omitempty"`
	TopK                *float64                `json:"top_k,omitempty"`
	PresencePenalty     *float64                `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64                `json:"frequency_penalty,omitempty"`
	ThinkingBudget      *int64                  `json:"thinking_budget,omitempty"`
	Stop                []string                `json:"stop,omitempty"`
	ResponseFormat      *wireResponseFormat     `json:"response_format,omitempty"`
	Tools               []wireTool              `json:"tools,omitempty"`
	ToolChoice          any                     `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool                   `json:"parallel_tool_calls,omitempty"`
	StreamOptions       *wireStreamOptions      `json:"stream_options,omitempty"`
	Thinking            *wireThinking           `json:"thinking,omitempty"`
	ReasoningEffort     string                  `json:"reasoning_effort,omitempty"`
	EnableThinking      *bool                   `json:"enable_thinking,omitempty"`
	ToolStream          *bool                   `json:"tool_stream,omitempty"`
	RequestID           string                  `json:"request_id,omitempty"`
	PromptCacheKey      string                  `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier    string                  `json:"safety_identifier,omitempty"`
	ChatTemplateKwargs  *wireChatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
}

type wireMessage struct {
	Role             canonical.Role `json:"role"`
	Name             string         `json:"name,omitempty"`
	Content          any            `json:"content"`
	ToolCalls        []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Partial          bool           `json:"partial,omitempty"`
}

type wireContentPart struct {
	Type     canonical.ContentPartType `json:"type"`
	Text     string                    `json:"text,omitempty"`
	ImageURL *wireImageURL             `json:"image_url,omitempty"`
	VideoURL *wireVideoURL             `json:"video_url,omitempty"`
}

type wireImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type wireVideoURL struct {
	URL string `json:"url"`
}

type wireTool struct {
	Type     string           `json:"type"`
	Function wireToolFunction `json:"function"`
}

type wireToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireResponseFormat struct {
	Type       canonical.ResponseFormatType `json:"type"`
	JSONSchema *wireJSONSchema              `json:"json_schema,omitempty"`
}

type wireJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

type wireStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireThinking struct {
	Type          string `json:"type"`
	ClearThinking *bool  `json:"clear_thinking,omitempty"`
	Keep          string `json:"keep,omitempty"`
}

type wireChatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type SiliconFlowOptions struct {
	BaseURL         string
	Capabilities    Capabilities
	RequestIDHeader string
}

func NewSiliconFlow(options SiliconFlowOptions) (Adapter, error) {
	if !options.Capabilities.Chat {
		return nil, fmt.Errorf("SiliconFlow chat capability is required")
	}
	if options.Capabilities.Responses {
		return nil, fmt.Errorf("SiliconFlow chat adapter cannot declare Responses capability")
	}
	if options.RequestIDHeader != "" {
		options.Capabilities.ResponseRequestID = true
	}
	return newAdapter(options.BaseURL, siliconFlowPolicy(options.Capabilities, options.RequestIDHeader))
}

func mustNewAdapter(baseURL string, policy wirePolicy) Adapter {
	adapter, err := newAdapter(baseURL, policy)
	if err != nil {
		panic(err)
	}
	return adapter
}

func newAdapter(rawBaseURL string, policy wirePolicy) (*openAIAdapter, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse provider base URL: %w", err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" || baseURL.User != nil || baseURL.ForceQuery || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("provider base URL must be an absolute HTTP URL without credentials, query, or fragment")
	}
	return &openAIAdapter{baseURL: baseURL, policy: policy}, nil
}

func (a *openAIAdapter) Kind() Kind {
	return a.policy.kind
}

func (a *openAIAdapter) Capabilities() Capabilities {
	return a.policy.capabilities
}

func (a *openAIAdapter) BuildRequest(ctx context.Context, credential Credential, request canonical.ChatRequest) (*http.Request, error) {
	if strings.TrimSpace(credential.APIKey) == "" {
		return nil, a.requestError(canonical.ErrorProviderConfiguration, "missing_api_key", "provider API key is required", "")
	}
	wireRequest, err := a.buildWireRequest(request)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(wireRequest)
	if err != nil {
		return nil, a.requestError(canonical.ErrorInternalInvariant, "encode_request", "could not encode provider request", "")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint(a.policy.chatPath), bytes.NewReader(body))
	if err != nil {
		return nil, a.requestError(canonical.ErrorProviderConfiguration, "build_request", "could not build provider request", "")
	}
	httpRequest.Header.Set("Authorization", "Bearer "+credential.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if request.Stream {
		httpRequest.Header.Set("Accept", "text/event-stream")
	} else {
		httpRequest.Header.Set("Accept", "application/json")
	}
	return httpRequest, nil
}

func (a *openAIAdapter) buildWireRequest(request canonical.ChatRequest) (wireChatRequest, error) {
	if strings.TrimSpace(request.Model) == "" {
		return wireChatRequest{}, a.requestError(canonical.ErrorInvalidRequest, "missing_model", "model is required", "model")
	}
	if len(request.Messages) == 0 {
		return wireChatRequest{}, a.requestError(canonical.ErrorInvalidRequest, "missing_messages", "at least one message is required", "messages")
	}
	if request.Stream && !a.policy.capabilities.Streaming {
		return wireChatRequest{}, a.unsupported("stream")
	}

	messages, err := a.encodeMessages(request.Messages)
	if err != nil {
		return wireChatRequest{}, err
	}
	tools, err := a.encodeTools(request.Tools)
	if err != nil {
		return wireChatRequest{}, err
	}
	toolChoice, err := a.encodeToolChoice(request.ToolChoice, len(tools))
	if err != nil {
		return wireChatRequest{}, err
	}
	if err := a.validateParameters(request); err != nil {
		return wireChatRequest{}, err
	}

	wireRequest := wireChatRequest{
		Model:             request.Model,
		Messages:          messages,
		Stream:            request.Stream,
		N:                 request.N,
		Temperature:       request.Temperature,
		TopP:              request.TopP,
		TopK:              request.TopK,
		PresencePenalty:   request.PresencePenalty,
		FrequencyPenalty:  request.FrequencyPenalty,
		ThinkingBudget:    request.ThinkingBudget,
		Stop:              request.Stop,
		Tools:             tools,
		ToolChoice:        toolChoice,
		ParallelToolCalls: request.ParallelToolCalls,
		PromptCacheKey:    request.PromptCacheKey,
		SafetyIdentifier:  request.SafetyIdentifier,
	}
	if a.policy.outputTokens == outputTokenWireMaxCompletionTokens {
		wireRequest.MaxCompletionTokens = request.MaxOutputTokens
	} else {
		wireRequest.MaxTokens = request.MaxOutputTokens
	}
	if request.ResponseFormat != nil && request.ResponseFormat.Type != canonical.ResponseFormatText {
		wireRequest.ResponseFormat = &wireResponseFormat{Type: request.ResponseFormat.Type}
		if request.ResponseFormat.JSONSchema != nil {
			wireRequest.ResponseFormat.JSONSchema = &wireJSONSchema{
				Name: request.ResponseFormat.JSONSchema.Name, Description: request.ResponseFormat.JSONSchema.Description,
				Schema: request.ResponseFormat.JSONSchema.Schema, Strict: request.ResponseFormat.JSONSchema.Strict,
			}
		}
	}
	if request.Stream && a.policy.includeStreamUsage {
		wireRequest.StreamOptions = &wireStreamOptions{IncludeUsage: true}
	}
	if request.Stream && len(tools) > 0 && a.policy.sendToolStream {
		value := true
		wireRequest.ToolStream = &value
	}
	if a.policy.responseRequestIDBody {
		wireRequest.RequestID = request.RequestID
	}
	if err := a.encodeReasoning(request.Reasoning, &wireRequest); err != nil {
		return wireChatRequest{}, err
	}
	return wireRequest, nil
}

func validRole(role canonical.Role) bool {
	switch role {
	case canonical.RoleSystem, canonical.RoleDeveloper, canonical.RoleUser, canonical.RoleAssistant, canonical.RoleTool:
		return true
	default:
		return false
	}
}

func validReasoningEffort(effort canonical.ReasoningEffort) bool {
	switch effort {
	case canonical.ReasoningEffortNone, canonical.ReasoningEffortMinimal, canonical.ReasoningEffortLow,
		canonical.ReasoningEffortMedium, canonical.ReasoningEffortHigh, canonical.ReasoningEffortXHigh,
		canonical.ReasoningEffortMax:
		return true
	default:
		return false
	}
}

func enabledType(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (a *openAIAdapter) endpoint(endpointPath string) string {
	copyURL := *a.baseURL
	copyURL.Path = path.Join(copyURL.Path, endpointPath)
	return copyURL.String()
}

func (a *openAIAdapter) unsupported(parameter string) *canonical.Error {
	return a.requestError(canonical.ErrorUnsupportedCapability, "unsupported_capability", "provider cannot represent the requested capability", parameter)
}

func (a *openAIAdapter) requestError(kind canonical.ErrorKind, code, message, parameter string) *canonical.Error {
	return &canonical.Error{Kind: kind, Code: code, Message: message, Parameter: parameter, Provider: string(a.policy.kind)}
}
