package providers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func TestZhipuBuildsPreservedToolStreamRequest(t *testing.T) {
	t.Parallel()

	adapter := NewZhipu()
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		RequestID: "gateway-request-7",
		Model:     "glm-5.2",
		Messages:  []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Check weather")}},
		Tools: []canonical.ToolDefinition{{
			Name: "get_weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
		ToolChoice: &canonical.ToolChoice{Mode: canonical.ToolChoiceAuto},
		Stream:     true,
		Reasoning: &canonical.ReasoningConfig{
			Enabled: boolPointer(true), Effort: canonical.ReasoningEffortMax, Preserve: boolPointer(true),
		},
	})
	if err != nil {
		t.Fatalf("build Zhipu request: %v", err)
	}
	if request.URL.String() != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
		t.Fatalf("request URL = %q", request.URL.String())
	}
	assertRequestJSON(t, request, `{
		"model":"glm-5.2",
		"messages":[{"role":"user","content":"Check weather"}],
		"stream":true,
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}],
		"tool_choice":"auto",
		"thinking":{"type":"enabled","clear_thinking":false},
		"reasoning_effort":"max",
		"tool_stream":true,
		"request_id":"gateway-request-7"
	}`)
}

func TestAgnesBuildsThinkingToolRequest(t *testing.T) {
	t.Parallel()

	adapter := NewAgnes()
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model:    "agnes-2.0-flash",
		Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Find the build status")}},
		Tools: []canonical.ToolDefinition{{
			Name: "get_build", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		}},
		Stream:    true,
		Reasoning: &canonical.ReasoningConfig{Enabled: boolPointer(true)},
	})
	if err != nil {
		t.Fatalf("build Agnes request: %v", err)
	}
	if request.URL.String() != "https://apihub.agnes-ai.com/v1/chat/completions" {
		t.Fatalf("request URL = %q", request.URL.String())
	}
	assertRequestJSON(t, request, `{
		"model":"agnes-2.0-flash",
		"messages":[{"role":"user","content":"Find the build status"}],
		"stream":true,
		"tools":[{"type":"function","function":{"name":"get_build","parameters":{"type":"object","properties":{"id":{"type":"string"}}}}}],
		"chat_template_kwargs":{"enable_thinking":true}
	}`)
}

