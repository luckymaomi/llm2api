package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func (a *openAIAdapter) Probe(ctx context.Context, credential Credential) (Probe, error) {
	if !a.policy.capabilities.Models || a.policy.modelsPath == "" {
		return Probe{}, nil
	}
	if strings.TrimSpace(credential.APIKey) == "" {
		return Probe{}, a.requestError(canonical.ErrorProviderConfiguration, "missing_api_key", "provider API key is required", "")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint(a.policy.modelsPath), nil)
	if err != nil {
		return Probe{}, a.requestError(canonical.ErrorProviderConfiguration, "build_probe", "could not build provider probe", "")
	}
	request.Header.Set("Authorization", "Bearer "+credential.APIKey)
	request.Header.Set("Accept", "application/json")
	return Probe{Available: true, MayConsumeTokens: false, Kind: ProbeModels, Request: request}, nil
}

func (a *openAIAdapter) ValidateProbe(kind ProbeKind, statusCode int, headers http.Header, body []byte) *canonical.Error {
	_, providerError := a.ParseProbe(kind, statusCode, headers, body)
	return providerError
}

func (a *openAIAdapter) ParseProbe(kind ProbeKind, statusCode int, headers http.Header, body []byte) ([]DiscoveredModel, *canonical.Error) {
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return nil, a.ClassifyError(statusCode, headers, body)
	}
	if kind != ProbeModels {
		return nil, a.requestError(canonical.ErrorProviderConfiguration, "unsupported_probe", "provider probe is not supported", "")
	}
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data == nil {
		return nil, a.requestError(canonical.ErrorProviderConfiguration, "invalid_probe_response", "provider returned an invalid models response", "")
	}
	if len(envelope.Data) > 5000 {
		return nil, a.requestError(canonical.ErrorProviderConfiguration, "too_many_models", "provider returned too many models", "")
	}
	seen := make(map[string]struct{}, len(envelope.Data))
	models := make([]DiscoveredModel, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 256 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
			return nil, a.requestError(canonical.ErrorProviderConfiguration, "invalid_model_id", "provider returned an invalid model identifier", "")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, DiscoveredModel{ID: id})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
