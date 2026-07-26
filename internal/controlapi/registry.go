package controlapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/httpserver"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/requestflow"
)

func (a *API) listProviders(w http.ResponseWriter, r *http.Request) {
	items, err := a.registry.ListProviders(r.Context(), principalFromContext(r.Context()))
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (a *API) getProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "providerID")
	if !ok {
		return
	}
	item, err := a.registry.GetProvider(r.Context(), principalFromContext(r.Context()), id)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, item)
}

func (a *API) listModels(w http.ResponseWriter, r *http.Request) {
	items, err := a.registry.ListModels(r.Context(), principalFromContext(r.Context()))
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (a *API) listResourcePools(w http.ResponseWriter, r *http.Request) {
	includeRetired, _ := strconv.ParseBool(r.URL.Query().Get("includeRetired"))
	items, err := a.registry.ListResourcePools(r.Context(), principalFromContext(r.Context()), includeRetired)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (a *API) createResourcePool(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProviderID uuid.UUID `json:"providerId"`
		Name       string    `json:"name"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	item, err := a.registry.CreateResourcePool(r.Context(), principalFromContext(r.Context()), registry.NewResourcePool{ProviderID: input.ProviderID, Name: input.Name}, mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, item)
}

func (a *API) updateResourcePool(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "resourcePoolID")
	if !ok {
		return
	}
	var input struct {
		Name              string    `json:"name"`
		ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	item, err := a.registry.UpdateResourcePool(r.Context(), principalFromContext(r.Context()), registry.ResourcePoolChange{ID: id, Name: input.Name, ExpectedUpdatedAt: input.ExpectedUpdatedAt}, mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, item)
}

func (a *API) setResourcePoolStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "resourcePoolID")
	if !ok {
		return
	}
	var input struct {
		Status            registry.ResourcePoolStatus `json:"status"`
		ExpectedUpdatedAt time.Time                   `json:"expectedUpdatedAt"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	item, err := a.registry.SetResourcePoolStatus(r.Context(), principalFromContext(r.Context()), id, input.Status, input.ExpectedUpdatedAt, mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, item)
}

type credentialInput struct {
	Name                      string    `json:"name"`
	Secret                    string    `json:"secret"`
	RPMLimit                  *int32    `json:"rpmLimit"`
	TPMLimit                  *int64    `json:"tpmLimit"`
	ConcurrencyLimit          *int32    `json:"concurrencyLimit"`
	Priority                  int32     `json:"priority"`
	Weight                    int32     `json:"weight"`
	SharedCapacityScope       *string   `json:"sharedCapacityScope"`
	SharedRPMLimit            *int32    `json:"sharedRpmLimit"`
	SharedTPMLimit            *int64    `json:"sharedTpmLimit"`
	SharedConcurrencyLimit    *int32    `json:"sharedConcurrencyLimit"`
	SharedDailyTokenLimit     *int64    `json:"sharedDailyTokenLimit"`
	SharedDailyResetMinuteUTC *int32    `json:"sharedDailyResetMinuteUtc"`
	ExpectedUpdatedAt         time.Time `json:"expectedUpdatedAt"`
}

type credentialCapacityView struct {
	State                   string     `json:"state"`
	Scope                   string     `json:"scope"`
	ObservedAt              *time.Time `json:"observed_at,omitempty"`
	RequestsPerMinuteLimit  int64      `json:"requests_per_minute_limit,omitempty"`
	RequestsPerMinuteRemain int64      `json:"requests_per_minute_remaining,omitempty"`
	TokensPerMinuteLimit    int64      `json:"tokens_per_minute_limit,omitempty"`
	TokensPerMinuteRemain   int64      `json:"tokens_per_minute_remaining,omitempty"`
	DailyTokenLimit         int64      `json:"daily_token_limit,omitempty"`
	DailyTokenRemain        int64      `json:"daily_token_remaining,omitempty"`
	DailyTokenResetAt       *time.Time `json:"daily_token_reset_at,omitempty"`
	ConcurrencyLimit        int64      `json:"concurrency_limit,omitempty"`
	ConcurrencyInUse        int64      `json:"concurrency_in_use,omitempty"`
}

type credentialView struct {
	registry.Credential
	Capacity       credentialCapacityView  `json:"capacity"`
	SharedCapacity *credentialCapacityView `json:"shared_capacity,omitempty"`
}

type credentialBatchResultView struct {
	Line       int             `json:"line"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	Credential *credentialView `json:"credential,omitempty"`
	ErrorKind  string          `json:"error_kind,omitempty"`
}

type credentialModelProbeView struct {
	Credential credentialView                   `json:"credential"`
	Execution  registry.ModelDiscoveryExecution `json:"execution"`
}

type credentialModelProbeBatchView struct {
	Results     []credentialModelProbeView `json:"results"`
	Succeeded   int                        `json:"succeeded"`
	Failed      int                        `json:"failed"`
	Unavailable int                        `json:"unavailable"`
	Uncertain   int                        `json:"uncertain"`
}

func (a *API) listCredentials(w http.ResponseWriter, r *http.Request) {
	includeRetired, _ := strconv.ParseBool(r.URL.Query().Get("includeRetired"))
	items, err := a.registry.ListCredentials(r.Context(), principalFromContext(r.Context()), includeRetired)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredentials(r.Context(), items))
}

func (a *API) importCredentials(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ResourcePoolID uuid.UUID `json:"resourcePoolId"`
		Items          []struct {
			Name   string `json:"name"`
			Secret string `json:"secret"`
		} `json:"items"`
		RPMLimit         *int32 `json:"rpmLimit"`
		TPMLimit         *int64 `json:"tpmLimit"`
		ConcurrencyLimit *int32 `json:"concurrencyLimit"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	batch := make([]registry.CredentialBatchItem, 0, len(input.Items))
	for _, item := range input.Items {
		batch = append(batch, registry.CredentialBatchItem{Name: item.Name, Secret: item.Secret})
	}
	items, err := a.registry.ImportCredentials(r.Context(), principalFromContext(r.Context()), input.ResourcePoolID, batch, input.RPMLimit, input.TPMLimit, input.ConcurrencyLimit, mutation)
	for index := range input.Items {
		input.Items[index].Secret = ""
	}
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredentialBatch(r.Context(), items))
}

func (a *API) probeAllCredentials(w http.ResponseWriter, r *http.Request) {
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	batch, err := a.registry.RefreshAllCredentialModels(r.Context(), principalFromContext(r.Context()), mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredentialModelProbeBatch(r.Context(), batch))
}

func (a *API) updateCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	var input credentialInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		input.Secret = ""
		return
	}
	item, err := a.registry.UpdateCredential(r.Context(), principalFromContext(r.Context()), registry.CredentialChange{ID: id, Name: input.Name, RPMLimit: input.RPMLimit, TPMLimit: input.TPMLimit, ConcurrencyLimit: input.ConcurrencyLimit, Priority: input.Priority, Weight: input.Weight, SharedCapacityScope: input.SharedCapacityScope, SharedRPMLimit: input.SharedRPMLimit, SharedTPMLimit: input.SharedTPMLimit, SharedConcurrencyLimit: input.SharedConcurrencyLimit, SharedDailyTokenLimit: input.SharedDailyTokenLimit, SharedDailyResetMinuteUTC: input.SharedDailyResetMinuteUTC, ExpectedUpdatedAt: input.ExpectedUpdatedAt}, input.Secret, mutation)
	input.Secret = ""
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredential(r.Context(), item))
}