func TestKimiK3BuildsFullFidelityRequest(t *testing.T) {
	t.Parallel()

	adapter := NewKimi()
	enabled := true
	preserve := true
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k3",
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentPart{
				{Type: canonical.ContentPartText, Text: "Describe these inputs"},
				{Type: canonical.ContentPartImageURL, ImageURL: &canonical.ImageURL{URL: "data:image/png;base64,AAAA"}},
				{Type: canonical.ContentPartVideoURL, VideoURL: &canonical.VideoURL{URL: "ms://video-1"}},
			}},
			{Role: canonical.RoleAssistant, Content: canonical.TextContent("{"), Partial: true},
		},
		Tools: []canonical.ToolDefinition{{
			Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`), Strict: boolPointer(true),
		}},
		ToolChoice:       &canonical.ToolChoice{Mode: canonical.ToolChoiceFunction, FunctionName: "get_weather"},
		Stream:           true,
		MaxOutputTokens:  int64Pointer(4096),
		PromptCacheKey:   "session-42",
		SafetyIdentifier: "member-42",
		Reasoning: &canonical.ReasoningConfig{
			Enabled: &enabled, Preserve: &preserve, Effort: canonical.ReasoningEffortHigh,
		},
	})
	if err != nil {
		t.Fatalf("build Kimi request: %v", err)
	}
	if request.URL.String() != "https://api.moonshot.cn/v1/chat/completions" {
		t.Fatalf("request URL = %q", request.URL.String())
	}
	assertRequestJSON(t, request, `{
		"model":"kimi-k3",
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"Describe these inputs"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}},
				{"type":"video_url","video_url":{"url":"ms://video-1"}}
			]},
			{"role":"assistant","content":"{","partial":true}
		],
		"stream":true,
		"max_completion_tokens":4096,
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"},"strict":true}}],
		"tool_choice":{"type":"function","function":{"name":"get_weather"}},
		"stream_options":{"include_usage":true},
		"reasoning_effort":"high",
		"prompt_cache_key":"session-42",
		"safety_identifier":"member-42"
	}`)
}

func TestKimiK3RejectsDisablingItsAlwaysOnReasoning(t *testing.T) {
	t.Parallel()

	disabled := false
	_, err := NewKimi().BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k3", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
		Reasoning: &canonical.ReasoningConfig{Enabled: &disabled},
	})
	if err == nil {
		t.Fatal("Kimi K3 accepted disabled reasoning")
	}
}

func TestKimiK26AppliesItsThinkingModeParameterRules(t *testing.T) {
	t.Parallel()

	adapter, err := NewKimiWithCapabilities("https://api.moonshot.cn/v1", kimiK26Capabilities())
	if err != nil {
		t.Fatalf("create Kimi K2.6 adapter: %v", err)
	}
	enabled, temperature, topP, penalty := true, 1.0, 0.95, 0.0
	n := int64(1)
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k2.6", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
		N: &n, Temperature: &temperature, TopP: &topP, PresencePenalty: &penalty, FrequencyPenalty: &penalty,
		Reasoning: &canonical.ReasoningConfig{Enabled: &enabled},
	})
	if err != nil {
		t.Fatalf("build Kimi K2.6 request: %v", err)
	}
	assertRequestJSON(t, request, `{
		"model":"kimi-k2.6","messages":[{"role":"user","content":"Reply"}],"stream":false,
		"n":1,"temperature":1,"top_p":0.95,"presence_penalty":0,"frequency_penalty":0,
		"thinking":{"type":"enabled"}
	}`)

	wrongTemperature := 0.6
	_, err = adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k2.6", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
		Temperature: &wrongTemperature, Reasoning: &canonical.ReasoningConfig{Enabled: &enabled},
	})
	if err == nil {
		t.Fatal("Kimi K2.6 accepted the non-thinking temperature while thinking was enabled")
	}

	_, err = adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k2.6", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
		Tools:      []canonical.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: &canonical.ToolChoice{Mode: canonical.ToolChoiceFunction, FunctionName: "lookup"}, Reasoning: &canonical.ReasoningConfig{Enabled: &enabled},
	})
	if err == nil {
		t.Fatal("Kimi K2.6 accepted named tool_choice while thinking was enabled")
	}
}

func TestKimiStatusProbePreservesOfficialBalanceWithoutFloatConversion(t *testing.T) {
	t.Parallel()

	adapter := NewKimi()
	probe, err := adapter.StatusProbe(context.Background(), Credential{APIKey: "fixture-key"})
	if err != nil {
		t.Fatalf("build Kimi status probe: %v", err)
	}
	if !probe.Available || probe.Kind != StatusProbeKimiBalance || probe.Request == nil {
		t.Fatalf("Kimi status probe = %#v", probe)
	}
	if probe.Request.URL.String() != "https://api.moonshot.cn/v1/users/me/balance" {
		t.Fatalf("status probe URL = %q", probe.Request.URL.String())
	}
	if got := probe.Request.Header.Get("Authorization"); got != "Bearer fixture-key" {
		t.Fatalf("status probe authorization = %q", got)
	}
	observation, providerErr := adapter.ParseStatusProbe(probe.Kind, 200, nil, []byte(`{
		"code":0,"status":true,"data":{"available_balance":49.58894,"voucher_balance":46.58893,"cash_balance":3.00001}
	}`))
	if providerErr != nil {
		t.Fatalf("parse Kimi balance: %v", providerErr)
	}
	if observation.State != UpstreamStatusObserved || observation.Scope != UpstreamStatusScopeAccount || observation.Source != "official_balance_endpoint" {
		t.Fatalf("Kimi observation = %#v", observation)
	}
	if observation.Balance == nil || observation.Balance.Available != "49.58894" || observation.Balance.Voucher != "46.58893" || observation.Balance.Cash != "3.00001" {
		t.Fatalf("Kimi balance = %#v", observation.Balance)
	}
}

func TestSiliconFlowBuildsDeclaredContractRequest(t *testing.T) {
	t.Parallel()

	capabilities := siliconFlowModel("general-chat", 0, 0, true).Capabilities
	adapter, err := NewSiliconFlow(SiliconFlowOptions{
		BaseURL: "https://llm.example/v1", Capabilities: capabilities,
	})
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model:    "general-chat",
		Messages: []canonical.Message{{Role: canonical.RoleUser, Name: "caller", Content: canonical.TextContent("Summarize")}},
		Stream:   true, N: int64Pointer(2), TopK: float64Pointer(50), ThinkingBudget: int64Pointer(512),
		Reasoning: &canonical.ReasoningConfig{Enabled: boolPointer(true)},
	})
	if err != nil {
		t.Fatalf("build SiliconFlow request: %v", err)
	}
	assertRequestJSON(t, request, `{
		"model":"general-chat",
		"messages":[{"role":"user","name":"caller","content":"Summarize"}],
		"stream":true,
		"n":2,"top_k":50,"thinking_budget":512,
		"stream_options":{"include_usage":true},
		"enable_thinking":true
	}`)
}

func TestSiliconFlowToggleReasoningProfileControlsThinking(t *testing.T) {
	t.Parallel()

	capabilities := SiliconFlowCapabilities()
	capabilities.ReasoningEffort = false
	capabilities.ReasoningToggle = true
	adapter, err := NewSiliconFlow(SiliconFlowOptions{BaseURL: "https://llm.example/v1", Capabilities: capabilities})
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	base := canonical.ChatRequest{Model: "thinking-chat", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}}}
	request, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, base)
	if err != nil {
		t.Fatalf("build default request: %v", err)
	}
	assertRequestJSON(t, request, `{"model":"thinking-chat","messages":[{"role":"user","content":"Reply"}],"stream":false,"enable_thinking":false}`)

	enabled := true
	base.Reasoning = &canonical.ReasoningConfig{Enabled: &enabled}
	request, err = adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, base)
	if err != nil {
		t.Fatalf("build thinking request: %v", err)
	}
	assertRequestJSON(t, request, `{"model":"thinking-chat","messages":[{"role":"user","content":"Reply"}],"stream":false,"enable_thinking":true}`)

	base.Reasoning = &canonical.ReasoningConfig{Effort: canonical.ReasoningEffortLow}
	if _, err := adapter.BuildRequest(context.Background(), Credential{APIKey: "fixture-key"}, base); err == nil {
		t.Fatal("toggle-only reasoning profile accepted reasoning_effort")
	}
}

func TestSiliconFlowRejectsBaseURLParameters(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{
		"https://llm.example/v1?",
		"https://llm.example/v1?tenant=other",
		"https://llm.example/v1#other",
	} {
		t.Run(baseURL, func(t *testing.T) {
			t.Parallel()
			if _, err := NewSiliconFlow(SiliconFlowOptions{
				BaseURL: baseURL, Capabilities: SiliconFlowCapabilities(),
			}); err == nil {
				t.Fatalf("NewSiliconFlow(%q) succeeded", baseURL)
			}
		})
	}
}
