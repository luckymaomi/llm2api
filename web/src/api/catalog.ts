import { apiClient } from './client'
import type {
  Credential,
  CredentialCapacity,
  CredentialBatchInput,
  CredentialBatchResult,
  CredentialModelProbeBatchResult,
  ModelDiscoveryResult,
  CredentialProbeResult,
  CredentialHealthStatus,
  CredentialStatus,
  CredentialUpdateInput,
  Model,
  ProviderModelProfile,
  Provider,
  ResourcePool,
  ResourcePoolInput,
  ResourcePoolStatus,
  UpstreamStatusObservation,
} from './types'

const base = '/api/control'
const mutationHeaders = (idempotencyKey: string) => ({ 'Idempotency-Key': idempotencyKey })

export const catalogApi = {
  providers: (signal?: AbortSignal) =>
    apiClient
      .request<ProviderWire[]>(`${base}/providers`, { ...(signal ? { signal } : {}) })
      .then((items) => items.map(mapProvider)),
  models: (signal?: AbortSignal) =>
    apiClient
      .request<ModelWire[]>(`${base}/models`, { ...(signal ? { signal } : {}) })
      .then((items) => items.map(mapModel)),
  resourcePools: (includeRetired = false, signal?: AbortSignal) =>
    apiClient
      .request<ResourcePoolWire[]>(`${base}/resource-pools`, {
        query: { includeRetired },
        ...(signal ? { signal } : {}),
      })
      .then((items) => items.map(mapResourcePool)),
  createResourcePool: (input: ResourcePoolInput, idempotencyKey: string) =>
    apiClient
      .request<ResourcePoolWire>(`${base}/resource-pools`, {
        method: 'POST',
        body: input,
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapResourcePool),
  updateResourcePool: (
    id: string,
    input: { name: string; expectedUpdatedAt: string },
    idempotencyKey: string,
  ) =>
    apiClient
      .request<ResourcePoolWire>(`${base}/resource-pools/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: input,
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapResourcePool),
  setResourcePoolStatus: (
    id: string,
    status: ResourcePoolStatus,
    expectedUpdatedAt: string,
    idempotencyKey: string,
  ) =>
    apiClient
      .request<ResourcePoolWire>(`${base}/resource-pools/${encodeURIComponent(id)}/status`, {
        method: 'PUT',
        body: { status, expectedUpdatedAt },
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapResourcePool),
  credentials: (includeRetired = false, signal?: AbortSignal) =>
    apiClient
      .request<CredentialWire[]>(`${base}/credentials`, {
        query: { includeRetired },
        ...(signal ? { signal } : {}),
      })
      .then((items) => items.map(mapCredential)),
  updateCredential: (id: string, input: CredentialUpdateInput, idempotencyKey: string) =>
    apiClient
      .request<CredentialWire>(`${base}/credentials/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: credentialBody(input),
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapCredential),
  importCredentials: (input: CredentialBatchInput, idempotencyKey: string) =>
    apiClient
      .request<CredentialBatchResultWire[]>(`${base}/credentials/batch`, {
        method: 'POST',
        body: input,
        headers: mutationHeaders(idempotencyKey),
      })
      .then((items) => items.map(mapBatchResult)),
  probeAllCredentials: (idempotencyKey: string) =>
    apiClient
      .request<CredentialModelProbeBatchWire>(`${base}/credentials/probe`, {
        method: 'POST',
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapCredentialModelProbeBatch),
  setCredentialStatus: (
    id: string,
    status: CredentialStatus,
    expectedUpdatedAt: string,
    idempotencyKey: string,
  ) =>
    apiClient
      .request<CredentialWire>(`${base}/credentials/${encodeURIComponent(id)}/status`, {
        method: 'PUT',
        body: { status, expectedUpdatedAt },
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapCredential),
  retireCredential: (id: string, expectedUpdatedAt: string, idempotencyKey: string) =>
    apiClient
      .request<CredentialWire>(`${base}/credentials/${encodeURIComponent(id)}`, {
        method: 'DELETE',
        query: { expectedUpdatedAt },
        headers: mutationHeaders(idempotencyKey),
      })
      .then(mapCredential),
  probeCredential: (
    id: string,
    expectedUpdatedAt: string,
    idempotencyKey: string,
    signal?: AbortSignal,
  ) =>
    apiClient
      .request<ModelDiscoveryWire>(`${base}/credentials/${encodeURIComponent(id)}/probe`, {
        method: 'POST',
        body: { expectedUpdatedAt },
        headers: mutationHeaders(idempotencyKey),
        ...(signal ? { signal } : {}),
      })
      .then(mapDiscoveryResult),
  deepTestCredential: (id: string, modelId: string, signal?: AbortSignal) =>
    apiClient
      .request<CredentialProbeWire>(`${base}/credentials/${encodeURIComponent(id)}/deep-test`, {
        method: 'POST',
        body: { modelId },
        ...(signal ? { signal } : {}),
      })
      .then(mapProbeResult),
  fetchCredentialUpstreamStatus: (id: string, signal?: AbortSignal) =>
    apiClient
      .request<CredentialUpstreamStatusWire>(
        `${base}/credentials/${encodeURIComponent(id)}/upstream-status`,
        { method: 'POST', ...(signal ? { signal } : {}) },
      )
      .then((result) => ({
        observation: mapUpstreamStatus(result.observation),
        credential: mapCredential(result.credential),
      })),
}

interface ModelWire {
  id: string
  provider_id: string
  provider_slug?: string
  provider_name?: string
  public_name: string
  upstream_name: string
  display_name: string
  capabilities: {
    chat: boolean
    streaming: boolean
    tools: boolean
    tool_choice_modes?: string[]
    strict_tools?: boolean
    parallel_tool_calls?: boolean
    tool_streaming?: boolean
    image_input?: boolean
    video_input?: boolean
    partial_mode?: boolean
    reasoning: boolean
    reasoning_mode?: 'toggle' | 'effort' | 'hybrid' | 'always_on'
    reasoning_always_on?: boolean
    reasoning_default_enabled?: boolean
    reasoning_config?: boolean
    reasoning_content?: boolean
    reasoning_preserve?: boolean
    reasoning_efforts?: string[]
    tool_choice_modes_with_reasoning?: string[]
    structured_output: boolean
    json_schema_output?: boolean
    message_name?: boolean
    prompt_cache_key?: boolean
    safety_identifier?: boolean
    response_usage?: boolean
    stream_usage?: boolean
    context_tokens: number
    output_tokens: number
    parameters: {
      max_completion_tokens: ParameterIntegerWire
      temperature: ParameterNumberWire
      top_p: ParameterNumberWire
      presence_penalty: ParameterNumberWire
      frequency_penalty: ParameterNumberWire
      n: ParameterIntegerWire
      top_k: ParameterNumberWire
      thinking_budget: ParameterIntegerWire
      sampling_conditions?: SamplingConditionWire[]
    }
  }
  created_at: string
  updated_at: string
}

interface ParameterIntegerWire {
  supported: boolean
  minimum?: number
  maximum?: number
  exact_values?: number[]
}

interface ParameterNumberWire {
  supported: boolean
  minimum?: number
  maximum?: number
  exact_values?: number[]
}

interface SamplingConditionWire {
  thinking_enabled?: boolean
  temperature_exact?: number
  temperature_at_most?: number
  n_maximum?: number
}

interface ProviderWire {
  id: string
  catalog_id: string
  slug: string
  name: string
  kind: string
  base_url: string
  source_url: string
  verified_at: string
  contract: ProviderContractWire
  models: Array<{
    upstream_name: string
    display_name: string
    capabilities: ModelWire['capabilities']
  }>
  resource_pool_count: number
  active_credential_count: number
  created_at: string
  updated_at: string
}

interface ProviderContractWire {
  reference_url: string
  contract_snapshot: string
  verified_at: string
  reference_provider?: string
  verified_models: string[]
  live_capabilities: string[]
  status: 'verified' | 'degraded'
}

interface ResourcePoolWire {
  id: string
  provider_id: string
  provider_catalog_id: string
  provider_slug: string
  provider_name: string
  provider_kind: string
  provider_base_url: string
  slug: string
  name: string
  status: ResourcePoolStatus
  models: ModelWire[]
  model_count: number
  credential_count: number
  active_credential_count: number
  retired_at?: string
  created_at: string
  updated_at: string
}

interface CredentialCapacityWire {
  state: 'observed' | 'unavailable'
  scope: 'gateway_credential' | 'gateway_shared_upstream'
  observed_at?: string
  requests_per_minute_limit?: number
  requests_per_minute_remaining?: number
  tokens_per_minute_limit?: number
  tokens_per_minute_remaining?: number
  daily_token_limit?: number
  daily_token_remaining?: number
  daily_token_reset_at?: string
  concurrency_limit?: number
  concurrency_in_use?: number
}

interface CredentialWire {
  id: string
  resource_pool_id: string
  resource_pool_name: string
  resource_pool_slug: string
  provider_id: string
  provider_name: string
  provider_kind: string
  provider_base_url: string
  name: string
  status: CredentialStatus
  health_status: CredentialHealthStatus
  upstream_status?: UpstreamStatusWire
  capacity: CredentialCapacityWire
  shared_capacity?: CredentialCapacityWire
  rpm_limit?: number
  tpm_limit?: number
  concurrency_limit?: number
  priority: number
  weight: number
  shared_capacity_scope?: string
  shared_rpm_limit?: number
  shared_tpm_limit?: number
  shared_concurrency_limit?: number
  shared_daily_token_limit?: number
  shared_daily_reset_minute_utc?: number
  cooldown_until?: string
  consecutive_failures: number
  last_success_at?: string
  last_error_kind?: string
  last_probe_at?: string
  last_probe_latency_ms?: number
  last_probe_kind?: string
  last_probe_status?: string
  last_probe_error_kind?: string
  last_checked_at?: string
  recent_success_rate?: number
  first_byte_p95_ms?: number
  total_latency_p95_ms?: number
  retired_at?: string
  created_at: string
  updated_at: string
  model_bindings: Array<{
    model_id: string
    model_name?: string
  }>
}

interface UpstreamStatusWire {
  state: 'observed' | 'unknown' | 'unavailable'
  scope: 'account' | 'project' | 'credential' | 'unknown'
  observed_at: string
  source: string
  reason?: string
  balance?: {
    currency: string
    available: string
    voucher: string
    cash: string
  }
}

interface CredentialUpstreamStatusWire {
  observation: UpstreamStatusWire
  credential: CredentialWire
}

interface CredentialBatchResultWire {
  line: number
  name: string
  status: 'created' | 'duplicate' | 'rejected'
  credential?: CredentialWire
  error_kind?: string
}

interface CredentialProbeWire {
  credential: CredentialWire
  execution: {
    kind: string
    status: CredentialProbeResult['status']
    error_kind?: string
    retryable: boolean
    may_use_tokens: boolean
    latency_ms: number
    model_id: string
    model_name: string
    input_tokens?: number
    output_tokens?: number
    request_id: string
  }
}

interface ModelDiscoveryWire {
  credential: CredentialWire
  execution: {
    status: ModelDiscoveryResult['status']
    error_kind?: string
    retryable: boolean
    latency_ms: number
    models: string[]
  }
}

interface CredentialModelProbeBatchWire {
  results: ModelDiscoveryWire[]
  succeeded: number
  failed: number
  unavailable: number
  uncertain: number
}

function mapModel(model: ModelWire): Model {
  return {
    id: model.id,
    providerId: model.provider_id,
    providerSlug: model.provider_slug ?? '',
    providerName: model.provider_name ?? '',
    publicName: model.public_name,
    upstreamName: model.upstream_name,
    displayName: model.display_name,
    capabilities: mapModelCapabilities(model.capabilities),
    createdAt: model.created_at,
    updatedAt: model.updated_at,
  }
}

function mapProvider(provider: ProviderWire): Provider {
  return {
    id: provider.id,
    catalogId: provider.catalog_id,
    slug: provider.slug,
    name: provider.name,
    kind: provider.kind,
    baseUrl: provider.base_url,
    sourceUrl: provider.source_url,
    verifiedAt: provider.verified_at,
    contract: {
      referenceUrl: provider.contract.reference_url,
      contractSnapshot: provider.contract.contract_snapshot,
      verifiedAt: provider.contract.verified_at,
      ...(provider.contract.reference_provider
        ? { referenceProvider: provider.contract.reference_provider }
        : {}),
      verifiedModels: provider.contract.verified_models,
      liveCapabilities: provider.contract.live_capabilities,
      status: provider.contract.status,
    },
    models: provider.models.map(mapProviderModelProfile),
    resourcePoolCount: provider.resource_pool_count,
    activeCredentialCount: provider.active_credential_count,
    createdAt: provider.created_at,
    updatedAt: provider.updated_at,
  }
}

function mapProviderModelProfile(profile: ProviderWire['models'][number]): ProviderModelProfile {
  return {
    upstreamName: profile.upstream_name,
    displayName: profile.display_name,
    capabilities: mapModelCapabilities(profile.capabilities),
  }
}

function mapResourcePool(pool: ResourcePoolWire): ResourcePool {
  return {
    id: pool.id,
    providerId: pool.provider_id,
    providerCatalogId: pool.provider_catalog_id,
    providerSlug: pool.provider_slug,
    providerName: pool.provider_name,
    providerKind: pool.provider_kind,
    providerBaseUrl: pool.provider_base_url,
    slug: pool.slug,
    name: pool.name,
    status: pool.status,
    models: pool.models.map(mapModel),
    modelCount: pool.model_count,
    credentialCount: pool.credential_count,
    activeCredentialCount: pool.active_credential_count,
    ...(pool.retired_at ? { retiredAt: pool.retired_at } : {}),
    createdAt: pool.created_at,
    updatedAt: pool.updated_at,
  }
}

function mapModelCapabilities(capabilities: ModelWire['capabilities']): Model['capabilities'] {
  return {
    chat: capabilities.chat,
    streaming: capabilities.streaming,
    tools: capabilities.tools,
    toolChoiceModes: capabilities.tool_choice_modes ?? [],
    strictTools: capabilities.strict_tools ?? false,
    parallelToolCalls: capabilities.parallel_tool_calls ?? false,
    toolStreaming: capabilities.tool_streaming ?? false,
    imageInput: capabilities.image_input ?? false,
    videoInput: capabilities.video_input ?? false,
    partialMode: capabilities.partial_mode ?? false,
    reasoning: capabilities.reasoning,
    ...(capabilities.reasoning_mode ? { reasoningMode: capabilities.reasoning_mode } : {}),
    reasoningAlwaysOn: capabilities.reasoning_always_on ?? false,
    reasoningDefaultEnabled: capabilities.reasoning_default_enabled ?? false,
    reasoningConfig: capabilities.reasoning_config ?? false,
    reasoningContent: capabilities.reasoning_content ?? false,
    reasoningPreserve: capabilities.reasoning_preserve ?? false,
    reasoningEfforts: capabilities.reasoning_efforts ?? [],
    toolChoiceModesWithReasoning: capabilities.tool_choice_modes_with_reasoning ?? [],
    structuredOutput: capabilities.structured_output,
    jsonSchemaOutput: capabilities.json_schema_output ?? false,
    messageName: capabilities.message_name ?? false,
    promptCacheKey: capabilities.prompt_cache_key ?? false,
    safetyIdentifier: capabilities.safety_identifier ?? false,
    responseUsage: capabilities.response_usage ?? false,
    streamUsage: capabilities.stream_usage ?? false,
    contextTokens: capabilities.context_tokens,
    outputTokens: capabilities.output_tokens,
    parameters: {
      maxCompletionTokens: mapIntegerParameter(capabilities.parameters.max_completion_tokens),
      temperature: mapNumberParameter(capabilities.parameters.temperature),
      topP: mapNumberParameter(capabilities.parameters.top_p),
      presencePenalty: mapNumberParameter(capabilities.parameters.presence_penalty),
      frequencyPenalty: mapNumberParameter(capabilities.parameters.frequency_penalty),
      n: mapIntegerParameter(capabilities.parameters.n),
      topK: mapNumberParameter(capabilities.parameters.top_k),
      thinkingBudget: mapIntegerParameter(capabilities.parameters.thinking_budget),
      samplingConditions: (capabilities.parameters.sampling_conditions ?? []).map((condition) => ({
        ...(condition.thinking_enabled !== undefined
          ? { thinkingEnabled: condition.thinking_enabled }
          : {}),
        ...(condition.temperature_exact !== undefined
          ? { temperatureExact: condition.temperature_exact }
          : {}),
        ...(condition.temperature_at_most !== undefined
          ? { temperatureAtMost: condition.temperature_at_most }
          : {}),
        ...(condition.n_maximum !== undefined ? { nMaximum: condition.n_maximum } : {}),
      })),
    },
  }
}

function mapIntegerParameter(value: ParameterIntegerWire) {
  return {
    supported: value.supported,
    ...(value.minimum !== undefined ? { minimum: value.minimum } : {}),
    ...(value.maximum !== undefined ? { maximum: value.maximum } : {}),
    exactValues: value.exact_values ?? [],
  }
}

function mapNumberParameter(value: ParameterNumberWire) {
  return {
    supported: value.supported,
    ...(value.minimum !== undefined ? { minimum: value.minimum } : {}),
    ...(value.maximum !== undefined ? { maximum: value.maximum } : {}),
    exactValues: value.exact_values ?? [],
  }
}

function mapCredentialCapacity(capacity: CredentialCapacityWire): CredentialCapacity {
  return {
    state: capacity.state,
    scope: capacity.scope,
    ...(capacity.observed_at ? { observedAt: capacity.observed_at } : {}),
    ...(capacity.requests_per_minute_limit !== undefined
      ? { requestsPerMinuteLimit: capacity.requests_per_minute_limit }
      : {}),
    ...(capacity.requests_per_minute_remaining !== undefined
      ? { requestsPerMinuteRemaining: capacity.requests_per_minute_remaining }
      : {}),
    ...(capacity.tokens_per_minute_limit !== undefined
      ? { tokensPerMinuteLimit: capacity.tokens_per_minute_limit }
      : {}),
    ...(capacity.tokens_per_minute_remaining !== undefined
      ? { tokensPerMinuteRemaining: capacity.tokens_per_minute_remaining }
      : {}),
    ...(capacity.daily_token_limit !== undefined
      ? { dailyTokenLimit: capacity.daily_token_limit }
      : {}),
    ...(capacity.daily_token_remaining !== undefined
      ? { dailyTokenRemaining: capacity.daily_token_remaining }
      : {}),
    ...(capacity.daily_token_reset_at ? { dailyTokenResetAt: capacity.daily_token_reset_at } : {}),
    ...(capacity.concurrency_limit !== undefined
      ? { concurrencyLimit: capacity.concurrency_limit }
      : {}),
    ...(capacity.concurrency_in_use !== undefined
      ? { concurrencyInUse: capacity.concurrency_in_use }
      : {}),
  }
}

function mapCredential(credential: CredentialWire): Credential {
  return {
    id: credential.id,
    resourcePoolId: credential.resource_pool_id,
    resourcePoolName: credential.resource_pool_name,
    resourcePoolSlug: credential.resource_pool_slug,
    providerId: credential.provider_id,
    providerName: credential.provider_name,
    providerKind: credential.provider_kind,
    providerBaseUrl: credential.provider_base_url,
    name: credential.name,
    status: credential.status,
    healthStatus: credential.health_status,
    capacity: {
      ...mapCredentialCapacity(credential.capacity),
      ...(credential.shared_capacity
        ? { sharedCapacity: mapCredentialCapacity(credential.shared_capacity) }
        : {}),
    },
    ...(credential.upstream_status
      ? { upstreamStatus: mapUpstreamStatus(credential.upstream_status) }
      : {}),
    ...(credential.rpm_limit !== undefined ? { rpmLimit: credential.rpm_limit } : {}),
    ...(credential.tpm_limit !== undefined ? { tpmLimit: credential.tpm_limit } : {}),
    ...(credential.concurrency_limit !== undefined
      ? { concurrencyLimit: credential.concurrency_limit }
      : {}),
    priority: credential.priority,
    weight: credential.weight,
    ...(credential.shared_capacity_scope
      ? { sharedCapacityScope: credential.shared_capacity_scope }
      : {}),
    ...(credential.shared_rpm_limit !== undefined
      ? { sharedRpmLimit: credential.shared_rpm_limit }
      : {}),
    ...(credential.shared_tpm_limit !== undefined
      ? { sharedTpmLimit: credential.shared_tpm_limit }
      : {}),
    ...(credential.shared_concurrency_limit !== undefined
      ? { sharedConcurrencyLimit: credential.shared_concurrency_limit }
      : {}),
    ...(credential.shared_daily_token_limit !== undefined
      ? { sharedDailyTokenLimit: credential.shared_daily_token_limit }
      : {}),
    ...(credential.shared_daily_reset_minute_utc !== undefined
      ? { sharedDailyResetMinuteUtc: credential.shared_daily_reset_minute_utc }
      : {}),
    ...(credential.cooldown_until ? { cooldownUntil: credential.cooldown_until } : {}),
    consecutiveFailures: credential.consecutive_failures,
    ...(credential.last_success_at ? { lastSuccessAt: credential.last_success_at } : {}),
    ...(credential.last_error_kind ? { lastErrorKind: credential.last_error_kind } : {}),
    ...(credential.last_probe_at ? { lastProbeAt: credential.last_probe_at } : {}),
    ...(credential.last_probe_latency_ms !== undefined
      ? { lastProbeLatencyMs: credential.last_probe_latency_ms }
      : {}),
    ...(credential.last_probe_kind ? { lastProbeKind: credential.last_probe_kind } : {}),
    ...(credential.last_probe_status ? { lastProbeStatus: credential.last_probe_status } : {}),
    ...(credential.last_probe_error_kind
      ? { lastProbeErrorKind: credential.last_probe_error_kind }
      : {}),
    ...(credential.last_checked_at ? { lastCheckedAt: credential.last_checked_at } : {}),
    ...(credential.recent_success_rate !== undefined
      ? { recentSuccessRate: credential.recent_success_rate }
      : {}),
    ...(credential.first_byte_p95_ms !== undefined
      ? { firstByteP95Ms: credential.first_byte_p95_ms }
      : {}),
    ...(credential.total_latency_p95_ms !== undefined
      ? { totalLatencyP95Ms: credential.total_latency_p95_ms }
      : {}),
    ...(credential.retired_at ? { retiredAt: credential.retired_at } : {}),
    createdAt: credential.created_at,
    updatedAt: credential.updated_at,
    modelBindings: credential.model_bindings.map((binding) => ({
      modelId: binding.model_id,
      modelName: binding.model_name ?? binding.model_id,
    })),
  }
}

function mapUpstreamStatus(status: UpstreamStatusWire): UpstreamStatusObservation {
  return {
    state: status.state,
    scope: status.scope,
    observedAt: status.observed_at,
    source: status.source,
    ...(status.reason ? { reason: status.reason } : {}),
    ...(status.balance
      ? {
          balance: {
            currency: status.balance.currency,
            available: status.balance.available,
            voucher: status.balance.voucher,
            cash: status.balance.cash,
          },
        }
      : {}),
  }
}

function credentialBody(input: CredentialUpdateInput) {
  return input
}

function mapBatchResult(result: CredentialBatchResultWire): CredentialBatchResult {
  return {
    line: result.line,
    name: result.name,
    status: result.status,
    ...(result.credential ? { credential: mapCredential(result.credential) } : {}),
    ...(result.error_kind ? { errorKind: result.error_kind } : {}),
  }
}

function mapProbeResult(result: CredentialProbeWire): CredentialProbeResult {
  const execution = result.execution
  return {
    credential: mapCredential(result.credential),
    kind: execution.kind,
    status: execution.status,
    ...(execution.error_kind ? { errorKind: execution.error_kind } : {}),
    retryable: execution.retryable,
    mayUseTokens: execution.may_use_tokens,
    latencyMillis: execution.latency_ms,
    modelId: execution.model_id,
    modelName: execution.model_name,
    ...(execution.input_tokens !== undefined ? { inputTokens: execution.input_tokens } : {}),
    ...(execution.output_tokens !== undefined ? { outputTokens: execution.output_tokens } : {}),
    requestId: execution.request_id,
  }
}

function mapDiscoveryResult(result: ModelDiscoveryWire): ModelDiscoveryResult {
  const execution = result.execution
  return {
    credential: mapCredential(result.credential),
    status: execution.status,
    ...(execution.error_kind ? { errorKind: execution.error_kind } : {}),
    retryable: execution.retryable,
    latencyMillis: execution.latency_ms,
    models: execution.models,
  }
}

function mapCredentialModelProbeBatch(
  result: CredentialModelProbeBatchWire,
): CredentialModelProbeBatchResult {
  return {
    results: result.results.map(mapDiscoveryResult),
    succeeded: result.succeeded,
    failed: result.failed,
    unavailable: result.unavailable,
    uncertain: result.uncertain,
  }
}