func (a *API) setCredentialStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	var input struct {
		Status            registry.CredentialStatus `json:"status"`
		ExpectedUpdatedAt time.Time                 `json:"expectedUpdatedAt"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	item, err := a.registry.SetCredentialStatus(r.Context(), principalFromContext(r.Context()), id, input.Status, input.ExpectedUpdatedAt, mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredential(r.Context(), item))
}

func (a *API) retireCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	expectedUpdatedAt, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("expectedUpdatedAt"))
	if err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	item, err := a.registry.RetireCredential(r.Context(), principalFromContext(r.Context()), id, expectedUpdatedAt, mutation)
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, a.presentCredential(r.Context(), item))
}

func (a *API) probeCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	var input struct {
		ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	mutation, ok := registryMutationRequest(w, r)
	if !ok {
		return
	}
	execution, credential, err := a.registry.RefreshCredentialModels(r.Context(), principalFromContext(r.Context()), id, input.ExpectedUpdatedAt, mutation)
	if err != nil && !errors.Is(err, registry.ErrModelDiscovery) {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, struct {
		Execution  registry.ModelDiscoveryExecution `json:"execution"`
		Credential credentialView                   `json:"credential"`
	}{execution, a.presentCredential(r.Context(), credential)})
}

func (a *API) fetchCredentialUpstreamStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	observation, credential, err := a.registry.FetchCredentialUpstreamStatus(r.Context(), principalFromContext(r.Context()), id, httpserver.RequestIDFromContext(r.Context()))
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, struct {
		Observation providers.UpstreamStatusObservation `json:"observation"`
		Credential  credentialView                      `json:"credential"`
	}{Observation: observation, Credential: a.presentCredential(r.Context(), credential)})
}

func (a *API) deepTestCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "credentialID")
	if !ok {
		return
	}
	var input struct {
		ModelID uuid.UUID `json:"modelId"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeDecodeError(w, r, err)
		return
	}
	execution, credential, err := a.registry.ProbeCredential(r.Context(), principalFromContext(r.Context()), id, input.ModelID, httpserver.RequestIDFromContext(r.Context()))
	if err != nil {
		a.writeRegistryError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, struct {
		Execution  registry.CredentialProbeExecution `json:"execution"`
		Credential credentialView                    `json:"credential"`
	}{execution, a.presentCredential(r.Context(), credential)})
}

