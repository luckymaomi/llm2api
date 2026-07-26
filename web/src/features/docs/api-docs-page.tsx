import { useQuery } from '@tanstack/react-query'
import { Check, Copy, ExternalLink } from 'lucide-react'
import { useMemo, useState } from 'react'

import { accessApi, gatewayDocumentationApi } from '@/api'
import { Page, PageHeader } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { FormProblem } from '@/features/auth/form-problem'
import { formatNumber } from '@/lib/format'

type ExampleLanguage = 'curl' | 'python' | 'node'

export function APIDocsPage() {
  const [language, setLanguage] = useState<ExampleLanguage>('curl')
  const [copied, setCopied] = useState<string>()
  const gatewayDocumentation = useQuery({
    queryKey: ['gateway-documentation'],
    queryFn: ({ signal }) => gatewayDocumentationApi.get(signal),
  })
  const keys = useQuery({
    queryKey: ['gateway-keys', 'api-docs'],
    queryFn: ({ signal }) => accessApi.keys({ page: 1, pageSize: 100, status: 'active' }, signal),
  })
  const models = useMemo(() => {
    const unique = new Map<string, string>()
    for (const key of keys.data?.items ?? []) {
      for (const route of key.routes) unique.set(route.modelId, route.modelName)
    }
    return Array.from(unique, ([id, name]) => ({ id, name })).sort((left, right) =>
      left.name.localeCompare(right.name),
    )
  }, [keys.data?.items])
  const model = models[0]?.name ?? 'your-model'
  const endpoint = gatewayDocumentation.data
  const examples = endpoint ? exampleSource(endpoint.baseURL, model) : undefined

  async function copy(value: string, id: string) {
    await navigator.clipboard.writeText(value)
    setCopied(id)
  }

  if (!endpoint || !examples) {
    return (
      <Page className="api-docs-page">
        <PageHeader
          title="接口文档"
          description="正在读取 Gateway 的真实 OpenAI-compatible 地址。"
        />
        <FormProblem error={gatewayDocumentation.error} />
        {gatewayDocumentation.isLoading ? (
          <p className="api-docs-muted">正在读取 Gateway 文档。</p>
        ) : null}
        {gatewayDocumentation.error ? (
          <Button variant="secondary" onClick={() => void gatewayDocumentation.refetch()}>
            重新读取
          </Button>
        ) : null}
      </Page>
    )
  }

  return (
    <Page className="api-docs-page">
      <PageHeader
        title="接口文档"
        description="使用 API 密钥调用 LLM2API 的 OpenAI-compatible 接口。"
        actions={
          <Button
            variant="secondary"
            icon={<ExternalLink size={16} />}
            onClick={() => window.open(endpoint.agentIndexURL, '_blank', 'noopener,noreferrer')}
          >
            Agent 文档索引
          </Button>
        }
      />

      <div className="api-docs-layout">
        <nav className="api-docs-nav" aria-label="接口文档目录">
          <a href="#quickstart">快速接入</a>
          <a href="#models">模型列表</a>
          <a href="#chat">对话接口</a>
          <a href="#stream">流式响应</a>
          <a href="#responses">Responses</a>
          <a href="#agents">Agent 配置</a>
          <a href="#errors">错误处理</a>
        </nav>

        <div className="api-docs-content">
          <section id="quickstart" className="api-docs-section">
            <h2>快速接入</h2>
            <dl className="api-docs-facts">
              <div>
                <dt>Base URL</dt>
                <dd>
                  <code>{endpoint.baseURL}</code>
                </dd>
              </div>
              <div>
                <dt>认证</dt>
                <dd>
                  <code>Authorization: Bearer $LLM2API_API_KEY</code>
                </dd>
              </div>
              <div>
                <dt>协议</dt>
                <dd>OpenAI Chat Completions</dd>
              </div>
            </dl>
            <p>
              在“API 密钥”创建一把下游 API
              密钥并保存在服务端环境变量中。模型名称由这把密钥的授权路由决定；调用前先读取模型列表，不要猜测或写死上游模型。
            </p>
          </section>

          <section id="models" className="api-docs-section">
            <h2>模型列表</h2>
            <Endpoint method="GET" path="/models" />
            <p>返回当前 API 密钥可以调用的全部模型。套餐、密钥或路由变更后，以这个接口为准。</p>
            <CodeBlock id="models-curl" value={examples.models} copied={copied} onCopy={copy} />
            {keys.isLoading ? (
              <p className="api-docs-muted">正在读取当前密钥可调用的模型。</p>
            ) : null}
            {!keys.isLoading && models.length === 0 ? (
              <p className="api-docs-muted">
                创建 API 密钥并分配模型路由后，这里会显示可调用模型。
              </p>
            ) : null}
            {models.length > 0 ? (
              <div className="api-model-list" aria-label="当前可调用模型">
                <span>{formatNumber(models.length)} 个当前可调用模型</span>
                {models.map((item) => (
                  <code key={item.id}>{item.name}</code>
                ))}
              </div>
            ) : null}
          </section>

          <section id="chat" className="api-docs-section">
            <h2>对话接口</h2>
            <Endpoint method="POST" path="/chat/completions" />
            <p>
              请求体至少包含 <code>model</code> 和 <code>messages</code>。响应遵循 Chat Completions
              结构，<code>usage</code> 返回实际输入和输出 Token。
            </p>
            <ExampleTabs language={language} onChange={setLanguage} />
            <CodeBlock
              id={`chat-${language}`}
              value={examples[language]}
              copied={copied}
              onCopy={copy}
            />
          </section>

          <section id="stream" className="api-docs-section">
            <h2>流式响应</h2>
            <Endpoint method="POST" path="/chat/completions" suffix="stream: true" />
            <p>
              设置 <code>stream: true</code> 后，服务返回 Server-Sent Events。持续读取{' '}
              <code>data:</code> 事件，收到 <code>[DONE]</code> 后关闭连接。
            </p>
            <CodeBlock id="stream-curl" value={examples.stream} copied={copied} onCopy={copy} />
          </section>

          <section id="responses" className="api-docs-section">
            <h2>Responses</h2>
            <Endpoint method="POST" path="/responses" />
            <p>
              支持 OpenAI Responses 请求。设置 <code>store: true</code> 可保存响应；设置{' '}
              <code>background: true</code> 会创建可查询、可取消的后台任务。
            </p>
            <div className="api-route-list">
              <code>GET /responses/&#123;response_id&#125;</code>
              <code>DELETE /responses/&#123;response_id&#125;</code>
              <code>GET /responses/&#123;response_id&#125;/input_items</code>
              <code>POST /responses/&#123;response_id&#125;/cancel</code>
            </div>
          </section>

          <section id="agents" className="api-docs-section">
            <h2>Agent 配置</h2>
            <p>
              支持 OpenAI-compatible 配置的 Agent 只需要 Base URL、API 密钥和模型名。让 Agent 先请求{' '}
              <code>/v1/models</code>，再从返回值中选择模型。
            </p>
            <CodeBlock id="agent-config" value={examples.agent} copied={copied} onCopy={copy} />
            <p className="api-docs-muted">
              机器可读索引：<code>{endpoint.agentIndexURL}</code>。OpenAPI：
              <code>{endpoint.openAPIURL}</code>。
            </p>
          </section>

          <section id="errors" className="api-docs-section">
            <h2>错误处理</h2>
            <div className="api-error-table" role="table" aria-label="常见错误处理">
              <div role="row" className="api-error-table__head">
                <span role="columnheader">状态</span>
                <span role="columnheader">含义</span>
                <span role="columnheader">处理</span>
              </div>
              <ErrorRow
                status="400"
                meaning="请求格式或参数无效"
                action="检查 model、messages 与请求体。"
              />
              <ErrorRow
                status="401"
                meaning="密钥缺失、无效或已删除"
                action="确认使用本网关签发的 Bearer 密钥。"
              />
              <ErrorRow
                status="403 / 404"
                meaning="密钥没有该模型或资源池路由"
                action="读取 /models，检查套餐和 API 密钥授权。"
              />
              <ErrorRow
                status="429"
                meaning="当前上游容量暂时不足"
                action="遵循 Retry-After，不并发重放已提交请求。"
              />
              <ErrorRow
                status="502 / 503"
                meaning="上游或网关暂时不可用"
                action="仅对尚未收到响应的幂等工作做有界重试。"
              />
            </div>
          </section>
        </div>
      </div>
    </Page>
  )
}

