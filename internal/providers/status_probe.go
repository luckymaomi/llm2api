package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"

	"github.com/luckymaomi/llm2api/internal/canonical"
)

func (a *openAIAdapter) StatusProbe(ctx context.Context, credential Credential) (StatusProbe, error) {
	if a.policy.statusPath == "" || a.policy.statusKind == "" {
		return StatusProbe{}, nil
	}
	if strings.TrimSpace(credential.APIKey) == "" {
		return StatusProbe{}, a.requestError(canonical.ErrorProviderConfiguration, "missing_api_key", "provider API key is required", "")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint(a.policy.statusPath), nil)
	if err != nil {
		return StatusProbe{}, a.requestError(canonical.ErrorProviderConfiguration, "build_status_probe", "could not build provider status probe", "")
	}
	request.Header.Set("Authorization", "Bearer "+credential.APIKey)
	request.Header.Set("Accept", "application/json")
	return StatusProbe{Available: true, Kind: a.policy.statusKind, Request: request}, nil
}

func (a *openAIAdapter) ParseStatusProbe(kind StatusProbeKind, statusCode int, headers http.Header, body []byte) (UpstreamStatusObservation, *canonical.Error) {
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return UpstreamStatusObservation{}, a.ClassifyError(statusCode, headers, body)
	}
	switch kind {
	case StatusProbeKimiBalance:
		return a.parseKimiBalance(body)
	default:
		return UpstreamStatusObservation{}, a.requestError(canonical.ErrorProviderConfiguration, "unsupported_status_probe", "provider status probe is not supported", "")
	}
}

func (a *openAIAdapter) parseKimiBalance(body []byte) (UpstreamStatusObservation, *canonical.Error) {
	var envelope struct {
		Code   int  `json:"code"`
		Status bool `json:"status"`
		Data   *struct {
			Available json.Number `json:"available_balance"`
			Voucher   json.Number `json:"voucher_balance"`
			Cash      json.Number `json:"cash_balance"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil || envelope.Code != 0 || !envelope.Status || envelope.Data == nil {
		return UpstreamStatusObservation{}, a.requestError(canonical.ErrorProviderConfiguration, "invalid_status_response", "provider returned an invalid balance response", "")
	}
	available, ok := decimalString(envelope.Data.Available, false)
	if !ok {
		return UpstreamStatusObservation{}, a.requestError(canonical.ErrorProviderConfiguration, "invalid_status_response", "provider returned an invalid available balance", "")
	}
	voucher, ok := decimalString(envelope.Data.Voucher, true)
	if !ok {
		return UpstreamStatusObservation{}, a.requestError(canonical.ErrorProviderConfiguration, "invalid_status_response", "provider returned an invalid voucher balance", "")
	}
	cash, ok := decimalString(envelope.Data.Cash, false)
	if !ok {
		return UpstreamStatusObservation{}, a.requestError(canonical.ErrorProviderConfiguration, "invalid_status_response", "provider returned an invalid cash balance", "")
	}
	return UpstreamStatusObservation{
		State: UpstreamStatusObserved, Scope: UpstreamStatusScopeAccount, Source: "official_balance_endpoint",
		Balance: &UpstreamBalance{Currency: "CNY", Available: available, Voucher: voucher, Cash: cash},
	}, nil
}

func decimalString(value json.Number, nonNegative bool) (string, bool) {
	text := strings.TrimSpace(value.String())
	if text == "" || len(text) > 128 {
		return "", false
	}
	rational, ok := new(big.Rat).SetString(text)
	if !ok || (nonNegative && rational.Sign() < 0) {
		return "", false
	}
	return text, true
}
