package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/security"
)

const credentialProbePersistenceTimeout = 3 * time.Second

type Service struct {
	repository Repository
	envelope   *security.EnvelopeCipher
	prober     CredentialProbeExecutor
	discoverer ModelDiscoveryExecutor
	catalog    *providers.Catalog
}

func NewService(repository Repository, envelope *security.EnvelopeCipher, urls *security.URLValidator) (*Service, error) {
	if repository == nil || envelope == nil || urls == nil {
		return nil, fmt.Errorf("registry dependencies are required")
	}
	return &Service{repository: repository, envelope: envelope, catalog: providers.DefaultCatalog()}, nil
}

func (s *Service) WithCredentialProbeExecutor(prober CredentialProbeExecutor) *Service {
	s.prober = prober
	if discoverer, ok := prober.(ModelDiscoveryExecutor); ok {
		s.discoverer = discoverer
	}
	return s
}

func (s *Service) SyncCatalog(ctx context.Context) error {
	presets := s.catalog.Presets()
	projections := make([]ProviderProjection, 0, len(presets))
	for _, preset := range presets {
		verifiedAt, err := time.Parse(time.DateOnly, preset.VerifiedAt)
		if err != nil {
			return fmt.Errorf("catalog preset %s verified date: %w", preset.ID, err)
		}
		projection := ProviderProjection{
			CatalogID: preset.ID, Slug: preset.Slug, Name: preset.Name, Kind: preset.Kind,
			BaseURL: preset.BaseURL, SourceURL: preset.SourceURL, VerifiedAt: verifiedAt.UTC(),
		}
		projections = append(projections, projection)
	}
	return s.repository.SyncCatalog(ctx, projections)
}

func (s *Service) ListProviders(ctx context.Context, actor identity.Principal) ([]Provider, error) {
	if !activeAdministrator(actor) {
		return nil, ErrForbidden
	}
	items, err := s.repository.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	for index := range items {
		s.enrichProvider(&items[index])
	}
	return items, nil
}

func (s *Service) GetProvider(ctx context.Context, actor identity.Principal, providerID uuid.UUID) (Provider, error) {
	if !activeAdministrator(actor) || providerID == uuid.Nil {
		return Provider{}, ErrForbidden
	}
	provider, err := s.repository.GetProvider(ctx, providerID)
	if err == nil {
		s.enrichProvider(&provider)
	}
	return provider, err
}

func (s *Service) provider(ctx context.Context, providerID uuid.UUID) (Provider, error) {
	provider, err := s.repository.GetProvider(ctx, providerID)
	if err == nil {
		s.enrichProvider(&provider)
	}
	return provider, err
}

func (s *Service) enrichProvider(provider *Provider) {
	for _, info := range s.catalog.Kinds() {
		if info.Kind == provider.Kind {
			provider.Contract = info.Contract
			return
		}
	}
}

func (s *Service) ListModels(ctx context.Context, actor identity.Principal) ([]Model, error) {
	if actor.Status != identity.StatusActive {
		return nil, ErrForbidden
	}
	return s.repository.ListModels(ctx)
}

