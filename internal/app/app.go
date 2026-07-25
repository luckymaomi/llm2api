package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/luckymaomi/llm2api/internal/config"
	"github.com/luckymaomi/llm2api/internal/controlapi"
	"github.com/luckymaomi/llm2api/internal/credentialprobe"
	"github.com/luckymaomi/llm2api/internal/httpserver"
	"github.com/luckymaomi/llm2api/internal/identity"
	"github.com/luckymaomi/llm2api/internal/observability"
	"github.com/luckymaomi/llm2api/internal/operations"
	"github.com/luckymaomi/llm2api/internal/publicapi"
	"github.com/luckymaomi/llm2api/internal/registry"
	"github.com/luckymaomi/llm2api/internal/requestflow"
	responseowner "github.com/luckymaomi/llm2api/internal/responses"
	"github.com/luckymaomi/llm2api/internal/security"
	"github.com/luckymaomi/llm2api/internal/siteprofile"
	"github.com/luckymaomi/llm2api/internal/store"
	"github.com/luckymaomi/llm2api/internal/subscription"
	"github.com/luckymaomi/llm2api/internal/usage"
	webassets "github.com/luckymaomi/llm2api/web"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Application struct {
	config      config.Config
	logger      *slog.Logger
	connections *store.Connections
	workflow    *requestflow.Service
	publicAPI   *publicapi.API
	server      *http.Server
	metrics     *observability.RuntimeMetrics
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Application, error) {
	connections, err := store.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	runtimeMetrics := observability.NewRuntimeMetrics(metricsRegistry, logger)
	identityService, err := identity.NewService(store.NewIdentityRepository(connections.Postgres), cfg.Security.SessionPepper, cfg.Security.APIKeyPepper)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize identity service: %w", err)
	}
	envelope, err := security.NewEnvelopeCipher(cfg.Security.ActiveMasterKeyVersion, cfg.Security.MasterKeys)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize envelope cipher: %w", err)
	}
	urlValidator, err := security.NewURLValidator(security.SSRFPolicy{AllowedPrivatePrefixes: cfg.Security.AllowedPrivatePrefixes, AllowedResolvedPrefixes: cfg.Security.AllowedResolvedPrefixes, AllowLoopback: cfg.Profile == config.ProfileTest, MaxRedirects: 5})
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize outbound URL policy: %w", err)
	}
	registryService, err := registry.NewService(store.NewRegistryRepository(connections), envelope, urlValidator)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize registry service: %w", err)
	}
	rootCAs, err := providerRootCAs(cfg.Security.ProviderCABundleFile)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("load provider CA bundle for credential probes: %w", err)
	}
	probeExecutor, err := credentialprobe.New(security.SSRFPolicy{
		AllowedPrivatePrefixes: cfg.Security.AllowedPrivatePrefixes, AllowedResolvedPrefixes: cfg.Security.AllowedResolvedPrefixes,
		AllowLoopback: cfg.Profile == config.ProfileTest, MaxRedirects: 5, RootCAs: rootCAs,
	}, cfg.ProviderProbe.Timeout, cfg.ProviderProbe.MaxResponseBytes)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize credential probe: %w", err)
	}
	registryService.WithCredentialProbeExecutor(probeExecutor)
	if err := registryService.SyncCatalog(ctx); err != nil {
		connections.Close()
		return nil, fmt.Errorf("synchronize provider catalog: %w", err)
	}
	subscriptionService, err := subscription.NewService(store.NewSubscriptionRepository(connections))
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize subscription service: %w", err)
	}
	loginGuard := store.NewLoginGuard(connections.Valkey, cfg.Security.LoginAccountAttempts, cfg.Security.LoginAddressAttempts, cfg.Security.LoginWindow)
	usageService, err := usage.NewService(store.NewUsageRepository(connections))
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize usage service: %w", err)
	}
	usageAPI := controlapi.NewUsageAPI(usageService, logger)
	siteProfileService, err := siteprofile.NewService(store.NewSiteProfileRepository(connections))
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize site profile service: %w", err)
	}
	controlAPI := controlapi.New(identityService, registryService, subscriptionService, loginGuard, cfg.Security, logger).
		WithUsageAPI(usageAPI).
		WithSiteProfileAPI(controlapi.NewSiteProfileAPI(siteProfileService, logger))
	operationsService, err := operations.NewService(store.NewOperationsRepository(connections))
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize operations service: %w", err)
	}
	controlAPI.WithOperationsAPI(controlapi.NewOperationsAPI(operationsService, logger))
	workflow, err := newRequestWorkflow(cfg, connections, registryService, usageService, runtimeMetrics)
	if err != nil {
		connections.Close()
		return nil, err
	}
	controlAPI.WithGatewayKeyTestWorkflow(workflow)
	responseService, err := responseowner.NewService(store.NewResponseRepository(connections), envelope)
	if err != nil {
		connections.Close()
		return nil, fmt.Errorf("initialize response service: %w", err)
	}
	responseService.WithObserver(runtimeMetrics)
	publicAPI := publicapi.New(identityService, workflow, logger, responseService)

	assets, embedded := webassets.Assets()
	routerAssets := []fs.FS(nil)
	if embedded {
		routerAssets = append(routerAssets, assets)
	}
	server := &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           httpserver.NewRouter(cfg, logger, connections, metricsRegistry, controlAPI.Routes(), publicAPI.Routes(), routerAssets...),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	return &Application{config: cfg, logger: logger, connections: connections, workflow: workflow, publicAPI: publicAPI, server: server, metrics: runtimeMetrics}, nil
}

func (a *Application) Run(ctx context.Context) error {
	go a.runRequestRecovery(ctx)
	go a.publicAPI.RunResponseWorker(ctx, publicapi.ResponseWorkerConfig{
		PollInterval: a.config.Responses.PollInterval, HeartbeatInterval: a.config.Responses.HeartbeatInterval,
		StaleAfter: a.config.Responses.StaleAfter, RecoveryBatchSize: a.config.Responses.RecoveryBatchSize,
		MaxWorkers: a.config.Responses.MaxWorkers,
	})
	errorChannel := make(chan error, 1)
	go func() {
		a.logger.Info("gateway listening", "address", a.server.Addr, "profile", a.config.Profile)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorChannel <- err
			return
		}
		errorChannel <- nil
	}()

	select {
	case err := <-errorChannel:
		_ = a.connections.Close()
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		_ = a.server.Close()
		_ = a.connections.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	select {
	case err := <-errorChannel:
		if closeErr := a.connections.Close(); closeErr != nil {
			return fmt.Errorf("close valkey: %w", closeErr)
		}
		return err
	case <-time.After(time.Second):
		_ = a.connections.Close()
		return errors.New("HTTP server did not stop after shutdown")
	}
}

func (a *Application) runRequestRecovery(ctx context.Context) {
	run := func() {
		staleBefore := time.Now().UTC().Add(-a.config.RequestFlow.ExecutionStaleAfter)
		result, err := a.workflow.RecoverOnce(ctx, staleBefore, a.config.RequestFlow.RecoveryBatchSize)
		if err != nil {
			a.metrics.RequestRecoveryFailed()
			a.logger.Error("request recovery failed", "event", "request.recovery_failed", "error", err)
			return
		}
		a.metrics.RequestRecovery(result)
		if result.Completed > 0 || result.FailedAccepted > 0 || result.Uncertain > 0 {
			a.logger.Info("request recovery completed", "event", "request.recovery_completed", "completed", result.Completed, "failed_accepted", result.FailedAccepted, "uncertain", result.Uncertain)
		}
	}
	run()
	ticker := time.NewTicker(a.config.RequestFlow.RecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
