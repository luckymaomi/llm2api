package providers

import (
	"context"
	"net/http"
	"testing"
)

func TestProviderCapabilitiesDescribeExecutableContracts(t *testing.T) {
	t.Parallel()

	zhipu := NewZhipu().Capabilities()
	if !zhipu.Chat || !zhipu.Models || !zhipu.Streaming || !zhipu.Tools || !zhipu.ToolStreaming || !zhipu.ReasoningReplay || !zhipu.ResponseUsage || !zhipu.StreamUsage || !zhipu.ResponseRequestID {
		t.Fatalf("Zhipu capabilities = %#v", zhipu)
	}
	agnes := NewAgnes().Capabilities()
	if !agnes.Chat || !agnes.Models || !agnes.Streaming || !agnes.Tools || !agnes.ToolStreaming || !agnes.ReasoningToggle || !agnes.ResponseUsage || !agnes.StreamUsage {
		t.Fatalf("Agnes capabilities = %#v", agnes)
	}
	kimi := NewKimi().Capabilities()
	if !kimi.Chat || !kimi.Models || !kimi.Streaming || !kimi.Tools || !kimi.ToolStreaming || !kimi.VideoInput || !kimi.PartialMode || !kimi.ReasoningAlwaysOn || !kimi.JSONSchemaOutput {
		t.Fatalf("Kimi capabilities = %#v", kimi)
	}
	silicon, err := NewSiliconFlow(SiliconFlowOptions{BaseURL: "https://llm.example/v1", Capabilities: SiliconFlowCapabilities()})
	if err != nil {
		t.Fatalf("create SiliconFlow adapter: %v", err)
	}
	capabilities := silicon.Capabilities()
	if !capabilities.Chat || !capabilities.Models || !capabilities.Streaming || !capabilities.Tools || !capabilities.JSONOutput || !capabilities.StreamUsage {
		t.Fatalf("SiliconFlow capabilities = %#v", capabilities)
	}
}

func TestModelsProbePreservesAllUniqueUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	models, providerError := NewAgnes().ParseProbe(ProbeModels, http.StatusOK, nil, []byte(`{
		"data":[{"id":"agnes-2.0-flash"},{"id":"agnes-2.0-pro"},{"id":"agnes-2.0-flash"}]
	}`))
	if providerError != nil {
		t.Fatalf("ParseProbe() error = %v", providerError)
	}
	if len(models) != 2 || models[0].ID != "agnes-2.0-flash" || models[1].ID != "agnes-2.0-pro" {
		t.Fatalf("ParseProbe() models = %#v", models)
	}
}

func TestModelsProbeRejectsMalformedModelIdentity(t *testing.T) {
	t.Parallel()

	models, providerError := NewZhipu().ParseProbe(ProbeModels, http.StatusOK, nil, []byte(`{"data":[{"id":"glm-5.2\nsecret"}]}`))
	if providerError == nil || providerError.Code != "invalid_model_id" || models != nil {
		t.Fatalf("ParseProbe() = (%#v, %#v), want safe invalid_model_id", models, providerError)
	}
}

func TestPublishedProviderModelsProbesAreNonGenerating(t *testing.T) {
	t.Parallel()

	silicon, err := NewSiliconFlow(SiliconFlowOptions{
		BaseURL: "https://llm.example/v1", Capabilities: SiliconFlowCapabilities(),
	})
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	adapters := []struct {
		name     string
		adapter  Adapter
		endpoint string
	}{
		{name: "siliconflow", adapter: silicon, endpoint: "https://llm.example/v1/models"},
		{name: "zhipu", adapter: NewZhipu(), endpoint: "https://open.bigmodel.cn/api/paas/v4/models"},
		{name: "agnes", adapter: NewAgnes(), endpoint: "https://apihub.agnes-ai.com/v1/models"},
		{name: "kimi", adapter: NewKimi(), endpoint: "https://api.moonshot.cn/v1/models"},
	}
	for _, test := range adapters {
		probe, probeErr := test.adapter.Probe(context.Background(), Credential{APIKey: "fixture-key"})
		if probeErr != nil {
			t.Fatalf("%s build probe: %v", test.name, probeErr)
		}
		if !probe.Available || probe.MayConsumeTokens || probe.Kind != ProbeModels || probe.Request == nil {
			t.Fatalf("%s probe = %#v", test.name, probe)
		}
		if probe.Request.Method != http.MethodGet || probe.Request.URL.String() != test.endpoint {
			t.Fatalf("%s probe request = %s %s", test.name, probe.Request.Method, probe.Request.URL)
		}
	}
}
