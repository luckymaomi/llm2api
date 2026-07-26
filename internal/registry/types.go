package registry

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/providers"
)

var (
	ErrInvalidInput             = errors.New("invalid registry input")
	ErrNotFound                 = errors.New("registry record not found")
	ErrConflict                 = errors.New("registry conflict")
	ErrForbidden                = errors.New("registry operation forbidden")
	ErrIdempotencyConflict      = errors.New("registry idempotency key conflict")
	ErrOutcomeUnknown           = errors.New("registry operation outcome is unknown")
	ErrModelDiscovery           = errors.New("upstream model discovery failed")
	ErrCredentialAlreadyManaged = errors.New("upstream credential is already managed")
)

type MutationRequest struct {
	IdempotencyKey uuid.UUID
	RequestID      string
}

type Mutation struct {
	Action             string
	IdempotencyKey     uuid.UUID
	RequestFingerprint []byte
	RequestID          string
}

type ModelCapabilities struct {
	Chat                         bool                            `json:"chat"`
	Streaming                    bool                            `json:"streaming"`
	Tools                        bool                            `json:"tools"`
	ToolChoiceModes              []string                        `json:"tool_choice_modes,omitempty"`
	StrictTools                  bool                            `json:"strict_tools"`
	ParallelToolCalls            bool                            `json:"parallel_tool_calls"`
	ToolStreaming                bool                            `json:"tool_streaming"`
	ImageInput                   bool                            `json:"image_input"`
	VideoInput                   bool                            `json:"video_input"`
	PartialMode                  bool                            `json:"partial_mode"`
	Reasoning                    bool                            `json:"reasoning"`
	ReasoningMode                ReasoningMode                   `json:"reasoning_mode,omitempty"`
	ReasoningAlwaysOn            bool                            `json:"reasoning_always_on"`
	ReasoningDefaultEnabled      bool                            `json:"reasoning_default_enabled"`
	ReasoningConfig              bool                            `json:"reasoning_config"`
	ReasoningContent             bool                            `json:"reasoning_content"`
	ReasoningPreserve            bool                            `json:"reasoning_preserve"`
	ReasoningEfforts             []string                        `json:"reasoning_efforts,omitempty"`
	ToolChoiceModesWithReasoning []string                        `json:"tool_choice_modes_with_reasoning,omitempty"`
	StructuredOutput             bool                            `json:"structured_output"`
	JSONSchemaOutput             bool                            `json:"json_schema_output"`
	MessageName                  bool                            `json:"message_name"`
	PromptCacheKey               bool                            `json:"prompt_cache_key"`
	SafetyIdentifier             bool                            `json:"safety_identifier"`
	ResponseUsage                bool                            `json:"response_usage"`
	StreamUsage                  bool                            `json:"stream_usage"`
	ContextTokens                int64                           `json:"context_tokens"`
	OutputTokens                 int64                           `json:"output_tokens"`
	Parameters                   providers.ParameterCapabilities `json:"parameters"`
}

