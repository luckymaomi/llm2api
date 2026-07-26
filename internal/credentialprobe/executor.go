package credentialprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/luckymaomi/llm2api/internal/canonical"
	"github.com/luckymaomi/llm2api/internal/providers"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/security"
)

type Executor struct {
	policy           security.SSRFPolicy
	timeout          time.Duration
	maxResponseBytes int64
	catalog          *providers.Catalog
}

func New(policy security.SSRFPolicy, timeout time.Duration, maxResponseBytes int64) (*Executor, error) {
	if timeout <= 0 || maxResponseBytes < 1024 {
		return nil, errors.New("credential probe bounds are invalid")
	}
	if _, err := security.NewURLValidator(policy); err != nil {
		return nil, fmt.Errorf("credential probe SSRF policy: %w", err)
	}
	return &Executor{policy: policy, timeout: timeout, maxResponseBytes: maxResponseBytes, catalog: providers.DefaultCatalog()}, nil
}

func (e *Executor) Discover(ctx context.Context, target registry.ModelDiscoveryTarget) registry.ModelDiscoveryExecution {
	startedAt := time.Now()
	result := registry.ModelDiscoveryExecution{Status: "failed", Models: []string{}}
	adapter, err := e.catalog.Build(target.Provider.Kind, providers.AdapterOptions{
		BaseURL: target.Provider.BaseURL, Capabilities: providers.ModelDiscoveryCapabilities(),
	})
	if err != nil {
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		return withDiscoveryLatency(result, startedAt)
	}
	probeContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	probe, err := adapter.Probe(probeContext, providers.Credential{APIKey: target.Secret})
	if err != nil {
		result.ErrorKind = stringPointer(canonicalErrorKind(err))
		return withDiscoveryLatency(result, startedAt)
	}
	if !probe.Available || probe.Request == nil {
		result.ErrorKind = stringPointer("model_discovery_unsupported")
		return withDiscoveryLatency(result, startedAt)
	}
	client, err := security.NewSSRFSafeClient(e.policy)
	if err != nil {
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		return withDiscoveryLatency(result, startedAt)
	}
	response, err := client.Do(probe.Request)
	if err != nil {
		result.Status, result.ErrorKind, result.Retryable = classifyTransportFailure(err)
		return withDiscoveryLatency(result, startedAt)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, e.maxResponseBytes+1))
	if err != nil {
		result.Status = "uncertain"
		result.ErrorKind = stringPointer(string(canonical.ErrorUncertain))
		return withDiscoveryLatency(result, startedAt)
	}
	if int64(len(body)) > e.maxResponseBytes {
		result.ErrorKind = stringPointer("probe_response_too_large")
		return withDiscoveryLatency(result, startedAt)
	}
	models, providerError := adapter.ParseProbe(probe.Kind, response.StatusCode, response.Header, body)
	if providerError != nil {
		kind := string(providerError.Kind)
		result.ErrorKind = &kind
		result.Retryable = retryableKind(kind)
		return withDiscoveryLatency(result, startedAt)
	}
	result.Models = make([]string, 0, len(models))
	for _, model := range models {
		result.Models = append(result.Models, model.ID)
	}
	result.Status = "succeeded"
	return withDiscoveryLatency(result, startedAt)
}

