import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'

import {
  accessApi,
  catalogApi,
  subscriptionsApi,
  type CreatedGatewayKey,
  type GatewayKeyRoute,
} from '@/api'
import { useSession } from '@/app/session'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input, NativeSelect } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

export function KeyForm({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const session = useSession()
  const [ownerId, setOwnerId] = useState('')
  const [name, setName] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [routes, setRoutes] = useState<GatewayKeyRoute[]>([])
  const [created, setCreated] = useState<CreatedGatewayKey>()
  const [copied, setCopied] = useState(false)
  const [operationKey, setOperationKey] = useState('')
  const members = useQuery({
    queryKey: ['members', 'key-form'],
    queryFn: ({ signal }) =>
      accessApi.members({ page: 1, pageSize: 100, status: 'active' }, signal),
    enabled: open && session.role === 'administrator',
  })
  const effectiveOwnerId = session.role === 'member' ? session.userId : ownerId
  const ownsRouteAccess = session.role === 'administrator' && effectiveOwnerId === session.userId
  const subscriptions = useQuery({
    queryKey: ['subscriptions', 'key-form', effectiveOwnerId],
    queryFn: ({ signal }) =>
      subscriptionsApi.subscriptions(
        { page: 1, pageSize: 100, userId: effectiveOwnerId, status: 'active' },
        signal,
      ),
    enabled: open && Boolean(effectiveOwnerId) && !ownsRouteAccess,
  })
  const pools = useQuery({
    queryKey: ['resource-pools', 'key-form'],
    queryFn: ({ signal }) => catalogApi.resourcePools(false, signal),
    enabled: open && ownsRouteAccess,
  })
  const availableRoutes = useMemo(() => {
    const grouped = new Map<string, GatewayKeyRoute[]>()
    const add = (route: GatewayKeyRoute) => {
      const alternatives = grouped.get(route.modelId) ?? []
      if (!alternatives.some((item) => item.resourcePoolId === route.resourcePoolId)) {
        alternatives.push(route)
        grouped.set(route.modelId, alternatives)
      }
    }
    if (ownsRouteAccess) {
      for (const pool of pools.data ?? []) {
        if (pool.status !== 'active') continue
        for (const model of pool.models) {
          add({
            modelId: model.id,
            modelName: model.publicName,
            resourcePoolId: pool.id,
            resourcePoolName: pool.name,
          })
        }
      }
    } else {
      for (const subscription of subscriptions.data?.items ?? []) {
        subscription.routes.forEach(add)
      }
    }
    return Array.from(grouped.values())
  }, [ownsRouteAccess, pools.data, subscriptions.data?.items])
  const mutation = useMutation({
    mutationFn: () =>
      accessApi.createKey(
        {
          ownerId: effectiveOwnerId,
          name: name.trim(),
          routes: routes.map(({ modelId, resourcePoolId }) => ({ modelId, resourcePoolId })),
          ...(expiresAt ? { expiresAt: new Date(expiresAt).toISOString() } : {}),
        },
        operationKey || crypto.randomUUID(),
      ),
    async onSuccess(result) {
      setCreated(result)
      await queryClient.invalidateQueries({ queryKey: ['gateway-keys'] })
    },
  })

  function close() {
    if (mutation.isPending) return
    setOwnerId('')
    setName('')
    setExpiresAt('')
    setRoutes([])
    setCreated(undefined)
    setCopied(false)
    setOperationKey('')
    mutation.reset()
    onOpenChange(false)
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!effectiveOwnerId || !name.trim() || routes.length === 0) return
    if (!operationKey) setOperationKey(crypto.randomUUID())
    mutation.mutate()
  }

  const gatewayBaseURL = `${window.location.origin}/v1`
  return (
    <DialogFrame
      open={open}
      onOpenChange={(next) => !next && close()}
      title={created ? 'API 密钥已创建' : '创建 API 密钥'}
      dismissible={!mutation.isPending}
      footer={
        created ? (
          <Button onClick={close}>完成</Button>
        ) : (
          <>
            <Button type="button" variant="secondary" disabled={mutation.isPending} onClick={close}>
              取消
            </Button>
            <Button type="submit" form="key-form" disabled={mutation.isPending}>
              {mutation.isPending ? '创建中' : '创建'}
            </Button>
          </>
        )
      }
    >
      {created ? (
        <div className="one-time-result">
          <div className="secret-reveal">
            <code>{created.secret}</code>
            <Button
              variant="secondary"
              icon={copied ? <Check size={16} /> : <Copy size={16} />}
              onClick={() =>
                void navigator.clipboard
                  .writeText(`OPENAI_BASE_URL=${gatewayBaseURL}\nOPENAI_API_KEY=${created.secret}`)
                  .then(() => setCopied(true))
              }
            >
              {copied ? '已复制' : '复制调用配置'}
            </Button>
          </div>
          <dl className="fact-list">
            <div>
              <dt>Base URL</dt>
              <dd>
                <code>{gatewayBaseURL}</code>
              </dd>
            </div>
            <div>
              <dt>名称</dt>
              <dd>{created.key.name}</dd>
            </div>
            <div>
              <dt>授权路由</dt>
              <dd>{created.key.routes.map(routeLabel).join('、')}</dd>
            </div>
          </dl>
          <ConnectionExamples
            baseURL={gatewayBaseURL}
            secret={created.secret}
            {...(created.key.routes[0] ? { route: created.key.routes[0] } : {})}
          />
        </div>
      ) : (
        <form id="key-form" className="form-grid" onSubmit={submit}>
          {session.role === 'administrator' ? (
            <Field label="所属账号" htmlFor="key-owner">
              <NativeSelect
                id="key-owner"
                autoFocus
                required
                value={ownerId}
                disabled={mutation.isPending}
                onChange={(event) => {
                  setOwnerId(event.target.value)
                  setRoutes([])
                }}
              >
                <option value="">请选择</option>
                <option value={session.userId}>我自己（管理员）</option>
                {(members.data?.items ?? [])
                  .filter((member) => member.role === 'member')
                  .map((member) => (
                    <option key={member.id} value={member.id}>
                      {member.displayName} · {member.email}
                    </option>
                  ))}
              </NativeSelect>
            </Field>
          ) : (
            <Field label="所属账号" htmlFor="key-owner">
              <Input id="key-owner" autoFocus value={session.displayName} readOnly />
            </Field>
          )}
          <Field label="名称" htmlFor="key-name">
            <Input
              id="key-name"
              required
              value={name}
              readOnly={mutation.isPending}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <fieldset className="choice-field field--full">
            <legend>授权路由</legend>
            <div className="choice-grid choice-grid--routes">
              {availableRoutes.map((alternatives) => {
                const model = alternatives[0]!
                const selected = routes.find((route) => route.modelId === model.modelId)
                return (
                  <label key={model.modelId}>
                    <input
                      type="checkbox"
                      checked={Boolean(selected)}
                      disabled={mutation.isPending}
                      onChange={(event) =>
                        setRoutes((current) =>
                          event.target.checked
                            ? [...current, model]
                            : current.filter((route) => route.modelId !== model.modelId),
                        )
                      }
                    />
                    <span>{model.modelName}</span>
                    {selected && alternatives.length > 1 ? (
                      <NativeSelect
                        value={selected.resourcePoolId}
                        disabled={mutation.isPending}
                        onChange={(event) => {
                          const replacement = alternatives.find(
                            (route) => route.resourcePoolId === event.target.value,
                          )
                          if (replacement)
                            setRoutes((current) =>
                              current.map((route) =>
                                route.modelId === model.modelId ? replacement : route,
                              ),
                            )
                        }}
                      >
                        {alternatives.map((route) => (
                          <option key={route.resourcePoolId} value={route.resourcePoolId}>
                            {route.resourcePoolName}
                          </option>
                        ))}
                      </NativeSelect>
                    ) : selected ? (
                      <small>{selected.resourcePoolName}</small>
                    ) : null}
                  </label>
                )
              })}
            </div>
            {!effectiveOwnerId ? (
              <p className="choice-field__empty">选择账号后显示可用路由</p>
            ) : null}
            {subscriptions.isLoading || pools.isLoading ? (
              <p className="choice-field__empty">正在读取可用路由</p>
            ) : null}
            {effectiveOwnerId &&
            !subscriptions.isLoading &&
            !pools.isLoading &&
            availableRoutes.length === 0 ? (
              <p className="choice-field__empty">当前没有可用于新 API 密钥的有效路由</p>
            ) : null}
            {effectiveOwnerId && availableRoutes.length > 0 && routes.length === 0 ? (
              <span className="field__error">至少选择一个模型和资源池</span>
            ) : null}
          </fieldset>
          <Field label="到期时间" htmlFor="key-expiry" hint="留空表示不单独设置密钥到期时间">
            <Input
              id="key-expiry"
              type="datetime-local"
              value={expiresAt}
              readOnly={mutation.isPending}
              onChange={(event) => setExpiresAt(event.target.value)}
            />
          </Field>
          <FormProblem
            error={mutation.error ?? members.error ?? subscriptions.error ?? pools.error}
          />
        </form>
      )}
    </DialogFrame>
  )
}