// ModelCapabilitiesFromProfile converts the Provider-owned verified matrix
// into the persisted/public model contract. No caller derives capabilities
// from a model-name prefix or a generic OpenAI-compatible assumption.
func ModelCapabilitiesFromProfile(profile providers.ModelProfile) ModelCapabilities {
	capability := profile.Capabilities
	reasoningMode := ReasoningMode("")
	if capability.ReasoningAlwaysOn {
		reasoningMode = ReasoningAlwaysOn
	} else if capability.ReasoningToggle && capability.ReasoningEffort {
		reasoningMode = ReasoningHybrid
	} else if capability.ReasoningToggle {
		reasoningMode = ReasoningToggle
	} else if capability.ReasoningEffort {
		reasoningMode = ReasoningEffort
	}
	toolChoice := make([]string, 0, 4)
	if capability.ToolChoiceNone {
		toolChoice = append(toolChoice, "none")
	}
	if capability.ToolChoiceAuto {
		toolChoice = append(toolChoice, "auto")
	}
	if capability.ToolChoiceRequired {
		toolChoice = append(toolChoice, "required")
	}
	if capability.ToolChoiceNamed {
		toolChoice = append(toolChoice, "function")
	}
	efforts := make([]string, len(capability.AllowedReasoningEfforts))
	for index, effort := range capability.AllowedReasoningEfforts {
		efforts[index] = string(effort)
	}
	thinkingToolChoice := make([]string, len(capability.ToolChoiceModesWithReasoning))
	for index, mode := range capability.ToolChoiceModesWithReasoning {
		thinkingToolChoice[index] = string(mode)
	}
	return ModelCapabilities{
		Chat: capability.Chat, Streaming: capability.Streaming, Tools: capability.Tools,
		ToolChoiceModes: toolChoice, StrictTools: capability.StrictTools, ParallelToolCalls: capability.ParallelToolCalls, ToolStreaming: capability.ToolStreaming,
		ImageInput: capability.ImageInput, VideoInput: capability.VideoInput, PartialMode: capability.PartialMode,
		Reasoning:     capability.ReasoningToggle || capability.ReasoningAlwaysOn || capability.ReasoningEffort || capability.ReasoningContent,
		ReasoningMode: reasoningMode, ReasoningAlwaysOn: capability.ReasoningAlwaysOn, ReasoningDefaultEnabled: capability.ReasoningDefaultEnabled, ReasoningConfig: capability.ReasoningConfig, ReasoningContent: capability.ReasoningContent,
		ReasoningPreserve: capability.ReasoningReplay, ReasoningEfforts: efforts, ToolChoiceModesWithReasoning: thinkingToolChoice,
		StructuredOutput: capability.JSONOutput, JSONSchemaOutput: capability.JSONSchemaOutput, MessageName: capability.MessageName,
		PromptCacheKey: capability.PromptCacheKey, SafetyIdentifier: capability.SafetyIdentifier, ResponseUsage: capability.ResponseUsage, StreamUsage: capability.StreamUsage,
		ContextTokens: profile.ContextTokens, OutputTokens: profile.OutputTokens,
		Parameters: providers.CloneParameterCapabilities(capability.Parameters),
	}
}

// AdapterCapabilities is the only conversion from persisted model capability
// facts back to a provider adapter policy.
func (c ModelCapabilities) AdapterCapabilities() providers.Capabilities {
	capability := providers.Capabilities{
		Chat: c.Chat, Models: true, Streaming: c.Streaming, Tools: c.Tools, ToolStreaming: c.ToolStreaming, StrictTools: c.StrictTools,
		ParallelToolCalls: c.ParallelToolCalls, ImageInput: c.ImageInput, VideoInput: c.VideoInput, PartialMode: c.PartialMode, JSONOutput: c.StructuredOutput,
		JSONSchemaOutput: c.JSONSchemaOutput, MessageName: c.MessageName, ReasoningContent: c.ReasoningContent, ReasoningReplay: c.ReasoningPreserve,
		ReasoningAlwaysOn: c.ReasoningAlwaysOn, ReasoningDefaultEnabled: c.ReasoningDefaultEnabled, ReasoningConfig: c.ReasoningConfig,
		PromptCacheKey: c.PromptCacheKey, SafetyIdentifier: c.SafetyIdentifier, ResponseUsage: c.ResponseUsage, StreamUsage: c.StreamUsage,
		Parameters: providers.CloneParameterCapabilities(c.Parameters),
	}
	for _, mode := range c.ToolChoiceModes {
		switch mode {
		case "none":
			capability.ToolChoiceNone = true
		case "auto":
			capability.ToolChoiceAuto = true
		case "required":
			capability.ToolChoiceRequired = true
		case "function":
			capability.ToolChoiceNamed = true
		}
	}
	capability.ReasoningToggle = c.ReasoningMode == ReasoningToggle || c.ReasoningMode == ReasoningHybrid
	capability.ReasoningEffort = c.ReasoningMode == ReasoningEffort || c.ReasoningMode == ReasoningHybrid || len(c.ReasoningEfforts) > 0
	for _, effort := range c.ReasoningEfforts {
		capability.AllowedReasoningEfforts = append(capability.AllowedReasoningEfforts, canonicalReasoningEffort(effort))
	}
	for _, mode := range c.ToolChoiceModesWithReasoning {
		capability.ToolChoiceModesWithReasoning = append(capability.ToolChoiceModesWithReasoning, canonical.ToolChoiceMode(mode))
	}
	return capability
}

func canonicalReasoningEffort(value string) canonical.ReasoningEffort {
	return canonical.ReasoningEffort(value)
}

type ReasoningMode string

const (
	ReasoningToggle   ReasoningMode = "toggle"
	ReasoningEffort   ReasoningMode = "effort"
	ReasoningHybrid   ReasoningMode = "hybrid"
	ReasoningAlwaysOn ReasoningMode = "always_on"
)

