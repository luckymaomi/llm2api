import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState, type FormEvent } from 'react'

import { catalogApi, subscriptionsApi, type PlanInput, type ServicePlan } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input, NativeSelect, Textarea } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

interface EditableRoute {
  modelId: string
  resourcePoolId: string
}

export function PlanForm({
  plan,
  open,
  onOpenChange,
}: {
  plan: ServicePlan | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const version = plan?.currentVersion
  const [name, setName] = useState(plan?.name ?? '')
  const [description, setDescription] = useState(plan?.description ?? '')
  const [routes, setRoutes] = useState<EditableRoute[]>(
    version?.routes.map(({ modelId, resourcePoolId }) => ({ modelId, resourcePoolId })) ?? [],
  )
  const pools = useQuery({
    queryKey: ['resource-pools', 'plan-form'],
    queryFn: ({ signal }) => catalogApi.resourcePools(false, signal),
    enabled: open,
  })
  const models = useMemo(() => {
    const byId = new Map<string, { id: string; name: string; publicName: string }>()
    for (const pool of pools.data ?? []) {
      for (const model of pool.models) {
        byId.set(model.id, {
          id: model.id,
          name: model.displayName,
          publicName: model.publicName,
        })
      }
    }
    return Array.from(byId.values()).sort((left, right) => left.name.localeCompare(right.name))
  }, [pools.data])

  const mutation = useMutation({
    mutationFn: () => {
      const input: PlanInput = {
        name: name.trim(),
        description: description.trim(),
        routes,
      }
      return subscriptionsApi.publishPlan(input, crypto.randomUUID(), plan?.id)
    },
    async onSuccess() {
      await queryClient.invalidateQueries({ queryKey: ['plans'] })
      onOpenChange(false)
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim() || routes.length === 0 || routes.some((route) => !route.resourcePoolId)) {
      return
    }
    mutation.mutate()
  }

  const locked = mutation.isPending
  return (
    <DialogFrame
      open={open}
      onOpenChange={(next) => !locked && onOpenChange(next)}
      title={plan ? '发布套餐新版本' : '创建并发布套餐'}
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
          <Button type="submit" form="plan-form" disabled={locked}>
            {locked ? '发布中' : '发布版本'}
          </Button>
        </>
      }
    >
      <form id="plan-form" className="form-grid" onSubmit={submit}>
        <Field label="套餐名称" htmlFor="plan-name">
          <Input
            id="plan-name"
            autoFocus
            required
            value={name}
            readOnly={locked}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field label="说明" htmlFor="plan-description" className="field--full">
          <Textarea
            id="plan-description"
            rows={3}
            value={description}
            readOnly={locked}
            onChange={(event) => setDescription(event.target.value)}
          />
        </Field>
        <fieldset className="choice-field field--full">
          <legend>套餐包含的模型</legend>
          <p className="choice-field__hint">勾选成员可用的模型，并指定请求只能使用哪个资源池</p>
          <div className="binding-grid">
            {models.map((model) => {
              const route = routes.find((item) => item.modelId === model.id)
              const modelPools = (pools.data ?? []).filter((pool) =>
                pool.models.some((item) => item.id === model.id),
              )
              return (
                <div className="binding-row" key={model.id}>
                  <label>
                    <input
                      type="checkbox"
                      checked={route !== undefined}
                      disabled={locked}
                      onChange={(event) =>
                        setRoutes((current) =>
                          event.target.checked
                            ? [
                                ...current,
                                { modelId: model.id, resourcePoolId: modelPools[0]?.id ?? '' },
                              ]
                            : current.filter((item) => item.modelId !== model.id),
                        )
                      }
                    />
                    <span>
                      {model.name}
                      <small className="table-subline">{model.publicName}</small>
                    </span>
                  </label>
                  <NativeSelect
                    aria-label={`${model.name} 资源池`}
                    value={route?.resourcePoolId ?? ''}
                    disabled={!route || locked}
                    onChange={(event) =>
                      setRoutes((current) =>
                        current.map((item) =>
                          item.modelId === model.id
                            ? { ...item, resourcePoolId: event.target.value }
                            : item,
                        ),
                      )
                    }
                  >
                    {modelPools.map((pool) => (
                      <option key={pool.id} value={pool.id}>
                        {pool.name}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
              )
            })}
          </div>
          {pools.isLoading ? (
            <p className="choice-field__empty">正在读取资源池模型</p>
          ) : models.length === 0 ? (
            <p className="choice-field__empty">当前没有可发布到套餐的资源池模型</p>
          ) : null}
          {models.length > 0 && routes.length === 0 ? (
            <span className="field__error">至少选择一个模型并指定资源池</span>
          ) : null}
        </fieldset>
        <FormProblem error={mutation.error ?? pools.error} />
      </form>
    </DialogFrame>
  )
}
