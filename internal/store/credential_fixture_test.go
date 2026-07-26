package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/security"
)

var credentialFixtureFingerprintPepper = []byte("store-credential-fixture-fingerprint-pepper")

func credentialFixtureFingerprint(t *testing.T, credentialID uuid.UUID) string {
	t.Helper()
	fingerprint, err := security.DigestToken("provider-credential-fixture:\x00"+credentialID.String(), credentialFixtureFingerprintPepper)
	if err != nil {
		t.Fatalf("digest fixture credential identity: %v", err)
	}
	return fingerprint
}
