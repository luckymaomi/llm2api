package requestflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/execution"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/resilience"
	"github.com/luckymaomi/llm2api/internal/routing"
)

type Config struct {
	MaxResponseBytes           int64
	ExecutionHeartbeatInterval time.Duration
	Circuit                    resilience.CircuitConfig
}

type Service struct {
	repository  Repository
	accounting  Accounting
	secrets     SecretResolver
	admitter    Admitter
	coordinator Coordinator
	factory     AdapterFactory
	router      *routing.Router
	retry       *resilience.RetryPolicy
	clock       Clock
	config      Config
	observer    Observer
}

// healthPermit records that PostgreSQL admitted this credential's shared health
// state. Completion is persisted with the attempt; it must not create a local
// process-only circuit state.
type healthPermit struct{}

func (*healthPermit) Complete(resilience.PermitResult) {}

func New(repository Repository, accounting Accounting, secrets SecretResolver, admitter Admitter, coordinator Coordinator, factory AdapterFactory, router *routing.Router, retry *resilience.RetryPolicy, clock Clock, config Config) (*Service, error) {
	if repository == nil || accounting == nil || secrets == nil || admitter == nil || coordinator == nil || factory == nil || router == nil || retry == nil || clock == nil {
		return nil, errors.New("request workflow dependencies are required")
	}
	if config.MaxResponseBytes < 1024 {
		return nil, errors.New("maximum provider response size must be at least 1024 bytes")
	}
	if config.ExecutionHeartbeatInterval <= 0 {
		return nil, errors.New("execution heartbeat interval must be positive")
	}
	if config.Circuit.OpenDuration <= 0 {
		return nil, errors.New("request workflow circuit configuration is invalid")
	}
	return &Service{
		repository: repository, accounting: accounting, secrets: secrets, admitter: admitter, coordinator: coordinator,
		factory: factory, router: router, retry: retry, clock: clock, config: config,
		observer: noopObserver{},
	}, nil
}

func (s *Service) WithObserver(observer Observer) *Service {
	if observer != nil {
		s.observer = observer
	}
	return s
}

type noopObserver struct{}

func (noopObserver) ProviderAttempt(providers.Kind, string, string) {}

func (s *Service) observeAttempt(kind providers.Kind, err *canonical.Error) {
	if err == nil {
		s.observer.ProviderAttempt(kind, "succeeded", "none")
		return
	}
	outcome := "failed"
	if err.Kind == canonical.ErrorUncertain || err.Kind == canonical.ErrorStreamInterrupted {
		outcome = "uncertain"
	}
	s.observer.ProviderAttempt(kind, outcome, string(err.Kind))
}

func (s *Service) Models(ctx context.Context, gatewayKeyID uuid.UUID) ([]Model, error) {
	return s.repository.ListAvailableModels(ctx, gatewayKeyID)
}

