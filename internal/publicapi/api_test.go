package publicapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/protocol"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/requestflow"
)

type fakeIdentity struct {
	principal identity.GatewayPrincipal
	err       error
}

func (f fakeIdentity) AuthenticateGatewayKey(context.Context, string) (identity.GatewayPrincipal, error) {
	return f.principal, f.err
}

func (f fakeIdentity) GatewayPrincipalByID(context.Context, uuid.UUID) (identity.GatewayPrincipal, error) {
	return f.principal, f.err
}

type fakeWorkflow struct {
	models          []requestflow.Model
	chatResult      requestflow.ChatResult
	chatError       *canonical.Error
	streamRequestID uuid.UUID
	streamEvents    []canonical.StreamEvent
	streamError     *canonical.Error
}

func (f fakeWorkflow) Models(context.Context, uuid.UUID) ([]requestflow.Model, error) {
	return f.models, nil
}

func (f fakeWorkflow) Chat(context.Context, requestflow.ChatCommand) (requestflow.ChatResult, *canonical.Error) {
	return f.chatResult, f.chatError
}

func (f fakeWorkflow) Stream(_ context.Context, _ requestflow.ChatCommand, sink requestflow.StreamSink) *canonical.Error {
	requestID := f.streamRequestID
	if requestID == uuid.Nil {
		requestID = uuid.New()
	}
	for _, event := range f.streamEvents {
		if err := sink(requestID, event); err != nil {
			return &canonical.Error{Kind: canonical.ErrorStreamInterrupted, Code: "sink", Message: err.Error()}
		}
	}
	return f.streamError
}

func TestModelsRequireGatewayKeyAndExposeOnlyWorkflowCatalog(t *testing.T) {
	userID := uuid.New()
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: userID}}, fakeWorkflow{models: []requestflow.Model{{
		PublicName: "glm-free", ProviderSlug: "zhipu", CreatedAt: time.Unix(42, 0),
		Capabilities: registry.ModelCapabilities{
			Chat: true, Streaming: true, Tools: true, ToolChoiceModes: []string{"auto"}, ToolStreaming: true,
			Reasoning: true, ReasoningConfig: true, ReasoningContent: true, ReasoningEfforts: []string{"high"}, MessageName: true,
			ResponseUsage: true, StreamUsage: true, ContextTokens: 200_000, OutputTokens: 8_192,
			Parameters: providers.ParameterCapabilities{N: providers.IntegerParameterLimit{Supported: true, ExactValues: []int64{1}}},
		},
	}}}, testLogger())

	unauthorized := httptest.NewRecorder()
	api.Routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/models", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/models", nil)
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("models status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			ID           string `json:"id"`
			OwnedBy      string `json:"owned_by"`
			Capabilities struct {
				Responses bool `json:"responses"`
				Tools     struct {
					Streaming bool `json:"streaming_tool_calls"`
				} `json:"tools"`
				Reasoning struct {
					Configurable bool `json:"configurable"`
					Content      bool `json:"content"`
				} `json:"reasoning"`
				Extensions struct {
					MessageName bool `json:"message_name"`
				} `json:"extensions"`
				Usage struct {
					Response bool `json:"response"`
					Stream   bool `json:"stream"`
				} `json:"usage"`
				Limits struct {
					ContextTokens int64 `json:"context_tokens"`
				} `json:"limits"`
				Parameters providers.ParameterCapabilities `json:"parameters"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "glm-free" || payload.Data[0].OwnedBy != protocol.PublicProvider {
		t.Fatalf("public model identity = %#v", payload.Data)
	}
	capabilities := payload.Data[0].Capabilities
	if !capabilities.Responses || !capabilities.Tools.Streaming || !capabilities.Reasoning.Configurable || !capabilities.Reasoning.Content || !capabilities.Extensions.MessageName ||
		!capabilities.Usage.Response || !capabilities.Usage.Stream || capabilities.Limits.ContextTokens != 200_000 ||
		!capabilities.Parameters.N.Supported || len(capabilities.Parameters.N.ExactValues) != 1 || capabilities.Parameters.N.ExactValues[0] != 1 {
		t.Fatalf("public model capabilities = %#v", capabilities)
	}
	if strings.Contains(response.Body.String(), "zhipu") {
		t.Fatalf("public model catalog leaked an internal Provider: %s", response.Body.String())
	}
}

func TestModelsExposeToggleReasoningAsPubliclyConfigurable(t *testing.T) {
	profile, found := providers.DefaultCatalog().ModelCapabilities(providers.KindAgnes, "agnes-2.0-flash")
	if !found {
		t.Fatal("Agnes model profile is missing")
	}
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: uuid.New()}}, fakeWorkflow{models: []requestflow.Model{{
		PublicName: "agnes-flash", ProviderSlug: "agnes", CreatedAt: time.Unix(42, 0), Capabilities: registry.ModelCapabilitiesFromProfile(profile),
	}}}, testLogger())
	request := httptest.NewRequest(http.MethodGet, "/models", nil)
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"toggle"`) || !strings.Contains(response.Body.String(), `"configurable":true`) {
		t.Fatalf("Agnes public reasoning capability is inconsistent: %d %s", response.Code, response.Body.String())
	}
}

