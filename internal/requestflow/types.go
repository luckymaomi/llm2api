package requestflow

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/execution"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
)

var (
	ErrModelNotFound       = errors.New("model not found")
	ErrModelNotAuthorized  = errors.New("model not authorized")
	ErrNoEligibleUpstream  = errors.New("no eligible upstream")
	ErrCoordinationFailed  = errors.New("coordination unavailable")
	ErrAdmissionQueueFull  = errors.New("admission queue full")
	ErrAdmissionTimedOut   = errors.New("admission queue timed out")
	ErrAdmissionCanceled   = errors.New("admission canceled")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrInvalidAccounting   = errors.New("invalid accounting command")
)

type Model struct {
	ID              uuid.UUID
	PublicName      string
	UpstreamName    string
	ProviderID      uuid.UUID
	ProviderSlug    string
	ProviderKind    providers.Kind
	ProviderBaseURL string
	Capabilities    registry.ModelCapabilities
	CreatedAt       time.Time
}

type Candidate struct {
	ID                        uuid.UUID
	RPMLimit                  *int32
	TPMLimit                  *int64
	ConcurrencyLimit          *int32
	Priority                  int32
	Weight                    int32
	SharedCapacityScope       *string
	SharedRPMLimit            *int32
	SharedTPMLimit            *int64
	SharedConcurrencyLimit    *int32
	SharedDailyTokenLimit     *int64
	SharedDailyResetMinuteUTC *int32
	ConsecutiveFailures       int32
	LastSuccessAt             *time.Time
	CooldownUntil             *time.Time
	HealthGeneration          int64
}

type AttemptUpdate struct {
	Status            string
	HTTPStatus        *int
	UpstreamRequestID *string
	ErrorKind         *string
	RetryAfterAt      *time.Time
	SentAt            *time.Time
	FirstByteAt       *time.Time
	CompletedAt       *time.Time
	Usage             *Usage
	Credential        *CredentialObservation
}

type CredentialObservationKind string

const (
	CredentialSucceeded CredentialObservationKind = "succeeded"
	CredentialFailed    CredentialObservationKind = "failed"
)

type CredentialObservation struct {
	Kind             CredentialObservationKind
	HealthGeneration int64
	ObservedAt       time.Time
	ErrorKind        string
	CooldownUntil    *time.Time
}

type CatalogRepository interface {
	ListAvailableModels(context.Context, uuid.UUID) ([]Model, error)
}

type Repository interface {
	CatalogRepository
	ResolveAvailableModel(context.Context, uuid.UUID, string) (Model, error)
	ListResourcePoolCandidates(context.Context, uuid.UUID, uuid.UUID) ([]Candidate, error)
	AcquireCredentialHealthPermit(context.Context, uuid.UUID) (int64, error)
	ClaimExecution(context.Context, uuid.UUID, uuid.UUID) (execution.Claim, error)
	HeartbeatExecution(context.Context, execution.Claim) error
	MarkExecutionStreaming(context.Context, execution.Claim, uuid.UUID, AttemptUpdate) error
	MarkExecutionUncertain(context.Context, execution.Claim, uuid.UUID, AttemptUpdate, string, string) error
	RecoverStaleExecutions(context.Context, time.Time, int32) (int64, error)
	ListRecoverableCompletions(context.Context, time.Time, int32) ([]RecoverableCompletion, error)
	ListStaleQueuedRequests(context.Context, time.Time, int32) ([]uuid.UUID, error)
	CreateAttempt(context.Context, execution.Claim, uuid.UUID, int) (uuid.UUID, error)
	UpdateAttempt(context.Context, execution.Claim, uuid.UUID, AttemptUpdate) error
}

type RecoverableCompletion struct {
	Claim execution.Claim
	Usage Usage
}

type RecoveryResult struct {
	Completed      int64
	FailedAccepted int64
	Uncertain      int64
}

type Accepted struct {
	RequestID      uuid.UUID
	ResourcePoolID uuid.UUID
	Existing       bool
}

type AdmissionRequest struct {
	RequestID uuid.UUID
	UserID    uuid.UUID
}

type AdmissionPermit interface {
	Release()
}

