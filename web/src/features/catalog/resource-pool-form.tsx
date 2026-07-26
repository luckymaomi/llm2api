import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { catalogApi, type ResourcePool } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input, NativeSelect } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

import { ProviderCapabilityTable } from './provider-capability-table'

export function ResourcePoolForm({
  pool,
  open,
  onOpenChange,
}: {
  pool: ResourcePool | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [providerId, setProviderId] = useState(pool?.providerId ?? '')
  const [name, setName] = useState(pool?.name ?? '')
  const providers = useQuery({
    queryKey: ['providers', 'resource-pool-form'],
    queryFn: ({ signal }) => catalogApi.providers(signal),
    enabled: open && pool === null,
  })
  const selectedProvider = (providers.data ?? []).find((provider) => provider.id === providerId)

  const mutation = useMutation({
    mutationFn: () =>
      pool
        ? catalogApi.updateResourcePool(
            pool.id,
            { name: name.trim(), expectedUpdatedAt: pool.updatedAt },
            crypto.randomUUID(),
          )
        : catalogApi.createResourcePool({ providerId, name: name.trim() }, crypto.randomUUID()),
    async onSuccess() {
      await queryClient.invalidateQueries({ queryKey: ['resource-pools'] })
      onOpenChange(false)
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim() || (!pool && !providerId)) return
    mutation.mutate()
  }

  const locked = mutation.isPending
  return (
    <DialogFrame
      open={open}
      onOpenChange={(next) => !locked && onOpenChange(next)}
      title={pool ? '编辑资源池' : '创建资源池'}
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
          <Button type="submit" form="resource-pool-form" disabled={locked}>
            {locked ? '保存中' : '保存'}
          </Button>
        </>
      }
    >
      <form id="resource-pool-form" className="form-grid" onSubmit={submit}>
        <Field label="上游平台" htmlFor="pool-provider">
          <NativeSelect
            id="pool-provider"
            autoFocus
            required
            value={providerId}
            disabled={locked || pool !== null}
            onChange={(event) => setProviderId(event.target.value)}
          >
            <option value="">选择平台</option>
            {(providers.data ?? []).map((provider) => (
              <option key={provider.id} value={provider.id}>
                {provider.name}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="资源池名称" htmlFor="pool-name">
          <Input
            id="pool-name"
            required
            value={name}
            readOnly={locked}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        {pool && pool.models.length > 0 ? (
          <Field label="模型" htmlFor="pool-model-summary">
            <Input
              id="pool-model-summary"
              value={pool.models.map((model) => model.publicName).join('、')}
              readOnly
            />
          </Field>
        ) : null}
        {selectedProvider ? (
          <ProviderCapabilityTable provider={selectedProvider} models={selectedProvider.models} />
        ) : null}
        <FormProblem error={mutation.error ?? providers.error} />
      </form>
    </DialogFrame>
  )
}
