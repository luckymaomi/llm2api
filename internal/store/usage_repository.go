package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/luckymaomi/llmgateway/internal/store/db"
	"github.com/luckymaomi/llmgateway/internal/usage"
)

type UsageRepository struct {
	connections *Connections
	queries     *db.Queries
}

func NewUsageRepository(connections *Connections) *UsageRepository {
	return &UsageRepository{connections: connections, queries: db.New(connections.Postgres)}
}

func (r *UsageRepository) ListRequestLogs(ctx context.Context, query usage.RequestLogQuery) (usage.PageResult[usage.RequestLog], error) {
	params := db.CountRequestLogsParams{
		UserID: query.UserID, GatewayKeyID: query.GatewayKeyID, ModelID: query.ModelID, Status: string(query.Status),
		FromTime: requiredTimestamp(query.From), ToTime: requiredTimestamp(query.To), Search: query.Search, ResourcePoolID: query.ResourcePoolID,
	}
	total, err := r.queries.CountRequestLogs(ctx, params)
	if err != nil {
		return usage.PageResult[usage.RequestLog]{}, translateUsageError(err)
	}
	rows, err := r.queries.ListRequestLogs(ctx, db.ListRequestLogsParams{
		UserID: params.UserID, GatewayKeyID: params.GatewayKeyID, ModelID: params.ModelID, Status: params.Status,
		FromTime: params.FromTime, ToTime: params.ToTime, Search: params.Search, ResourcePoolID: params.ResourcePoolID,
		PageOffset: query.Page.Offset, PageSize: query.Page.Size,
	})
	if err != nil {
		return usage.PageResult[usage.RequestLog]{}, translateUsageError(err)
	}
	items := make([]usage.RequestLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, requestLogFromParts(row.ID, row.UserID, row.UserName, row.GatewayKeyID, row.KeyPrefix, row.ModelID, row.ModelAlias, row.ResourcePoolID, row.ResourcePoolName, row.ResourcePoolSlug, row.Status, row.Stream, row.InputTokens, row.OutputTokens, row.UsageSource, row.ErrorKind, row.AcceptedAt, row.CompletedAt, row.UpdatedAt, row.AttemptCount, row.LastAttemptStatus))
	}
	return usage.PageResult[usage.RequestLog]{Items: items, Total: total}, nil
}

func (r *UsageRepository) GetRequestLog(ctx context.Context, requestID uuid.UUID, userID *uuid.UUID) (usage.RequestLogDetail, error) {
	row, err := r.queries.GetRequestLog(ctx, db.GetRequestLogParams{RequestID: requestID, UserID: userID})
	if err != nil {
		return usage.RequestLogDetail{}, translateUsageError(err)
	}
	result := usage.RequestLogDetail{RequestLog: requestLogFromParts(row.ID, row.UserID, row.UserName, row.GatewayKeyID, row.KeyPrefix, row.ModelID, row.ModelAlias, row.ResourcePoolID, row.ResourcePoolName, row.ResourcePoolSlug, row.Status, row.Stream, row.InputTokens, row.OutputTokens, row.UsageSource, row.ErrorKind, row.AcceptedAt, row.CompletedAt, row.UpdatedAt, row.AttemptCount, row.LastAttemptStatus)}
	attempts, err := r.queries.ListRequestLogAttempts(ctx, requestID)
	if err != nil {
		return usage.RequestLogDetail{}, translateUsageError(err)
	}
	for _, attempt := range attempts {
		result.Attempts = append(result.Attempts, usage.RequestAttempt{
			ID: attempt.ID, Sequence: attempt.Sequence, Status: string(attempt.Status), ProviderName: attempt.ProviderName,
			CredentialName: attempt.CredentialName, UpstreamRequestID: attempt.UpstreamRequestID, HTTPStatus: attempt.HttpStatus,
			ErrorKind: attempt.ErrorKind, RetryAfterAt: timePointer(attempt.RetryAfterAt), SentAt: timePointer(attempt.SentAt),
			FirstByteAt: timePointer(attempt.FirstByteAt), CompletedAt: timePointer(attempt.CompletedAt), InputTokens: attempt.InputTokens,
			OutputTokens: attempt.OutputTokens, UsageSource: usage.UsageSource(attempt.UsageSource), CreatedAt: attempt.CreatedAt.Time.UTC(),
		})
	}
	return result, nil
}

func requestLogFromParts(id, userID uuid.UUID, userName string, gatewayKeyID uuid.UUID, keyPrefix string, modelID uuid.UUID, modelAlias string, resourcePoolID uuid.UUID, poolName, poolSlug string, status db.RequestStatus, stream bool, inputTokens, outputTokens *int64, source db.UsageSource, errorKind *string, acceptedAt, completedAt, updatedAt pgtype.Timestamptz, attemptCount int64, lastAttemptStatus string) usage.RequestLog {
	return usage.RequestLog{
		RequestID: id, UserID: userID, UserName: userName, GatewayKeyID: gatewayKeyID, KeyPrefix: keyPrefix,
		ModelID: modelID, ModelAlias: modelAlias, ResourcePoolID: resourcePoolID, ResourcePoolName: poolName, ResourcePoolSlug: poolSlug,
		Status: usage.RequestStatus(status), Stream: stream, InputTokens: inputTokens, OutputTokens: outputTokens, UsageSource: usage.UsageSource(source),
		ErrorKind: errorKind, AcceptedAt: acceptedAt.Time.UTC(), CompletedAt: timePointer(completedAt), UpdatedAt: updatedAt.Time.UTC(),
		AttemptCount: attemptCount, LastAttemptStatus: optionalString(lastAttemptStatus),
	}
}

func requiredTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func translateUsageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return usage.ErrNotFound
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "23503":
			return usage.ErrNotFound
		case "23505", "40001", "40P01":
			return usage.ErrConflict
		case "23514", "23502", "22P02":
			return usage.ErrInvalidInput
		}
	}
	return fmt.Errorf("usage store: %w", err)
}

func retryableTransaction(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && (databaseError.Code == "40001" || databaseError.Code == "40P01")
}
