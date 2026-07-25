import { useMutation, useQueryClient } from '@tanstack/react-query'
import { FlaskConical, RefreshCw } from 'lucide-react'
import { useRef, useState } from 'react'

import {
  catalogApi,
  type Credential,
  type CredentialProbeResult,
  type ModelDiscoveryResult,
} from '@/api'
import { StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, NativeSelect } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'
import { formatNumber } from '@/lib/format'

import { probeErrorLabel } from './credential-probe-copy'

type ProbeMode = 'models' | 'generation'

export function CredentialProbeDialog({
  credential,
  onOpenChange,
}: {
  credential: Credential
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<ProbeMode>('models')
  const [currentCredential, setCurrentCredential] = useState(credential)
  const [modelId, setModelId] = useState(credential.modelBindings[0]?.modelId ?? '')
  const [discoveryResult, setDiscoveryResult] = useState<ModelDiscoveryResult>()
  const [generationResult, setGenerationResult] = useState<CredentialProbeResult>()
  const controller = useRef<AbortController | undefined>(undefined)

  const discovery = useMutation({
    mutationFn: () => {
      const nextController = new AbortController()
      controller.current = nextController
      return catalogApi.probeCredential(
        currentCredential.id,
        currentCredential.updatedAt,
        crypto.randomUUID(),
        nextController.signal,
      )
    },
    async onSuccess(value) {
      setCurrentCredential(value.credential)
      setDiscoveryResult(value)
      setModelId(value.credential.modelBindings[0]?.modelId ?? '')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['credentials'] }),
        queryClient.invalidateQueries({ queryKey: ['resource-pools'] }),
      ])
    },
    onSettled() {
      controller.current = undefined
    },
  })

  const generation = useMutation({
    mutationFn: () => {
      const nextController = new AbortController()
      controller.current = nextController
      return catalogApi.deepTestCredential(currentCredential.id, modelId, nextController.signal)
    },
    async onSuccess(value) {
      setCurrentCredential(value.credential)
      setGenerationResult(value)
      await queryClient.invalidateQueries({ queryKey: ['credentials'] })
    },
    onSettled() {
      controller.current = undefined
    },
  })

  const pending = discovery.isPending || generation.isPending
  const stopWaiting = () => controller.current?.abort()

  return (
    <DialogFrame
      open
      onOpenChange={(open) => (!pending || open) && onOpenChange(open)}
      title="检查上游 API Key"
      description={`${credential.name} · ${credential.resourcePoolName}`}
      dismissible={!pending}
      width="md"
      footer={
        pending ? (
          <Button type="button" variant="secondary" onClick={stopWaiting}>
            停止等待
          </Button>
        ) : (
          <>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
              关闭
            </Button>
            {mode === 'models' ? (
              <Button
                type="button"
                icon={<RefreshCw size={16} />}
                onClick={() => {
                  setDiscoveryResult(undefined)
                  discovery.reset()
                  discovery.mutate()
                }}
              >
                {discoveryResult ? '重新探测' : '探测模型'}
              </Button>
            ) : (
              <Button
                type="button"
                icon={<FlaskConical size={16} />}
                disabled={!modelId}
                onClick={() => {
                  setGenerationResult(undefined)
                  generation.reset()
                  generation.mutate()
                }}
              >
                {generationResult ? '重新测试' : '开始深度测试'}
              </Button>
            )}
          </>
        )
      }
    >
      <div className="probe-dialog">
        <div className="probe-mode-switch" role="tablist" aria-label="检查方式">
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'models'}
            disabled={pending}
            onClick={() => setMode('models')}
          >
            模型探测
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === 'generation'}
            disabled={pending}
            onClick={() => setMode('generation')}
          >
            深度测试
          </button>
        </div>

        <dl className="probe-key-facts">
          <div>
            <dt>上游平台</dt>
            <dd>{currentCredential.providerName}</dd>
          </div>
          <div>
            <dt>资源池</dt>
            <dd>{currentCredential.resourcePoolName}</dd>
          </div>
          <div>
            <dt>已发现模型</dt>
            <dd>{formatNumber(currentCredential.modelBindings.length)}</dd>
          </div>
        </dl>

        <div className="probe-workspace">
          {mode === 'models' ? (
            <ModelDiscoveryPanel pending={discovery.isPending} result={discoveryResult} />
          ) : (
            <GenerationPanel
              credential={currentCredential}
              modelId={modelId}
              pending={generation.isPending}
              result={generationResult}
              onModelChange={(value) => {
                setModelId(value)
                setGenerationResult(undefined)
              }}
            />
          )}
        </div>

        <FormProblem error={mode === 'models' ? discovery.error : generation.error} />
      </div>
    </DialogFrame>
  )
}