func (s *Service) prepare(ctx context.Context, command ChatCommand) (workflowRun, *canonical.Error) {
	model, err := s.repository.ResolveAvailableModel(ctx, command.Principal.KeyID, command.Request.Model)
	if err != nil {
		return workflowRun{}, workflowError(err)
	}
	if validationError := validateCapabilities(model, command.Request); validationError != nil {
		return workflowRun{}, validationError
	}
	estimatedTokens := EstimateTokens(command.Request)
	if model.Capabilities.ContextTokens > 0 && estimatedTokens > model.Capabilities.ContextTokens {
		return workflowRun{}, &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "context_length_exceeded", Message: "request exceeds the configured model context", Parameter: "messages", HTTPStatus: http.StatusBadRequest}
	}
	upstreamRequest := command.Request
	upstreamRequest.Model = model.UpstreamName
	run := workflowRun{command: command, model: model, request: upstreamRequest, estimatedTokens: estimatedTokens}
	requestID := command.RequestID
	if requestID == uuid.Nil {
		requestID = uuid.New()
	}
	admissionPermit, _, err := s.admitter.Acquire(ctx, AdmissionRequest{RequestID: requestID, UserID: command.Principal.UserID})
	if err != nil {
		return workflowRun{}, admissionError(err)
	}
	releaseAdmission := func() {
		if admissionPermit != nil {
			admissionPermit.Release()
			admissionPermit = nil
		}
	}
	accepted, err := s.accounting.AcceptRequest(ctx, AcceptCommand{
		RequestID: requestID, UserID: command.Principal.UserID, GatewayKeyID: command.Principal.KeyID, ModelID: model.ID,
		IdempotencyKey: command.IdempotencyKey,
		RequestDigest:  command.RequestDigest, Stream: command.Request.Stream,
	})
	if err != nil {
		releaseAdmission()
		return workflowRun{}, workflowError(err)
	}
	if accepted.Existing {
		releaseAdmission()
		return workflowRun{}, &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "request_already_accepted", Message: "idempotent request already exists", HTTPStatus: http.StatusConflict}
	}
	if accepted.RequestID != requestID || accepted.ResourcePoolID == uuid.Nil {
		releaseAdmission()
		if accepted.RequestID != uuid.Nil {
			_ = s.accounting.FailAccepted(context.WithoutCancel(ctx), accepted.RequestID, "invalid_acceptance", "accepted request is missing its coordination capacity")
		}
		return workflowRun{}, &canonical.Error{Kind: canonical.ErrorInternalInvariant, Code: "invalid_acceptance", Message: "accepted request is missing required execution capacity"}
	}
	candidates, err := s.repository.ListResourcePoolCandidates(ctx, accepted.ResourcePoolID, model.ID)
	if err != nil {
		releaseAdmission()
		_ = s.accounting.FailAccepted(context.WithoutCancel(ctx), accepted.RequestID, "candidate_lookup_failed", "upstream candidates could not be read")
		return workflowRun{}, &canonical.Error{Kind: canonical.ErrorStorageUnavailable, Code: "candidate_lookup_failed", Message: "upstream candidates could not be read", Cause: err}
	}
	run.accepted, run.candidates = accepted, candidates
	if len(candidates) == 0 {
		releaseAdmission()
		_ = s.accounting.FailAccepted(context.WithoutCancel(ctx), accepted.RequestID, "resource_pool_unavailable", "no eligible upstream credential is available")
		return workflowRun{}, &canonical.Error{Kind: canonical.ErrorProviderTemporary, Code: poolUnavailableCode(), Message: "no eligible upstream credential is available"}
	}
	if command.AcceptedSink != nil {
		if err := command.AcceptedSink(context.WithoutCancel(ctx), accepted.RequestID); err != nil {
			releaseAdmission()
			_ = s.accounting.FailAccepted(context.WithoutCancel(ctx), accepted.RequestID, "acceptance_persistence_failed", "accepted request could not be linked to its caller")
			return workflowRun{}, storageError("acceptance_persistence_failed", err)
		}
	}
	claim, err := s.repository.ClaimExecution(ctx, accepted.RequestID, uuid.New())
	if err != nil {
		releaseAdmission()
		_ = s.accounting.FailAccepted(context.WithoutCancel(ctx), accepted.RequestID, "execution_claim_failed", "request execution could not be claimed")
		return workflowRun{}, storageError("execution_claim_failed", err)
	}
	run.claim = claim
	run.context, run.stopHeartbeat = s.executionContext(ctx, claim)
	run.admissionPermit = admissionPermit
	return run, nil
}

type workflowRun struct {
	command         ChatCommand
	model           Model
	request         canonical.ChatRequest
	accepted        Accepted
	claim           execution.Claim
	context         context.Context
	stopHeartbeat   context.CancelFunc
	admissionPermit AdmissionPermit
	candidates      []Candidate
	estimatedTokens int64
}

func (run *workflowRun) releaseAdmission() {
	if run == nil || run.admissionPermit == nil {
		return
	}
	run.admissionPermit.Release()
	run.admissionPermit = nil
}

func (run *workflowRun) stopExecution() {
	if run == nil || run.stopHeartbeat == nil {
		return
	}
	run.stopHeartbeat()
	run.stopHeartbeat = nil
}

func validateCapabilities(model Model, request canonical.ChatRequest) *canonical.Error {
	unsupported := func(parameter string) *canonical.Error {
		return &canonical.Error{Kind: canonical.ErrorUnsupportedCapability, Code: "unsupported_capability", Message: "model does not support the requested capability", Parameter: parameter, HTTPStatus: http.StatusBadRequest}
	}
	if request.Stream && !model.Capabilities.Streaming {
		return unsupported("stream")
	}
	if len(request.Tools) > 0 && !model.Capabilities.Tools {
		return unsupported("tools")
	}
	if request.Reasoning != nil && !model.Capabilities.Reasoning {
		return unsupported("thinking")
	}
	if request.ResponseFormat != nil && request.ResponseFormat.Type != canonical.ResponseFormatText && !model.Capabilities.StructuredOutput {
		return unsupported("response_format")
	}
	if request.MaxOutputTokens != nil && model.Capabilities.OutputTokens > 0 && *request.MaxOutputTokens > model.Capabilities.OutputTokens {
		return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "max_output_tokens_exceeded", Message: "requested output exceeds the model limit", Parameter: "max_completion_tokens", HTTPStatus: http.StatusBadRequest}
	}
	return nil
}

