package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Profile string

const (
	ProfileDevelopment Profile = "development"
	ProfileTest        Profile = "test"
	ProfileProduction  Profile = "production"
)

type Config struct {
	Profile       Profile
	HTTP          HTTP
	Database      Database
	Valkey        Valkey
	Security      Security
	ProviderProbe ProviderProbe
	RequestFlow   RequestFlow
	Responses     Responses
	Logging       Logging
}

type Responses struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	RecoveryBatchSize int32
	MaxWorkers        int
}

type ProviderProbe struct {
	Timeout          time.Duration
	MaxResponseBytes int64
}

type HTTP struct {
	Address           string
	PublicOrigin      string
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
}

type Database struct {
	URL            string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
	MigrateOnStart bool
}

type Valkey struct {
	Address        string
	Password       string
	Database       int
	ConnectTimeout time.Duration
}

type Security struct {
	MasterKeys                  map[uint32][]byte
	ActiveMasterKeyVersion      uint32
	SessionPepper               []byte
	APIKeyPepper                []byte
	CredentialFingerprintPepper []byte
	CoordinationKeyHash         []byte
	ProviderCABundleFile        string
	CookieSecure                bool
	TrustedProxy                string
	LoginAccountAttempts        int
	LoginAddressAttempts        int
	LoginWindow                 time.Duration
	AllowedPrivatePrefixes      []netip.Prefix
	AllowedResolvedPrefixes     []netip.Prefix
}

type Capacity struct {
	RequestsPerMinute int64
	TokensPerMinute   int64
	Concurrency       int64
}

type RequestFlow struct {
	MaxResponseBytes           int64
	ExecutionHeartbeatInterval time.Duration
	ExecutionStaleAfter        time.Duration
	RecoveryInterval           time.Duration
	RecoveryBatchSize          int32
	MaxQueued                  int
	MaxActive                  int
	MaxQueueWait               time.Duration
	AdmissionRetryInterval     time.Duration
	LeaseTTL                   time.Duration
	RetryMaxAttempts           int
	RetryMaxElapsed            time.Duration
	RetryInitialBackoff        time.Duration
	RetryMaximumBackoff        time.Duration
	CircuitFailureThreshold    int
	CircuitSuccessThreshold    int
	CircuitOpenDuration        time.Duration
	CircuitHalfOpenMaxInFlight int
	Global                     Capacity
	ResourcePool               Capacity
	Model                      Capacity
	Provider                   Capacity
	Credential                 Capacity
}

type Logging struct {
	Level string
}

