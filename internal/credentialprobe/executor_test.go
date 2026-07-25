package credentialprobe

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/luckymaomi/llmgateway/internal/providers"
	"github.com/luckymaomi/llmgateway/internal/registry"
	"github.com/luckymaomi/llmgateway/internal/security"
)

func TestTransportFailuresProduceActionableProbeResults(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		status    string
		errorKind string
		retryable bool
	}{
		{name: "timeout", err: context.DeadlineExceeded, status: "uncertain", errorKind: "probe_timeout_or_canceled"},
		{name: "dns", err: fmt.Errorf("resolve: %w", security.ErrURLResolution), status: "failed", errorKind: "dns_resolution_failed", retryable: true},
		{name: "outbound policy", err: fmt.Errorf("validate: %w", security.ErrUnsafeURL), status: "failed", errorKind: "outbound_address_blocked"},
		{name: "tls", err: x509.UnknownAuthorityError{}, status: "failed", errorKind: "tls_handshake_failed"},
		{name: "connection", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, status: "failed", errorKind: "upstream_connection_failed", retryable: true},
		{name: "transport", err: errors.New("transport failed"), status: "failed", errorKind: "provider_transport_failed", retryable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, errorKind, retryable := classifyTransportFailure(test.err)
			if status != test.status || errorKind == nil || *errorKind != test.errorKind || retryable != test.retryable {
				t.Fatalf("classifyTransportFailure() = (%q, %v, %t), want (%q, %q, %t)", status, errorKind, retryable, test.status, test.errorKind, test.retryable)
			}
		})
	}
}

func TestDiscoverUsesNonGeneratingModelsEndpointAndReturnsEveryModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.ContentLength > 0 {
			t.Fatalf("models probe unexpectedly sent a body of %d bytes", r.ContentLength)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b"},{"id":"model-a"}]}`))
	}))
	defer server.Close()

	executor, err := New(security.SSRFPolicy{AllowLoopback: true, MaxRedirects: 1}, time.Second, 64*1024)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result := executor.Discover(context.Background(), registry.ModelDiscoveryTarget{
		Provider: registry.Provider{ID: uuid.New(), Kind: providers.KindOpenAICompatible, BaseURL: server.URL + "/v1"},
		Secret:   "fixture-secret",
	})
	if result.Status != "succeeded" || len(result.Models) != 2 || result.Models[0] != "model-a" || result.Models[1] != "model-b" {
		t.Fatalf("Discover() = %#v", result)
	}
}
