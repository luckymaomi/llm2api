package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/migrations"
)

func TestGatewayKeyDeletionIsIdempotentAndAuditedOnce(t *testing.T) {
	pool := identityTestPool(t)
	ctx := context.Background()
	ownerID := insertGatewayKeyDeletionUser(t, pool, identity.RoleMember)
	administratorID := insertGatewayKeyDeletionUser(t, pool, identity.RoleAdministrator)
	ownerDeletedKeyID := insertGatewayKeyDeletionKey(t, pool, ownerID)
	administratorDeletedKeyID := insertGatewayKeyDeletionKey(t, pool, ownerID)
	repository := NewIdentityRepository(pool)
	commitResponseLost := errors.New("fixture: commit response lost")
	commitCalls := 0
	repository.commitGatewayKeyDeletion = func(ctx context.Context, tx pgx.Tx) error {
		commitCalls++
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return commitResponseLost
	}

	if err := repository.DeleteGatewayKey(ctx, ownerDeletedKeyID, ownerID, false); err != nil {
		t.Fatalf("DeleteGatewayKey(owner after committed response loss) error = %v", err)
	}
	if err := repository.DeleteGatewayKey(ctx, ownerDeletedKeyID, ownerID, false); err != nil {
		t.Fatalf("DeleteGatewayKey(owner replay) error = %v", err)
	}
	if err := repository.DeleteGatewayKey(ctx, ownerDeletedKeyID, administratorID, true); err != nil {
		t.Fatalf("DeleteGatewayKey(administrator replay) error = %v", err)
	}
	if err := repository.DeleteGatewayKey(ctx, administratorDeletedKeyID, administratorID, true); err != nil {
		t.Fatalf("DeleteGatewayKey(administrator after committed response loss) error = %v", err)
	}
	if err := repository.DeleteGatewayKey(ctx, administratorDeletedKeyID, administratorID, true); err != nil {
		t.Fatalf("DeleteGatewayKey(administrator replay) error = %v", err)
	}
	if commitCalls != 2 {
		t.Fatalf("gateway key deletion commit calls = %d, want 2", commitCalls)
	}

	for _, expectation := range []struct {
		keyID   uuid.UUID
		actorID uuid.UUID
	}{
		{keyID: ownerDeletedKeyID, actorID: ownerID},
		{keyID: administratorDeletedKeyID, actorID: administratorID},
	} {
		var deleted bool
		var auditCount int
		var auditActorID uuid.UUID
		if err := pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM gateway_keys WHERE id = $1", expectation.keyID).Scan(&deleted); err != nil {
			t.Fatalf("read deleted gateway key %s: %v", expectation.keyID, err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*), min(actor_user_id::text)::uuid FROM audit_events
WHERE action = 'gateway_key.deleted' AND target_type = 'gateway_key' AND target_id = $1`, expectation.keyID.String()).Scan(&auditCount, &auditActorID); err != nil {
			t.Fatalf("read gateway key deletion audit %s: %v", expectation.keyID, err)
		}
		if !deleted || auditCount != 1 || auditActorID != expectation.actorID {
			t.Fatalf("deleted/audit count/actor for %s = %t/%d/%s, want true/1/%s", expectation.keyID, deleted, auditCount, auditActorID, expectation.actorID)
		}
	}
}

func TestGatewayKeyDeletionDistinguishesMissingAndForbidden(t *testing.T) {
	pool := identityTestPool(t)
	ctx := context.Background()
	ownerID := insertGatewayKeyDeletionUser(t, pool, identity.RoleMember)
	otherMemberID := insertGatewayKeyDeletionUser(t, pool, identity.RoleMember)
	keyID := insertGatewayKeyDeletionKey(t, pool, ownerID)
	repository := NewIdentityRepository(pool)

	if err := repository.DeleteGatewayKey(ctx, uuid.New(), ownerID, false); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("DeleteGatewayKey(missing) error = %v, want ErrNotFound", err)
	}
	if err := repository.DeleteGatewayKey(ctx, keyID, otherMemberID, false); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("DeleteGatewayKey(foreign owner) error = %v, want ErrForbidden", err)
	}

	var deleted bool
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM gateway_keys WHERE id = $1", keyID).Scan(&deleted); err != nil {
		t.Fatalf("read protected gateway key: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events
WHERE action = 'gateway_key.deleted' AND target_type = 'gateway_key' AND target_id = $1`, keyID.String()).Scan(&auditCount); err != nil {
		t.Fatalf("count forbidden gateway key deletion audits: %v", err)
	}
	if deleted || auditCount != 0 {
		t.Fatalf("deleted/audit count after forbidden command = %t/%d, want false/0", deleted, auditCount)
	}
}