function Endpoint({ method, path, suffix }: { method: string; path: string; suffix?: string }) {
  return (
    <div className="api-endpoint">
      <strong>{method}</strong>
      <code>{path}</code>
      {suffix ? <span>{suffix}</span> : null}
    </div>
  )
}

function ExampleTabs({
  language,
  onChange,
}: {
  language: ExampleLanguage
  onChange: (language: ExampleLanguage) => void
}) {
  return (
    <div className="api-example-tabs" role="tablist" aria-label="调用示例语言">
      {(
        [
          ['curl', 'cURL'],
          ['python', 'Python'],
          ['node', 'Node.js'],
        ] as const
      ).map(([value, label]) => (
        <button
          key={value}
          type="button"
          role="tab"
          aria-selected={language === value}
          onClick={() => onChange(value)}
        >
          {label}
        </button>
      ))}
    </div>
  )
}

function CodeBlock({
  id,
  value,
  copied,
  onCopy,
}: {
  id: string
  value: string
  copied: string | undefined
  onCopy: (value: string, id: string) => Promise<void>
}) {
  return (
    <div className="api-code-block">
      <pre>
        <code>{value}</code>
      </pre>
      <Button
        variant="secondary"
        icon={copied === id ? <Check size={15} /> : <Copy size={15} />}
        onClick={() => void onCopy(value, id)}
      >
        {copied === id ? '已复制' : '复制'}
      </Button>
    </div>
  )
}

