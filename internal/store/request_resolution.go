package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/execution"
	db "github.com/luckymaomi/llm2api/internal/store/db"
	"github.com/luckymaomi/llm2api/internal/usage"
)

const requestTransactionAttempts = 4

type resolutionKind string

const (
	resolutionComplete      resolutionKind = "complete"
	resolutionFail          resolutionKind = "fail"
	resolutionFailWithUsage resolutionKind = "fail_with_usage"
)

type resolutionCommand struct {
	requestID                 uuid.UUID
	claim                     *execution.Claim
	kind                      resolutionKind
	inputTokens, outputTokens int64
	usageSource               usage.UsageSource
	errorKind, errorDetail    string
}

func (r *UsageRepository) Complete(ctx context.Context, requestID uuid.UUID, claim execution.Claim, inputTokens, outputTokens int64, source usage.UsageSource) (usage.Resolution, error) {
	return r.resolve(ctx, resolutionCommand{requestID: requestID, claim: &claim, kind: resolutionComplete, inputTokens: inputTokens, outputTokens: outputTokens, usageSource: source})
}

func (r *UsageRepository) Fail(ctx context.Context, requestID uuid.UUID, claim execution.Claim, errorKind, errorDetail string) (usage.Resolution, error) {
	return r.resolve(ctx, resolutionCommand{requestID: requestID, claim: &claim, kind: resolutionFail, usageSource: usage.UsageUnknown, errorKind: errorKind, errorDetail: errorDetail})
}

func (r *UsageRepository) FailAccepted(ctx context.Context, requestID uuid.UUID, errorKind, errorDetail string) (usage.Resolution, error) {
	return r.resolve(ctx, resolutionCommand{requestID: requestID, kind: resolutionFail, usageSource: usage.UsageUnknown, errorKind: errorKind, errorDetail: errorDetail})
}

func (r *UsageRepository) FailWithUsage(ctx context.Context, requestID uuid.UUID, claim execution.Claim, inputTokens, outputTokens int64, source usage.UsageSource, errorKind, errorDetail string) (usage.Resolution, error) {
	return r.resolve(ctx, resolutionCommand{requestID: requestID, claim: &claim, kind: resolutionFailWithUsage, inputTokens: inputTokens, outputTokens: outputTokens, usageSource: source, errorKind: errorKind, errorDetail: errorDetail})
}

func (r *UsageRepository) resolve(ctx context.Context, command resolutionCommand) (usage.Resolution, error) {
	tx, err := r.connections.Postgres.Begin(ctx)
	if err != nil {
		return usage.Resolution{}, err
	}
	defer tx.Rollback(ctx)
	queries := r.queries.WithTx(tx)
	request, err := queries.GetRequestForUpdate(ctx, command.requestID)
	if err != nil {
		return usage.Resolution{}, translateUsageError(err)
	}
	if err := validateRequestFence(request, command.claim); err != nil {
		return usage.Resolution{}, err
	}
	var resolved db.Request
	var executionID *uuid.UUID
	var generation int64
	if command.claim != nil {
		executionID, generation = &command.claim.ExecutionID, command.claim.Generation
	}
	switch command.kind {
	case resolutionComplete:
		resolved, err = queries.CompleteRequest(ctx, db.CompleteRequestParams{InputTokens: &command.inputTokens, OutputTokens: &command.outputTokens, UsageSource: db.UsageSource(command.usageSource), ID: command.requestID, ExecutionID: executionID, ExecutionGeneration: generation})
	case resolutionFail:
		resolved, err = queries.FailRequest(ctx, db.FailRequestParams{ErrorKind: &command.errorKind, ErrorDetail: optionalString(command.errorDetail), ID: command.requestID, ExecutionID: executionID, ExecutionGeneration: generation})
	case resolutionFailWithUsage:
		resolved, err = queries.FailRequestWithUsage(ctx, db.FailRequestWithUsageParams{InputTokens: &command.inputTokens, OutputTokens: &command.outputTokens, UsageSource: db.UsageSource(command.usageSource), ErrorKind: &command.errorKind, ErrorDetail: optionalString(command.errorDetail), ID: command.requestID, ExecutionID: executionID, ExecutionGeneration: generation})
	default:
		return usage.Resolution{}, usage.ErrInvariant
	}
	if err != nil {
		return usage.Resolution{}, translateUsageError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return usage.Resolution{}, translateUsageError(err)
	}
	return usage.Resolution{Request: requestFromDB(resolved)}, nil
}

func validateRequestFence(request db.Request, claim *execution.Claim) error {
	if claim == nil {
		if request.ExecutionID != nil || request.ExecutionGeneration != 0 || request.Status != db.RequestStatusQueued {
			return execution.ErrFenced
		}
		return nil
	}
	if !claim.Valid() || claim.RequestID != request.ID || request.ExecutionID == nil || *request.ExecutionID != claim.ExecutionID || request.ExecutionGeneration != claim.Generation || request.Status != db.RequestStatusDispatching && request.Status != db.RequestStatusStreaming {
		return execution.ErrFenced
	}
	return nil
}

func requestFromDB(value db.Request) usage.Request {
	return usage.Request{ID: value.ID, IdempotencyKey: value.IdempotencyKey, UserID: value.UserID, GatewayKeyID: value.GatewayKeyID, ModelID: value.ModelID, SubscriptionID: value.SubscriptionID, ResourcePoolID: value.ResourcePoolID, Status: usage.RequestStatus(value.Status), Stream: value.Stream, InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, UsageSource: usage.UsageSource(value.UsageSource), ErrorKind: value.ErrorKind, ErrorDetail: value.ErrorDetail, AcceptedAt: value.AcceptedAt.Time.UTC(), CompletedAt: timePointer(value.CompletedAt), UpdatedAt: value.UpdatedAt.Time.UTC()}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
