package usage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/execution"
	"github.com/luckymaomi/llm2api/internal/identity"
)

var (
	ErrInvalidInput       = errors.New("invalid usage input")
	ErrForbidden          = errors.New("usage operation forbidden")
	ErrNotFound           = errors.New("usage record not found")
	ErrConflict           = errors.New("usage conflict")
	ErrModelNotAuthorized = errors.New("model is not authorized")
	ErrUsageUnknown       = errors.New("usage is unknown")
	ErrOutcomeUnknown     = errors.New("usage operation outcome is unknown")
	ErrInvariant          = errors.New("usage invariant violated")
)

type UsageSource string

const (
	UsageAuthoritative UsageSource = "authoritative"
	UsageEstimated     UsageSource = "estimated"
	UsageUnknown       UsageSource = "unknown"
)

type RequestStatus string

const (
	RequestQueued      RequestStatus = "queued"
	RequestDispatching RequestStatus = "dispatching"
	RequestStreaming   RequestStatus = "streaming"
	RequestCompleted   RequestStatus = "completed"
	RequestFailed      RequestStatus = "failed"
	RequestCanceled    RequestStatus = "canceled"
	RequestUncertain   RequestStatus = "uncertain"
)

type Page struct {
	Offset int32
	Size   int32
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type RequestLogQuery struct {
	UserID         *uuid.UUID
	GatewayKeyID   *uuid.UUID
	ModelID        *uuid.UUID
	ResourcePoolID *uuid.UUID
	Search         string
	Status         RequestStatus
	From           time.Time
	To             time.Time
	Page           Page
}

type RequestLog struct {
	RequestID         uuid.UUID
	UserID            uuid.UUID
	UserName          string
	GatewayKeyID      uuid.UUID
	KeyPrefix         string
	ModelID           uuid.UUID
	ModelAlias        string
	ResourcePoolID    uuid.UUID
	ResourcePoolName  string
	ResourcePoolSlug  string
	Status            RequestStatus
	Stream            bool
	InputTokens       *int64
	OutputTokens      *int64
	UsageSource       UsageSource
	ErrorKind         *string
	AcceptedAt        time.Time
	CompletedAt       *time.Time
	UpdatedAt         time.Time
	AttemptCount      int64
	LastAttemptStatus *string
}

type RequestAttempt struct {
	ID                uuid.UUID
	Sequence          int32
	Status            string
	ProviderName      string
	CredentialName    string
	UpstreamRequestID *string
	HTTPStatus        *int32
	ErrorKind         *string
	RetryAfterAt      *time.Time
	SentAt            *time.Time
	FirstByteAt       *time.Time
	CompletedAt       *time.Time
	InputTokens       *int64
	OutputTokens      *int64
	UsageSource       UsageSource
	CreatedAt         time.Time
}

type RequestLogDetail struct {
	RequestLog
	Attempts []RequestAttempt
}

type AcceptInput struct {
	RequestID      uuid.UUID
	UserID         uuid.UUID
	GatewayKeyID   uuid.UUID
	ModelID        uuid.UUID
	Stream         bool
	RequestDigest  []byte
	IdempotencyKey *string
}

type Request struct {
	ID             uuid.UUID     `json:"id"`
	IdempotencyKey *string       `json:"idempotency_key,omitempty"`
	UserID         uuid.UUID     `json:"user_id"`
	GatewayKeyID   uuid.UUID     `json:"gateway_key_id"`
	ModelID        uuid.UUID     `json:"model_id"`
	SubscriptionID *uuid.UUID    `json:"subscription_id,omitempty"`
	ResourcePoolID uuid.UUID     `json:"resource_pool_id"`
	Status         RequestStatus `json:"status"`
	Stream         bool          `json:"stream"`
	InputTokens    *int64        `json:"input_tokens,omitempty"`
	OutputTokens   *int64        `json:"output_tokens,omitempty"`
	UsageSource    UsageSource   `json:"usage_source"`
	ErrorKind      *string       `json:"error_kind,omitempty"`
	ErrorDetail    *string       `json:"error_detail,omitempty"`
	AcceptedAt     time.Time     `json:"accepted_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type AcceptedRequest struct {
	Request  Request `json:"request"`
	Replayed bool    `json:"replayed"`
}

type Resolution struct {
	Request Request `json:"request"`
}

type Repository interface {
	ListRequestLogs(context.Context, RequestLogQuery) (PageResult[RequestLog], error)
	GetRequestLog(context.Context, uuid.UUID, *uuid.UUID) (RequestLogDetail, error)
	AcceptRequest(context.Context, AcceptInput) (AcceptedRequest, error)
	Complete(context.Context, uuid.UUID, execution.Claim, int64, int64, UsageSource) (Resolution, error)
	Fail(context.Context, uuid.UUID, execution.Claim, string, string) (Resolution, error)
	FailAccepted(context.Context, uuid.UUID, string, string) (Resolution, error)
	FailWithUsage(context.Context, uuid.UUID, execution.Claim, int64, int64, UsageSource, string, string) (Resolution, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, ErrInvalidInput
	}
	return &Service{repository: repository}, nil
}

func canReadUser(actor identity.Principal, userID uuid.UUID) bool {
	return actor.Status == identity.StatusActive && (actor.CanManageUsers() || actor.UserID == userID)
}