func (a *API) presentCredentials(ctx context.Context, credentials []registry.Credential) []credentialView {
	result := make([]credentialView, 0, len(credentials))
	for _, credential := range credentials {
		result = append(result, a.presentCredential(ctx, credential))
	}
	return result
}

func (a *API) presentCredentialBatch(ctx context.Context, results []registry.CredentialBatchResult) []credentialBatchResultView {
	views := make([]credentialBatchResultView, 0, len(results))
	for _, result := range results {
		view := credentialBatchResultView{Line: result.Line, Name: result.Name, Status: result.Status, ErrorKind: result.ErrorKind}
		if result.Credential != nil {
			credential := a.presentCredential(ctx, *result.Credential)
			view.Credential = &credential
		}
		views = append(views, view)
	}
	return views
}

func (a *API) presentCredentialModelProbeBatch(ctx context.Context, batch registry.CredentialModelProbeBatch) credentialModelProbeBatchView {
	results := make([]credentialModelProbeView, 0, len(batch.Results))
	for _, result := range batch.Results {
		results = append(results, credentialModelProbeView{
			Credential: a.presentCredential(ctx, result.Credential),
			Execution:  result.Execution,
		})
	}
	return credentialModelProbeBatchView{
		Results: results, Succeeded: batch.Succeeded, Failed: batch.Failed,
		Unavailable: batch.Unavailable, Uncertain: batch.Uncertain,
	}
}

func (a *API) presentCredential(ctx context.Context, credential registry.Credential) credentialView {
	view := credentialView{
		Credential: credential,
		Capacity:   credentialCapacityView{State: "unavailable", Scope: "gateway_credential"},
	}
	if a.credentialCapacity == nil {
		return view
	}
	capacity, err := a.credentialCapacity.InspectCredential(ctx, requestflow.CredentialCapacityInput{
		CredentialID: credential.ID, ProviderID: credential.ProviderID,
		RPMLimit: credential.RPMLimit, TPMLimit: credential.TPMLimit, ConcurrencyLimit: credential.ConcurrencyLimit,
		SharedCapacityScope: credential.SharedCapacityScope, SharedRPMLimit: credential.SharedRPMLimit,
		SharedTPMLimit: credential.SharedTPMLimit, SharedConcurrencyLimit: credential.SharedConcurrencyLimit,
		SharedDailyTokenLimit: credential.SharedDailyTokenLimit, SharedDailyResetMinuteUTC: credential.SharedDailyResetMinuteUTC,
	})
	if err != nil {
		return view
	}
	view.Capacity = presentCapacityObservation("gateway_credential", capacity.CapacityObservation)
	if capacity.Shared != nil {
		shared := presentCapacityObservation("gateway_shared_upstream", *capacity.Shared)
		view.SharedCapacity = &shared
	}
	return view
}

func presentCapacityObservation(scope string, capacity requestflow.CapacityObservation) credentialCapacityView {
	observedAt := capacity.ObservedAt
	return credentialCapacityView{
		State: "observed", Scope: scope, ObservedAt: &observedAt,
		RequestsPerMinuteLimit: capacity.RequestsPerMinuteLimit, RequestsPerMinuteRemain: capacity.RequestsPerMinuteRemain,
		TokensPerMinuteLimit: capacity.TokensPerMinuteLimit, TokensPerMinuteRemain: capacity.TokensPerMinuteRemain,
		DailyTokenLimit: capacity.DailyTokenLimit, DailyTokenRemain: capacity.DailyTokenRemain, DailyTokenResetAt: capacity.DailyTokenResetAt,
		ConcurrencyLimit: capacity.ConcurrencyLimit, ConcurrencyInUse: capacity.ConcurrencyInUse,
	}
}

func registryMutationRequest(w http.ResponseWriter, r *http.Request) (registry.MutationRequest, bool) {
	idempotencyKey, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || idempotencyKey == uuid.Nil {
		writeProblem(w, r, problem{Status: http.StatusBadRequest, Code: "invalid_idempotency_key", Message: "Idempotency-Key must be a UUID.", Stage: "registry"})
		return registry.MutationRequest{}, false
	}
	return registry.MutationRequest{IdempotencyKey: idempotencyKey, RequestID: httpserver.RequestIDFromContext(r.Context())}, true
}