func (s *Service) candidateDecision(ctx context.Context, run workflowRun, excluded *[]routing.CandidateID) (Candidate, *healthPermit, *canonical.Error) {
	selectionExcluded := append([]routing.CandidateID(nil), (*excluded)...)
	var earliestRetryAt *time.Time
	circuitUnavailable := false
	for {
		candidate, selectionError := s.selectCandidate(run, selectionExcluded)
		if selectionError != nil {
			if !circuitUnavailable {
				return Candidate{}, nil, selectionError
			}
			earliestRetryAt = earlierRetryAt(earliestRetryAt, retryAfterAt(selectionError.RetryAfter, s.clock.Now().UTC()))
			return Candidate{}, nil, circuitUnavailableError(earliestRetryAt)
		}
		healthGeneration, healthErr := s.repository.AcquireCredentialHealthPermit(ctx, candidate.ID)
		if healthErr != nil {
			circuitUnavailable = true
			candidateID := routing.CandidateID(candidate.ID.String())
			selectionExcluded = append(selectionExcluded, candidateID)
			*excluded = append(*excluded, candidateID)
			continue
		}
		candidate.HealthGeneration = healthGeneration
		return candidate, &healthPermit{}, nil
	}
}

func (s *Service) attemptDecision(ctx context.Context, run *workflowRun, excluded *[]routing.CandidateID, _ int) (Candidate, *healthPermit, Lease, *canonical.Error) {
	var earliestCapacityRetryAt *time.Time
	capacityUnavailable := false
	for {
		candidate, permit, selectionError := s.candidateDecision(ctx, *run, excluded)
		if selectionError != nil {
			if capacityUnavailable {
				return Candidate{}, nil, nil, admissionError(&CapacityError{RetryAt: valueOrNow(earliestCapacityRetryAt, s.clock.Now().UTC())})
			}
			return Candidate{}, nil, nil, selectionError
		}
		lease, _, err := s.coordinator.Acquire(ctx, s.leaseRequest(run.claim, *run, candidate))
		if err == nil {
			return candidate, permit, lease, nil
		}
		permit.Complete(resilience.PermitReleased)
		var capacityError *CapacityError
		if !errors.As(err, &capacityError) {
			return Candidate{}, nil, nil, admissionError(err)
		}
		capacityUnavailable = true
		retryAt := capacityError.RetryAt.UTC()
		earliestCapacityRetryAt = earlierRetryAt(earliestCapacityRetryAt, &retryAt)
		*excluded = append(*excluded, routing.CandidateID(candidate.ID.String()))
	}
}

func valueOrNow(value *time.Time, now time.Time) time.Time {
	if value == nil || value.IsZero() {
		return now
	}
	return value.UTC()
}

func circuitUnavailableError(retryAt *time.Time) *canonical.Error {
	providerError := &canonical.Error{
		Kind: canonical.ErrorProviderTemporary, Code: "upstream_circuit_open", Message: "all eligible upstream credentials are cooling down",
	}
	if retryAt != nil && !retryAt.IsZero() {
		at := retryAt.UTC()
		providerError.RetryAfter = &canonical.RetryAfter{At: &at}
	}
	return providerError
}

func earlierRetryAt(left, right *time.Time) *time.Time {
	if left == nil || left.IsZero() {
		return right
	}
	if right == nil || right.IsZero() || left.Before(*right) {
		return left
	}
	return right
}

func (s *Service) selectCandidate(run workflowRun, excluded []routing.CandidateID) (Candidate, *canonical.Error) {
	required := requiredCapabilities(run.request)
	routeCandidates := make([]routing.Candidate, 0, len(run.candidates))
	byID := make(map[routing.CandidateID]Candidate, len(run.candidates))
	now := s.clock.Now().UTC()
	for _, candidate := range run.candidates {
		id := routing.CandidateID(candidate.ID.String())
		byID[id] = candidate
		routeCandidates = append(routeCandidates, routing.Candidate{
			ID: id, ModelID: routing.ModelID(run.model.ID.String()), ResourcePoolID: routing.ResourcePoolID(run.accepted.ResourcePoolID.String()),
			ModelPublished: true, CredentialAuthorized: true, CredentialActive: true,
			Capabilities: required, CooldownUntil: timeOrZero(candidate.CooldownUntil),
		})
	}
	decision, err := s.router.Select(routing.Requirements{
		ModelID: routing.ModelID(run.model.ID.String()), ResourcePoolID: routing.ResourcePoolID(run.accepted.ResourcePoolID.String()),
		Capabilities: required, ExcludedCandidates: excluded, At: now,
	}, routeCandidates)
	if err != nil {
		return Candidate{}, &canonical.Error{Kind: canonical.ErrorInternalInvariant, Code: "routing_failed", Message: "upstream routing failed", Cause: err}
	}
	if decision.SelectedCandidateID == "" {
		providerError := &canonical.Error{Kind: canonical.ErrorProviderTemporary, Code: poolUnavailableCode(), Message: "no eligible upstream credential is available"}
		if !decision.NextAvailableAt.IsZero() {
			retryAt := decision.NextAvailableAt.UTC()
			providerError.RetryAfter = &canonical.RetryAfter{At: &retryAt}
		}
		return Candidate{}, providerError
	}
	candidate := byID[decision.SelectedCandidateID]
	return candidate, nil
}