func TestChatCompletionPresentsCanonicalResponse(t *testing.T) {
	completionID := "chatcmpl_test"
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()}}, fakeWorkflow{chatResult: requestflow.ChatResult{
		RequestID: uuid.New(), Response: canonical.ChatResponse{ID: completionID, Model: "glm-free", CreatedAt: time.Unix(50, 0), Choices: []canonical.ChatChoice{{Index: 0, Message: canonical.Message{Role: canonical.RoleAssistant, Content: canonical.TextContent("hello")}, FinishReason: canonical.FinishReasonStop}}},
	}}, testLogger())
	request := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-free","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), completionID) || !strings.Contains(response.Body.String(), `"content":"hello"`) {
		t.Fatalf("unexpected chat response: %d %s", response.Code, response.Body.String())
	}
}

func TestChatCompletionHidesInternalProviderFailure(t *testing.T) {
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()}}, fakeWorkflow{chatError: &canonical.Error{
		Kind: canonical.ErrorQuota, Code: "account_depleted", Message: "zhipu account 42 has no balance", Provider: "zhipu", HTTPStatus: http.StatusPaymentRequired,
	}}, testLogger())
	request := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-free","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusPaymentRequired {
		t.Fatalf("unexpected chat failure status: %d %s", response.Code, response.Body.String())
	}
	var envelope protocol.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode public error: %v", err)
	}
	if envelope.Error.Provider != protocol.PublicProvider || envelope.Error.Code != "model_capacity_exhausted" ||
		strings.Contains(response.Body.String(), "zhipu") || strings.Contains(response.Body.String(), "account 42") || strings.Contains(response.Body.String(), "account_depleted") {
		t.Fatalf("public error leaked internal Provider facts: %s", response.Body.String())
	}
}

func TestStreamCommitsOnlyWhenWorkflowEmits(t *testing.T) {
	streamRequestID := uuid.New()
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()}}, fakeWorkflow{streamRequestID: streamRequestID, streamEvents: []canonical.StreamEvent{
		{Type: canonical.StreamMessageStart, CompletionID: "chatcmpl_stream", Model: "glm-free", Role: canonical.RoleAssistant},
		{Type: canonical.StreamContentDelta, CompletionID: "chatcmpl_stream", Model: "glm-free", ContentDelta: "hello"},
		{Type: canonical.StreamDone, CompletionID: "chatcmpl_stream", Model: "glm-free"},
	}}, testLogger())
	request := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"glm-free","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" || response.Header().Get("X-Gateway-Request-ID") != streamRequestID.String() || !strings.Contains(response.Body.String(), "data: [DONE]") {
		t.Fatalf("unexpected stream response: %d %s", response.Code, response.Body.String())
	}
}

func TestCommittedStreamHidesInternalProviderFailure(t *testing.T) {
	api := New(fakeIdentity{principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()}}, fakeWorkflow{
		streamRequestID: uuid.New(),
		streamEvents:    []canonical.StreamEvent{{Type: canonical.StreamContentDelta, CompletionID: "chatcmpl_stream", Model: "public-model", ContentDelta: "partial"}},
		streamError:     &canonical.Error{Kind: canonical.ErrorStreamInterrupted, Code: "agnes_wire_broken", Message: "agnes upstream frame contained account metadata", Provider: "agnes"},
	}, testLogger())
	request := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"public-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer llmg_test")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"provider":"llm2api"`) || !strings.Contains(body, `"code":"model_stream_interrupted"`) ||
		strings.Contains(body, "agnes") || strings.Contains(body, "account metadata") || strings.Contains(body, "agnes_wire_broken") {
		t.Fatalf("committed stream leaked internal Provider facts: %d %s", response.Code, body)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
