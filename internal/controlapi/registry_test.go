package controlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
)

type providerListRegistry struct {
	registryService
	providers   []registry.Provider
	credentials []registry.Credential
	imported    []registry.CredentialBatchResult
}

func (s providerListRegistry) ListProviders(context.Context, identity.Principal) ([]registry.Provider, error) {
	return s.providers, nil
}

func (s providerListRegistry) ListCredentials(context.Context, identity.Principal, bool) ([]registry.Credential, error) {
	return s.credentials, nil
}

func (s providerListRegistry) ImportCredentials(_ context.Context, _ identity.Principal, _ uuid.UUID, _ []registry.CredentialBatchItem, _ *int32, _ *int64, _ *int32, _ registry.MutationRequest) ([]registry.CredentialBatchResult, error) {
	return s.imported, nil
}

func TestProviderListUsesStableSnakeCaseContract(t *testing.T) {
	api := &API{registry: providerListRegistry{providers: []registry.Provider{{
		CatalogID: "kimi",
		Contract: providers.ContractInfo{
			ReferenceURL: "https://platform.kimi.com/docs/api/chat", ContractSnapshot: "2026-07-26",
			VerifiedAt: "2026-07-26", ReferenceProvider: "Moonshot AI", VerifiedModels: []string{"kimi-k3"},
			LiveCapabilities: []string{"chat", "tools"}, Status: providers.VerificationVerified,
		},
	}}}}

	request := httptest.NewRequest(http.MethodGet, "/control/providers", nil)
	response := httptest.NewRecorder()
	api.listProviders(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("provider list status = %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data []struct {
			Contract struct {
				ReferenceURL      string   `json:"reference_url"`
				ContractSnapshot  string   `json:"contract_snapshot"`
				VerifiedAt        string   `json:"verified_at"`
				ReferenceProvider string   `json:"reference_provider"`
				VerifiedModels    []string `json:"verified_models"`
				LiveCapabilities  []string `json:"live_capabilities"`
				Status            string   `json:"status"`
			} `json:"contract"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("provider list data = %#v", payload.Data)
	}
	contract := payload.Data[0].Contract
	if contract.ReferenceURL == "" || contract.ContractSnapshot == "" || contract.VerifiedAt == "" || contract.ReferenceProvider == "" ||
		len(contract.VerifiedModels) != 1 || contract.VerifiedModels[0] != "kimi-k3" || len(contract.LiveCapabilities) != 2 || contract.Status != "verified" {
		t.Fatalf("provider contract wire = %#v", contract)
	}
	if containsPascalCaseContractField(response.Body.String()) {
		t.Fatalf("provider contract did not use its stable wire fields: %s", response.Body.String())
	}
}

func containsPascalCaseContractField(body string) bool {
	for _, field := range []string{"ReferenceURL", "ContractSnapshot", "VerifiedAt", "ReferenceProvider", "VerifiedModels", "LiveCapabilities", "Status"} {
		if strings.Contains(body, `"`+field+`"`) {
			return true
		}
	}
	return false
}

func TestCredentialListAlwaysProjectsGatewayCapacityState(t *testing.T) {
	api := &API{registry: providerListRegistry{credentials: []registry.Credential{{
		Name: "Kimi account A", Status: registry.CredentialActive, HealthStatus: registry.CredentialHealthy,
		ModelBindings: []registry.CredentialModelBinding{},
	}}}}

	request := httptest.NewRequest(http.MethodGet, "/control/credentials", nil)
	response := httptest.NewRecorder()
	api.listCredentials(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("credential list status = %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data []struct {
			Capacity struct {
				State string `json:"state"`
				Scope string `json:"scope"`
			} `json:"capacity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode credential list: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Capacity.State != "unavailable" || payload.Data[0].Capacity.Scope != "gateway_credential" {
		t.Fatalf("credential capacity wire = %#v", payload.Data)
	}
}

func TestCredentialBatchUsesTheSameCredentialViewAsTheList(t *testing.T) {
	poolID := uuid.New()
	created := registry.Credential{
		ID: poolID, Name: "Agnes primary", Status: registry.CredentialActive, HealthStatus: registry.CredentialHealthy,
		ModelBindings: []registry.CredentialModelBinding{},
	}
	api := &API{registry: providerListRegistry{imported: []registry.CredentialBatchResult{{
		Line: 1, Name: created.Name, Status: "created", Credential: &created,
	}}}}

	request := httptest.NewRequest(http.MethodPost, "/control/credentials/batch", strings.NewReader(`{
"resourcePoolId":"`+poolID.String()+`","items":[{"name":"Agnes primary","secret":"fixture-secret"}]}`))
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	api.importCredentials(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("credential batch status = %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data []struct {
			Status     string `json:"status"`
			Credential struct {
				Capacity struct {
					State string `json:"state"`
					Scope string `json:"scope"`
				} `json:"capacity"`
			} `json:"credential"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode credential batch: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Status != "created" || payload.Data[0].Credential.Capacity.State != "unavailable" || payload.Data[0].Credential.Capacity.Scope != "gateway_credential" {
		t.Fatalf("credential batch wire = %#v", payload.Data)
	}
}
