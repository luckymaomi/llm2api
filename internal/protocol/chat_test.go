package protocol

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func TestParseChatRequestPreservesToolAndReasoningRoundTrip(t *testing.T) {
	body := `{
  "model":"reasoning-chat",
  "messages":[
    {"role":"assistant","content":null,"reasoning_content":"checked","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}}]},
    {"role":"tool","tool_call_id":"call_1","content":"found"}
  ],
  "tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}}],
  "tool_choice":"auto",
  "stream":true,
	"stream_options":{"include_usage":true},
  "n":2,
  "top_k":50,
  "thinking_budget":512,
  "thinking":{"type":"enabled","clear_thinking":false},
  "reasoning_effort":"high"
}`
	request, parseError := ParseChatRequest(strings.NewReader(body), "req_1")
	if parseError != nil {
		t.Fatal(parseError)
	}
	if request.Model != "reasoning-chat" || !request.Stream || request.StreamUsage == nil || !*request.StreamUsage || len(request.Tools) != 1 || request.N == nil || *request.N != 2 || request.TopK == nil || *request.TopK != 50 || request.ThinkingBudget == nil || *request.ThinkingBudget != 512 || request.Reasoning == nil || request.Reasoning.Effort != canonical.ReasoningEffortHigh || request.Reasoning.Preserve == nil || !*request.Reasoning.Preserve {
		t.Fatalf("parsed request lost a canonical contract: %#v", request)
	}
	if len(request.Messages[0].ToolCalls) != 1 || request.Messages[1].ToolCallID != "call_1" {
		t.Fatal("tool round trip was not preserved")
	}
}

func TestParseChatRequestPreservesEveryDeclaredPublicField(t *testing.T) {
	body := `{
  "model":"public-model",
  "messages":[
    {"role":"system","content":"Follow policy"},
    {"role":"user","name":"member","content":[
      {"type":"text","text":"Inspect"},
      {"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA","detail":"high"}},
      {"type":"video_url","video_url":{"url":"ms://video-1"}}
    ]},
    {"role":"assistant","content":"{","partial":true,"reasoning_content":"continue"}
  ],
  "tools":[{"type":"function","function":{"name":"lookup","description":"Lookup","parameters":{"type":"object"},"strict":true}}],
  "tool_choice":{"type":"function","function":{"name":"lookup"}},
  "parallel_tool_calls":true,
  "stream":true,
  "stream_options":{"include_usage":true},
  "max_completion_tokens":4096,
  "n":2,
  "temperature":0.6,
  "top_p":0.95,
  "top_k":50,
  "presence_penalty":0.1,
  "frequency_penalty":-0.1,
  "thinking_budget":512,
  "stop":["END"],
  "response_format":{"type":"json_schema","json_schema":{"name":"result","description":"Result","schema":{"type":"object"},"strict":true}},
  "reasoning_effort":"high",
  "thinking":{"type":"enabled","keep":"all"},
  "prompt_cache_key":"session-42",
  "safety_identifier":"member-42"
}`
	request, parseError := ParseChatRequest(strings.NewReader(body), "request-42")
	if parseError != nil {
		t.Fatal(parseError)
	}
	if request.Model != "public-model" || !request.Stream || request.StreamUsage == nil || !*request.StreamUsage ||
		request.MaxOutputTokens == nil || *request.MaxOutputTokens != 4096 || request.N == nil || *request.N != 2 ||
		request.Temperature == nil || *request.Temperature != 0.6 || request.TopP == nil || *request.TopP != 0.95 ||
		request.TopK == nil || *request.TopK != 50 || request.PresencePenalty == nil || *request.PresencePenalty != 0.1 ||
		request.FrequencyPenalty == nil || *request.FrequencyPenalty != -0.1 || request.ThinkingBudget == nil || *request.ThinkingBudget != 512 {
		t.Fatalf("public scalar fields were not preserved: %#v", request)
	}
	if len(request.Messages) != 3 || request.Messages[1].Name != "member" || len(request.Messages[1].Content) != 3 || !request.Messages[2].Partial || request.Messages[2].Reasoning == nil {
		t.Fatalf("public message fields were not preserved: %#v", request.Messages)
	}
	if len(request.Tools) != 1 || request.Tools[0].Strict == nil || !*request.Tools[0].Strict || request.ToolChoice == nil ||
		request.ToolChoice.Mode != canonical.ToolChoiceFunction || request.ToolChoice.FunctionName != "lookup" || request.ParallelToolCalls == nil || !*request.ParallelToolCalls {
		t.Fatalf("public tool fields were not preserved: %#v", request)
	}
	if len(request.Stop) != 1 || request.Stop[0] != "END" || request.ResponseFormat == nil || request.ResponseFormat.JSONSchema == nil ||
		request.ResponseFormat.JSONSchema.Name != "result" || request.Reasoning == nil || request.Reasoning.Enabled == nil || !*request.Reasoning.Enabled ||
		request.Reasoning.Preserve == nil || !*request.Reasoning.Preserve || request.Reasoning.Effort != canonical.ReasoningEffortHigh ||
		request.PromptCacheKey != "session-42" || request.SafetyIdentifier != "member-42" {
		t.Fatalf("public structured fields were not preserved: %#v", request)
	}
}

