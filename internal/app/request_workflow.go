package app

import (
	"crypto/x509"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/luckymaomi/llm2api/internal/admission"
	"github.com/luckymaomi/llm2api/internal/config"
	"github.com/luckymaomi/llm2api/internal/coordination"
	"github.com/luckymaomi/llm2api/internal/observability"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/requestflow"
	"github.com/luckymaomi/llm2api/internal/resilience"
	"github.com/luckymaomi/llm2api/internal/routing"
	"github.com/luckymaomi/llm2api/internal/security"
	"github.com/luckymaomi/llm2api/internal/store"
	"github.com/luckymaomi/llm2api/internal/usage"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type runtimeRandom struct{}

func (runtimeRandom) Intn(limit int) int       { return rand.IntN(limit) }
func (runtimeRandom) Int63n(limit int64) int64 { return rand.Int64N(limit) }

func newRequestWorkflow(cfg config.Config, connections *store.Connections, registryService *registry.Service, usageService *usage.Service, runtimeMetrics *observability.RuntimeMetrics) (*requestflow.Service, error) {
	rootCAs, err := providerRootCAs(cfg.Security.ProviderCABundleFile)
	if err != nil {
		return nil, fmt.Errorf("load provider CA bundle: %w", err)
	}
	coordinator, err := coordination.New(connections.Valkey, coordination.Options{
		Prefix: "llm2api", KeyHashSecret: cfg.Security.CoordinationKeyHash,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize request coordinator: %w", err)
	}
	admissionCoordinator, err := coordination.New(connections.Valkey, coordination.Options{
		Prefix: "llm2api-admission", KeyHashSecret: cfg.Security.CoordinationKeyHash,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize admission coordinator: %w", err)
	}
	gate, err := admission.NewGate(admission.Config{
		MaxQueued: cfg.RequestFlow.MaxQueued, MaxActive: cfg.RequestFlow.MaxActive,
		MaxQueueWait: cfg.RequestFlow.MaxQueueWait,
	}, systemClock{})
	if err != nil {
		return nil, fmt.Errorf("initialize request admission gate: %w", err)
	}
	admitter, err := requestflow.NewAdmissionAdapter(gate, admissionCoordinator, requestflow.AdmissionCoordinationConfig{
		MaxActive:    int64(cfg.RequestFlow.MaxActive),
		MaxQueueWait: cfg.RequestFlow.MaxQueueWait, RetryInterval: cfg.RequestFlow.AdmissionRetryInterval,
		LeaseTTL: cfg.RequestFlow.LeaseTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize request admission adapter: %w", err)
	}
	coordinationAdapter, err := requestflow.NewCoordinationAdapter(coordinator, requestflow.CoordinationConfig{
		Global: capacity(cfg.RequestFlow.Global), ResourcePool: capacity(cfg.RequestFlow.ResourcePool), Model: capacity(cfg.RequestFlow.Model),
		Provider: capacity(cfg.RequestFlow.Provider), DefaultCredential: capacity(cfg.RequestFlow.Credential),
		LeaseTTL: cfg.RequestFlow.LeaseTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize request coordination adapter: %w", err)
	}
	accounting, err := requestflow.NewUsageAdapter(usageService)
	if err != nil {
		return nil, fmt.Errorf("initialize request accounting: %w", err)
	}
	random := runtimeRandom{}
	clock := systemClock{}
	router, err := routing.NewRouter(random)
	if err != nil {
		return nil, fmt.Errorf("initialize request router: %w", err)
	}
	retry, err := resilience.NewRetryPolicy(resilience.RetryConfig{
		MaxAttempts: cfg.RequestFlow.RetryMaxAttempts, MaxElapsed: cfg.RequestFlow.RetryMaxElapsed,
		Backoff: resilience.BackoffConfig{
			Initial: cfg.RequestFlow.RetryInitialBackoff, Maximum: cfg.RequestFlow.RetryMaximumBackoff,
			MultiplierNumerator: 2, MultiplierDenominator: 1, JitterPermille: 200,
		},
	}, clock, random)
	if err != nil {
		return nil, fmt.Errorf("initialize request retry policy: %w", err)
	}
	factory := requestflow.NewProviderFactory(security.SSRFPolicy{
		AllowedPrivatePrefixes: cfg.Security.AllowedPrivatePrefixes, AllowedResolvedPrefixes: cfg.Security.AllowedResolvedPrefixes,
		AllowLoopback: cfg.Profile == config.ProfileTest, MaxRedirects: 5, RootCAs: rootCAs,
	})
	workflow, err := requestflow.New(
		store.NewRequestRepository(connections), runtimeMetrics.ObserveAccounting(accounting), registryService,
		runtimeMetrics.ObserveAdmitter(admitter), runtimeMetrics.ObserveCoordinator(coordinationAdapter), factory, router, retry, clock,
		requestflow.Config{
			MaxResponseBytes:           cfg.RequestFlow.MaxResponseBytes,
			ExecutionHeartbeatInterval: cfg.RequestFlow.ExecutionHeartbeatInterval,
			Circuit: resilience.CircuitConfig{
				FailureThreshold: cfg.RequestFlow.CircuitFailureThreshold, SuccessThreshold: cfg.RequestFlow.CircuitSuccessThreshold,
				OpenDuration: cfg.RequestFlow.CircuitOpenDuration, HalfOpenMaxInFlight: cfg.RequestFlow.CircuitHalfOpenMaxInFlight,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("initialize request workflow: %w", err)
	}
	return workflow.WithObserver(runtimeMetrics), nil
}

func providerRootCAs(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("CA bundle contains no certificates")
	}
	return pool, nil
}

func capacity(value config.Capacity) requestflow.Capacity {
	return requestflow.Capacity{
		RequestsPerMinute: value.RequestsPerMinute,
		TokensPerMinute:   value.TokensPerMinute,
		Concurrency:       value.Concurrency,
	}
}
