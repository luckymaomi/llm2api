package providers

import (
	"fmt"
	"net/url"
	"sort"
	"time"
)

type AdapterOptions struct {
	BaseURL      string
	Capabilities Capabilities
}

type AdapterBuilder func(AdapterOptions) (Adapter, error)

type Definition struct {
	Kind        Kind
	DisplayName string
	Contract    ContractInfo
	Presets     []ProviderPreset
	Models      []ModelProfile
	Build       AdapterBuilder
}

type KindInfo struct {
	Kind        Kind
	DisplayName string
	Contract    ContractInfo
}

type VerificationStatus string

const (
	VerificationVerified VerificationStatus = "verified"
	VerificationDegraded VerificationStatus = "degraded"
)

type ContractInfo struct {
	ReferenceURL      string             `json:"reference_url"`
	ContractSnapshot  string             `json:"contract_snapshot"`
	VerifiedAt        string             `json:"verified_at"`
	ReferenceProvider string             `json:"reference_provider,omitempty"`
	VerifiedModels    []string           `json:"verified_models"`
	LiveCapabilities  []string           `json:"live_capabilities"`
	Status            VerificationStatus `json:"status"`
}

type Catalog struct {
	definitions  map[Kind]Definition
	kinds        []KindInfo
	presets      []ProviderPreset
	presetByID   map[string]ProviderPreset
	modelsByKind map[Kind][]ModelProfile
}

func NewCatalog(definitions []Definition) (*Catalog, error) {
	if len(definitions) == 0 {
		return nil, fmt.Errorf("at least one Provider definition is required")
	}
	catalog := &Catalog{
		definitions:  make(map[Kind]Definition, len(definitions)),
		kinds:        make([]KindInfo, 0, len(definitions)),
		presetByID:   make(map[string]ProviderPreset),
		modelsByKind: make(map[Kind][]ModelProfile, len(definitions)),
	}
	for _, definition := range definitions {
		if definition.Kind == "" || definition.DisplayName == "" || definition.Build == nil {
			return nil, fmt.Errorf("Provider definition requires kind, display name, and builder")
		}
		if err := validateContractInfo(definition.Contract); err != nil {
			return nil, fmt.Errorf("Provider kind %q contract: %w", definition.Kind, err)
		}
		if _, exists := catalog.definitions[definition.Kind]; exists {
			return nil, fmt.Errorf("Provider kind %q is registered more than once", definition.Kind)
		}
		if err := validateModelProfiles(definition.Models); err != nil {
			return nil, fmt.Errorf("Provider kind %q model profiles: %w", definition.Kind, err)
		}
		for _, preset := range definition.Presets {
			preset.Kind = definition.Kind
			if err := validateProviderPreset(preset); err != nil {
				return nil, fmt.Errorf("Provider preset %q: %w", preset.ID, err)
			}
			if _, exists := catalog.presetByID[preset.ID]; exists {
				return nil, fmt.Errorf("Provider preset %q is registered more than once", preset.ID)
			}
			preset = cloneProviderPreset(preset)
			catalog.presetByID[preset.ID] = preset
			catalog.presets = append(catalog.presets, preset)
		}
		definition.Presets = cloneProviderPresets(definition.Presets)
		definition.Models = cloneModelProfiles(definition.Models)
		definition.Contract = cloneContractInfo(definition.Contract)
		catalog.definitions[definition.Kind] = definition
		catalog.modelsByKind[definition.Kind] = cloneModelProfiles(definition.Models)
		catalog.kinds = append(catalog.kinds, KindInfo{Kind: definition.Kind, DisplayName: definition.DisplayName, Contract: cloneContractInfo(definition.Contract)})
	}
	sort.Slice(catalog.kinds, func(i, j int) bool { return catalog.kinds[i].Kind < catalog.kinds[j].Kind })
	sort.Slice(catalog.presets, func(i, j int) bool { return catalog.presets[i].ID < catalog.presets[j].ID })
	return catalog, nil
}

// ModelCapabilities returns the code-verified capability profile for an
// upstream model. Unknown upstream names deliberately have no profile: the
// gateway must not invent a capability contract from a provider prefix alone.
func (c *Catalog) ModelCapabilities(kind Kind, upstreamName string) (ModelProfile, bool) {
	if c == nil {
		return ModelProfile{}, false
	}
	for _, profile := range c.modelsByKind[kind] {
		if profile.UpstreamName == upstreamName {
			return cloneModelProfile(profile), true
		}
	}
	return ModelProfile{}, false
}