type Model struct {
	ID           uuid.UUID         `json:"id"`
	ProviderID   uuid.UUID         `json:"provider_id"`
	ProviderSlug string            `json:"provider_slug,omitempty"`
	ProviderName string            `json:"provider_name,omitempty"`
	PublicName   string            `json:"public_name"`
	UpstreamName string            `json:"upstream_name"`
	DisplayName  string            `json:"display_name"`
	Capabilities ModelCapabilities `json:"capabilities"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type Provider struct {
	ID                    uuid.UUID              `json:"id"`
	CatalogID             string                 `json:"catalog_id"`
	Slug                  string                 `json:"slug"`
	Name                  string                 `json:"name"`
	Kind                  providers.Kind         `json:"kind"`
	BaseURL               string                 `json:"base_url"`
	SourceURL             string                 `json:"source_url"`
	VerifiedAt            time.Time              `json:"verified_at"`
	Contract              providers.ContractInfo `json:"contract"`
	Models                []ProviderModelProfile `json:"models"`
	ResourcePoolCount     int64                  `json:"resource_pool_count"`
	ActiveCredentialCount int64                  `json:"active_credential_count"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

// ProviderModelProfile is the code-owned capability directory presented while
// an administrator chooses a Provider for a resource pool. It does not claim
// that every imported API Key is authorized for every listed model.
type ProviderModelProfile struct {
	UpstreamName string            `json:"upstream_name"`
	DisplayName  string            `json:"display_name"`
	Capabilities ModelCapabilities `json:"capabilities"`
}

type ProviderProjection struct {
	CatalogID  string
	Slug       string
	Name       string
	Kind       providers.Kind
	BaseURL    string
	SourceURL  string
	VerifiedAt time.Time
}

type ResourcePoolStatus string

const (
	ResourcePoolActive   ResourcePoolStatus = "active"
	ResourcePoolDisabled ResourcePoolStatus = "disabled"
	ResourcePoolRetired  ResourcePoolStatus = "retired"
)

type ResourcePool struct {
	ID                    uuid.UUID          `json:"id"`
	ProviderID            uuid.UUID          `json:"provider_id"`
	ProviderCatalogID     string             `json:"provider_catalog_id"`
	ProviderSlug          string             `json:"provider_slug"`
	ProviderName          string             `json:"provider_name"`
	ProviderKind          providers.Kind     `json:"provider_kind"`
	ProviderBaseURL       string             `json:"provider_base_url"`
	Slug                  string             `json:"slug"`
	Name                  string             `json:"name"`
	Status                ResourcePoolStatus `json:"status"`
	Models                []Model            `json:"models"`
	ModelCount            int64              `json:"model_count"`
	CredentialCount       int64              `json:"credential_count"`
	ActiveCredentialCount int64              `json:"active_credential_count"`
	RetiredAt             *time.Time         `json:"retired_at,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

type NewResourcePool struct {
	ProviderID uuid.UUID
	Slug       string
	Name       string
}

type ResourcePoolChange struct {
	ID                uuid.UUID
	Name              string
	ExpectedUpdatedAt time.Time
}

type CredentialStatus string

const (
	CredentialActive   CredentialStatus = "active"
	CredentialDisabled CredentialStatus = "disabled"
	CredentialRetired  CredentialStatus = "retired"
)

type CredentialModelBinding struct {
	ModelID   uuid.UUID `json:"model_id"`
	ModelName string    `json:"model_name,omitempty"`
}

type DiscoveredModel struct {
	UpstreamName string
	Capabilities ModelCapabilities
}

type CredentialHealthStatus string

const (
	CredentialHealthy        CredentialHealthStatus = "healthy"
	CredentialHealthCooling  CredentialHealthStatus = "cooling"
	CredentialHealthProbing  CredentialHealthStatus = "probing"
	CredentialRepairRequired CredentialHealthStatus = "repair_required"
)

type Credential struct {
	ID                        uuid.UUID                            `json:"id"`
	ResourcePoolID            uuid.UUID                            `json:"resource_pool_id"`
	ResourcePoolName          string                               `json:"resource_pool_name"`
	ResourcePoolSlug          string                               `json:"resource_pool_slug"`
	ProviderID                uuid.UUID                            `json:"provider_id"`
	ProviderName              string                               `json:"provider_name"`
	ProviderKind              providers.Kind                       `json:"provider_kind"`
	ProviderBaseURL           string                               `json:"provider_base_url"`
	Name                      string                               `json:"name"`
	Status                    CredentialStatus                     `json:"status"`
	HealthStatus              CredentialHealthStatus               `json:"health_status"`
	HealthGeneration          int64                                `json:"health_generation"`
	RPMLimit                  *int32                               `json:"rpm_limit,omitempty"`
	TPMLimit                  *int64                               `json:"tpm_limit,omitempty"`
	ConcurrencyLimit          *int32                               `json:"concurrency_limit,omitempty"`
	Priority                  int32                                `json:"priority"`
	Weight                    int32                                `json:"weight"`
	SharedCapacityScope       *string                              `json:"shared_capacity_scope,omitempty"`
	SharedRPMLimit            *int32                               `json:"shared_rpm_limit,omitempty"`
	SharedTPMLimit            *int64                               `json:"shared_tpm_limit,omitempty"`
	SharedConcurrencyLimit    *int32                               `json:"shared_concurrency_limit,omitempty"`
	SharedDailyTokenLimit     *int64                               `json:"shared_daily_token_limit,omitempty"`
	SharedDailyResetMinuteUTC *int32                               `json:"shared_daily_reset_minute_utc,omitempty"`
	CooldownUntil             *time.Time                           `json:"cooldown_until,omitempty"`
	ConsecutiveFailures       int32                                `json:"consecutive_failures"`
	LastSuccessAt             *time.Time                           `json:"last_success_at,omitempty"`
	LastErrorKind             *string                              `json:"last_error_kind,omitempty"`
	LastProbeAt               *time.Time                           `json:"last_probe_at,omitempty"`
	LastProbeLatencyMs        *int64                               `json:"last_probe_latency_ms,omitempty"`
	LastProbeKind             *string                              `json:"last_probe_kind,omitempty"`
	LastProbeStatus           *string                              `json:"last_probe_status,omitempty"`
	LastProbeErrorKind        *string                              `json:"last_probe_error_kind,omitempty"`
	UpstreamStatus            *providers.UpstreamStatusObservation `json:"upstream_status,omitempty"`
	LastCheckedAt             *time.Time                           `json:"last_checked_at,omitempty"`
	RecentSuccessRate         *float64                             `json:"recent_success_rate,omitempty"`
	FirstByteP95Ms            *int64                               `json:"first_byte_p95_ms,omitempty"`
	TotalLatencyP95Ms         *int64                               `json:"total_latency_p95_ms,omitempty"`
	RetiredAt                 *time.Time                           `json:"retired_at,omitempty"`
	CreatedAt                 time.Time                            `json:"created_at"`
	UpdatedAt                 time.Time                            `json:"updated_at"`
	ModelBindings             []CredentialModelBinding             `json:"model_bindings"`
}

type NewCredential struct {
	ID                        uuid.UUID
	ResourcePoolID            uuid.UUID
	Name                      string
	EncryptedSecret           []byte
	SecretFingerprint         string
	RPMLimit                  *int32
	TPMLimit                  *int64
	ConcurrencyLimit          *int32
	Priority                  int32
	Weight                    int32
	SharedCapacityScope       *string
	SharedRPMLimit            *int32
	SharedTPMLimit            *int64
	SharedConcurrencyLimit    *int32
	SharedDailyTokenLimit     *int64
	SharedDailyResetMinuteUTC *int32
	DiscoveredModels          []DiscoveredModel
	Discovery                 ModelDiscoveryExecution
}

type CredentialChange struct {
	ID                        uuid.UUID
	Name                      string
	EncryptedSecret           []byte
	SecretFingerprint         string
	ReplaceSecret             bool
	ReplaceModels             bool
	RPMLimit                  *int32
	TPMLimit                  *int64
	ConcurrencyLimit          *int32
	Priority                  int32
	Weight                    int32
	SharedCapacityScope       *string
	SharedRPMLimit            *int32
	SharedTPMLimit            *int64
	SharedConcurrencyLimit    *int32
	SharedDailyTokenLimit     *int64
	SharedDailyResetMinuteUTC *int32
	ModelBindings             []CredentialModelBinding
	DiscoveredModels          []DiscoveredModel
	Discovery                 ModelDiscoveryExecution
	ExpectedUpdatedAt         time.Time
}

type CredentialBatchItem struct {
	Name   string `json:"name"`
	Secret string `json:"-"`
}

type CredentialBatchResult struct {
	Line       int         `json:"line"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	Credential *Credential `json:"credential,omitempty"`
	ErrorKind  string      `json:"error_kind,omitempty"`
}

type CredentialProbeTarget struct {
	Provider     Provider
	Model        Model
	CredentialID uuid.UUID
	Secret       string
	RequestID    string
}

type CredentialProbeExecution struct {
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	ErrorKind     *string   `json:"error_kind,omitempty"`
	Retryable     bool      `json:"retryable"`
	MayUseTokens  bool      `json:"may_use_tokens"`
	LatencyMillis int64     `json:"latency_ms"`
	ModelID       uuid.UUID `json:"model_id"`
	ModelName     string    `json:"model_name"`
	RequestID     string    `json:"request_id"`
	ResponseText  string    `json:"-"`
	InputTokens   *int64    `json:"input_tokens,omitempty"`
	OutputTokens  *int64    `json:"output_tokens,omitempty"`
}

type CredentialProbeExecutor interface {
	Execute(context.Context, CredentialProbeTarget) CredentialProbeExecution
}

type CredentialUpstreamStatusTarget struct {
	Provider     Provider
	CredentialID uuid.UUID
	Secret       string
	RequestID    string
}

type CredentialUpstreamStatusExecution struct {
	Observation   providers.UpstreamStatusObservation `json:"observation"`
	LatencyMillis int64                               `json:"latency_ms"`
	ErrorKind     *string                             `json:"error_kind,omitempty"`
}

type CredentialUpstreamStatusExecutor interface {
	FetchUpstreamStatus(context.Context, CredentialUpstreamStatusTarget) CredentialUpstreamStatusExecution
}

type ModelDiscoveryTarget struct {
	Provider Provider
	Secret   string
}

type ModelDiscoveryExecution struct {
	Status        string   `json:"status"`
	ErrorKind     *string  `json:"error_kind,omitempty"`
	Retryable     bool     `json:"retryable"`
	LatencyMillis int64    `json:"latency_ms"`
	Models        []string `json:"models"`
}

type CredentialModelProbeResult struct {
	Credential Credential
	Execution  ModelDiscoveryExecution
}

type CredentialModelProbeBatch struct {
	Results     []CredentialModelProbeResult
	Succeeded   int
	Failed      int
	Unavailable int
	Uncertain   int
}

type ModelDiscoveryExecutor interface {
	Discover(context.Context, ModelDiscoveryTarget) ModelDiscoveryExecution
}

type Repository interface {
	SyncCatalog(context.Context, []ProviderProjection) error
	ListProviders(context.Context) ([]Provider, error)
	GetProvider(context.Context, uuid.UUID) (Provider, error)
	ListModels(context.Context) ([]Model, error)

	CreateResourcePool(context.Context, NewResourcePool, uuid.UUID, Mutation) (ResourcePool, error)
	UpdateResourcePool(context.Context, ResourcePoolChange, uuid.UUID, Mutation) (ResourcePool, error)
	SetResourcePoolStatus(context.Context, uuid.UUID, ResourcePoolStatus, time.Time, uuid.UUID, Mutation) (ResourcePool, error)
	ListResourcePools(context.Context, bool) ([]ResourcePool, error)
	GetResourcePool(context.Context, uuid.UUID) (ResourcePool, error)

	CreateCredential(context.Context, NewCredential, uuid.UUID, Mutation) (Credential, error)
	UpdateCredential(context.Context, CredentialChange, uuid.UUID, Mutation) (Credential, error)
	GetCredentialBySecretFingerprint(context.Context, string) (Credential, error)
	SetCredentialStatus(context.Context, uuid.UUID, CredentialStatus, time.Time, uuid.UUID, Mutation) (Credential, error)
	RetireCredential(context.Context, uuid.UUID, []byte, time.Time, uuid.UUID, Mutation) (Credential, error)
	ListCredentials(context.Context, bool) ([]Credential, error)
	GetCredential(context.Context, uuid.UUID) (Credential, error)
	GetEncryptedCredential(context.Context, uuid.UUID) ([]byte, error)
	RecordCredentialProbe(context.Context, uuid.UUID, time.Time, CredentialProbeExecution, uuid.UUID, string) (Credential, error)
	RecordCredentialUpstreamStatus(context.Context, uuid.UUID, providers.UpstreamStatusObservation, uuid.UUID, string) (Credential, error)
}
