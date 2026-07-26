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
	"github.com/luckymaomi/llm2api/internal/execution"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/resilience"
	"github.com/luckymaomi/llm2api/internal/routing"
)

func TestChatUsesEverySafeSamePoolCandidateBeforeReturning(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	thirdID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	modelID := uuid.New()
	poolID := uuid.New()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	minimumOutputTokens := int64(1)
	repository := &capacityFailoverRepository{
		model: Model{
			ID: modelID, PublicName: "public-model", UpstreamName: "upstream-model", ProviderID: uuid.New(),
			ProviderKind: providers.KindSiliconFlow, ProviderBaseURL: "https://provider.example/v1",
			Capabilities: registry.ModelCapabilities{Chat: true, ContextTokens: 8192, OutputTokens: 2048, Parameters: providers.ParameterCapabilities{
				MaxOutputTokens: providers.IntegerParameterLimit{Supported: true, Minimum: &minimumOutputTokens},
			}},
		},
		candidates: []Candidate{{ID: firstID}, {ID: secondID}, {ID: thirdID}},
	}
	accounting := &capacityFailoverAccounting{poolID: poolID}
	coordinator := &capacityFailoverCoordinator{fullCredentialID: firstID, retryAt: now.Add(time.Second)}
	providerCalls := 0
	rateLimitedClient := &http.Client{Transport: capacityFailoverRoundTrip(func(*http.Request) (*http.Response, error) {
		providerCalls++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"1"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"key capacity exhausted","type":"rate_limit_error","code":"rate_limit"}}`)),
		}, nil
	})}
	successClient := &http.Client{Transport: capacityFailoverRoundTrip(func(*http.Request) (*http.Response, error) {
		providerCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl-1","model":"upstream-model","created":1,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
			)),
		}, nil
	})}
	adapter, err := providers.NewSiliconFlow(providers.SiliconFlowOptions{
		BaseURL: "https://provider.example/v1", Capabilities: providers.SiliconFlowCapabilities(),
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
		capacityFailoverFactory{adapter: adapter, clients: map[uuid.UUID]*http.Client{
			secondID: rateLimitedClient,
			thirdID:  successClient,
		}}, router, retry, clock,
		Config{MaxResponseBytes: 1 << 20, ExecutionHeartbeatInterval: time.Hour, Circuit: resilience.CircuitConfig{OpenDuration: time.Minute}},
	)
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := int64(32)
	result, providerError := service.Chat(context.Background(), ChatCommand{
		Principal: identity.GatewayPrincipal{UserID: uuid.New(), KeyID: uuid.New()},
		Request: canonical.ChatRequest{
			Model: "public-model", MaxOutputTokens: &maxTokens,
			Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("hi")}},
		},
		RequestDigest: []byte("same-pool-capacity-failover"),
	})
	if providerError != nil {
		t.Fatalf("Chat() error = %v", providerError)
	}
	if result.Response.Model != "public-model" || result.Response.ID == "chatcmpl-1" || !strings.HasPrefix(result.Response.ID, "chatcmpl_") {
		t.Fatalf("public completion identity = id %q model %q", result.Response.ID, result.Response.Model)
	}
	if providerCalls != 2 {
		t.Fatalf("provider calls = %d, want 2", providerCalls)
	}
	if len(coordinator.requests) != 3 || coordinator.requests[0].CredentialID != firstID || coordinator.requests[1].CredentialID != secondID || coordinator.requests[2].CredentialID != thirdID {
		t.Fatalf("capacity candidates = %#v", coordinator.requests)
	}
	for _, request := range coordinator.requests {
		if request.ResourcePoolID != poolID || request.ModelID != modelID {
			t.Fatalf("capacity request escaped the accepted route: %#v", request)
		}
	}
	if len(repository.attemptCredentials) != 2 || repository.attemptCredentials[0] != secondID || repository.attemptCredentials[1] != thirdID || repository.attemptSequences[0] != 1 || repository.attemptSequences[1] != 2 {
		t.Fatalf("persisted upstream attempts = %v sequences %v", repository.attemptCredentials, repository.attemptSequences)
	}
	if len(accounting.completedUsage) != 1 || accounting.completedUsage[0].InputTokens != 2 || accounting.completedUsage[0].OutputTokens != 1 {
		t.Fatalf("completed usage = %#v", accounting.completedUsage)
	}
	if len(coordinator.reconciledTokens) != 1 || coordinator.reconciledTokens[0] != 3 {
		t.Fatalf("authoritative usage was not reconciled to the reservation: %#v", coordinator.reconciledTokens)
	}
}

type capacityFailoverRepository struct {
	model              Model
	candidates         []Candidate
	attemptCredentials []uuid.UUID
	attemptSequences   []int
}

func (r *capacityFailoverRepository) ListAvailableModels(context.Context, uuid.UUID) ([]Model, error) {
	return []Model{r.model}, nil
}

func (r *capacityFailoverRepository) ResolveAvailableModel(_ context.Context, _ uuid.UUID, name string) (Model, error) {
	if name != r.model.PublicName {
		return Model{}, ErrModelNotFound
	}
	return r.model, nil
}

func (r *capacityFailoverRepository) ListResourcePoolCandidates(context.Context, uuid.UUID, uuid.UUID) ([]Candidate, error) {
	return r.candidates, nil
}

func (*capacityFailoverRepository) AcquireCredentialHealthPermit(context.Context, uuid.UUID) (int64, error) {
	return 1, nil
}

func (*capacityFailoverRepository) ClaimExecution(_ context.Context, requestID, executionID uuid.UUID) (execution.Claim, error) {
	return execution.Claim{RequestID: requestID, ExecutionID: executionID, Generation: 1}, nil
}

func (*capacityFailoverRepository) HeartbeatExecution(context.Context, execution.Claim) error {
	return nil
}

func (*capacityFailoverRepository) MarkExecutionStreaming(context.Context, execution.Claim, uuid.UUID, AttemptUpdate) error {
	return nil
}

func (*capacityFailoverRepository) MarkExecutionUncertain(context.Context, execution.Claim, uuid.UUID, AttemptUpdate, string, string) error {
	return nil
}

func (*capacityFailoverRepository) RecoverStaleExecutions(context.Context, time.Time, int32) (int64, error) {
	return 0, nil
}

func (*capacityFailoverRepository) ListRecoverableCompletions(context.Context, time.Time, int32) ([]RecoverableCompletion, error) {
	return nil, nil
}

func (*capacityFailoverRepository) ListStaleQueuedRequests(context.Context, time.Time, int32) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *capacityFailoverRepository) CreateAttempt(_ context.Context, _ execution.Claim, credentialID uuid.UUID, sequence int) (uuid.UUID, error) {
	r.attemptCredentials = append(r.attemptCredentials, credentialID)
	r.attemptSequences = append(r.attemptSequences, sequence)
	return uuid.New(), nil
}

func (*capacityFailoverRepository) UpdateAttempt(context.Context, execution.Claim, uuid.UUID, AttemptUpdate) error {
	return nil
}

type capacityFailoverAccounting struct {
	poolID         uuid.UUID
	completedUsage []Usage
}

func (a *capacityFailoverAccounting) AcceptRequest(_ context.Context, command AcceptCommand) (Accepted, error) {
	return Accepted{RequestID: command.RequestID, ResourcePoolID: a.poolID}, nil
}

func (a *capacityFailoverAccounting) Complete(_ context.Context, _ execution.Claim, usage Usage) error {
	a.completedUsage = append(a.completedUsage, usage)
	return nil
}

func (*capacityFailoverAccounting) Fail(context.Context, execution.Claim, string, string) error {
	return nil
}
func (*capacityFailoverAccounting) FailAccepted(context.Context, uuid.UUID, string, string) error {
	return nil
}
func (*capacityFailoverAccounting) FailWithUsage(context.Context, execution.Claim, Usage, string) error {
	return nil
}

type capacityFailoverCoordinator struct {
	fullCredentialID uuid.UUID
	retryAt          time.Time
	requests         []LeaseRequest
	reconciledTokens []int64
}

func (c *capacityFailoverCoordinator) Acquire(ctx context.Context, request LeaseRequest) (Lease, time.Duration, error) {
	c.requests = append(c.requests, request)
	if request.CredentialID == c.fullCredentialID {
		return nil, 0, &CapacityError{RetryAt: c.retryAt}
	}
	return capacityFailoverLease{ctx: ctx, coordinator: c}, 0, nil
}

type capacityFailoverLease struct {
	ctx         context.Context
	coordinator *capacityFailoverCoordinator
}

func (l capacityFailoverLease) Context() context.Context { return l.ctx }
func (l capacityFailoverLease) Reconcile(_ context.Context, tokens int64) error {
	l.coordinator.reconciledTokens = append(l.coordinator.reconciledTokens, tokens)
	return nil
}
func (capacityFailoverLease) Release(context.Context) error { return nil }
func (capacityFailoverSecrets) CredentialSecret(context.Context, uuid.UUID) (string, error) {
	return "upstream-secret", nil
}

type capacityFailoverSecrets struct{}

type capacityFailoverAdmitter struct{}

func (capacityFailoverAdmitter) Acquire(context.Context, AdmissionRequest) (AdmissionPermit, time.Duration, error) {
	return capacityFailoverAdmissionPermit{}, 0, nil
}

type capacityFailoverAdmissionPermit struct{}

func (capacityFailoverAdmissionPermit) Release() {}

type capacityFailoverFactory struct {
	adapter providers.Adapter
	clients map[uuid.UUID]*http.Client
}

func (f capacityFailoverFactory) Adapter(Model) (providers.Adapter, error) { return f.adapter, nil }
func (f capacityFailoverFactory) Client(candidate Candidate) (*http.Client, error) {
	return f.clients[candidate.ID], nil
}

type capacityFailoverClock struct{ now time.Time }

func (c capacityFailoverClock) Now() time.Time { return c.now }

type capacityFailoverRandom struct{}

func (capacityFailoverRandom) Intn(int) int       { return 0 }
func (capacityFailoverRandom) Int63n(int64) int64 { return 0 }

type capacityFailoverRoundTrip func(*http.Request) (*http.Response, error)

func (f capacityFailoverRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