func Load() (Config, error) {
	profile := Profile(env("LLM2API_PROFILE", string(ProfileDevelopment)))
	httpAddress := env("LLM2API_HTTP_ADDRESS", "127.0.0.1:8080")
	databaseURL, err := secretEnv("LLM2API_DATABASE_URL", developmentSecret(profile, "postgres://llm2api:llm2api_dev@127.0.0.1:15432/llm2api?sslmode=disable"))
	if err != nil {
		return Config{}, err
	}
	valkeyPassword, err := secretEnv("LLM2API_VALKEY_PASSWORD", developmentSecret(profile, "llm2api_dev"))
	if err != nil {
		return Config{}, err
	}
	masterKeyValue, err := secretEnv("LLM2API_MASTER_KEYS", developmentSecret(profile, "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="))
	if err != nil {
		return Config{}, err
	}
	sessionPepper, err := secretEnv("LLM2API_SESSION_PEPPER", developmentSecret(profile, "llm2api-development-session-pepper"))
	if err != nil {
		return Config{}, err
	}
	apiKeyPepper, err := secretEnv("LLM2API_API_KEY_PEPPER", developmentSecret(profile, "llm2api-development-api-key-pepper"))
	if err != nil {
		return Config{}, err
	}
	credentialFingerprintPepper, err := secretEnv("LLM2API_CREDENTIAL_FINGERPRINT_PEPPER", developmentSecret(profile, "llm2api-development-credential-fingerprint-pepper"))
	if err != nil {
		return Config{}, err
	}
	coordinationKeyHash, err := secretEnv("LLM2API_COORDINATION_KEY_HASH_SECRET", developmentSecret(profile, "llm2api-development-coordination-key-hash-secret"))
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Profile: profile,
		HTTP: HTTP{
			Address:           httpAddress,
			PublicOrigin:      env("LLM2API_PUBLIC_ORIGIN", developmentPublicOrigin(profile, httpAddress)),
			ReadHeaderTimeout: durationEnv("LLM2API_HTTP_READ_HEADER_TIMEOUT", 10*time.Second),
			IdleTimeout:       durationEnv("LLM2API_HTTP_IDLE_TIMEOUT", 90*time.Second),
			ShutdownTimeout:   durationEnv("LLM2API_HTTP_SHUTDOWN_TIMEOUT", 30*time.Second),
			MaxBodyBytes:      int64Env("LLM2API_HTTP_MAX_BODY_BYTES", 4<<20),
		},
		Database: Database{
			URL:            databaseURL,
			MaxConnections: int32(intEnv("LLM2API_DATABASE_MAX_CONNECTIONS", 20)),
			MinConnections: int32(intEnv("LLM2API_DATABASE_MIN_CONNECTIONS", 2)),
			ConnectTimeout: durationEnv("LLM2API_DATABASE_CONNECT_TIMEOUT", 10*time.Second),
			MigrateOnStart: boolEnv("LLM2API_DATABASE_MIGRATE_ON_START", profile != ProfileProduction),
		},
		Valkey: Valkey{
			Address:        env("LLM2API_VALKEY_ADDRESS", "127.0.0.1:16380"),
			Password:       valkeyPassword,
			Database:       intEnv("LLM2API_VALKEY_DATABASE", 0),
			ConnectTimeout: durationEnv("LLM2API_VALKEY_CONNECT_TIMEOUT", 5*time.Second),
		},
		Security: Security{
			MasterKeys:                  masterKeys(masterKeyValue),
			ActiveMasterKeyVersion:      uint32(intEnv("LLM2API_ACTIVE_MASTER_KEY_VERSION", 1)),
			SessionPepper:               []byte(sessionPepper),
			APIKeyPepper:                []byte(apiKeyPepper),
			CredentialFingerprintPepper: []byte(credentialFingerprintPepper),
			CoordinationKeyHash:         []byte(coordinationKeyHash),
			ProviderCABundleFile:        strings.TrimSpace(os.Getenv("LLM2API_PROVIDER_CA_BUNDLE_FILE")),
			CookieSecure:                boolEnv("LLM2API_COOKIE_SECURE", profile == ProfileProduction),
			TrustedProxy:                strings.TrimSpace(os.Getenv("LLM2API_TRUSTED_PROXY")),
			LoginAccountAttempts:        intEnv("LLM2API_LOGIN_ACCOUNT_ATTEMPTS", 5),
			LoginAddressAttempts:        intEnv("LLM2API_LOGIN_ADDRESS_ATTEMPTS", 30),
			LoginWindow:                 durationEnv("LLM2API_LOGIN_WINDOW", 10*time.Minute),
			AllowedPrivatePrefixes:      prefixListEnv("LLM2API_ALLOWED_PRIVATE_NETWORKS"),
			AllowedResolvedPrefixes:     prefixListEnv("LLM2API_ALLOWED_RESOLVED_NETWORKS"),
		},
		ProviderProbe: ProviderProbe{
			Timeout:          durationEnv("LLM2API_PROVIDER_PROBE_TIMEOUT", 15*time.Second),
			MaxResponseBytes: int64Env("LLM2API_PROVIDER_PROBE_MAX_RESPONSE_BYTES", 1<<20),
		},
		RequestFlow: RequestFlow{
			MaxResponseBytes:           int64Env("LLM2API_REQUEST_MAX_RESPONSE_BYTES", 16<<20),
			ExecutionHeartbeatInterval: durationEnv("LLM2API_REQUEST_EXECUTION_HEARTBEAT_INTERVAL", 10*time.Second),
			ExecutionStaleAfter:        durationEnv("LLM2API_REQUEST_EXECUTION_STALE_AFTER", time.Minute),
			RecoveryInterval:           durationEnv("LLM2API_REQUEST_RECOVERY_INTERVAL", 15*time.Second),
			RecoveryBatchSize:          int32(intEnv("LLM2API_REQUEST_RECOVERY_BATCH_SIZE", 100)),
			MaxQueued:                  intEnv("LLM2API_REQUEST_MAX_QUEUED", 1024),
			MaxActive:                  intEnv("LLM2API_REQUEST_MAX_ACTIVE", 256),
			MaxQueueWait:               durationEnv("LLM2API_REQUEST_MAX_QUEUE_WAIT", 30*time.Second),
			AdmissionRetryInterval:     durationEnv("LLM2API_REQUEST_ADMISSION_RETRY_INTERVAL", 100*time.Millisecond),
			LeaseTTL:                   durationEnv("LLM2API_REQUEST_LEASE_TTL", 30*time.Second),
			RetryMaxAttempts:           intEnv("LLM2API_REQUEST_RETRY_MAX_ATTEMPTS", 2),
			RetryMaxElapsed:            durationEnv("LLM2API_REQUEST_RETRY_MAX_ELAPSED", 30*time.Second),
			RetryInitialBackoff:        durationEnv("LLM2API_REQUEST_RETRY_INITIAL_BACKOFF", 100*time.Millisecond),
			RetryMaximumBackoff:        durationEnv("LLM2API_REQUEST_RETRY_MAXIMUM_BACKOFF", 2*time.Second),
			CircuitFailureThreshold:    intEnv("LLM2API_REQUEST_CIRCUIT_FAILURE_THRESHOLD", 3),
			CircuitSuccessThreshold:    intEnv("LLM2API_REQUEST_CIRCUIT_SUCCESS_THRESHOLD", 1),
			CircuitOpenDuration:        durationEnv("LLM2API_REQUEST_CIRCUIT_OPEN_DURATION", 30*time.Second),
			CircuitHalfOpenMaxInFlight: intEnv("LLM2API_REQUEST_CIRCUIT_HALF_OPEN_MAX_IN_FLIGHT", 1),
			Global:                     capacityEnv("GLOBAL", Capacity{RequestsPerMinute: 12_000, TokensPerMinute: 6_000_000, Concurrency: 256}),
			ResourcePool:               capacityEnv("RESOURCE_POOL", Capacity{RequestsPerMinute: 9_000, TokensPerMinute: 3_000_000, Concurrency: 128}),
			Model:                      capacityEnv("MODEL", Capacity{RequestsPerMinute: 9_000, TokensPerMinute: 3_000_000, Concurrency: 128}),
			Provider:                   capacityEnv("PROVIDER", Capacity{RequestsPerMinute: 9_000, TokensPerMinute: 3_000_000, Concurrency: 128}),
			Credential:                 capacityEnv("CREDENTIAL", Capacity{RequestsPerMinute: 60, TokensPerMinute: 100_000, Concurrency: 4}),
		},
		Responses: Responses{
			PollInterval:      durationEnv("LLM2API_RESPONSES_POLL_INTERVAL", 500*time.Millisecond),
			HeartbeatInterval: durationEnv("LLM2API_RESPONSES_HEARTBEAT_INTERVAL", 5*time.Second),
			StaleAfter:        durationEnv("LLM2API_RESPONSES_STALE_AFTER", 30*time.Second),
			RecoveryBatchSize: int32(intEnv("LLM2API_RESPONSES_RECOVERY_BATCH_SIZE", 100)),
			MaxWorkers:        intEnv("LLM2API_RESPONSES_MAX_WORKERS", 8),
		},
		Logging: Logging{Level: env("LLM2API_LOG_LEVEL", "info")},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []error
	if c.Profile != ProfileDevelopment && c.Profile != ProfileTest && c.Profile != ProfileProduction {
		problems = append(problems, fmt.Errorf("LLM2API_PROFILE must be development, test, or production"))
	}
	if _, _, err := net.SplitHostPort(c.HTTP.Address); err != nil {
		problems = append(problems, fmt.Errorf("LLM2API_HTTP_ADDRESS: %w", err))
	}
	if err := validatePublicOrigin(c.HTTP.PublicOrigin); err != nil {
		problems = append(problems, fmt.Errorf("LLM2API_PUBLIC_ORIGIN: %w", err))
	}
	if c.Database.URL == "" {
		problems = append(problems, errors.New("LLM2API_DATABASE_URL is required"))
	}
	if c.Database.MaxConnections < 1 || c.Database.MinConnections < 0 || c.Database.MinConnections > c.Database.MaxConnections {
		problems = append(problems, errors.New("database connection bounds are invalid"))
	}
	if _, _, err := net.SplitHostPort(c.Valkey.Address); err != nil {
		problems = append(problems, fmt.Errorf("LLM2API_VALKEY_ADDRESS: %w", err))
	}
	if len(c.Security.MasterKeys) == 0 {
		problems = append(problems, errors.New("LLM2API_MASTER_KEYS must contain at least one versioned key"))
	}
	for version, key := range c.Security.MasterKeys {
		if version == 0 || len(key) != 32 {
			problems = append(problems, fmt.Errorf("master key version %d must decode to exactly 32 bytes", version))
		}
	}
	if _, ok := c.Security.MasterKeys[c.Security.ActiveMasterKeyVersion]; !ok {
		problems = append(problems, errors.New("LLM2API_ACTIVE_MASTER_KEY_VERSION must select a configured key"))
	}
	if len(c.Security.SessionPepper) < 32 {
		problems = append(problems, errors.New("LLM2API_SESSION_PEPPER must contain at least 32 bytes"))
	}
	if len(c.Security.APIKeyPepper) < 32 {
		problems = append(problems, errors.New("LLM2API_API_KEY_PEPPER must contain at least 32 bytes"))
	}
	if len(c.Security.CredentialFingerprintPepper) < 32 {
		problems = append(problems, errors.New("LLM2API_CREDENTIAL_FINGERPRINT_PEPPER must contain at least 32 bytes"))
	}
	if len(c.Security.CoordinationKeyHash) < 32 {
		problems = append(problems, errors.New("LLM2API_COORDINATION_KEY_HASH_SECRET must contain at least 32 bytes"))
	}
	if c.Security.LoginAccountAttempts < 1 || c.Security.LoginAddressAttempts < c.Security.LoginAccountAttempts || c.Security.LoginWindow < time.Minute {
		problems = append(problems, errors.New("login rate limit settings are invalid"))
	}
	if c.Security.AllowedPrivatePrefixes == nil {
		problems = append(problems, errors.New("LLM2API_ALLOWED_PRIVATE_NETWORKS contains an invalid CIDR"))
	}
	if c.Security.AllowedResolvedPrefixes == nil {
		problems = append(problems, errors.New("LLM2API_ALLOWED_RESOLVED_NETWORKS contains an invalid CIDR"))
	}
	if c.ProviderProbe.Timeout <= 0 || c.ProviderProbe.Timeout > 5*time.Minute || c.ProviderProbe.MaxResponseBytes < 1024 || c.ProviderProbe.MaxResponseBytes > 16<<20 {
		problems = append(problems, errors.New("provider probe bounds are invalid"))
	}
	if c.Profile == ProfileProduction {
		if strings.HasPrefix(c.HTTP.Address, "127.0.0.1:") && c.Security.TrustedProxy != "" {
			problems = append(problems, errors.New("trusted proxy cannot be enabled with a loopback-only listener"))
		}
		if !c.Security.CookieSecure {
			problems = append(problems, errors.New("secure cookies are required in production"))
		}
		if c.Valkey.Password == "" {
			problems = append(problems, errors.New("LLM2API_VALKEY_PASSWORD is required in production"))
		}
	}
	if c.RequestFlow.MaxResponseBytes < 1024 || c.RequestFlow.ExecutionHeartbeatInterval <= 0 ||
		c.RequestFlow.ExecutionStaleAfter <= 2*c.RequestFlow.ExecutionHeartbeatInterval ||
		c.RequestFlow.RecoveryInterval <= 0 || c.RequestFlow.RecoveryInterval > c.RequestFlow.ExecutionStaleAfter ||
		c.RequestFlow.RecoveryBatchSize < 1 || c.RequestFlow.RecoveryBatchSize > 1000 ||
		c.RequestFlow.MaxQueued < 1 || c.RequestFlow.MaxActive < 1 ||
		c.RequestFlow.MaxQueueWait <= 0 ||
		c.RequestFlow.AdmissionRetryInterval < 10*time.Millisecond || c.RequestFlow.AdmissionRetryInterval > time.Second ||
		c.RequestFlow.LeaseTTL < 3*time.Second || c.RequestFlow.LeaseTTL > time.Hour ||
		c.RequestFlow.RetryMaxAttempts < 1 || c.RequestFlow.RetryMaxAttempts > 1000 || c.RequestFlow.RetryMaxElapsed <= 0 ||
		c.RequestFlow.RetryInitialBackoff <= 0 || c.RequestFlow.RetryMaximumBackoff < c.RequestFlow.RetryInitialBackoff ||
		c.RequestFlow.CircuitFailureThreshold < 1 || c.RequestFlow.CircuitSuccessThreshold < 1 ||
		c.RequestFlow.CircuitOpenDuration <= 0 || c.RequestFlow.CircuitHalfOpenMaxInFlight < 1 {
		problems = append(problems, errors.New("request workflow timing and resilience settings are invalid"))
	}
	for _, capacity := range []Capacity{c.RequestFlow.Global, c.RequestFlow.ResourcePool, c.RequestFlow.Model, c.RequestFlow.Provider, c.RequestFlow.Credential} {
		if capacity.RequestsPerMinute < 1 || capacity.TokensPerMinute < 1 || capacity.Concurrency < 1 {
			problems = append(problems, errors.New("request workflow capacities must be positive"))
			break
		}
	}
	if c.Responses.PollInterval <= 0 || c.Responses.PollInterval > c.Responses.StaleAfter ||
		c.Responses.HeartbeatInterval <= 0 || c.Responses.StaleAfter <= 2*c.Responses.HeartbeatInterval ||
		c.Responses.RecoveryBatchSize < 1 || c.Responses.RecoveryBatchSize > 1000 ||
		c.Responses.MaxWorkers < 1 || c.Responses.MaxWorkers > 1000 {
		problems = append(problems, errors.New("background response execution settings are invalid"))
	}
	return errors.Join(problems...)
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(c.Logging.Level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func developmentSecret(profile Profile, value string) string {
	if profile == ProfileProduction {
		return ""
	}
	return value
}

func developmentPublicOrigin(profile Profile, address string) string {
	if profile == ProfileProduction {
		return ""
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || host == "0.0.0.0" || host == "::" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

func validatePublicOrigin(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("must be an absolute HTTP or HTTPS origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("must contain only an origin without credentials, path, query, or fragment")
	}
	if parsed.Hostname() == "" {
		return errors.New("must contain a host")
	}
	return nil
}

func secretEnv(key, fallback string) (string, error) {
	value, valueSet := os.LookupEnv(key)
	filePath, fileSet := os.LookupEnv(key + "_FILE")
	if valueSet && fileSet {
		return "", fmt.Errorf("%s and %s_FILE cannot both be set", key, key)
	}
	if valueSet {
		return strings.TrimSpace(value), nil
	}
	if !fileSet {
		return fallback, nil
	}
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", fmt.Errorf("%s_FILE must name a secret file", key)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("%s_FILE cannot be inspected: %w", key, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return "", fmt.Errorf("%s_FILE must be a regular file no larger than 64 KiB", key)
	}
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("%s_FILE cannot be read: %w", key, err)
	}
	result := strings.TrimSpace(string(contents))
	if result == "" || strings.ContainsRune(result, '\x00') {
		return "", fmt.Errorf("%s_FILE must contain a non-empty text secret", key)
	}
	return result, nil
}

func masterKeys(value string) map[uint32][]byte {
	keys := make(map[uint32][]byte)
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), ":", 2)
		if len(parts) != 2 {
			return nil
		}
		version, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || version == 0 {
			return nil
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
		keys[uint32(version)] = decoded
	}
	return keys
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return -1
	}
	return parsed
}

func intEnv(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return -1
	}
	return parsed
}

func int64Env(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return -1
	}
	return parsed
}

func capacityEnv(scope string, fallback Capacity) Capacity {
	prefix := "LLM2API_REQUEST_" + scope + "_"
	return Capacity{
		RequestsPerMinute: int64Env(prefix+"RPM", fallback.RequestsPerMinute),
		TokensPerMinute:   int64Env(prefix+"TPM", fallback.TokensPerMinute),
		Concurrency:       int64Env(prefix+"CONCURRENCY", fallback.Concurrency),
	}
}

func boolEnv(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed
}

func prefixListEnv(key string) []netip.Prefix {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return []netip.Prefix{}
	}
	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes
}
