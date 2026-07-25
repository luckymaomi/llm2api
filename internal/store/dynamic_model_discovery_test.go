package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/registry"
	db "github.com/luckymaomi/llm2api/internal/store/db"
)

// This protects the business result that Key-scoped discoveries form one pool union
// while routing remains restricted to Keys that discovered the requested model.
func TestDiscoveredKeyModelsFormExactResourcePoolUnion(t *testing.T) {
	pool := identityTestPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)

	providerID, resourcePoolID := uuid.New(), uuid.New()
	firstCredentialID, secondCredentialID := uuid.New(), uuid.New()
	suffix := resourcePoolID.String()[:12]
	if _, err := tx.Exec(ctx, `INSERT INTO providers
(id, catalog_id, slug, name, kind, base_url, source_url, verified_at)
VALUES ($1, $2, $3, 'Dynamic Provider', 'openai-compatible', 'https://provider.example/v1', 'https://provider.example/docs', now())`,
		providerID, "dynamic-provider-"+suffix, "dynamic-provider-"+suffix); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO resource_pools (id, provider_id, slug, name)
VALUES ($1, $2, $3, 'Dynamic Pool')`, resourcePoolID, providerID, "dynamic-pool-"+suffix); err != nil {
		t.Fatalf("insert resource pool: %v", err)
	}
	for _, credentialID := range []uuid.UUID{firstCredentialID, secondCredentialID} {
		if _, err := tx.Exec(ctx, `INSERT INTO provider_credentials (id, resource_pool_id, name, encrypted_secret)
VALUES ($1, $2, $3, $4)`, credentialID, resourcePoolID, "Key "+credentialID.String()[:4], []byte("encrypted-fixture")); err != nil {
			t.Fatalf("insert credential: %v", err)
		}
	}

	capabilities := registry.ModelCapabilities{Chat: true, Streaming: true}
	firstBindings, err := upsertDiscoveredModels(ctx, queries, resourcePoolID, []registry.DiscoveredModel{
		{UpstreamName: "model-alpha", Capabilities: capabilities},
		{UpstreamName: "model-shared", Capabilities: capabilities},
	})
	if err != nil {
		t.Fatalf("persist first discovery: %v", err)
	}
	if err := bindCredentialModels(ctx, queries, firstCredentialID, firstBindings); err != nil {
		t.Fatalf("bind first discovery: %v", err)
	}
	secondBindings, err := upsertDiscoveredModels(ctx, queries, resourcePoolID, []registry.DiscoveredModel{
		{UpstreamName: "model-shared", Capabilities: capabilities},
		{UpstreamName: "model-gamma", Capabilities: capabilities},
	})
	if err != nil {
		t.Fatalf("persist second discovery: %v", err)
	}
	if err := bindCredentialModels(ctx, queries, secondCredentialID, secondBindings); err != nil {
		t.Fatalf("bind second discovery: %v", err)
	}

	models, err := resourcePoolModels(ctx, queries, resourcePoolID)
	if err != nil || len(models) != 3 {
		t.Fatalf("pool models = %#v, %v; want three-model union", models, err)
	}
	sharedCandidates, err := queries.ListResourcePoolCandidates(ctx, db.ListResourcePoolCandidatesParams{
		ResourcePoolID: resourcePoolID, ModelID: secondBindings[0].ModelID,
	})
	if err != nil || len(sharedCandidates) != 2 {
		t.Fatalf("shared model candidates = %#v, %v; want both Keys", sharedCandidates, err)
	}
	if _, err := tx.Exec(ctx, "UPDATE provider_credentials SET status = 'disabled' WHERE id = $1", firstCredentialID); err != nil {
		t.Fatalf("disable first credential: %v", err)
	}
	models, err = resourcePoolModels(ctx, queries, resourcePoolID)
	if err != nil || len(models) != 2 {
		t.Fatalf("pool models after disable = %#v, %v; want active Key union", models, err)
	}
}