function routeLabel(route: GatewayKeyRoute): string {
  return `${route.modelName} · ${route.resourcePoolName}`
}

function ConnectionExamples({
  baseURL,
  secret,
  route,
}: {
  baseURL: string
  secret: string
  route?: GatewayKeyRoute
}) {
  const model = route?.modelName ?? 'your-model'
  const curl = `curl ${baseURL}/chat/completions -H "Authorization: Bearer ${secret}" -H "Content-Type: application/json" -d '{"model":"${model}","messages":[{"role":"user","content":"Hello"}]}'`
  const python = `from openai import OpenAI\nclient = OpenAI(base_url="${baseURL}", api_key="${secret}")\nprint(client.chat.completions.create(model="${model}", messages=[{"role": "user", "content": "Hello"}]))`
  const node = `import OpenAI from "openai"\nconst client = new OpenAI({ baseURL: "${baseURL}", apiKey: "${secret}" })\nconsole.log(await client.chat.completions.create({ model: "${model}", messages: [{ role: "user", content: "Hello" }] }))`
  return (
    <details className="connection-examples">
      <summary>调用示例</summary>
      <h4>cURL</h4>
      <pre>
        <code>{curl}</code>
      </pre>
      <h4>Python</h4>
      <pre>
        <code>{python}</code>
      </pre>
      <h4>Node.js</h4>
      <pre>
        <code>{node}</code>
      </pre>
    </details>
  )
}
