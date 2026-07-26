package requestflow

import (
	"testing"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/coordination"
)

// This protects the shared upstream account result: every request through
// any Key in the configured scope consumes the same atomic rate and lease
// dimensions, while incomplete scope data fails before an upstream send.
func TestRequestDimensionsAddsOneSharedUpstreamScope(t *testing.T) {
	t.Parallel()

	scope := "kimi-organization-a"
	rpm := int32(60)
	tpm := int64(120_000)
	concurrency := int32(4)
	dailyTokens := int64(5_000_000)
	dailyResetMinuteUTC := int32(8 * 60)
	providerID := uuid.New()
	dimensions, capacities, err := requestDimensions(LeaseRequest{
		ResourcePoolID: uuid.New(), ModelID: uuid.New(), ProviderID: providerID, CredentialID: uuid.New(),
		SharedCapacityScope: &scope, SharedRPMLimit: &rpm, SharedTPMLimit: &tpm, SharedConcurrency: &concurrency,
		SharedDailyTokenLimit: &dailyTokens, SharedDailyResetMinuteUTC: &dailyResetMinuteUTC,
	}, coordinationTestConfig(), Capacity{RequestsPerMinute: 10, TokensPerMinute: 10_000, Concurrency: 2})
	if err != nil {
		t.Fatalf("requestDimensions() error = %v", err)
	}
	if len(dimensions) != 6 || len(capacities) != 6 {
		t.Fatalf("shared dimensions = %d/%d, want 6/6", len(dimensions), len(capacities))
	}
	shared := dimensions[len(dimensions)-1]
	if shared.Scope != coordination.ScopeSharedUpstream || shared.SubjectID != providerID.String()+":"+scope {
		t.Fatalf("shared dimension = %#v", shared)
	}
	if got := capacities[len(capacities)-1]; got.RequestsPerMinute != int64(rpm) || got.TokensPerMinute != tpm || got.Concurrency != int64(concurrency) || got.DailyTokens != dailyTokens || got.DailyResetMinuteUTC != dailyResetMinuteUTC {
		t.Fatalf("shared capacity = %#v", got)
	}
}

func TestRequestDimensionsRejectsHalfConfiguredDailyQuota(t *testing.T) {
	t.Parallel()

	scope := "account-a"
	rpm, tpm, concurrency := int32(60), int64(120_000), int32(4)
	dailyTokens := int64(5_000_000)
	_, _, err := requestDimensions(LeaseRequest{
		ResourcePoolID: uuid.New(), ModelID: uuid.New(), ProviderID: uuid.New(), CredentialID: uuid.New(),
		SharedCapacityScope: &scope, SharedRPMLimit: &rpm, SharedTPMLimit: &tpm, SharedConcurrency: &concurrency,
		SharedDailyTokenLimit: &dailyTokens,
	}, coordinationTestConfig(), Capacity{RequestsPerMinute: 10, TokensPerMinute: 10_000, Concurrency: 2})
	if err == nil {
		t.Fatal("requestDimensions() accepted a daily quota without its UTC reset minute")
	}
}

func TestRequestDimensionsRejectsIncompleteSharedUpstreamScope(t *testing.T) {
	t.Parallel()

	scope := "account-a"
	rpm := int32(60)
	_, _, err := requestDimensions(LeaseRequest{
		ResourcePoolID: uuid.New(), ModelID: uuid.New(), ProviderID: uuid.New(), CredentialID: uuid.New(),
		SharedCapacityScope: &scope, SharedRPMLimit: &rpm,
	}, coordinationTestConfig(), Capacity{RequestsPerMinute: 10, TokensPerMinute: 10_000, Concurrency: 2})
	if err == nil {
		t.Fatal("requestDimensions() accepted an incomplete shared upstream scope")
	}
}

func coordinationTestConfig() CoordinationConfig {
	capacity := Capacity{RequestsPerMinute: 100, TokensPerMinute: 100_000, Concurrency: 10}
	return CoordinationConfig{
		Global: capacity, ResourcePool: capacity, Model: capacity, Provider: capacity, DefaultCredential: capacity,
	}
}
