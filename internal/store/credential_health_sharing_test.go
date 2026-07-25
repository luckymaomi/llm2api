package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luckymaomi/llmgateway/internal/requestflow"
	"github.com/luckymaomi/llmgateway/internal/store/db"
)

func TestCredentialRecoveryPermitIsSharedAndFencedAcrossRepositories(t *testing.T) {
	pool := identityTestPool(t)
	ctx := context.Background()
	credentialID := insertSharedHealthCredential(t, pool)
	queries := db.New(pool)
	repositoryA := NewRequestRepository(&Connections{Postgres: pool})
	repositoryB := NewRequestRepository(&Connections{Postgres: pool})

	generation, err := repositoryA.AcquireCredentialHealthPermit(ctx, credentialID)
	if err != nil || generation != 0 {
		t.Fatalf("initial health permit = %d, %v", generation, err)
	}
	cooldownUntil := time.Now().UTC().Add(time.Hour)
	if err := recordCredentialObservation(ctx, queries, credentialID, &requestflow.CredentialObservation{
		Kind: requestflow.CredentialFailed, HealthGeneration: generation, ObservedAt: time.Now().UTC(),
		ErrorKind: "rate_limit", CooldownUntil: &cooldownUntil,
	}); err != nil {
		t.Fatalf("record cooling observation: %v", err)
	}
	if _, err := repositoryB.AcquireCredentialHealthPermit(ctx, credentialID); !errors.Is(err, requestflow.ErrNoEligibleUpstream) {
		t.Fatalf("other repository acquired a cooling credential: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE provider_credentials SET cooldown_until = now() - interval '1 second' WHERE id = $1", credentialID); err != nil {
		t.Fatalf("expire credential cooldown: %v", err)
	}

	type permitResult struct {
		generation int64
		err        error
	}
	start := make(chan struct{})
	results := make(chan permitResult, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, repository := range []*RequestRepository{repositoryA, repositoryB} {
		go func(repository *RequestRepository) {
			ready.Done()
			<-start
			value, acquireErr := repository.AcquireCredentialHealthPermit(ctx, credentialID)
			results <- permitResult{generation: value, err: acquireErr}
		}(repository)
	}
	ready.Wait()
	close(start)
	succeeded := 0
	rejected := 0
	for range 2 {
		result := <-results
		switch {
		case result.err == nil && result.generation == 1:
			succeeded++
		case errors.Is(result.err, requestflow.ErrNoEligibleUpstream):
			rejected++
		default:
			t.Fatalf("unexpected recovery permit result: %#v", result)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("shared recovery permits succeeded/rejected = %d/%d, want 1/1", succeeded, rejected)
	}

	if err := recordCredentialObservation(ctx, queries, credentialID, &requestflow.CredentialObservation{
		Kind: requestflow.CredentialSucceeded, HealthGeneration: 0, ObservedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record stale success observation: %v", err)
	}
	assertCredentialHealth(t, pool, credentialID, "probing", 1)
	if err := recordCredentialObservation(ctx, queries, credentialID, &requestflow.CredentialObservation{
		Kind: requestflow.CredentialSucceeded, HealthGeneration: 1, ObservedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record current success observation: %v", err)
	}
	assertCredentialHealth(t, pool, credentialID, "healthy", 1)
}

func insertSharedHealthCredential(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	providerID := uuid.New()
	resourcePoolID := uuid.New()
	credentialID := uuid.New()
	suffix := credentialID.String()[:12]
	if _, err := pool.Exec(ctx, `INSERT INTO providers
(id, catalog_id, slug, name, kind, base_url, source_url, verified_at)
VALUES ($1, $2, $3, 'Shared Health Provider', 'openai_compatible', 'https://provider.example/v1', 'https://provider.example/docs', now())`,
		providerID, "shared-health-"+suffix, "shared-health-"+suffix); err != nil {
		t.Fatalf("insert shared health provider: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO resource_pools (id, provider_id, slug, name)
VALUES ($1, $2, $3, 'Shared Health Pool')`, resourcePoolID, providerID, "shared-health-"+suffix); err != nil {
		t.Fatalf("insert shared health pool: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credentials
(id, resource_pool_id, name, encrypted_secret)
VALUES ($1, $2, 'Shared Health Key', $3)`, credentialID, resourcePoolID, []byte("encrypted-fixture")); err != nil {
		t.Fatalf("insert shared health credential: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM provider_credentials WHERE id = $1", credentialID); err != nil {
			t.Errorf("delete shared health credential: %v", err)
		}
		if _, err := pool.Exec(context.Background(), "DELETE FROM resource_pools WHERE id = $1", resourcePoolID); err != nil {
			t.Errorf("delete shared health pool: %v", err)
		}
		if _, err := pool.Exec(context.Background(), "DELETE FROM providers WHERE id = $1", providerID); err != nil {
			t.Errorf("delete shared health provider: %v", err)
		}
	})
	return credentialID
}

func assertCredentialHealth(t *testing.T, pool *pgxpool.Pool, credentialID uuid.UUID, wantStatus string, wantGeneration int64) {
	t.Helper()
	var status string
	var generation int64
	if err := pool.QueryRow(context.Background(), `SELECT health_status::text, health_generation
FROM provider_credentials WHERE id = $1`, credentialID).Scan(&status, &generation); err != nil {
		t.Fatalf("read credential health: %v", err)
	}
	if status != wantStatus || generation != wantGeneration {
		t.Fatalf("credential health = %s/%d, want %s/%d", status, generation, wantStatus, wantGeneration)
	}
}
