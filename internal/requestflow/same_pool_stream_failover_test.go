package requestflow

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/resilience"
	"github.com/luckymaomi/llm2api/internal/routing"
)

func TestStreamUsesTheNextSamePoolKeyBeforeCommitting(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000011")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000012")
	modelID := uuid.New()
	poolID := uuid.New()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	minimumOutputTokens := int64(1)
	repository := &capacityFailoverRepository{
		model: Model{
			ID: modelID, PublicName: "public-model", UpstreamName: "upstream-model", ProviderID: uuid.New(),
			ProviderKind: providers.KindSiliconFlow, ProviderBaseURL: "https://provider.example/v1",
			Capabilities: registry.ModelCapabilities{Chat: true, Streaming: true, StreamUsage: true, ContextTokens: 8192, OutputTokens: 2048, Parameters: providers.ParameterCapabilities{
				MaxOutputTokens: providers.IntegerParameterLimit{Supported: true, Minimum: &minimumOutputTokens},
			}},
		},
		candidates: []Candidate{{ID: firstID}, {ID: secondID}},
	}
	accounting := &capacityFailoverAccounting{poolID: poolID}
	coordinator := &capacityFailoverCoordinator{}
	providerCalls := 0
	clients := map[uuid.UUID]*http.Client{
		firstID: {Transport: capacityFailoverRoundTrip(func(*http.Request) (*http.Response, error) {
			providerCalls++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"key capacity exhausted","type":"rate_limit_error","code":"rate_limit"}}`)),
			}, nil
		})},
		secondID: {Transport: capacityFailoverRoundTrip(func(*http.Request) (*http.Response, error) {
			providerCalls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"id\":\"stream-1\",\"created\":1,\"model\":\"upstream-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n" +
						"data: [DONE]\n\n",
				)),
			}, nil
		})},
	}
	capabilities := providers.SiliconFlowCapabilities()
	capabilities.Streaming = true
	adapter, err := providers.NewSiliconFlow(providers.SiliconFlowOptions{
		BaseURL: "https://provider.example/v1", Capabilities: capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := capacityFailoverClock{now: now}
	random := capacityFailoverRandom{}
	router, err := routing.NewRouter(random)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := resilience.NewRetryPolicy(resilience.RetryConfig{
		MaxAttempts: 2, MaxElapsed: time.Minute,
		Backoff: resilience.BackoffConfig{Initial: time.Millisecond, Maximum: time.Millisecond, MultiplierNumerator: 1, MultiplierDenominator: 1},
	}, clock, random)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(
		repository, accounting, capacityFailoverSecrets{}, capacityFailoverAdmitter{}, coordinator,
		capacityFailoverFactory{adapter: adapter, clients: clients}, router, retry, clock,
		Config{MaxResponseBytes: 1 << 20, ExecutionHeartbeatInterval: time.Hour, Circuit: resilience.CircuitConfig{OpenDuration: time.Minute}},
	)
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := int64(32)
	includeUsage := true
	events := make([]canonical.StreamEvent, 0, 4)
	providerError := service.Stream(context.Background(), ChatCommand{
		Principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()},
		Request: canonical.ChatRequest{
			Model: "public-model", Stream: true, StreamUsage: &includeUsage, MaxOutputTokens: &maxTokens,
			Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("hi")}},
		},
		RequestDigest: []byte("same-pool-stream-failover"),
	}, func(_ uuid.UUID, event canonical.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if providerError != nil {
		t.Fatalf("Stream() error = %v", providerError)
	}
	if providerCalls != 2 {
		t.Fatalf("provider calls = %d, want 2", providerCalls)
	}
	if len(coordinator.requests) != 2 || coordinator.requests[0].CredentialID != firstID || coordinator.requests[1].CredentialID != secondID {
		t.Fatalf("stream candidates = %#v", coordinator.requests)
	}
	for _, request := range coordinator.requests {
		if request.ResourcePoolID != poolID || request.ModelID != modelID {
			t.Fatalf("stream request escaped the accepted route: %#v", request)
		}
	}
	if len(repository.attemptCredentials) != 2 || repository.attemptCredentials[0] != firstID || repository.attemptCredentials[1] != secondID {
		t.Fatalf("persisted stream attempts = %v", repository.attemptCredentials)
	}
	if len(events) == 0 || events[len(events)-1].Type != canonical.StreamDone {
		t.Fatalf("stream events = %#v", events)
	}
	for _, event := range events {
		if event.Model != "public-model" || event.CompletionID == "stream-1" || !strings.HasPrefix(event.CompletionID, "chatcmpl_") {
			t.Fatalf("public stream identity = id %q model %q", event.CompletionID, event.Model)
		}
	}
	if !containsStreamEvent(events, canonical.StreamUsage) {
		t.Fatalf("requested stream usage was not emitted: %#v", events)
	}
	if len(accounting.completedUsage) != 1 || accounting.completedUsage[0].InputTokens != 2 || accounting.completedUsage[0].OutputTokens != 1 {
		t.Fatalf("completed usage = %#v", accounting.completedUsage)
	}
}

func containsStreamEvent(events []canonical.StreamEvent, eventType canonical.StreamEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