func (s *Service) CreateResourcePool(ctx context.Context, actor identity.Principal, input NewResourcePool, request MutationRequest) (ResourcePool, error) {
	if !activeAdministrator(actor) {
		return ResourcePool{}, ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.ProviderID == uuid.Nil || request.IdempotencyKey == uuid.Nil || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 80 {
		return ResourcePool{}, ErrInvalidInput
	}
	input.Slug = "pool-" + strings.ReplaceAll(request.IdempotencyKey.String(), "-", "")
	mutation, err := resourcePoolMutation(request, "resource_pool.create", input)
	if err != nil {
		return ResourcePool{}, err
	}
	return s.repository.CreateResourcePool(ctx, input, actor.UserID, mutation)
}

func (s *Service) UpdateResourcePool(ctx context.Context, actor identity.Principal, input ResourcePoolChange, request MutationRequest) (ResourcePool, error) {
	if !activeAdministrator(actor) {
		return ResourcePool{}, ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	input.ExpectedUpdatedAt = input.ExpectedUpdatedAt.UTC()
	if input.ID == uuid.Nil || input.ExpectedUpdatedAt.IsZero() || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 80 {
		return ResourcePool{}, ErrInvalidInput
	}
	mutation, err := resourcePoolMutation(request, "resource_pool.update", input)
	if err != nil {
		return ResourcePool{}, err
	}
	return s.repository.UpdateResourcePool(ctx, input, actor.UserID, mutation)
}

func (s *Service) SetResourcePoolStatus(ctx context.Context, actor identity.Principal, id uuid.UUID, status ResourcePoolStatus, expectedUpdatedAt time.Time, request MutationRequest) (ResourcePool, error) {
	if !activeAdministrator(actor) {
		return ResourcePool{}, ErrForbidden
	}
	if id == uuid.Nil || expectedUpdatedAt.IsZero() || status != ResourcePoolActive && status != ResourcePoolDisabled && status != ResourcePoolRetired {
		return ResourcePool{}, ErrInvalidInput
	}
	payload := struct {
		ID                uuid.UUID          `json:"id"`
		Status            ResourcePoolStatus `json:"status"`
		ExpectedUpdatedAt time.Time          `json:"expected_updated_at"`
	}{id, status, expectedUpdatedAt.UTC().Truncate(time.Microsecond)}
	mutation, err := resourcePoolMutation(request, "resource_pool.status", payload)
	if err != nil {
		return ResourcePool{}, err
	}
	return s.repository.SetResourcePoolStatus(ctx, id, status, expectedUpdatedAt.UTC(), actor.UserID, mutation)
}

func (s *Service) ListResourcePools(ctx context.Context, actor identity.Principal, includeRetired bool) ([]ResourcePool, error) {
	if !activeAdministrator(actor) {
		return nil, ErrForbidden
	}
	return s.repository.ListResourcePools(ctx, includeRetired)
}

func (s *Service) GetResourcePool(ctx context.Context, actor identity.Principal, id uuid.UUID) (ResourcePool, error) {
	if !activeAdministrator(actor) || id == uuid.Nil {
		return ResourcePool{}, ErrForbidden
	}
	return s.repository.GetResourcePool(ctx, id)
}

func (s *Service) CreateCredential(ctx context.Context, actor identity.Principal, input NewCredential, secret string, request MutationRequest) (Credential, error) {
	if !activeAdministrator(actor) {
		return Credential{}, ErrForbidden
	}
	input.ID = uuid.New()
	input.Name = strings.TrimSpace(input.Name)
	if input.ResourcePoolID == uuid.Nil || len(secret) < 8 || len(secret) > 8192 || !validCredentialFields(input.Name, input.RPMLimit, input.TPMLimit, input.ConcurrencyLimit) {
		return Credential{}, ErrInvalidInput
	}
	pool, err := s.repository.GetResourcePool(ctx, input.ResourcePoolID)
	if err != nil {
		return Credential{}, err
	}
	input.DiscoveredModels, input.Discovery, err = s.discoverModels(ctx, providerFromPool(pool), secret)
	if err != nil {
		return Credential{}, err
	}
	mutation, err := credentialCreateMutation(request, input, secret)
	if err != nil {
		return Credential{}, err
	}
	input.EncryptedSecret, err = s.envelope.Encrypt([]byte(secret), CredentialEncryptionContext(input.ID))
	if err != nil {
		return Credential{}, fmt.Errorf("encrypt credential: %w", err)
	}
	return s.repository.CreateCredential(ctx, input, actor.UserID, mutation)
}

func (s *Service) ImportCredentials(ctx context.Context, actor identity.Principal, resourcePoolID uuid.UUID, items []CredentialBatchItem, rpmLimit *int32, tpmLimit *int64, concurrencyLimit *int32, request MutationRequest) ([]CredentialBatchResult, error) {
	if !activeAdministrator(actor) {
		return nil, ErrForbidden
	}
	if resourcePoolID == uuid.Nil || len(items) == 0 || len(items) > 500 {
		return nil, ErrInvalidInput
	}
	results := make([]CredentialBatchResult, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		item.Name, item.Secret = strings.TrimSpace(item.Name), strings.TrimSpace(item.Secret)
		result := CredentialBatchResult{Line: index + 1, Name: item.Name}
		if _, duplicate := seen[item.Secret]; duplicate {
			result.Status = "skipped"
			results = append(results, result)
			continue
		}
		seen[item.Secret] = struct{}{}
		childRequest := request
		childRequest.IdempotencyKey = uuid.NewSHA1(request.IdempotencyKey, []byte(fmt.Sprintf("credential-line:%d", index+1)))
		created, err := s.CreateCredential(ctx, actor, NewCredential{ResourcePoolID: resourcePoolID, Name: item.Name, RPMLimit: rpmLimit, TPMLimit: tpmLimit, ConcurrencyLimit: concurrencyLimit}, item.Secret, childRequest)
		if err != nil {
			result.Status, result.ErrorKind = "rejected", credentialImportError(err)
		} else {
			result.Status, result.Credential = "created", &created
		}
		results = append(results, result)
	}
	return results, nil
}

func credentialImportError(err error) string {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrConflict), errors.Is(err, ErrIdempotencyConflict):
		return "conflict"
	case errors.Is(err, ErrModelDiscovery):
		return "model_discovery_failed"
	default:
		return "persistence_failed"
	}
}

