import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { catalogApi, type Credential } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

function resetMinuteToTime(minute: number | undefined) {
  const value = minute ?? 0
  return `${String(Math.floor(value / 60)).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}`
}

function resetTimeToMinute(value: string) {
  const [hour = 0, minute = 0] = value.split(':').map(Number)
  return hour * 60 + minute
}

export function CredentialForm({
  credential,
  open,
  onOpenChange,
}: {
  credential: Credential
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(credential.name)
  const [secret, setSecret] = useState('')
  const [rpmLimit, setRpmLimit] = useState(credential.rpmLimit ?? 0)
  const [tpmLimit, setTpmLimit] = useState(credential.tpmLimit ?? 0)
  const [concurrencyLimit, setConcurrencyLimit] = useState(credential.concurrencyLimit ?? 0)
  const [priority, setPriority] = useState(credential.priority)
  const [weight, setWeight] = useState(credential.weight)
  const [sharedCapacityScope, setSharedCapacityScope] = useState(
    credential.sharedCapacityScope ?? '',
  )
  const [sharedRpmLimit, setSharedRpmLimit] = useState(credential.sharedRpmLimit ?? 0)
  const [sharedTpmLimit, setSharedTpmLimit] = useState(credential.sharedTpmLimit ?? 0)
  const [sharedConcurrencyLimit, setSharedConcurrencyLimit] = useState(
    credential.sharedConcurrencyLimit ?? 0,
  )
  const [sharedDailyTokenLimit, setSharedDailyTokenLimit] = useState(
    credential.sharedDailyTokenLimit ?? 0,
  )
  const [sharedDailyResetTime, setSharedDailyResetTime] = useState(
    resetMinuteToTime(credential.sharedDailyResetMinuteUtc),
  )
  const mutation = useMutation({
    mutationFn: () => {
      const limits = {
        ...(rpmLimit > 0 ? { rpmLimit } : {}),
        ...(tpmLimit > 0 ? { tpmLimit } : {}),
        ...(concurrencyLimit > 0 ? { concurrencyLimit } : {}),
        priority,
        weight,
      }
      const sharedScope = sharedCapacityScope.trim()
      if (sharedScope && (sharedRpmLimit < 1 || sharedTpmLimit < 1 || sharedConcurrencyLimit < 1)) {
        throw new Error('共享上游限制需要同时填写 RPM、TPM 和并发上限。')
      }
      const sharedDailyResetMinuteUtc = resetTimeToMinute(sharedDailyResetTime)
      if (
        sharedDailyTokenLimit > 0 &&
        (!sharedScope ||
          !/^\d{2}:\d{2}$/.test(sharedDailyResetTime) ||
          sharedDailyResetMinuteUtc > 1439)
      ) {
        throw new Error('每日 Token 上限需要共享范围和有效的 UTC 重置时刻。')
      }
      return catalogApi.updateCredential(
        credential.id,
        {
          name: name.trim(),
          secret,
          expectedUpdatedAt: credential.updatedAt,
          ...limits,
          ...(sharedScope
            ? {
                sharedCapacityScope: sharedScope,
                sharedRpmLimit,
                sharedTpmLimit,
                sharedConcurrencyLimit,
                ...(sharedDailyTokenLimit > 0
                  ? { sharedDailyTokenLimit, sharedDailyResetMinuteUtc }
                  : {}),
              }
            : {}),
        },
        crypto.randomUUID(),
      )
    },
    async onSuccess() {
      setSecret('')
      await queryClient.invalidateQueries({ queryKey: ['credentials'] })
      await queryClient.invalidateQueries({ queryKey: ['resource-pools'] })
      onOpenChange(false)
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim()) return
    mutation.mutate()
  }

  const locked = mutation.isPending
  return (
    <DialogFrame
      open={open}
      onOpenChange={(next) => !locked && onOpenChange(next)}
      title="编辑上游 API Key"
      width="lg"
      dismissible={!locked}
      footer={
        <>
          <Button
            type="button"
            variant="secondary"
            disabled={locked}
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button type="submit" form="credential-form" disabled={locked}>
            {locked ? '保存中' : '保存'}
          </Button>
        </>
      }
    >
      <form id="credential-form" className="form-grid" onSubmit={submit}>
        <Field label="资源池" htmlFor="credential-pool">
          <Input id="credential-pool" value={credential.resourcePoolName} readOnly />
        </Field>
        <Field label="Key 名称" htmlFor="credential-name" hint="用于区分不同的上游 Key">
          <Input
            id="credential-name"
            autoFocus
            required
            value={name}
            readOnly={locked}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field
          label="上游 API Key"
          htmlFor="credential-secret"
          className="field--full"
          hint="不填写则保留当前 Key；填写后先探测模型，成功才替换"
        >
          <Input
            id="credential-secret"
            type="password"
            autoComplete="new-password"
            value={secret}
            readOnly={locked}
            onChange={(event) => setSecret(event.target.value)}
          />
        </Field>
        <Field
          label="每分钟请求上限（RPM）"
          htmlFor="credential-rpm"
          hint="0 表示跟随上游本身的限制"
        >
          <Input
            id="credential-rpm"
            type="number"
            min={0}
            value={rpmLimit}
            readOnly={locked}
            onChange={(event) => setRpmLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="每分钟 Token 上限（TPM）"
          htmlFor="credential-tpm"
          hint="0 表示跟随上游本身的限制"
        >
          <Input
            id="credential-tpm"
            type="number"
            min={0}
            value={tpmLimit}
            readOnly={locked}
            onChange={(event) => setTpmLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="同时请求上限"
          htmlFor="credential-concurrency"
          hint="0 表示跟随上游本身的限制"
        >
          <Input
            id="credential-concurrency"
            type="number"
            min={0}
            value={concurrencyLimit}
            readOnly={locked}
            onChange={(event) => setConcurrencyLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="调度优先级"
          htmlFor="credential-priority"
          hint="数值越小越先使用；同级再按权重分配"
        >
          <Input
            id="credential-priority"
            type="number"
            min={1}
            max={1000}
            value={priority}
            readOnly={locked}
            onChange={(event) => setPriority(Number(event.target.value))}
          />
        </Field>
        <Field label="同级权重" htmlFor="credential-weight" hint="同一优先级内按权重比例选择">
          <Input
            id="credential-weight"
            type="number"
            min={1}
            max={1000}
            value={weight}
            readOnly={locked}
            onChange={(event) => setWeight(Number(event.target.value))}
          />
        </Field>
        <Field
          label="共享上游范围"
          htmlFor="credential-shared-scope"
          className="field--full"
          hint="同一 Provider 下属于同一上游账号或项目的 Key 使用完全相同的范围名和限制；留空表示此 Key 只使用自身限制"
        >
          <Input
            id="credential-shared-scope"
            maxLength={120}
            value={sharedCapacityScope}
            readOnly={locked}
            onChange={(event) => setSharedCapacityScope(event.target.value)}
          />
        </Field>
        <Field label="共享 RPM" htmlFor="credential-shared-rpm" hint="共享范围每分钟请求上限">
          <Input
            id="credential-shared-rpm"
            type="number"
            min={0}
            value={sharedRpmLimit}
            readOnly={locked}
            onChange={(event) => setSharedRpmLimit(Number(event.target.value))}
          />
        </Field>
        <Field label="共享 TPM" htmlFor="credential-shared-tpm" hint="共享范围每分钟 Token 上限">
          <Input
            id="credential-shared-tpm"
            type="number"
            min={0}
            value={sharedTpmLimit}
            readOnly={locked}
            onChange={(event) => setSharedTpmLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="共享并发"
          htmlFor="credential-shared-concurrency"
          hint="共享范围同时执行请求上限"
        >
          <Input
            id="credential-shared-concurrency"
            type="number"
            min={0}
            value={sharedConcurrencyLimit}
            readOnly={locked}
            onChange={(event) => setSharedConcurrencyLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="每日 Token 上限（TPD）"
          htmlFor="credential-shared-daily-tokens"
          hint="0 表示该共享范围不配置日额度"
        >
          <Input
            id="credential-shared-daily-tokens"
            type="number"
            min={0}
            value={sharedDailyTokenLimit}
            readOnly={locked}
            onChange={(event) => setSharedDailyTokenLimit(Number(event.target.value))}
          />
        </Field>
        <Field
          label="每日重置时刻（UTC）"
          htmlFor="credential-shared-daily-reset"
          hint="按 Provider 公布的额度重置时刻填写"
        >
          <Input
            id="credential-shared-daily-reset"
            type="time"
            value={sharedDailyResetTime}
            readOnly={locked || sharedDailyTokenLimit < 1}
            onChange={(event) => setSharedDailyResetTime(event.target.value)}
          />
        </Field>
        <FormProblem error={mutation.error} />
      </form>
    </DialogFrame>
  )
}
