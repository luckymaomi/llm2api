import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { catalogApi, type Credential } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

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
  const mutation = useMutation({
    mutationFn: () => {
      const limits = {
        ...(rpmLimit > 0 ? { rpmLimit } : {}),
        ...(tpmLimit > 0 ? { tpmLimit } : {}),
        ...(concurrencyLimit > 0 ? { concurrencyLimit } : {}),
      }
      return catalogApi.updateCredential(
        credential.id,
        {
          name: name.trim(),
          secret,
          expectedUpdatedAt: credential.updatedAt,
          ...limits,
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
        <FormProblem error={mutation.error} />
      </form>
    </DialogFrame>
  )
}