func (s *Service) UpdateCredential(ctx context.Context, actor identity.Principal, input CredentialChange, secret string, request MutationRequest) (Credential, error) {
	if !activeAdministrator(actor) {
		return Credential{}, ErrForbidden
	}
	input.Name, input.ReplaceSecret, input.ExpectedUpdatedAt = strings.TrimSpace(input.Name), secret != "", input.ExpectedUpdatedAt.UTC()
	if input.ID == uuid.Nil || input.ExpectedUpdatedAt.IsZero() || !validCredentialFields(input.Name, input.RPMLimit, input.TPMLimit, input.ConcurrencyLimit) || input.ReplaceSecret && (len(secret) < 8 || len(secret) > 8192) {
		return Credential{}, ErrInvalidInput
	}
	var err error
	if input.ReplaceSecret {
		current, loadErr := s.repository.GetCredential(ctx, input.ID)
		if loadErr != nil {
			return Credential{}, loadErr
		}
		input.ReplaceModels = true
		input.DiscoveredModels, input.Discovery, err = s.discoverModels(ctx, providerFromCredential(current), secret)
		if err != nil {
			return Credential{}, err
		}
	}
	mutation, err := credentialUpdateMutation(request, input, secret)
	if err != nil {
		return Credential{}, err
	}
	if input.ReplaceSecret {
		input.EncryptedSecret, err = s.envelope.Encrypt([]byte(secret), CredentialEncryptionContext(input.ID))
		if err != nil {
			return Credential{}, fmt.Errorf("encrypt credential: %w", err)
		}
	}
	return s.repository.UpdateCredential(ctx, input, actor.UserID, mutation)
}

func (s *Service) RefreshCredentialModels(ctx context.Context, actor identity.Principal, id uuid.UUID, expectedUpdatedAt time.Time, request MutationRequest) (ModelDiscoveryExecution, Credential, error) {
	if !activeAdministrator(actor) || id == uuid.Nil || expectedUpdatedAt.IsZero() {
		return ModelDiscoveryExecution{}, Credential{}, ErrForbidden
	}
	current, err := s.repository.GetCredential(ctx, id)
	if err != nil {
		return ModelDiscoveryExecution{}, Credential{}, err
	}
	secret, err := s.CredentialSecret(ctx, id)
	if err != nil {
		return ModelDiscoveryExecution{}, Credential{}, err
	}
	models, execution, err := s.discoverModels(ctx, providerFromCredential(current), secret)
	if err != nil {
		probe := CredentialProbeExecution{
			Kind: "models", Status: execution.Status, ErrorKind: execution.ErrorKind,
			Retryable: execution.Retryable, MayUseTokens: false, LatencyMillis: execution.LatencyMillis,
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialProbePersistenceTimeout)
		defer cancel()
		if recorded, recordErr := s.repository.RecordCredentialProbe(persistCtx, id, time.Now().UTC(), probe, actor.UserID, request.RequestID); recordErr == nil {
			current = recorded
		}
		return execution, current, err
	}
	change := CredentialChange{
		ID: id, Name: current.Name, RPMLimit: current.RPMLimit, TPMLimit: current.TPMLimit,
		ConcurrencyLimit: current.ConcurrencyLimit, ReplaceModels: true, DiscoveredModels: models,
		Discovery: execution, ExpectedUpdatedAt: expectedUpdatedAt.UTC(),
	}
	mutation, err := credentialUpdateMutation(request, change, "")
	if err != nil {
		return execution, current, err
	}
	updated, err := s.repository.UpdateCredential(ctx, change, actor.UserID, mutation)
	return execution, updated, err
}