function ModelDiscoveryPanel({
  pending,
  result,
}: {
  pending: boolean
  result: ModelDiscoveryResult | undefined
}) {
  if (pending) return <ProbePending label="正在读取上游模型列表" />
  if (!result) {
    return <p className="probe-empty">尚未开始本次模型探测</p>
  }
  return (
    <section className="probe-result" data-status={result.status} aria-live="polite">
      <div className="probe-result__header">
        <strong>{result.status === 'succeeded' ? '模型探测成功' : '模型探测失败'}</strong>
        <StatusBadge status={result.status} />
      </div>
      <dl className="probe-result__facts">
        <div>
          <dt>模型数量</dt>
          <dd>{formatNumber(result.models.length)}</dd>
        </div>
        <div>
          <dt>耗时</dt>
          <dd>{formatNumber(result.latencyMillis)} ms</dd>
        </div>
      </dl>
      {result.errorKind ? (
        <div className="probe-result__error">
          <strong>{probeErrorLabel(result.errorKind)}</strong>
          <span>{result.retryable ? '可以重新探测' : '请检查 Key 或上游设置'}</span>
        </div>
      ) : (
        <div className="probe-model-list" aria-label="探测到的模型">
          {result.models.map((model) => (
            <code key={model}>{model}</code>
          ))}
        </div>
      )}
    </section>
  )
}

function GenerationPanel({
  credential,
  modelId,
  pending,
  result,
  onModelChange,
}: {
  credential: Credential
  modelId: string
  pending: boolean
  result: CredentialProbeResult | undefined
  onModelChange: (value: string) => void
}) {
  return (
    <div className="probe-generation">
      <Field label="模型" htmlFor="credential-deep-test-model">
        <NativeSelect
          id="credential-deep-test-model"
          value={modelId}
          disabled={pending || credential.modelBindings.length === 0}
          onChange={(event) => onModelChange(event.target.value)}
        >
          {credential.modelBindings.length === 0 ? <option value="">暂无可测试模型</option> : null}
          {credential.modelBindings.map((binding) => (
            <option key={binding.modelId} value={binding.modelId}>
              {binding.modelName}
            </option>
          ))}
        </NativeSelect>
      </Field>
      <p className="probe-note">深度测试会发送最小生成请求，可能消耗少量 Token。</p>
      {pending ? <ProbePending label="正在等待模型响应" /> : null}
      {result ? <GenerationResult result={result} /> : null}
      {!pending && !result ? <p className="probe-empty">尚未开始本次深度测试</p> : null}
    </div>
  )
}

function ProbePending({ label }: { label: string }) {
  return (
    <div className="probe-pending" aria-live="polite">
      <strong>{label}</strong>
      <StatusBadge status="running" />
    </div>
  )
}

function GenerationResult({ result }: { result: CredentialProbeResult }) {
  return (
    <section className="probe-result" data-status={result.status} aria-live="polite">
      <div className="probe-result__header">
        <strong>{result.status === 'succeeded' ? '深度测试成功' : '深度测试失败'}</strong>
        <StatusBadge status={result.status} />
      </div>
      <dl className="probe-result__facts">
        <div>
          <dt>模型</dt>
          <dd>{result.modelName}</dd>
        </div>
        <div>
          <dt>耗时</dt>
          <dd>{formatNumber(result.latencyMillis)} ms</dd>
        </div>
        <div>
          <dt>输入 Token</dt>
          <dd>{formatTokenCount(result.inputTokens)}</dd>
        </div>
        <div>
          <dt>输出 Token</dt>
          <dd>{formatTokenCount(result.outputTokens)}</dd>
        </div>
      </dl>
      {result.errorKind ? (
        <div className="probe-result__error">
          <strong>{probeErrorLabel(result.errorKind)}</strong>
          <span>{result.retryable ? '可以重新测试' : '请检查 Key、模型或网络设置'}</span>
        </div>
      ) : null}
      <div className="probe-result__request">
        <span>Request ID</span>
        <code>{result.requestId}</code>
      </div>
    </section>
  )
}

function formatTokenCount(value: number | undefined): string {
  return value === undefined ? '未知' : formatNumber(value)
}