type Admitter interface {
	Acquire(context.Context, AdmissionRequest) (AdmissionPermit, time.Duration, error)
}

type AcceptCommand struct {
	RequestID      uuid.UUID
	UserID         uuid.UUID
	GatewayKeyID   uuid.UUID
	ModelID        uuid.UUID
	IdempotencyKey *string
	RequestDigest  []byte
	Stream         bool
}

type Usage struct {
	InputTokens  int64
	OutputTokens int64
	Source       canonical.UsageSource
}

type Accounting interface {
	AcceptRequest(context.Context, AcceptCommand) (Accepted, error)
	Complete(context.Context, execution.Claim, Usage) error
	Fail(context.Context, execution.Claim, string, string) error
	FailAccepted(context.Context, uuid.UUID, string, string) error
	FailWithUsage(context.Context, execution.Claim, Usage, string) error
}

type SecretResolver interface {
	CredentialSecret(context.Context, uuid.UUID) (string, error)
}

type LeaseRequest struct {
	RequestID                 uuid.UUID
	ExecutionID               uuid.UUID
	ModelID                   uuid.UUID
	ProviderID                uuid.UUID
	CredentialID              uuid.UUID
	ResourcePoolID            uuid.UUID
	EstimatedTokens           int64
	RPMLimit                  *int32
	TPMLimit                  *int64
	Concurrency               *int32
	SharedCapacityScope       *string
	SharedRPMLimit            *int32
	SharedTPMLimit            *int64
	SharedConcurrency         *int32
	SharedDailyTokenLimit     *int64
	SharedDailyResetMinuteUTC *int32
}

type Lease interface {
	Context() context.Context
	Reconcile(context.Context, int64) error
	Release(context.Context) error
}

type Coordinator interface {
	Acquire(context.Context, LeaseRequest) (Lease, time.Duration, error)
}

// CredentialCapacityInput contains only the routing identity and configured
// gateway limits needed to inspect one upstream API key. It never contains the
// upstream secret or an upstream balance.
type CredentialCapacityInput struct {
	CredentialID              uuid.UUID
	ProviderID                uuid.UUID
	RPMLimit                  *int32
	TPMLimit                  *int64
	ConcurrencyLimit          *int32
	SharedCapacityScope       *string
	SharedRPMLimit            *int32
	SharedTPMLimit            *int64
	SharedConcurrencyLimit    *int32
	SharedDailyTokenLimit     *int64
	SharedDailyResetMinuteUTC *int32
}

// CapacityObservation is the admission state recorded by the gateway limiter.
// It is not an official Provider account balance.
type CapacityObservation struct {
	ObservedAt              time.Time
	RequestsPerMinuteLimit  int64
	RequestsPerMinuteRemain int64
	TokensPerMinuteLimit    int64
	TokensPerMinuteRemain   int64
	DailyTokenLimit         int64
	DailyTokenRemain        int64
	DailyTokenResetAt       *time.Time
	ConcurrencyLimit        int64
	ConcurrencyInUse        int64
}

// CredentialCapacity projects the Key-local admission state and, when the
// Key belongs to one, the single shared upstream account/project state.
type CredentialCapacity struct {
	CapacityObservation
	Shared *CapacityObservation
}

type CredentialCapacityInspector interface {
	InspectCredential(context.Context, CredentialCapacityInput) (CredentialCapacity, error)
}

type Observer interface {
	ProviderAttempt(providerKind providers.Kind, outcome, errorKind string)
}

type AdapterFactory interface {
	Adapter(Model) (providers.Adapter, error)
	Client(Candidate) (*http.Client, error)
}

type ChatCommand struct {
	Principal      identity.GatewayPrincipal
	Request        canonical.ChatRequest
	RequestDigest  []byte
	IdempotencyKey *string
	RequestID      uuid.UUID
	AcceptedSink   func(context.Context, uuid.UUID) error
	ResultSink     func(context.Context, ChatResult) error
}

type ChatResult struct {
	RequestID uuid.UUID
	Response  canonical.ChatResponse
}

type StreamSink func(uuid.UUID, canonical.StreamEvent) error

type Clock interface {
	Now() time.Time
}

type Random interface {
	Intn(int) int
}