func (s *Service) discoverModels(ctx context.Context, provider Provider, secret string) ([]DiscoveredModel, ModelDiscoveryExecution, error) {
	if s.discoverer == nil {
		return nil, ModelDiscoveryExecution{}, ErrModelDiscovery
	}
	execution := s.discoverer.Discover(ctx, ModelDiscoveryTarget{Provider: provider, Secret: secret})
	if execution.Status != "succeeded" {
		return nil, execution, ErrModelDiscovery
	}
	capabilities, err := s.discoveredModelCapabilities(provider)
	if err != nil {
		return nil, execution, err
	}
	models := make([]DiscoveredModel, 0, len(execution.Models))
	for _, upstreamName := range execution.Models {
		models = append(models, DiscoveredModel{UpstreamName: upstreamName, Capabilities: capabilities})
	}
	return models, execution, nil
}

func (s *Service) discoveredModelCapabilities(provider Provider) (ModelCapabilities, error) {
	adapter, err := s.catalog.Build(provider.Kind, providers.AdapterOptions{BaseURL: provider.BaseURL, Capabilities: providers.NarrowOpenAICompatibleCapabilities()})
	if err != nil {
		return ModelCapabilities{}, err
	}
	capability := adapter.Capabilities()
	reasoningMode := ReasoningMode("")
	if capability.ReasoningToggle && capability.ReasoningEffort {
		reasoningMode = ReasoningHybrid
	} else if capability.ReasoningToggle {
		reasoningMode = ReasoningToggle
	} else if capability.ReasoningEffort {
		reasoningMode = ReasoningEffort
	}
	return ModelCapabilities{
		Chat: capability.Chat, Streaming: capability.Streaming, Tools: capability.Tools,
		Reasoning:     capability.ReasoningToggle || capability.ReasoningEffort || capability.ReasoningContent,
		ReasoningMode: reasoningMode, StructuredOutput: capability.JSONOutput,
	}, nil
}

func providerFromPool(pool ResourcePool) Provider {
	return Provider{ID: pool.ProviderID, CatalogID: pool.ProviderCatalogID, Slug: pool.ProviderSlug, Name: pool.ProviderName, Kind: pool.ProviderKind, BaseURL: pool.ProviderBaseURL}
}

func providerFromCredential(credential Credential) Provider {
	return Provider{ID: credential.ProviderID, Name: credential.ProviderName, Kind: credential.ProviderKind, BaseURL: credential.ProviderBaseURL}
}

func (s *Service) SetCredentialStatus(ctx context.Context, actor identity.Principal, id uuid.UUID, status CredentialStatus, expectedUpdatedAt time.Time, request MutationRequest) (Credential, error) {
	if !activeAdministrator(actor) {
		return Credential{}, ErrForbidden
	}
	if id == uuid.Nil || expectedUpdatedAt.IsZero() || status != CredentialActive && status != CredentialDisabled {
		return Credential{}, ErrInvalidInput
	}
	mutation, err := mutationFingerprint(request, "credential.status", struct {
		ID                uuid.UUID        `json:"id"`
		Status            CredentialStatus `json:"status"`
		ExpectedUpdatedAt time.Time        `json:"expected_updated_at"`
	}{id, status, expectedUpdatedAt.UTC().Truncate(time.Microsecond)})
	if err != nil {
		return Credential{}, err
	}
	return s.repository.SetCredentialStatus(ctx, id, status, expectedUpdatedAt.UTC(), actor.UserID, mutation)
}

