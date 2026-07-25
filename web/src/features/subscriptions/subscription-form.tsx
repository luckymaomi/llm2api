import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'

import { accessApi, subscriptionsApi, type Subscription } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { Field, Input, NativeSelect, Textarea } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'

export function SubscriptionForm({
  subscription,
  open,
  onOpenChange,
}: {
  subscription: Subscription | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [userId, setUserId] = useState(subscription?.userId ?? '')
  const [servicePlanId, setServicePlanId] = useState(subscription?.servicePlanId ?? '')
  const [startsAt, setStartsAt] = useState(() =>
    localDateTime(subscription?.startsAt ?? new Date().toISOString()),
  )
  const [expiresAt, setExpiresAt] = useState(() =>
    subscription?.expiresAt ? localDateTime(subscription.expiresAt) : '',
  )
  const [notes, setNotes] = useState(subscription?.notes ?? '')
  const members = useQuery({
    queryKey: ['members', 'subscription-form'],
    queryFn: ({ signal }) =>
      accessApi.members({ page: 1, pageSize: 200, status: 'active' }, signal),
    enabled: open && subscription === null,
  })
  const plans = useQuery({
    queryKey: ['plans', 'subscription-form'],
    queryFn: ({ signal }) => subscriptionsApi.plans(false, signal),
    enabled: open,
  })
  function selectPlan(planId: string) {
    setServicePlanId(planId)
  }

  const mutation = useMutation({
    mutationFn: () =>
      subscription
        ? subscriptionsApi.updateSubscription(
            subscription.id,
            {
              startsAt: new Date(startsAt).toISOString(),
              ...(expiresAt ? { expiresAt: new Date(expiresAt).toISOString() } : {}),
              notes: notes.trim(),
              expectedUpdatedAt: subscription.updatedAt,
            },
            crypto.randomUUID(),
          )
        : subscriptionsApi.createSubscription(
            {
              userId,
              servicePlanId,
              startsAt: new Date(startsAt).toISOString(),
              ...(expiresAt ? { expiresAt: new Date(expiresAt).toISOString() } : {}),
              notes: notes.trim(),
            },
            crypto.randomUUID(),
          ),
    async onSuccess() {
      await queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      await queryClient.invalidateQueries({ queryKey: ['plans'] })
      onOpenChange(false)
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (
      !userId ||
      !servicePlanId ||
      !startsAt ||
      (expiresAt && new Date(expiresAt) <= new Date(startsAt))
    )
      return
    mutation.mutate()
  }

  const locked = mutation.isPending
  return (
    <DialogFrame
      open={open}
      onOpenChange={(next) => !locked && onOpenChange(next)}
      title={subscription ? '调整订阅' : '分配订阅'}
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
          <Button type="submit" form="subscription-form" disabled={locked}>
            {locked ? '保存中' : '保存'}
          </Button>
        </>
      }
    >
      <form id="subscription-form" className="form-grid" onSubmit={submit}>
        <Field label="成员" htmlFor="subscription-member">
          <NativeSelect
            id="subscription-member"
            autoFocus
            required
            value={userId}
            disabled={locked || subscription !== null}
            onChange={(event) => setUserId(event.target.value)}
          >
            <option value="">选择成员</option>
            {(members.data?.items ?? [])
              .filter((member) => member.role === 'member')
              .map((member) => (
                <option key={member.id} value={member.id}>
                  {member.displayName} · {member.email}
                </option>
              ))}
            {subscription ? (
              <option value={subscription.userId}>
                {subscription.memberName} · {subscription.memberEmail}
              </option>
            ) : null}
          </NativeSelect>
        </Field>
        <Field label="套餐" htmlFor="subscription-plan">
          <NativeSelect
            id="subscription-plan"
            required
            value={servicePlanId}
            disabled={locked || subscription !== null}
            onChange={(event) => selectPlan(event.target.value)}
          >
            <option value="">选择套餐</option>
            {(plans.data ?? [])
              .filter((plan) => plan.status === 'active' && plan.currentVersion)
              .map((plan) => (
                <option key={plan.id} value={plan.id}>
                  {plan.name} · v{plan.currentVersion?.version}
                </option>
              ))}
          </NativeSelect>
        </Field>
        <Field label="开始时间" htmlFor="subscription-start">
          <Input
            id="subscription-start"
            type="datetime-local"
            required
            value={startsAt}
            readOnly={locked}
            onChange={(event) => setStartsAt(event.target.value)}
          />
        </Field>
        <Field label="到期时间" htmlFor="subscription-expiry" hint="留空表示永久有效">
          <Input
            id="subscription-expiry"
            type="datetime-local"
            value={expiresAt}
            readOnly={locked}
            onChange={(event) => setExpiresAt(event.target.value)}
          />
        </Field>
        <Field
          label="内部备注"
          htmlFor="subscription-notes"
          className="field--full"
          hint="仅管理员可见"
        >
          <Textarea
            id="subscription-notes"
            rows={3}
            value={notes}
            readOnly={locked}
            onChange={(event) => setNotes(event.target.value)}
          />
        </Field>
        <FormProblem error={mutation.error ?? members.error ?? plans.error} />
      </form>
    </DialogFrame>
  )
}

function localDateTime(value: string): string {
  const date = new Date(value)
  const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return shifted.toISOString().slice(0, 16)
}
