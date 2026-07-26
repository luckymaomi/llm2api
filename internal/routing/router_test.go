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
	if decision.Mode != SelectionPriorityWeighted || decision.SelectedCandidateID != "second" {
		t.Fatalf("selected %q in mode %q", decision.SelectedCandidateID, decision.Mode)
	}
	if len(decision.Eligible) != 2 {
		t.Fatalf("eligible = %v", decision.Eligible)
	}
}

func TestSelectPrefersTheLowestEligiblePriority(t *testing.T) {
	router, err := NewRouter(fixedRandom(99))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	decision, err := router.Select(Requirements{ModelID: "model-a", ResourcePoolID: "pool-a", Capabilities: []Capability{"chat"}, At: now}, []Candidate{
		{ID: "reserve", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}, Priority: 200, Weight: 1000},
		{ID: "preferred", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}, Priority: 10, Weight: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.SelectedCandidateID != "preferred" {
		t.Fatalf("selected %q, want the lowest-priority-number candidate", decision.SelectedCandidateID)
	}
}

func TestSelectUsesWeightWithinTheSamePriority(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{ID: "light", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}, Priority: 10, Weight: 1},
		{ID: "heavy", ModelID: "model-a", ResourcePoolID: "pool-a", ModelPublished: true, CredentialAuthorized: true, CredentialActive: true, Capabilities: []Capability{"chat"}, Priority: 10, Weight: 3},
	}
	for random, want := range map[fixedRandom]CandidateID{0: "heavy", 1: "heavy", 2: "heavy", 3: "light"} {
		router, err := NewRouter(random)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := router.Select(Requirements{ModelID: "model-a", ResourcePoolID: "pool-a", Capabilities: []Capability{"chat"}, At: now}, candidates)
		if err != nil {
			t.Fatal(err)
		}
		if decision.SelectedCandidateID != want {
			t.Fatalf("random %d selected %q, want %q", random, decision.SelectedCandidateID, want)
		}
	}
}
