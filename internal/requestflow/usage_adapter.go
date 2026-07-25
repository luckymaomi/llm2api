package requestflow

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/execution"
	"github.com/luckymaomi/llm2api/internal/usage"
)

type UsageAdapter struct {
	service *usage.Service
}

func NewUsageAdapter(service *usage.Service) (*UsageAdapter, error) {
	if service == nil {
		return nil, errors.New("usage service is required")
	}
	return &UsageAdapter{service: service}, nil
}

func (a *UsageAdapter) AcceptRequest(ctx context.Context, command AcceptCommand) (Accepted, error) {
	accepted, err := a.service.AcceptRequest(ctx, usage.AcceptInput{
		RequestID: command.RequestID,
		UserID:    command.UserID, GatewayKeyID: command.GatewayKeyID, ModelID: command.ModelID,
		Stream: command.Stream, RequestDigest: command.RequestDigest, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return Accepted{}, accountingError(err)
	}
	return Accepted{
		RequestID: accepted.Request.ID, ResourcePoolID: accepted.Request.ResourcePoolID, Existing: accepted.Replayed,
	}, nil
}

func (a *UsageAdapter) Complete(ctx context.Context, claim execution.Claim, observedUsage Usage) error {
	_, err := a.service.Complete(ctx, claim.RequestID, claim, observedUsage.InputTokens, observedUsage.OutputTokens, usage.UsageSource(observedUsage.Source))
	return accountingError(err)
}

func (a *UsageAdapter) Fail(ctx context.Context, claim execution.Claim, errorKind, errorDetail string) error {
	_, err := a.service.Fail(ctx, claim.RequestID, claim, errorKind, errorDetail)
	return accountingError(err)
}

func (a *UsageAdapter) FailAccepted(ctx context.Context, requestID uuid.UUID, errorKind, errorDetail string) error {
	_, err := a.service.FailAccepted(ctx, requestID, errorKind, errorDetail)
	return accountingError(err)
}

func (a *UsageAdapter) FailWithUsage(ctx context.Context, claim execution.Claim, observedUsage Usage, detail string) error {
	_, err := a.service.FailWithUsage(ctx, claim.RequestID, claim, observedUsage.InputTokens, observedUsage.OutputTokens, usage.UsageSource(observedUsage.Source), string(canonical.ErrorUncertain), detail)
	return accountingError(err)
}

func accountingError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, usage.ErrModelNotAuthorized):
		return ErrModelNotAuthorized
	case errors.Is(err, usage.ErrConflict):
		return ErrIdempotencyConflict
	case errors.Is(err, usage.ErrInvalidInput), errors.Is(err, usage.ErrUsageUnknown):
		return ErrInvalidAccounting
	default:
		return err
	}
}