func (s *Service) leaseRequest(claim execution.Claim, run workflowRun, candidate Candidate) LeaseRequest {
	return LeaseRequest{RequestID: claim.RequestID, ExecutionID: claim.ExecutionID,
		ModelID: run.model.ID, ProviderID: run.model.ProviderID, CredentialID: candidate.ID,
		ResourcePoolID:  run.accepted.ResourcePoolID,
		EstimatedTokens: run.estimatedTokens,
		RPMLimit:        candidate.RPMLimit, TPMLimit: candidate.TPMLimit, Concurrency: candidate.ConcurrencyLimit}
}

func admissionError(err error) *canonical.Error {
	var capacity *CapacityError
	if errors.As(err, &capacity) {
		return &canonical.Error{Kind: canonical.ErrorRateLimit, Code: "admission_capacity_exhausted", Message: "request capacity is exhausted; retry after the reported time", RetryAfter: &canonical.RetryAfter{At: &capacity.RetryAt}, Cause: err}
	}
	switch {
	case errors.Is(err, ErrAdmissionQueueFull):
		return &canonical.Error{Kind: canonical.ErrorRateLimit, Code: "admission_queue_full", Message: "the request queue is full", HTTPStatus: http.StatusTooManyRequests, Cause: err}
	case errors.Is(err, ErrAdmissionTimedOut):
		return &canonical.Error{Kind: canonical.ErrorRateLimit, Code: "admission_timeout", Message: "the request waited too long for capacity", HTTPStatus: http.StatusTooManyRequests, Cause: err}
	case errors.Is(err, ErrAdmissionCanceled):
		return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "request_canceled", Message: "the request was canceled while waiting for capacity", HTTPStatus: http.StatusRequestTimeout, Cause: err}
	case errors.Is(err, ErrCoordinationFailed):
		return &canonical.Error{Kind: canonical.ErrorStorageUnavailable, Code: "admission_unavailable", Message: "request coordination is temporarily unavailable", HTTPStatus: http.StatusServiceUnavailable, Cause: err}
	}
	return &canonical.Error{Kind: canonical.ErrorInternalInvariant, Code: "admission_failed", Message: "request admission failed", Cause: err}
}

func requiredCapabilities(request canonical.ChatRequest) []routing.Capability {
	capabilities := []routing.Capability{"chat"}
	if request.Stream {
		capabilities = append(capabilities, "streaming")
	}
	if len(request.Tools) > 0 {
		capabilities = append(capabilities, "tools")
	}
	if request.Reasoning != nil {
		capabilities = append(capabilities, "reasoning")
	}
	if request.ResponseFormat != nil && request.ResponseFormat.Type != canonical.ResponseFormatText {
		capabilities = append(capabilities, "structured_output")
	}
	return capabilities
}

func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func poolUnavailableCode() string {
	return "resource_pool_unavailable"
}

func workflowError(err error) *canonical.Error {
	switch {
	case errors.Is(err, ErrModelNotFound):
		return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "model_not_found", Message: "model was not found", Parameter: "model", HTTPStatus: http.StatusNotFound}
	case errors.Is(err, ErrModelNotAuthorized):
		return &canonical.Error{Kind: canonical.ErrorPermission, Code: "model_not_authorized", Message: "model is not authorized for this key", Parameter: "model", HTTPStatus: http.StatusForbidden}
	case errors.Is(err, ErrIdempotencyConflict):
		return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "idempotency_conflict", Message: "idempotency key was reused with a different request", HTTPStatus: http.StatusConflict}
	case errors.Is(err, ErrInvalidAccounting):
		return &canonical.Error{Kind: canonical.ErrorInvalidRequest, Code: "invalid_request_accounting", Message: "request usage could not be recorded", HTTPStatus: http.StatusBadRequest}
	default:
		return &canonical.Error{Kind: canonical.ErrorStorageUnavailable, Code: "request_state_unavailable", Message: "request state could not be persisted", Cause: err}
	}
}

func readResponse(response *http.Response, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("provider response exceeds %d bytes", limit)
	}
	return body, nil
}

func providerCredential(secret string) providers.Credential {
	return providers.Credential{APIKey: secret}
}
