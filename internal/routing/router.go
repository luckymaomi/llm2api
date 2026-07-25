package routing

import "sort"

type Router struct {
	random Random
}

func NewRouter(random Random) (*Router, error) {
	if random == nil {
		return nil, newError(ErrorRandomSource, "candidate rotation requires a random source", "")
	}
	return &Router{random: random}, nil
}

func (r *Router) Select(requirements Requirements, candidates []Candidate) (Decision, error) {
	if err := validateRequirements(requirements); err != nil {
		return Decision{}, err
	}
	excluded := make(map[CandidateID]struct{}, len(requirements.ExcludedCandidates))
	for _, candidateID := range requirements.ExcludedCandidates {
		if candidateID == "" {
			return Decision{}, newError(ErrorInvalidInput, "excluded candidate ID cannot be empty", "")
		}
		excluded[candidateID] = struct{}{}
	}

	decision := Decision{Mode: SelectionNone, Evaluations: make([]Evaluation, 0, len(candidates))}
	seen := make(map[CandidateID]struct{}, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidate(candidate); err != nil {
			return Decision{}, err
		}
		if _, exists := seen[candidate.ID]; exists {
			return Decision{}, newError(ErrorDuplicate, "candidate IDs must be unique", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		reasons := evaluateEligibility(requirements, candidate, excluded)
		decision.Evaluations = append(decision.Evaluations, Evaluation{CandidateID: candidate.ID, Eligible: len(reasons) == 0, Exclusions: reasons})
		if len(reasons) == 1 && reasons[0].Reason == ExcludeCredentialCooling &&
			(decision.NextAvailableAt.IsZero() || reasons[0].AvailableAt.Before(decision.NextAvailableAt)) {
			decision.NextAvailableAt = reasons[0].AvailableAt
		}
		if len(reasons) == 0 {
			decision.Eligible = append(decision.Eligible, candidate.ID)
		}
	}
	if len(decision.Eligible) == 0 {
		return decision, nil
	}
	sort.Slice(decision.Eligible, func(i, j int) bool {
		return decision.Eligible[i] < decision.Eligible[j]
	})
	selected := r.random.Intn(len(decision.Eligible))
	if selected < 0 || selected >= len(decision.Eligible) {
		return Decision{}, newError(ErrorRandomSource, "random source returned a value outside its requested range", "")
	}
	decision.SelectedCandidateID = decision.Eligible[selected]
	decision.Mode = SelectionEqualRotate
	return decision, nil
}