// ModelProfiles returns the code-owned, verified model directory for one
// Provider. It is distinct from models discovered under a particular API Key:
// a Key may have access to only a subset of this directory.
func (c *Catalog) ModelProfiles(kind Kind) []ModelProfile {
	if c == nil {
		return nil
	}
	return cloneModelProfiles(c.modelsByKind[kind])
}

func (c *Catalog) Build(kind Kind, options AdapterOptions) (Adapter, error) {
	if c == nil {
		return nil, fmt.Errorf("Provider catalog is required")
	}
	definition, found := c.definitions[kind]
	if !found {
		return nil, fmt.Errorf("unsupported Provider kind %q", kind)
	}
	return definition.Build(options)
}

func (c *Catalog) Supports(kind Kind) bool {
	if c == nil {
		return false
	}
	_, found := c.definitions[kind]
	return found
}

func (c *Catalog) Kinds() []KindInfo {
	if c == nil {
		return nil
	}
	result := make([]KindInfo, len(c.kinds))
	for index, info := range c.kinds {
		result[index] = KindInfo{Kind: info.Kind, DisplayName: info.DisplayName, Contract: cloneContractInfo(info.Contract)}
	}
	return result
}

func validateContractInfo(info ContractInfo) error {
	reference, err := url.ParseRequestURI(info.ReferenceURL)
	if err != nil || reference.Scheme != "https" || reference.Host == "" {
		return fmt.Errorf("reference URL must be an absolute HTTPS URL")
	}
	if info.ContractSnapshot == "" {
		return fmt.Errorf("contract snapshot is required")
	}
	if _, err := time.Parse(time.DateOnly, info.VerifiedAt); err != nil {
		return fmt.Errorf("verified date must use YYYY-MM-DD")
	}
	if len(info.VerifiedModels) == 0 || len(info.LiveCapabilities) == 0 {
		return fmt.Errorf("verified models and live capabilities are required")
	}
	if info.Status != VerificationVerified && info.Status != VerificationDegraded {
		return fmt.Errorf("verification status must be verified or degraded")
	}
	for label, values := range map[string][]string{"verified models": info.VerifiedModels, "live capabilities": info.LiveCapabilities} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" {
				return fmt.Errorf("%s cannot contain an empty value", label)
			}
			if _, found := seen[value]; found {
				return fmt.Errorf("%s contains duplicate %q", label, value)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func cloneContractInfo(info ContractInfo) ContractInfo {
	info.VerifiedModels = append([]string(nil), info.VerifiedModels...)
	info.LiveCapabilities = append([]string(nil), info.LiveCapabilities...)
	return info
}

var defaultCatalog = mustCatalog([]Definition{
	{
		Kind: KindKimi, DisplayName: "Kimi",
		Contract: ContractInfo{
			ReferenceURL: "https://platform.kimi.com/docs/api/chat", ContractSnapshot: "2026-07-26",
			VerifiedAt: "2026-07-26", ReferenceProvider: "Moonshot AI", VerifiedModels: []string{"kimi-k3", "kimi-k2.6"},
			LiveCapabilities: []string{"models", "chat", "stream", "tools", "streaming_tools", "reasoning", "vision", "structured_output", "partial_mode", "prompt_cache", "usage", "balance_endpoint"}, Status: VerificationVerified,
		},
		Presets: []ProviderPreset{{
			ID: "kimi", Slug: "kimi", Name: "Kimi", BaseURL: "https://api.moonshot.cn/v1",
			SourceURL: "https://platform.kimi.com/docs/api/overview", VerifiedAt: "2026-07-26",
		}},
		Models: []ModelProfile{
			kimiK3Model(), kimiK27CodeModel("kimi-k2.7-code"), kimiK27CodeModel("kimi-k2.7-code-highspeed"),
			kimiK26Model(), kimiK25Model(),
		},
		Build: func(options AdapterOptions) (Adapter, error) {
			return NewKimiWithCapabilities(options.BaseURL, options.Capabilities)
		},
	},
	{
		Kind: KindSiliconFlow, DisplayName: "硅基流动",
		Contract: ContractInfo{
			ReferenceURL: "https://api-docs.siliconflow.cn/docs/api/chat-completions-post", ContractSnapshot: "2026-07-22",
			VerifiedAt: "2026-07-22", ReferenceProvider: "SiliconFlow", VerifiedModels: []string{"Qwen/Qwen3.5-9B"},
			LiveCapabilities: []string{"models", "chat", "responses", "stream", "tools", "reasoning", "usage", "error", "cancel"}, Status: VerificationVerified,
		},
		Presets: []ProviderPreset{{
			ID: "siliconflow", Slug: "siliconflow", Name: "硅基流动", BaseURL: "https://api.siliconflow.cn/v1",
			SourceURL: "https://api-docs.siliconflow.cn/docs/api/chat-completions-post", VerifiedAt: "2026-07-22",
		}},
		Models: []ModelProfile{
			siliconFlowModel("Qwen/Qwen3.5-9B", 0, 0, true),
			siliconFlowModel("Pro/zai-org/GLM-4.7", 0, 0, true),
		},
		Build: func(options AdapterOptions) (Adapter, error) {
			return NewSiliconFlow(SiliconFlowOptions{BaseURL: options.BaseURL, Capabilities: options.Capabilities})
		},
	},
	{
		Kind: KindZhipu, DisplayName: "智谱 GLM",
		Contract: ContractInfo{
			ReferenceURL: "https://docs.bigmodel.cn/cn/guide/develop/http/introduction", ContractSnapshot: "2026-07-22",
			VerifiedAt: "2026-07-22", VerifiedModels: []string{"glm-5.2"},
			LiveCapabilities: []string{"models", "chat", "stream", "tools", "reasoning", "usage", "quota_error", "same_pool_takeover"}, Status: VerificationVerified,
		},
		Presets: []ProviderPreset{{
			ID: "zhipu", Slug: "zhipu", Name: "智谱 GLM", BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			SourceURL: "https://docs.bigmodel.cn/cn/guide/develop/http/introduction", VerifiedAt: "2026-07-22",
		}},
		Models: []ModelProfile{
			zhipuTextModel("glm-5.2", 1_000_000, 131_072, true, true),
			zhipuTextModel("glm-5.1", 0, 0, false, true),
			zhipuTextModel("glm-5", 0, 0, false, true),
			zhipuTextModel("glm-5-turbo", 0, 0, false, false),
			zhipuTextModel("glm-4.7-flash", 200_000, 131_072, false, true),
			zhipuTextModel("glm-4.7", 0, 0, false, true),
			zhipuTextModel("glm-4.6", 0, 0, false, false),
			zhipuTextModel("glm-4.5", 0, 0, false, false),
			zhipuVisionModel("glm-5v-turbo", 200_000, 131_072),
		},
		Build: func(options AdapterOptions) (Adapter, error) {
			return NewZhipuWithCapabilities(options.BaseURL, options.Capabilities)
		},
	},
	{
		Kind: KindAgnes, DisplayName: "Agnes",
		Contract: ContractInfo{
			ReferenceURL: "https://apihub.agnes-ai.com/v1", ContractSnapshot: "2026-07-22 live API wire",
			VerifiedAt: "2026-07-22", VerifiedModels: []string{"agnes-2.0-flash"},
			LiveCapabilities: []string{"models", "chat", "stream", "tools", "streaming_tools", "reasoning", "usage", "cancel"}, Status: VerificationVerified,
		},
		Presets: []ProviderPreset{{
			ID: "agnes", Slug: "agnes", Name: "Agnes", BaseURL: "https://apihub.agnes-ai.com/v1",
			SourceURL: "https://apihub.agnes-ai.com/v1", VerifiedAt: "2026-07-22",
		}},
		Models: []ModelProfile{
			agnesTextModel("agnes-1.5-flash", 256_000, 65_536, false),
			agnesTextModel("agnes-2.0-flash", 512_000, 65_536, true),
		},
		Build: func(options AdapterOptions) (Adapter, error) {
			return NewAgnesWithCapabilities(options.BaseURL, options.Capabilities)
		},
	},
})

func DefaultCatalog() *Catalog {
	return defaultCatalog
}

func mustCatalog(definitions []Definition) *Catalog {
	catalog, err := NewCatalog(definitions)
	if err != nil {
		panic(err)
	}
	return catalog
}
