package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/luckymaomi/llm2api/internal/store/db"
	"github.com/luckymaomi/llm2api/internal/usage"
)

func (r *UsageRepository) AcceptRequest(ctx context.Context, input usage.AcceptInput) (usage.AcceptedRequest, error) {
	var lastErr error
	for attempt := 0; attempt < requestTransactionAttempts; attempt++ {
		accepted, err := r.acceptRequestOnce(ctx, input)
		if err == nil {
			return accepted, nil
		}
		if !retryableTransaction(err) {
			return usage.AcceptedRequest{}, translateUsageError(err)
		}
		lastErr = err
	}
	return usage.AcceptedRequest{}, translateUsageError(lastErr)
}

func (r *UsageRepository) acceptRequestOnce(ctx context.Context, input usage.AcceptInput) (usage.AcceptedRequest, error) {
	tx, err := r.connections.Postgres.Begin(ctx)
	if err != nil {
		return usage.AcceptedRequest{}, err
	}
	defer tx.Rollback(ctx)
	queries := r.queries.WithTx(tx)
	if input.IdempotencyKey != nil {
		lockKey := input.GatewayKeyID.String() + ":" + *input.IdempotencyKey
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
			return usage.AcceptedRequest{}, err
		}
		existing, err := queries.GetRequestByIdempotencyKey(ctx, db.GetRequestByIdempotencyKeyParams{GatewayKeyID: input.GatewayKeyID, IdempotencyKey: input.IdempotencyKey})
		switch {
		case err == nil:
			return replayAcceptedRequest(ctx, tx, input, existing)
		case !errors.Is(err, pgx.ErrNoRows):
			return usage.AcceptedRequest{}, err
		}
	}
	principal, err := queries.GetActiveGatewayKeyForRequest(ctx, db.GetActiveGatewayKeyForRequestParams{GatewayKeyID: input.GatewayKeyID, UserID: input.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return usage.AcceptedRequest{}, usage.ErrForbidden
	}
	if err != nil {
		return usage.AcceptedRequest{}, err
	}
	route, err := queries.GetGatewayKeyRoute(ctx, db.GetGatewayKeyRouteParams{GatewayKeyID: input.GatewayKeyID, ModelID: input.ModelID})
	if errors.Is(err, pgx.ErrNoRows) {
		return usage.AcceptedRequest{}, usage.ErrModelNotAuthorized
	}
	if err != nil {
		return usage.AcceptedRequest{}, err
	}
	var subscriptionID *uuid.UUID
	if principal.Role == db.UserRoleMember {
		routes, err := queries.GetApplicableSubscriptionRoutesForUpdate(ctx, db.GetApplicableSubscriptionRoutesForUpdateParams{UserID: input.UserID, ModelID: input.ModelID})
		if err != nil {
			return usage.AcceptedRequest{}, err
		}
		for _, assigned := range routes {
			if assigned.ResourcePoolID == route.ResourcePoolID {
				selected := assigned.ID
				subscriptionID = &selected
				break
			}
		}
		if subscriptionID == nil {
			return usage.AcceptedRequest{}, usage.ErrModelNotAuthorized
		}
	}
	requestRecord, err := queries.CreateRequest(ctx, db.CreateRequestParams{
		ID: input.RequestID, IdempotencyKey: input.IdempotencyKey, RequestDigest: input.RequestDigest,
		UserID: input.UserID, GatewayKeyID: input.GatewayKeyID, ModelID: input.ModelID,
		SubscriptionID: subscriptionID, ResourcePoolID: route.ResourcePoolID, Status: db.RequestStatusQueued, Stream: input.Stream,
	})
	if err != nil {
		return usage.AcceptedRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return usage.AcceptedRequest{}, err
	}
	return usage.AcceptedRequest{Request: requestFromDB(requestRecord)}, nil
}

func replayAcceptedRequest(ctx context.Context, tx pgx.Tx, input usage.AcceptInput, existing db.Request) (usage.AcceptedRequest, error) {
	if existing.UserID != input.UserID || existing.GatewayKeyID != input.GatewayKeyID || existing.ModelID != input.ModelID || existing.Stream != input.Stream || !bytes.Equal(existing.RequestDigest, input.RequestDigest) {
		return usage.AcceptedRequest{}, usage.ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return usage.AcceptedRequest{}, err
	}
	return usage.AcceptedRequest{Request: requestFromDB(existing), Replayed: true}, nil
}