// FetchUpstreamStatus is deliberately separate from model discovery and the
// billable generation probe. It only follows a Provider adapter's documented,
// read-only status endpoint.
func (e *Executor) FetchUpstreamStatus(ctx context.Context, target registry.CredentialUpstreamStatusTarget) registry.CredentialUpstreamStatusExecution {
	startedAt := time.Now()
	result := registry.CredentialUpstreamStatusExecution{Observation: providers.UpstreamStatusObservation{
		State: providers.UpstreamStatusUnavailable, Scope: providers.UpstreamStatusScopeUnknown,
		Source: "manual_status_fetch_failed", Reason: "无法获取上游状态",
	}}
	adapter, err := e.catalog.Build(target.Provider.Kind, providers.AdapterOptions{
		BaseURL: target.Provider.BaseURL, Capabilities: providers.ModelDiscoveryCapabilities(),
	})
	if err != nil {
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		result.Observation.Reason = "上游状态适配器不可用"
		return withUpstreamStatusLatency(result, startedAt)
	}
	probeContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	probe, err := adapter.StatusProbe(probeContext, providers.Credential{APIKey: target.Secret})
	if err != nil {
		kind := canonicalErrorKind(err)
		result.ErrorKind = &kind
		result.Observation.Reason = upstreamStatusReason(kind)
		return withUpstreamStatusLatency(result, startedAt)
	}
	if !probe.Available || probe.Request == nil {
		result.Observation = providers.UpstreamStatusObservation{
			State: providers.UpstreamStatusUnknown, Scope: providers.UpstreamStatusScopeUnknown,
			Source: "official_endpoint_unavailable", Reason: "该 Provider 未公开可读取的上游状态端点",
		}
		return withUpstreamStatusLatency(result, startedAt)
	}
	client, err := security.NewSSRFSafeClient(e.policy)
	if err != nil {
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		result.Observation.Reason = "上游状态网络策略不可用"
		return withUpstreamStatusLatency(result, startedAt)
	}
	response, err := client.Do(probe.Request)
	if err != nil {
		_, kind, _ := classifyTransportFailure(err)
		result.ErrorKind = kind
		result.Observation.Reason = upstreamStatusReason(pointerValue(kind))
		return withUpstreamStatusLatency(result, startedAt)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, e.maxResponseBytes+1))
	if err != nil {
		result.ErrorKind = stringPointer(string(canonical.ErrorUncertain))
		result.Observation.Reason = "上游状态响应读取中断"
		return withUpstreamStatusLatency(result, startedAt)
	}
	if int64(len(body)) > e.maxResponseBytes {
		result.ErrorKind = stringPointer("status_response_too_large")
		result.Observation.Reason = "上游状态响应超过安全上限"
		return withUpstreamStatusLatency(result, startedAt)
	}
	observation, providerErr := adapter.ParseStatusProbe(probe.Kind, response.StatusCode, response.Header, body)
	if providerErr != nil {
		kind := string(providerErr.Kind)
		result.ErrorKind = &kind
		result.Observation.Source = "official_status_endpoint"
		result.Observation.Reason = upstreamStatusReason(kind)
		return withUpstreamStatusLatency(result, startedAt)
	}
	result.Observation = observation
	return withUpstreamStatusLatency(result, startedAt)
}

func (e *Executor) Execute(ctx context.Context, target registry.CredentialProbeTarget) registry.CredentialProbeExecution {
	startedAt := time.Now()
	result := registry.CredentialProbeExecution{
		Kind: "generation", Status: "unavailable", MayUseTokens: true,
		ModelID: target.Model.ID, ModelName: target.Model.PublicName,
	}
	adapter, err := e.catalog.Build(target.Provider.Kind, providers.AdapterOptions{
		BaseURL: target.Provider.BaseURL, Capabilities: probeCapabilities(target.Model.Capabilities),
	})
	if err != nil {
		result.Status = "failed"
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		return withLatency(result, startedAt)
	}
	probeContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	maxOutputTokens := int64(8)
	request, err := adapter.BuildRequest(probeContext, providers.Credential{APIKey: target.Secret}, canonical.ChatRequest{
		RequestID:       target.RequestID,
		Model:           target.Model.UpstreamName,
		Messages:        []canonical.Message{{Role: canonical.RoleUser, Content: canonical.TextContent("hi")}},
		MaxOutputTokens: &maxOutputTokens,
	})
	if err != nil {
		result.Status = "failed"
		result.ErrorKind = stringPointer(canonicalErrorKind(err))
		return withLatency(result, startedAt)
	}
	client, err := security.NewSSRFSafeClient(e.policy)
	if err != nil {
		result.Status = "failed"
		result.ErrorKind = stringPointer(string(canonical.ErrorProviderConfiguration))
		return withLatency(result, startedAt)
	}
	response, err := client.Do(request)
	if err != nil {
		result.Status, result.ErrorKind, result.Retryable = classifyTransportFailure(err)
		return withLatency(result, startedAt)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, e.maxResponseBytes+1))
	if err != nil {
		result.Status = "uncertain"
		result.ErrorKind = stringPointer(string(canonical.ErrorUncertain))
		return withLatency(result, startedAt)
	}
	if int64(len(body)) > e.maxResponseBytes {
		result.Status = "failed"
		result.ErrorKind = stringPointer("probe_response_too_large")
		return withLatency(result, startedAt)
	}
	parsed, err := adapter.ParseResponse(response.StatusCode, response.Header, body)
	if err != nil {
		result.Status = "failed"
		kind := canonicalErrorKind(err)
		result.ErrorKind = stringPointer(kind)
		result.Retryable = retryableKind(kind)
		return withLatency(result, startedAt)
	}
	result.ResponseText = responsePreview(parsed)
	if parsed.Usage != nil {
		result.InputTokens = parsed.Usage.InputTokens
		result.OutputTokens = parsed.Usage.OutputTokens
	}
	result.Status = "succeeded"
	return withLatency(result, startedAt)
}