function ErrorRow({
  status,
  meaning,
  action,
}: {
  status: string
  meaning: string
  action: string
}) {
  return (
    <div role="row">
      <code role="cell">{status}</code>
      <span role="cell">{meaning}</span>
      <span role="cell">{action}</span>
    </div>
  )
}

function exampleSource(baseURL: string, model: string) {
  const authorization = 'Authorization: Bearer $LLM2API_API_KEY'
  return {
    models: [`curl ${baseURL}/models \\`, `  -H "${authorization}"`].join('\n'),
    curl: [
      `curl ${baseURL}/chat/completions \\`,
      `  -H "${authorization}" \\`,
      '  -H "Content-Type: application/json" \\',
      `  -d '{"model":"${model}","messages":[{"role":"user","content":"你好"}]}'`,
    ].join('\n'),
    python: `import os
from openai import OpenAI

client = OpenAI(
    base_url="${baseURL}",
    api_key=os.environ["LLM2API_API_KEY"],
)
response = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "你好"}],
)
print(response.choices[0].message.content)`,
    node: `import OpenAI from "openai"

const client = new OpenAI({
  baseURL: "${baseURL}",
  apiKey: process.env.LLM2API_API_KEY,
})
const response = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "你好" }],
})
console.log(response.choices[0].message.content)`,
    stream: [
      `curl --no-buffer ${baseURL}/chat/completions \\`,
      `  -H "${authorization}" \\`,
      '  -H "Content-Type: application/json" \\',
      `  -d '{"model":"${model}","stream":true,"messages":[{"role":"user","content":"你好"}]}'`,
    ].join('\n'),
    agent: `{
  "provider": "openai-compatible",
  "base_url": "${baseURL}",
  "api_key_env": "LLM2API_API_KEY",
  "models_endpoint": "${baseURL}/models",
  "model": "${model}"
}`,
  }
}
