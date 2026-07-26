export type Role = 'administrator' | 'member'

export type Capability =
  | 'providers:read'
  | 'resource-pools:write'
  | 'credentials:write'
  | 'members:write'
  | 'plans:write'
  | 'subscriptions:write'
  | 'subscriptions:read'
  | 'keys:write'
  | 'operations:read'
  | 'usage:read'
  | 'api-key:test'

export interface Session {
  userId: string
  displayName: string
  role: Role
  capabilities: Capability[]
  csrfToken: string
  expiresAt: string
}

export interface SiteProfile {
  name: string
  description: string
  contact: string
  version: number
  updatedAt: string
}

export interface SiteProfileInput {
  name: string
  description: string
  contact: string
  expectedVersion: number
}

export interface RequestWindowSummary {
  requestCount: number
  completedCount: number
  failedCount: number
  uncertainCount: number
  inputTokens: number
  outputTokens: number
  firstByteP95Ms: number
  totalLatencyP95Ms: number
}

export interface OverviewTrendPoint {
  bucket: string
  requestCount: number
  inputTokens: number
  outputTokens: number
}

interface OverviewBase {
  requests: RequestWindowSummary
  trend: OverviewTrendPoint[]
  errors: Array<{ kind: string; count: number }>
}

export interface AdministratorOverview extends OverviewBase {
  scope: 'administrator'
  resources: {
    resourcePoolCount: number
    activeResourcePoolCount: number
    connectedProviderCount: number
    modelCount: number
    credentialCount: number
    activeCredentialCount: number
    coolingCredentialCount: number
    successfulCredentialProbeCount: number
    activeMemberCount: number
    activeApiKeyCount: number
    activeServicePlanCount: number
    activeSubscriptionCount: number
    hasActiveUpstream: boolean
    hasCompletedRequest: boolean
  }
}

export interface MemberOverview extends OverviewBase {
  scope: 'member'
  access: {
    activeApiKeyCount: number
    activeSubscriptionCount: number
    nearestSubscriptionExpiry?: string
  }
}

export type OperationsOverview = AdministratorOverview | MemberOverview

export interface Page<T> {
  items: T[]
  page: number
  pageSize: number
  total: number
}

export interface ListQuery {
  page?: number
  pageSize?: number
  search?: string
  status?: string
  sort?: string
  order?: 'asc' | 'desc'
  providerId?: string
  resourcePoolId?: string
  subscriptionId?: string
  userId?: string
  apiKeyId?: string
  modelId?: string
  from?: string
  to?: string
}

export interface ModelCapabilities {
  chat: boolean
  streaming: boolean
  tools: boolean
  toolChoiceModes: string[]
  strictTools: boolean
  parallelToolCalls: boolean
  toolStreaming: boolean
  imageInput: boolean
  videoInput: boolean
  partialMode: boolean
  reasoning: boolean
  reasoningMode?: 'toggle' | 'effort' | 'hybrid' | 'always_on'
  reasoningAlwaysOn: boolean
  reasoningDefaultEnabled: boolean
  reasoningConfig: boolean
  reasoningContent: boolean
  reasoningPreserve: boolean
  reasoningEfforts: string[]
  toolChoiceModesWithReasoning: string[]
  structuredOutput: boolean
  jsonSchemaOutput: boolean
  messageName: boolean
  promptCacheKey: boolean
  safetyIdentifier: boolean
  responseUsage: boolean
  streamUsage: boolean
  contextTokens: number
  outputTokens: number
  parameters: ModelParameterCapabilities
}

export interface IntegerParameterLimit {
  supported: boolean
  minimum?: number
  maximum?: number
  exactValues: number[]
}

export interface NumberParameterLimit {
  supported: boolean
  minimum?: number
  maximum?: number
  exactValues: number[]
}

export interface SamplingCondition {
  thinkingEnabled?: boolean
  temperatureExact?: number
  temperatureAtMost?: number
  nMaximum?: number
}

export interface ModelParameterCapabilities {
  maxCompletionTokens: IntegerParameterLimit
  temperature: NumberParameterLimit
  topP: NumberParameterLimit
  presencePenalty: NumberParameterLimit
  frequencyPenalty: NumberParameterLimit
  n: IntegerParameterLimit
  topK: NumberParameterLimit
  thinkingBudget: IntegerParameterLimit
  samplingConditions: SamplingCondition[]
}

export interface Model {
  id: string
  providerId: string
  providerSlug: string
  providerName: string
  publicName: string
  upstreamName: string
  displayName: string
  capabilities: ModelCapabilities
  createdAt: string
  updatedAt: string
}

export interface ProviderModelProfile {
  upstreamName: string
  displayName: string
  capabilities: ModelCapabilities
}

