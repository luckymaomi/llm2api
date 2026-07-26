package registry

import (
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func mutationFingerprint(request MutationRequest, action string, payload any) (Mutation, error) {
	if request.IdempotencyKey == uuid.Nil || request.RequestID == "" || len(request.RequestID) > 128 {
		return Mutation{}, ErrInvalidInput
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Mutation{}, ErrInvalidInput
	}
	digest := sha256.Sum256(encoded)
	return Mutation{Action: action, IdempotencyKey: request.IdempotencyKey, RequestFingerprint: digest[:], RequestID: request.RequestID}, nil
}

func resourcePoolMutation(request MutationRequest, action string, input any) (Mutation, error) {
	return mutationFingerprint(request, action, input)
}

func credentialCreateMutation(request MutationRequest, input NewCredential) (Mutation, error) {
	payload := struct {
		ResourcePoolID            uuid.UUID         `json:"resource_pool_id"`
		Name                      string            `json:"name"`
		RPMLimit                  *int32            `json:"rpm_limit"`
		TPMLimit                  *int64            `json:"tpm_limit"`
		ConcurrencyLimit          *int32            `json:"concurrency_limit"`
		Priority                  int32             `json:"priority"`
		Weight                    int32             `json:"weight"`
		SharedCapacityScope       *string           `json:"shared_capacity_scope"`
		SharedRPMLimit            *int32            `json:"shared_rpm_limit"`
		SharedTPMLimit            *int64            `json:"shared_tpm_limit"`
		SharedConcurrencyLimit    *int32            `json:"shared_concurrency_limit"`
		SharedDailyTokenLimit     *int64            `json:"shared_daily_token_limit"`
		SharedDailyResetMinuteUTC *int32            `json:"shared_daily_reset_minute_utc"`
		DiscoveredModels          []DiscoveredModel `json:"discovered_models"`
		SecretFingerprint         string            `json:"secret_fingerprint"`
	}{input.ResourcePoolID, input.Name, input.RPMLimit, input.TPMLimit, input.ConcurrencyLimit, input.Priority, input.Weight, input.SharedCapacityScope, input.SharedRPMLimit, input.SharedTPMLimit, input.SharedConcurrencyLimit, input.SharedDailyTokenLimit, input.SharedDailyResetMinuteUTC, input.DiscoveredModels, input.SecretFingerprint}
	return mutationFingerprint(request, "credential.create", payload)
}

func credentialUpdateMutation(request MutationRequest, input CredentialChange) (Mutation, error) {
	payload := struct {
		ID                        uuid.UUID         `json:"id"`
		Name                      string            `json:"name"`
		RPMLimit                  *int32            `json:"rpm_limit"`
		TPMLimit                  *int64            `json:"tpm_limit"`
		ConcurrencyLimit          *int32            `json:"concurrency_limit"`
		Priority                  int32             `json:"priority"`
		Weight                    int32             `json:"weight"`
		SharedCapacityScope       *string           `json:"shared_capacity_scope"`
		SharedRPMLimit            *int32            `json:"shared_rpm_limit"`
		SharedTPMLimit            *int64            `json:"shared_tpm_limit"`
		SharedConcurrencyLimit    *int32            `json:"shared_concurrency_limit"`
		SharedDailyTokenLimit     *int64            `json:"shared_daily_token_limit"`
		SharedDailyResetMinuteUTC *int32            `json:"shared_daily_reset_minute_utc"`
		ReplaceModels             bool              `json:"replace_models"`
		DiscoveredModels          []DiscoveredModel `json:"discovered_models"`
		ExpectedUpdatedAt         time.Time         `json:"expected_updated_at"`
		SecretFingerprint         string            `json:"secret_fingerprint,omitempty"`
	}{input.ID, input.Name, input.RPMLimit, input.TPMLimit, input.ConcurrencyLimit, input.Priority, input.Weight, input.SharedCapacityScope, input.SharedRPMLimit, input.SharedTPMLimit, input.SharedConcurrencyLimit, input.SharedDailyTokenLimit, input.SharedDailyResetMinuteUTC, input.ReplaceModels, input.DiscoveredModels, input.ExpectedUpdatedAt.UTC().Truncate(time.Microsecond), input.SecretFingerprint}
	return mutationFingerprint(request, "credential.update", payload)
}