func TestGatewayKeyDeletionConcurrentReplayCommitsOneAudit(t *testing.T) {
	pool := identityTestPool(t)
	ctx := context.Background()
	ownerID := insertGatewayKeyDeletionUser(t, pool, identity.RoleMember)
	keyID := insertGatewayKeyDeletionKey(t, pool, ownerID)
	repository := NewIdentityRepository(pool)

	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			errorsByCall <- repository.DeleteGatewayKey(ctx, keyID, ownerID, false)
		}()
	}
	ready.Wait()
	close(start)
	for call := 1; call <= 2; call++ {
		if err := <-errorsByCall; err != nil {
			t.Fatalf("concurrent DeleteGatewayKey() call %d error = %v", call, err)
		}
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events
WHERE action = 'gateway_key.deleted' AND target_type = 'gateway_key' AND target_id = $1`, keyID.String()).Scan(&auditCount); err != nil {
		t.Fatalf("count concurrent gateway key deletion audits: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("concurrent gateway key deletion audits = %d, want 1", auditCount)
	}
}

func identityTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("LLM2API_CONTROL_TEST_DATABASE_URL")
	if databaseURL == "" {
		if os.Getenv("LLM2API_CONTROL_TEST_REQUIRED") == "true" {
			t.Fatal("LLM2API_CONTROL_TEST_DATABASE_URL is required for the gateway key deletion repository test")
		}
		t.Skip("LLM2API_CONTROL_TEST_DATABASE_URL is required for the isolated gateway key deletion repository test")
	}
	ctx := context.Background()
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := migrations.Up(ctx, database); err != nil {
		t.Fatalf("migrations.Up() error = %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertGatewayKeyDeletionUser(t *testing.T, pool *pgxpool.Pool, role identity.Role) uuid.UUID {
	t.Helper()
	id := uuid.New()
	email := "gateway-key-deletion-" + id.String() + "@example.test"
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, display_name, password_hash, role, status)
VALUES ($1, $2, 'Gateway Key Deletion Fixture', 'fixture-hash', $3, 'active')`, id, email, role); err != nil {
		t.Fatalf("insert gateway key deletion user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM audit_events WHERE actor_user_id = $1", id); err != nil {
			t.Errorf("delete gateway key deletion audits: %v", err)
		}
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
			t.Errorf("delete gateway key deletion user: %v", err)
		}
	})
	return id
}

func insertGatewayKeyDeletionKey(t *testing.T, pool *pgxpool.Pool, ownerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	digest := []byte("gateway-key-deletion-" + id.String())
	encryptedSecret := []byte("gateway-key-deletion-fixture-" + id.String())
	if _, err := pool.Exec(context.Background(), `INSERT INTO gateway_keys (id, user_id, name, prefix, secret_digest, encrypted_secret)
VALUES ($1, $2, 'Deletion Fixture', $3, $4, $5)`, id, ownerID, "llmg_"+id.String()[:12], digest, encryptedSecret); err != nil {
		t.Fatalf("insert gateway key deletion fixture: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM gateway_keys WHERE id = $1", id); err != nil {
			t.Errorf("delete gateway key deletion fixture: %v", err)
		}
	})
	return id
}