export interface Provider {
  id: string
  catalogId: string
  slug: string
  name: string
  kind: string
  baseUrl: string
  sourceUrl: string
  verifiedAt: string
  contract: {
    referenceUrl: string
    contractSnapshot: string
    verifiedAt: string
    referenceProvider?: string
    verifiedModels: string[]
    liveCapabilities: string[]
    status: 'verified' | 'degraded'
  }
  models: ProviderModelProfile[]
  resourcePoolCount: number
  activeCredentialCount: number
  createdAt: string
  updatedAt: string
}

export type ResourcePoolStatus = 'active' | 'disabled' | 'retired'

export interface ResourcePool {
  id: string
  providerId: string
  providerCatalogId: string
  providerSlug: string
  providerName: string
  providerKind: string
  providerBaseUrl: string
  slug: string
  name: string
  status: ResourcePoolStatus
  models: Model[]
  modelCount: number
  credentialCount: number
  activeCredentialCount: number
  retiredAt?: string
  createdAt: string
  updatedAt: string
}

export interface ResourcePoolInput {
  providerId: string
  name: string
}

export interface CredentialModelBinding {
  modelId: string
  modelName: string
}

export type CredentialStatus = 'active' | 'disabled' | 'retired'
export type CredentialHealthStatus = 'healthy' | 'cooling' | 'probing' | 'repair_required'

export interface CredentialCapacity {
  state: 'observed' | 'unavailable'
  scope: 'gateway_credential' | 'gateway_shared_upstream'
  observedAt?: string
  requestsPerMinuteLimit?: number
  requestsPerMinuteRemaining?: number
  tokensPerMinuteLimit?: number
  tokensPerMinuteRemaining?: number
  dailyTokenLimit?: number
  dailyTokenRemaining?: number
  dailyTokenResetAt?: string
  concurrencyLimit?: number
  concurrencyInUse?: number
  sharedCapacity?: CredentialCapacity
}

export type UpstreamStatusState = 'observed' | 'unknown' | 'unavailable'
export type UpstreamStatusScope = 'account' | 'project' | 'credential' | 'unknown'

export interface UpstreamStatusBalance {
  currency: string
  available: string
  voucher: string
  cash: string
}

export interface UpstreamStatusObservation {
  state: UpstreamStatusState
  scope: UpstreamStatusScope
  observedAt: string
  source: string
  reason?: string
  balance?: UpstreamStatusBalance
}

export interface Credential {
  id: string
  resourcePoolId: string
  resourcePoolName: string
  resourcePoolSlug: string
  providerId: string
  providerName: string
  providerKind: string
  providerBaseUrl: string
  name: string
  status: CredentialStatus
  healthStatus: CredentialHealthStatus
  capacity: CredentialCapacity
  upstreamStatus?: UpstreamStatusObservation
  rpmLimit?: number
  tpmLimit?: number
  concurrencyLimit?: number
  priority: number
  weight: number
  sharedCapacityScope?: string
  sharedRpmLimit?: number
  sharedTpmLimit?: number
  sharedConcurrencyLimit?: number
  sharedDailyTokenLimit?: number
  sharedDailyResetMinuteUtc?: number
  cooldownUntil?: string
  consecutiveFailures: number
  lastSuccessAt?: string
  lastErrorKind?: string
  lastProbeAt?: string
  lastProbeLatencyMs?: number
  lastProbeKind?: string
  lastProbeStatus?: string
  lastProbeErrorKind?: string
  lastCheckedAt?: string
  recentSuccessRate?: number
  firstByteP95Ms?: number
  totalLatencyP95Ms?: number
  retiredAt?: string
  createdAt: string
  updatedAt: string
  modelBindings: CredentialModelBinding[]
}

export interface CredentialUpdateInput {
  name: string
  secret: string
  rpmLimit?: number
  tpmLimit?: number
  concurrencyLimit?: number
  priority: number
  weight: number
  sharedCapacityScope?: string
  sharedRpmLimit?: number
  sharedTpmLimit?: number
  sharedConcurrencyLimit?: number
  sharedDailyTokenLimit?: number
  sharedDailyResetMinuteUtc?: number
  expectedUpdatedAt: string
}

export interface CredentialBatchInput {
  resourcePoolId: string
  items: Array<{ name: string; secret: string }>
  rpmLimit?: number
  tpmLimit?: number
  concurrencyLimit?: number
}

export interface ModelDiscoveryResult {
  credential: Credential
  status: 'succeeded' | 'failed' | 'unavailable' | 'uncertain'
  errorKind?: string
  retryable: boolean
  latencyMillis: number
  models: string[]
}

export interface CredentialModelProbeBatchResult {
  results: ModelDiscoveryResult[]
  succeeded: number
  failed: number
  unavailable: number
  uncertain: number
}

export interface CredentialBatchResult {
  line: number
  name: string
  status: 'created' | 'duplicate' | 'rejected'
  credential?: Credential
  errorKind?: string
}

