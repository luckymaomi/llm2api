import { useMutation } from '@tanstack/react-query'
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'

import { accessApi, type GatewayKey } from '@/api'
import { Button } from '@/components/ui/button'
import { DialogFrame } from '@/components/ui/dialog'
import { FormProblem } from '@/features/auth/form-problem'

interface KeyRevealDialogProps {
  gatewayKey: GatewayKey | null
  onOpenChange: (open: boolean) => void
}

export function KeyRevealDialog({ gatewayKey, onOpenChange }: KeyRevealDialogProps) {
  const [secret, setSecret] = useState('')
  const [copied, setCopied] = useState(false)
  const [copyFailed, setCopyFailed] = useState(false)
  const mutation = useMutation({
    mutationFn: (keyID: string) => accessApi.revealKey(keyID),
    onSuccess: (result) => {
      setSecret(result.secret)
      setCopied(false)
      setCopyFailed(false)
    },
  })

  function close(): void {
    if (mutation.isPending) return
    mutation.reset()
    setSecret('')
    setCopied(false)
    setCopyFailed(false)
    onOpenChange(false)
  }

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(secret)
      setCopied(true)
      setCopyFailed(false)
    } catch {
      setCopyFailed(true)
    }
  }

  return (
    <DialogFrame
      open={gatewayKey !== null}
      onOpenChange={(open) => !open && close()}
      title={secret ? 'API 密钥' : '显示 API 密钥'}
      description={gatewayKey?.name ?? ''}
      dismissible={!mutation.isPending}
      footer={
        secret ? (
          <Button onClick={close}>完成</Button>
        ) : (
          <>
            <Button variant="secondary" disabled={mutation.isPending} onClick={close}>
              取消
            </Button>
            <Button
              disabled={mutation.isPending}
              onClick={() => gatewayKey && mutation.mutate(gatewayKey.id)}
            >
              {mutation.isPending ? '正在读取' : '显示密钥'}
            </Button>
          </>
        )
      }
    >
      {secret ? (
        <>
          <div className="secret-reveal">
            <code>{secret}</code>
            <Button
              variant="secondary"
              icon={copied ? <Check size={16} /> : <Copy size={16} />}
              onClick={() => void copy()}
            >
              {copied ? '已复制' : '复制 API 密钥'}
            </Button>
          </div>
          {copyFailed ? (
            <div className="inline-problem" role="alert">
              浏览器未允许写入剪贴板。
            </div>
          ) : null}
        </>
      ) : (
        <FormProblem error={mutation.error} />
      )}
    </DialogFrame>
  )
}
