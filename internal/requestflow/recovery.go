package requestflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/luckymaomi/llm2api/internal/execution"
)

func (s *Service) RecoverOnce(ctx context.Context, staleBefore time.Time, batchSize int32) (RecoveryResult, error) {
	if staleBefore.IsZero() || batchSize < 1 || batchSize > 1000 {
		return RecoveryResult{}, fmt.Errorf("invalid request recovery input")
	}
	var result RecoveryResult
	var recoveryErrors []error

	completions, err := s.repository.ListRecoverableCompletions(ctx, staleBefore, batchSize)
	if err != nil {
		return result, fmt.Errorf("list recoverable completions: %w", err)
	}
	for _, completion := range completions {
		if err := s.accounting.Complete(ctx, completion.Claim, completion.Usage); err != nil {
			if !errors.Is(err, execution.ErrFenced) {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("complete request %s: %w", completion.Claim.RequestID, err))
			}
			continue
		}
		result.Completed++
	}

	queuedRequests, err := s.repository.ListStaleQueuedRequests(ctx, staleBefore, batchSize)
	if err != nil {
		return result, errors.Join(append(recoveryErrors, fmt.Errorf("list stale queued requests: %w", err))...)
	}
	for _, requestID := range queuedRequests {
		if err := s.accounting.FailAccepted(ctx, requestID, "execution_abandoned", "request was accepted but no execution claimed it"); err != nil {
			if !errors.Is(err, execution.ErrFenced) {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("fail queued request %s: %w", requestID, err))
			}
			continue
		}
		result.FailedAccepted++
	}

	result.Uncertain, err = s.repository.RecoverStaleExecutions(ctx, staleBefore, batchSize)
	if err != nil {
		recoveryErrors = append(recoveryErrors, fmt.Errorf("fence stale executions: %w", err))
	}
	return result, errors.Join(recoveryErrors...)
}
