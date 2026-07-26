import { useMutation, useQueryClient } from '@tanstack/react-query'

import { catalogApi, type Credential, type CredentialStatus } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { FormProblem } from '@/features/auth/form-problem'

export type CredentialOperation =
  | { kind: 'set-status'; credential: Credential; status: CredentialStatus }
  | { kind: 'retire'; credential: Credential }

export function CredentialOperationDialog({
  operation,
  onOpenChange,
}: {
  operation: CredentialOperation | null
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: (current: CredentialOperation) => {
      if (current.kind === 'retire') {
        return catalogApi.retireCredential(
          current.credential.id,
          current.credential.updatedAt,
          crypto.randomUUID(),
        )
      }
      return catalogApi.setCredentialStatus(
        current.credential.id,
        current.status,
        current.credential.updatedAt,
        crypto.randomUUID(),
      )
    },
    async onSuccess() {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['credentials'] }),
        queryClient.invalidateQueries({ queryKey: ['resource-pools'] }),
      ])
      onOpenChange(false)
    },
  })
  const credential = operation?.credential
  const nextStatus = operation?.kind === 'retire' ? 'retired' : operation?.status
  const isRetiring = operation?.kind === 'retire'

  function close(): void {
    if (mutation.isPending) return
    mutation.reset()
    onOpenChange(false)
  }

  return (
    <DialogFrame
      open={operation !== null}
      onOpenChange={(open) => {
        if (!open) close()
      }}
      title={isRetiring ? '退役上游 API Key' : '更改上游 API Key 状态'}
      description={
        isRetiring
          ? '退役会立即移出调度，历史请求与账本引用会保留。'
          : '状态变更会立即影响新请求的候选资格。'
      }
      width="md"
      dismissible={!mutation.isPending}
      footer={
        <>
          <Button type="button" variant="secondary" disabled={mutation.isPending} onClick={close}>
            取消
          </Button>
          <Button
            type="button"
            variant={isRetiring ? 'danger' : 'primary'}
            disabled={mutation.isPending || !operation}
            onClick={() => operation && mutation.mutate(operation)}
          >
            {mutation.isPending ? '提交中' : isRetiring ? '确认退役' : '确认更改'}
          </Button>
        </>
      }
    >
      <dl className="fact-list credential-operation-facts">
        <div>
          <dt>上游 API Key</dt>
          <dd>{credential?.name}</dd>
        </div>
        <div>
          <dt>资源池</dt>
          <dd>{credential?.resourcePoolName}</dd>
        </div>
        <div>
          <dt>上游平台</dt>
          <dd>{credential?.providerName}</dd>
        </div>
        <div>
          <dt>当前状态</dt>
          <dd>{credential ? credentialStatusLabel(credential.status) : ''}</dd>
        </div>
        <div>
          <dt>提交后状态</dt>
          <dd>{nextStatus ? credentialStatusLabel(nextStatus) : ''}</dd>
        </div>
        <div>
          <dt>影响范围</dt>
          <dd>{isRetiring ? '调度与凭据读取' : '新请求调度'}</dd>
        </div>
      </dl>
      <FormProblem error={mutation.error} />
    </DialogFrame>
  )
}

function credentialStatusLabel(status: CredentialStatus): string {
  switch (status) {
    case 'active':
      return '可用'
    case 'disabled':
      return '已停用'
    case 'retired':
      return '已退役'
  }
}