func (s *Service) RetireCredential(ctx context.Context, actor identity.Principal, id uuid.UUID, expectedUpdatedAt time.Time, request MutationRequest) (Credential, error) {
	if !activeAdministrator(actor) || id == uuid.Nil || expectedUpdatedAt.IsZero() {
		return Credential{}, ErrForbidden
	}
	mutation, err := mutationFingerprint(request, "credential.retire", struct {
		ID                uuid.UUID `json:"id"`
		ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	}{id, expectedUpdatedAt.UTC().Truncate(time.Microsecond)})
	if err != nil {
		return Credential{}, err
	}
	tombstone, err := s.envelope.Encrypt([]byte("retired"), CredentialEncryptionContext(id))
	if err != nil {
		return Credential{}, err
	}
	return s.repository.RetireCredential(ctx, id, tombstone, expectedUpdatedAt.UTC(), actor.UserID, mutation)
}

func (s *Service) ProbeCredential(ctx context.Context, actor identity.Principal, credentialID, modelID uuid.UUID, requestID string) (CredentialProbeExecution, Credential, error) {
	if !activeAdministrator(actor) || credentialID == uuid.Nil || modelID == uuid.Nil || strings.TrimSpace(requestID) == "" || len(requestID) > 128 {
		return CredentialProbeExecution{}, Credential{}, ErrForbidden
	}
	credential, err := s.repository.GetCredential(ctx, credentialID)
	if err != nil {
		return CredentialProbeExecution{}, Credential{}, err
	}
	model, err := s.credentialProbeModel(ctx, credential, modelID)
	if err != nil {
		return CredentialProbeExecution{}, Credential{}, err
	}
	provider, err := s.provider(ctx, credential.ProviderID)
	if err != nil {
		return CredentialProbeExecution{}, Credential{}, err
	}
	unavailable := "probe_runtime_unavailable"
	execution := CredentialProbeExecution{Kind: "generation", Status: "unavailable", ErrorKind: &unavailable, Retryable: true, MayUseTokens: true, ModelID: model.ID, ModelName: model.PublicName}
	if s.prober != nil {
		secret, secretErr := s.CredentialSecret(ctx, credentialID)
		if secretErr != nil {
			return CredentialProbeExecution{}, Credential{}, secretErr
		}
		execution = s.prober.Execute(ctx, CredentialProbeTarget{Provider: provider, Model: model, CredentialID: credentialID, Secret: secret, RequestID: requestID})
	}
	execution.RequestID = requestID
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialProbePersistenceTimeout)
	defer cancel()
	credential, err = s.repository.RecordCredentialProbe(persistCtx, credentialID, time.Now().UTC(), execution, actor.UserID, requestID)
	return execution, credential, err
}

func (s *Service) credentialProbeModel(ctx context.Context, credential Credential, modelID uuid.UUID) (Model, error) {
	for _, binding := range credential.ModelBindings {
		if binding.ModelID == modelID {
			models, err := s.repository.ListModels(ctx)
			if err != nil {
				return Model{}, err
			}
			for _, model := range models {
				if model.ID == modelID && model.ProviderID == credential.ProviderID {
					return model, nil
				}
			}
		}
	}
	return Model{}, ErrInvalidInput
}

func (s *Service) ListCredentials(ctx context.Context, actor identity.Principal, includeRetired bool) ([]Credential, error) {
	if !activeAdministrator(actor) {
		return nil, ErrForbidden
	}
	return s.repository.ListCredentials(ctx, includeRetired)
}

func (s *Service) CredentialSecret(ctx context.Context, credentialID uuid.UUID) (string, error) {
	encrypted, err := s.repository.GetEncryptedCredential(ctx, credentialID)
	if err != nil {
		return "", err
	}
	plaintext, err := s.envelope.Decrypt(encrypted, CredentialEncryptionContext(credentialID))
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(plaintext), nil
}

func activeAdministrator(actor identity.Principal) bool {
	return actor.Status == identity.StatusActive && actor.Role == identity.RoleAdministrator
}

func validCredentialFields(name string, rpmLimit *int32, tpmLimit *int64, concurrencyLimit *int32) bool {
	return name != "" && utf8.RuneCountInString(name) <= 120 &&
		(rpmLimit == nil || *rpmLimit > 0) && (tpmLimit == nil || *tpmLimit > 0) && (concurrencyLimit == nil || *concurrencyLimit > 0)
}

func CredentialEncryptionContext(id uuid.UUID) []byte {
	return []byte("provider-credential:" + id.String())
}
