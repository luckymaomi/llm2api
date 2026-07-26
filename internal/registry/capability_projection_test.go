package registry

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/providers"
)

// This protects the public model-contract result that a capability declared
// for Kimi code models survives catalog projection, JSON persistence, and
// adapter reconstruction before its Provider request is sent.
func TestKimiReasoningConfigurationSurvivesCapabilityProjection(t *testing.T) {
	t.Parallel()

	profile, found := providers.DefaultCatalog().ModelCapabilities(providers.KindKimi, "kimi-k2.7-code")
	if !found {
		t.Fatal("Kimi code model profile is missing")
	}
	projected := ModelCapabilitiesFromProfile(profile)
	if !projected.ReasoningConfig || !projected.ReasoningContent || !projected.MessageName || !projected.ResponseUsage || !projected.StreamUsage {
		t.Fatalf("projected capabilities lost executable fields: %#v", projected)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal model capabilities: %v", err)
	}
	var restored ModelCapabilities
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("unmarshal model capabilities: %v", err)
	}
	if !restored.ReasoningConfig || !restored.ReasoningContent || !restored.MessageName || !restored.ResponseUsage || !restored.StreamUsage ||
		!restored.AdapterCapabilities().ReasoningConfig || !restored.AdapterCapabilities().ReasoningContent ||
		!restored.AdapterCapabilities().MessageName || !restored.AdapterCapabilities().ResponseUsage || !restored.AdapterCapabilities().StreamUsage {
		t.Fatalf("restored adapter capabilities lost reasoning configuration: %#v", restored.AdapterCapabilities())
	}

	adapter, err := providers.NewKimiWithCapabilities("https://provider.example.test/v1", restored.AdapterCapabilities())
	if err != nil {
		t.Fatalf("create Kimi adapter: %v", err)
	}
	enabled, preserve := true, true
	request, err := adapter.BuildRequest(context.Background(), providers.Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k2.7-code", Stream: true,
		Messages:  []canonical.Message{{Role: canonical.RoleUser, Name: "member", Content: canonical.TextContent("Reply")}},
		Reasoning: &canonical.ReasoningConfig{Enabled: &enabled, Preserve: &preserve},
	})
	if err != nil {
		t.Fatalf("build Kimi code request: %v", err)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read Kimi code request: %v", err)
	}
	for _, expected := range []string{
		`"name":"member"`,
		`"thinking":{"type":"enabled","keep":"all"}`,
		`"stream_options":{"include_usage":true}`,
	} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("Kimi code request lost declared capability %s: %s", expected, body)
		}
	}

	k3, found := providers.DefaultCatalog().ModelCapabilities(providers.KindKimi, "kimi-k3")
	if !found {
		t.Fatal("Kimi K3 model profile is missing")
	}
	k3Capabilities := ModelCapabilitiesFromProfile(k3).AdapterCapabilities()
	if !k3Capabilities.ReasoningAlwaysOn || !k3Capabilities.ReasoningEffort {
		t.Fatalf("Kimi K3 lost always-on effort capability: %#v", k3Capabilities)
	}
	k3Adapter, err := providers.NewKimiWithCapabilities("https://provider.example.test/v1", k3Capabilities)
	if err != nil {
		t.Fatalf("create Kimi K3 adapter: %v", err)
	}
	k3Request, err := k3Adapter.BuildRequest(context.Background(), providers.Credential{APIKey: "fixture-key"}, canonical.ChatRequest{
		Model: "kimi-k3", Messages: []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("Reply")}},
		Reasoning: &canonical.ReasoningConfig{Effort: canonical.ReasoningEffortHigh},
	})
	if err != nil {
		t.Fatalf("build Kimi K3 effort request: %v", err)
	}
	k3Body, err := io.ReadAll(k3Request.Body)
	if err != nil {
		t.Fatalf("read Kimi K3 effort request: %v", err)
	}
	if !strings.Contains(string(k3Body), `"reasoning_effort":"high"`) {
		t.Fatalf("Kimi K3 request lost reasoning effort: %s", k3Body)
	}
}