func TestUncertainErrorPreventsImplicitSDKReplay(t *testing.T) {
	retryAt := time.Date(2026, 7, 21, 16, 0, 0, 0, time.UTC)
	recorder := httptest.NewRecorder()
	WriteError(recorder, "request_1", &canonical.Error{
		Kind: canonical.ErrorUncertain, Code: "upstream_outcome_uncertain", Message: "upstream request outcome is uncertain",
		RetryAfter: &canonical.RetryAfter{At: &retryAt},
	})
	if recorder.Code != http.StatusConflict || recorder.Header().Get("Retry-After") != retryAt.Format(http.TimeFormat) {
		t.Fatalf("status/retry-after = %d/%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}

func TestPublicErrorHidesInternalProviderDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	WriteError(recorder, "request_1", &canonical.Error{
		Kind: canonical.ErrorRateLimit, Code: "1302", Message: "zhipu account 42 is exhausted", Provider: "zhipu",
	})
	var envelope ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode public error: %v", err)
	}
	if envelope.Error.Provider != PublicProvider || envelope.Error.Code != "model_rate_limited" || envelope.Error.Message != "the selected model is temporarily rate limited" {
		t.Fatalf("public error = %#v", envelope)
	}
	if strings.Contains(recorder.Body.String(), "zhipu") || strings.Contains(recorder.Body.String(), "account 42") || strings.Contains(recorder.Body.String(), "1302") {
		t.Fatalf("public error leaked upstream details: %s", recorder.Body.String())
	}
}

func TestFailedResponsePersistsOnlyPublicErrorFacts(t *testing.T) {
	response := PresentResponseFailed("resp_1", "public-model", 1, ResponsesRequest{}, &canonical.Error{
		Kind: canonical.ErrorQuota, Code: "upstream-secret-code", Message: "Agnes account exhausted", Provider: "agnes",
	})
	detail := response["error"].(map[string]any)
	if detail["provider"] != PublicProvider || detail["code"] != "model_capacity_exhausted" || detail["message"] != "the selected model has no available upstream capacity" {
		t.Fatalf("stored public error = %#v", detail)
	}
}

func TestPresentStreamEventProducesOpenAIChunk(t *testing.T) {
	event := canonical.StreamEvent{Type: canonical.StreamToolCallDelta, CompletionID: "chatcmpl_1", Model: "model", ChoiceIndex: 0, ToolCallDelta: &canonical.ToolCallDelta{Index: 0, ID: "call_1", Type: "function", FunctionName: "lookup", ArgumentsFragment: "{\"id\":"}}
	chunk := PresentStreamEvent(event)
	choices, ok := chunk["choices"].([]map[string]any)
	if !ok || len(choices) != 1 {
		t.Fatalf("stream chunk choices = %#v", chunk["choices"])
	}
	delta := choices[0]["delta"].(map[string]any)
	toolCalls := delta["tool_calls"].([]map[string]any)
	if toolCalls[0]["id"] != "call_1" {
		t.Fatalf("tool call delta = %#v", toolCalls[0])
	}
}