func probeCapabilities(model registry.ModelCapabilities) providers.Capabilities {
	capabilities := model.AdapterCapabilities()
	capabilities.Streaming = false
	capabilities.Tools = false
	capabilities.ToolStreaming = false
	capabilities.ParallelToolCalls = false
	capabilities.JSONOutput = false
	capabilities.JSONSchemaOutput = false
	return capabilities
}

func responsePreview(response canonical.ChatResponse) string {
	if len(response.Choices) == 0 {
		return ""
	}
	var text strings.Builder
	for _, part := range response.Choices[0].Message.Content {
		if part.Type == canonical.ContentPartText {
			text.WriteString(part.Text)
		}
	}
	value := []rune(strings.TrimSpace(text.String()))
	if len(value) > 200 {
		value = value[:200]
	}
	return string(value)
}

func retryableKind(kind string) bool {
	return kind == string(canonical.ErrorProviderTemporary) || kind == string(canonical.ErrorRateLimit)
}

func probeOutcomeUnknown(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func classifyTransportFailure(err error) (string, *string, bool) {
	if probeOutcomeUnknown(err) {
		return "uncertain", stringPointer("probe_timeout_or_canceled"), false
	}
	if errors.Is(err, security.ErrURLResolution) {
		return "failed", stringPointer("dns_resolution_failed"), true
	}
	if errors.Is(err, security.ErrUnsafeURL) {
		return "failed", stringPointer("outbound_address_blocked"), false
	}

	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	var invalidCertificate x509.CertificateInvalidError
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameError) ||
		errors.As(err, &invalidCertificate) || errors.As(err, &recordHeader) {
		return "failed", stringPointer("tls_handshake_failed"), false
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return "failed", stringPointer("upstream_connection_failed"), true
	}
	return "failed", stringPointer("provider_transport_failed"), true
}

func canonicalErrorKind(err error) string {
	var providerError *canonical.Error
	if errors.As(err, &providerError) {
		return string(providerError.Kind)
	}
	return string(canonical.ErrorProviderConfiguration)
}

func withLatency(result registry.CredentialProbeExecution, startedAt time.Time) registry.CredentialProbeExecution {
	result.LatencyMillis = max(time.Since(startedAt).Milliseconds(), 0)
	return result
}

func withDiscoveryLatency(result registry.ModelDiscoveryExecution, startedAt time.Time) registry.ModelDiscoveryExecution {
	result.LatencyMillis = max(time.Since(startedAt).Milliseconds(), 0)
	return result
}

func withUpstreamStatusLatency(result registry.CredentialUpstreamStatusExecution, startedAt time.Time) registry.CredentialUpstreamStatusExecution {
	result.LatencyMillis = max(time.Since(startedAt).Milliseconds(), 0)
	return result
}

func upstreamStatusReason(kind string) string {
	switch kind {
	case string(canonical.ErrorAuthentication):
		return "上游拒绝了 API Key"
	case string(canonical.ErrorPermission):
		return "上游 API Key 没有读取该状态的权限"
	case string(canonical.ErrorRateLimit):
		return "上游限流，稍后再获取"
	case string(canonical.ErrorQuota):
		return "上游配额拒绝了状态请求"
	case string(canonical.ErrorProviderTemporary):
		return "上游暂时不可用"
	case "probe_timeout_or_canceled":
		return "获取上游状态超时或被取消"
	case "dns_resolution_failed", "upstream_connection_failed", "provider_transport_failed":
		return "无法连接上游状态端点"
	default:
		return "上游状态暂不可用"
	}
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	return &value
}