// This protects every catalogued model from capability drift between the
// Provider-owned profile and the adapter rebuilt from its persisted/public
// model contract.
func TestModelCapabilityProjectionPreservesExecutableAdapterFields(t *testing.T) {
	t.Parallel()

	catalog := providers.DefaultCatalog()
	for _, kind := range []providers.Kind{providers.KindAgnes, providers.KindKimi, providers.KindSiliconFlow, providers.KindZhipu} {
		for _, profile := range catalog.ModelProfiles(kind) {
			profile := profile
			t.Run(string(kind)+"/"+profile.UpstreamName, func(t *testing.T) {
				t.Parallel()
				want := profile.Capabilities
				got := ModelCapabilitiesFromProfile(profile).AdapterCapabilities()
				for _, field := range []struct {
					name string
					want bool
					got  bool
				}{
					{"chat", want.Chat, got.Chat},
					{"streaming", want.Streaming, got.Streaming},
					{"tools", want.Tools, got.Tools},
					{"tool_streaming", want.ToolStreaming, got.ToolStreaming},
					{"tool_choice_none", want.ToolChoiceNone, got.ToolChoiceNone},
					{"tool_choice_auto", want.ToolChoiceAuto, got.ToolChoiceAuto},
					{"tool_choice_required", want.ToolChoiceRequired, got.ToolChoiceRequired},
					{"tool_choice_named", want.ToolChoiceNamed, got.ToolChoiceNamed},
					{"strict_tools", want.StrictTools, got.StrictTools},
					{"parallel_tool_calls", want.ParallelToolCalls, got.ParallelToolCalls},
					{"image_input", want.ImageInput, got.ImageInput},
					{"video_input", want.VideoInput, got.VideoInput},
					{"partial_mode", want.PartialMode, got.PartialMode},
					{"json_output", want.JSONOutput, got.JSONOutput},
					{"json_schema_output", want.JSONSchemaOutput, got.JSONSchemaOutput},
					{"message_name", want.MessageName, got.MessageName},
					{"prompt_cache_key", want.PromptCacheKey, got.PromptCacheKey},
					{"safety_identifier", want.SafetyIdentifier, got.SafetyIdentifier},
					{"reasoning_toggle", want.ReasoningToggle, got.ReasoningToggle},
					{"reasoning_always_on", want.ReasoningAlwaysOn, got.ReasoningAlwaysOn},
					{"reasoning_default_enabled", want.ReasoningDefaultEnabled, got.ReasoningDefaultEnabled},
					{"reasoning_config", want.ReasoningConfig, got.ReasoningConfig},
					{"reasoning_content", want.ReasoningContent, got.ReasoningContent},
					{"reasoning_replay", want.ReasoningReplay, got.ReasoningReplay},
					{"reasoning_effort", want.ReasoningEffort, got.ReasoningEffort},
					{"response_usage", want.ResponseUsage, got.ResponseUsage},
					{"stream_usage", want.StreamUsage, got.StreamUsage},
				} {
					if field.want != field.got {
						t.Fatalf("%s = %t, want %t", field.name, field.got, field.want)
					}
				}
				if !reflect.DeepEqual(got.AllowedReasoningEfforts, want.AllowedReasoningEfforts) {
					t.Fatalf("allowed reasoning efforts = %#v, want %#v", got.AllowedReasoningEfforts, want.AllowedReasoningEfforts)
				}
				if !reflect.DeepEqual(got.ToolChoiceModesWithReasoning, want.ToolChoiceModesWithReasoning) {
					t.Fatalf("reasoning tool-choice modes = %#v, want %#v", got.ToolChoiceModesWithReasoning, want.ToolChoiceModesWithReasoning)
				}
				if !reflect.DeepEqual(got.Parameters, want.Parameters) {
					t.Fatalf("parameter capabilities = %#v, want %#v", got.Parameters, want.Parameters)
				}
			})
		}
	}
}
