package routing

import (
	"testing"
	"time"
)

type fixedRandom int

func (r fixedRandom) Intn(limit int) int { return int(r) % limit }

func TestSelectUsesOnlyEligibleCandidatesWithinExactRoute(t *testing.T) {
	router, err := NewRouter(fixedRandom(1))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	decision, err := router.Select(Requirements{
		ModelID: "model-a", ResourcePoolID: "pool-a", Capabilities: []Capability{"chat"}, At: now,
	}, []Candidate{
		{ID: "first", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}},
		{ID: "second", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}},
		{ID: "other-pool", ModelID: "model-a", ResourcePoolID: "pool-b", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != SelectionEqualRotate || decision.SelectedCandidateID != "second" {
		t.Fatalf("selected %q in mode %q", decision.SelectedCandidateID, decision.Mode)
	}
	if len(decision.Eligible) != 2 {
		t.Fatalf("eligible = %v", decision.Eligible)
	}
}
