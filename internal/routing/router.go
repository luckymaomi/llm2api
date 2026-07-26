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
	priority := candidatePriority(decision.Eligible, candidates)
	weighted := make([]CandidateID, 0, len(decision.Eligible))
	totalWeight := 0
	for _, candidateID := range decision.Eligible {
		candidate := candidateByID(candidateID, candidates)
		if effectivePriority(candidate.Priority) != priority {
			continue
		}
		weighted = append(weighted, candidateID)
		totalWeight += int(effectiveWeight(candidate.Weight))
	}
	selected := r.random.Intn(totalWeight)
	if selected < 0 || selected >= totalWeight {
		return Decision{}, newError(ErrorRandomSource, "random source returned a value outside its requested range", "")
	}
	for _, candidateID := range weighted {
		selected -= int(effectiveWeight(candidateByID(candidateID, candidates).Weight))
		if selected < 0 {
			decision.SelectedCandidateID = candidateID
			break
		}
	}
	decision.Mode = SelectionPriorityWeighted
	return decision, nil
}

func candidatePriority(eligible []CandidateID, candidates []Candidate) int32 {
	lowest := int32(1001)
	for _, candidateID := range eligible {
		priority := effectivePriority(candidateByID(candidateID, candidates).Priority)
		if priority < lowest {
			lowest = priority
		}
	}
	return lowest
}

func candidateByID(id CandidateID, candidates []Candidate) Candidate {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	return Candidate{}
}

func effectivePriority(priority int32) int32 {
	if priority == 0 {
		return 100
	}
	return priority
}

func effectiveWeight(weight int32) int32 {
	if weight == 0 {
		return 1
	}
	return weight
}
