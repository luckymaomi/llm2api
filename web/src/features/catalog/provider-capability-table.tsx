import { ExternalLink } from 'lucide-react'

import { type Provider, type ProviderModelProfile } from '@/api'
import { Badge, StatusBadge } from '@/components/ui/badge'
import { formatNumber } from '@/lib/format'

export function ProviderCapabilityTable({
  provider,
  models,
}: {
  provider: Provider
  models: ProviderModelProfile[]
}) {
  return (
    <section className="provider-capability-section" aria-label={`${provider.name} 模型能力`}>
      <header className="provider-capability-heading">
        <div>
          <div className="provider-capability-title">
            <h2>{provider.name}</h2>
            <StatusBadge status={provider.contract.status} />
          </div>
          <p>
            {provider.contract.contractSnapshot}，核验于 {provider.contract.verifiedAt}
          </p>
        </div>
        <a
          href={provider.contract.referenceUrl}
          target="_blank"
          rel="noreferrer"
          className="provider-reference-link"
        >
          官方能力文档 <ExternalLink size={14} />
        </a>
      </header>
      <dl className="provider-capability-facts">
        <div>
          <dt>模型目录</dt>
          <dd>{formatNumber(models.length)}</dd>
        </div>
        <div>
          <dt>资源池</dt>
          <dd>{formatNumber(provider.resourcePoolCount)}</dd>
        </div>
        <div>
          <dt>活动上游 API Key</dt>
          <dd>{formatNumber(provider.activeCredentialCount)}</dd>
        </div>
      </dl>
      <div className="provider-model-directory">
        {models.map((model) => (
          <ModelCapabilityCard key={model.upstreamName} model={model} />
        ))}
      </div>
    </section>
  )
}

function ModelCapabilityCard({ model }: { model: ProviderModelProfile }) {
  const capabilities = model.capabilities
  return (
    <article className="provider-model-card">
      <header className="provider-model-card__header">
        <div>
          <strong>{model.displayName}</strong>
          <small>{model.upstreamName}</small>
        </div>
        <div className="provider-model-card__badges">
          {capabilities.chat ? <Badge tone="positive">Chat</Badge> : null}
          {capabilities.streaming ? <Badge tone="neutral">流式</Badge> : null}
        </div>
      </header>
      <dl className="provider-model-capability-grid">
        <CapabilityFact label="工具调用" value={toolSummary(model)} />
        <CapabilityFact label="思考 / 推理" value={reasoningSummary(model)} />
        <CapabilityFact label="输入 / 输出" value={inputOutputSummary(model)} />
        <CapabilityFact label="参数与上下文" value={parameterAndLimitSummary(model)} />
      </dl>
    </article>
  )
}

function CapabilityFact({ label, value }: { label: string; value: string[] }) {
  return (
    <div>
      <dt>{label}</dt>
      {value.map((line) => (
        <dd key={line}>{line}</dd>
      ))}
    </div>
  )
}

function toolSummary({ capabilities }: ProviderModelProfile): string[] {
  if (!capabilities.tools) return ['不支持']
  const modes =
    capabilities.toolChoiceModes.length > 0 ? capabilities.toolChoiceModes.join('、') : '默认选择'
  const extensions = [
    capabilities.strictTools && '严格 Schema',
    capabilities.parallelToolCalls && '并行调用',
    capabilities.toolStreaming && '流式 tool calls',
  ].filter((value): value is string => Boolean(value))
  return [`选择：${modes}`, extensions.length > 0 ? extensions.join('、') : '基础函数调用']
}

function reasoningSummary({ capabilities }: ProviderModelProfile): string[] {
  if (!capabilities.reasoning) return ['不支持']
  const mode = capabilities.reasoningAlwaysOn
    ? '始终开启'
    : capabilities.reasoningMode === 'toggle'
      ? '可开关'
      : capabilities.reasoningMode === 'effort'
        ? '可调强度'
        : '可配置'
  const details = [
    capabilities.reasoningDefaultEnabled && '默认思考',
    capabilities.reasoningEfforts.length > 0 && capabilities.reasoningEfforts.join('、'),
    capabilities.reasoningPreserve && '可保留推理',
  ].filter((value): value is string => Boolean(value))
  return [mode, details.length > 0 ? details.join('、') : '无额外限制']
}

function inputOutputSummary({ capabilities }: ProviderModelProfile): string[] {
  const inputs = [capabilities.imageInput && '图片', capabilities.videoInput && '视频'].filter(
    (value): value is string => Boolean(value),
  )
  const outputs = [
    capabilities.structuredOutput && 'JSON 对象',
    capabilities.jsonSchemaOutput && 'JSON Schema',
  ].filter((value): value is string => Boolean(value))
  const extensions = [
    capabilities.partialMode && 'Partial',
    capabilities.promptCacheKey && '上下文缓存',
    capabilities.safetyIdentifier && '安全标识',
  ].filter((value): value is string => Boolean(value))
  return [
    `输入：${inputs.length > 0 ? inputs.join('、') : '文本'}`,
    `输出：${outputs.length > 0 ? outputs.join('、') : '文本'}`,
    extensions.length > 0 ? extensions.join('、') : '无扩展能力',
  ]
}

function parameterAndLimitSummary({ capabilities }: ProviderModelProfile): string[] {
  const parameters = capabilities.parameters
  const values = [
    parameterLabel('最大输出', parameters.maxCompletionTokens),
    parameterLabel('温度', parameters.temperature),
    parameterLabel('Top P', parameters.topP),
    parameterLabel('Top K', parameters.topK),
    parameterLabel('N', parameters.n),
    parameterLabel('Presence', parameters.presencePenalty),
    parameterLabel('Frequency', parameters.frequencyPenalty),
    parameterLabel('思考预算', parameters.thinkingBudget),
  ].filter((value): value is string => Boolean(value))
  const conditions = parameters.samplingConditions
    .map((condition) => {
      if (condition.temperatureExact === undefined) return null
      return `${condition.thinkingEnabled === false ? '非思考' : '思考'}温度=${condition.temperatureExact}`
    })
    .filter((value): value is string => Boolean(value))
  return [
    values.length > 0 ? values.join('；') : '未公开高级参数',
    conditions.length > 0 ? conditions.join('；') : '无模式专属参数',
    `上下文 ${tokenLimit(capabilities.contextTokens)} / 输出 ${tokenLimit(capabilities.outputTokens)}`,
  ]
}

function parameterLabel(
  label: string,
  parameter: { supported: boolean; minimum?: number; maximum?: number; exactValues: number[] },
) {
  if (!parameter.supported) return null
  if (parameter.exactValues.length > 0) return `${label}=${parameter.exactValues.join('/')}`
  if (parameter.minimum !== undefined && parameter.maximum !== undefined)
    return `${label} ${parameter.minimum}-${parameter.maximum}`
  if (parameter.minimum !== undefined) return `${label} >=${parameter.minimum}`
  if (parameter.maximum !== undefined) return `${label} <=${parameter.maximum}`
  return label
}

function tokenLimit(value: number) {
  return value > 0 ? formatNumber(value) : '厂商未公开'
}