export interface CredentialProbeResult {
  credential: Credential
  kind: string
  status: 'succeeded' | 'failed' | 'unavailable' | 'uncertain'
  errorKind?: string
  retryable: boolean
  mayUseTokens: boolean
  latencyMillis: number
  modelId: string
  modelName: string
  inputTokens?: number
  outputTokens?: number
  requestId: string
}

export interface UserAccount {
  id: string
  displayName: string
  email: string
  role: Role
  status: 'active' | 'disabled' | 'deleted'
  keyCount: number
  disabledAt?: string
  deletedAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreatedMember {
  member: UserAccount
  initialPassword: string
}

export interface GatewayKey {
  id: string
  ownerId: string
  ownerName: string
  name: string
  prefix: string
  status: 'active' | 'deleted' | 'expired'
  routes: GatewayKeyRoute[]
  expiresAt?: string
  createdAt: string
  lastUsedAt?: string
}

export interface GatewayKeyRoute {
  modelId: string
  modelName: string
  resourcePoolId: string
  resourcePoolName: string
}

export interface CreatedGatewayKey {
  key: GatewayKey
  secret: string
}

export interface SessionRevocation {
  revokedSessions: number
}

export type PlanStatus = 'active' | 'disabled' | 'archived'

export interface PlanRoute {
  modelId: string
  modelName: string
  resourcePoolId: string
  resourcePoolName: string
  resourcePoolSlug: string
  providerName: string
}

export interface PlanVersion {
  id: string
  version: number
  routes: PlanRoute[]
  createdAt: string
}

export interface ServicePlan {
  id: string
  slug: string
  name: string
  description: string
  status: PlanStatus
  currentVersion?: PlanVersion
  activeSubscriptionCount: number
  createdAt: string
  updatedAt: string
}

export interface PlanInput {
  name: string
  description: string
  routes: Array<{ modelId: string; resourcePoolId: string }>
}

export type SubscriptionStatus = 'scheduled' | 'active' | 'suspended' | 'canceled' | 'expired'

export interface Subscription {
  id: string
  userId: string
  memberEmail: string
  memberName: string
  servicePlanId: string
  servicePlanVersionId: string
  servicePlanName: string
  planVersion: number
  status: SubscriptionStatus
  startsAt: string
  expiresAt?: string
  notes: string
  routes: PlanRoute[]
  suspendedAt?: string
  canceledAt?: string
  createdAt: string
  updatedAt: string
}

export interface SubscriptionInput {
  userId: string
  servicePlanId: string
  startsAt: string
  expiresAt?: string
  notes: string
}

export interface SubscriptionUpdateInput {
  startsAt: string
  expiresAt?: string
  notes: string
  expectedUpdatedAt: string
}

export type RequestStatus =
  'queued' | 'dispatching' | 'streaming' | 'completed' | 'failed' | 'canceled' | 'uncertain'

export interface RequestLog {
  requestId: string
  acceptedAt: string
  completedAt?: string
  updatedAt: string
  userId: string
  userName: string
  apiKeyId: string
  keyPrefix: string
  modelId: string
  modelAlias: string
  resourcePoolId: string
  resourcePoolName: string
  resourcePoolSlug: string
  status: RequestStatus
  stream: boolean
  inputTokens?: number
  outputTokens?: number
  usageSource: 'authoritative' | 'estimated' | 'unknown'
  errorKind?: string
  attemptCount: number
}

export interface RequestAttempt {
  id: string
  sequence: number
  status: string
  providerName?: string
  credentialName?: string
  httpStatus?: number
  errorKind?: string
  retryAfterAt?: string
  sentAt?: string
  firstByteAt?: string
  completedAt?: string
  inputTokens?: number
  outputTokens?: number
  usageSource: 'authoritative' | 'estimated' | 'unknown'
  createdAt: string
}

export interface RequestLogDetail {
  request: RequestLog
  attempts: RequestAttempt[]
}

export type OperationPhase =
  | 'submitted'
  | 'validating'
  | 'queued'
  | 'running'
  | 'streaming'
  | 'waiting'
  | 'completed'
  | 'failed'
  | 'canceled'
  | 'uncertain'

export interface ApiProblemShape {
  status: number
  code: string
  message: string
  stage?: string
  requestId?: string
  retryable: boolean
  fieldErrors?: Record<string, string>
}

export interface GatewayKeyTestModel {
  id: string
  alias: string
}

export interface GatewayKeyTestInput {
  apiKeyId: string
  model: string
  message: string
}

export type GatewayKeyTestEvent =
  | { type: 'phase'; phase: OperationPhase; step: string; requestId: string }
  | { type: 'content'; delta: string }
  | {
      type: 'usage'
      inputTokens: number
      outputTokens: number
      source: 'authoritative' | 'estimated'
    }
  | { type: 'completed'; requestId: string }
  | { type: 'error'; problem: ApiProblemShape }
