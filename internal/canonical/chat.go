package canonical

import "encoding/json"

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Name       string
	Content    []ContentPart
	Partial    bool
	ToolCalls  []ToolCall
	ToolCallID string
	Reasoning  *ReasoningContent
}

type ResponseFormatType string

const (
	ResponseFormatText       ResponseFormatType = "text"
	ResponseFormatJSONObject ResponseFormatType = "json_object"
	ResponseFormatJSONSchema ResponseFormatType = "json_schema"
)

type ResponseFormat struct {
	Type       ResponseFormatType
	JSONSchema *JSONSchema
}

// JSONSchema preserves the OpenAI-compatible structured-output contract. The
// provider adapter owns any vendor-specific representation or rejection.
type JSONSchema struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Strict      *bool
}

type ChatRequest struct {
	RequestID         string
	Model             string
	Messages          []Message
	Tools             []ToolDefinition
	ToolChoice        *ToolChoice
	ParallelToolCalls *bool
	Stream            bool
	StreamUsage       *bool
	MaxOutputTokens   *int64
	N                 *int64
	Temperature       *float64
	TopP              *float64
	TopK              *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	ThinkingBudget    *int64
	Stop              []string
	ResponseFormat    *ResponseFormat
	Reasoning         *ReasoningConfig
	PromptCacheKey    string
	SafetyIdentifier  string
}

type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
)

type ChatChoice struct {
	Index        int
	Message      Message
	FinishReason FinishReason
}

type ChatResponse struct {
	ID        string
	RequestID string
	Model     string
	CreatedAt time.Time
	Choices   []ChatChoice
	Usage     *Usage
}
