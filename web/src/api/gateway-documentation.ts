import { ApiProblem } from './client'

export interface GatewayDocumentation {
  baseURL: string
  agentIndexURL: string
  openAPIURL: string
}

interface OpenAPIWire {
  servers?: Array<{ url?: unknown }>
}

export const gatewayDocumentationApi = {
  async get(signal?: AbortSignal): Promise<GatewayDocumentation> {
    let response: Response
    try {
      response = await fetch('/openapi.json', {
        headers: { Accept: 'application/json' },
        ...(signal ? { signal } : {}),
      })
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') throw error
      throw unavailable(0, '无法读取 Gateway 的 OpenAPI 文档。')
    }
    if (!response.ok) throw unavailable(response.status, 'Gateway 的 OpenAPI 文档暂不可用。')
    let document: OpenAPIWire
    try {
      document = (await response.json()) as OpenAPIWire
    } catch {
      throw unavailable(response.status, 'Gateway 返回的 OpenAPI 文档无效。')
    }
    const rawURL = document.servers?.[0]?.url
    if (typeof rawURL !== 'string')
      throw unavailable(response.status, 'Gateway 未声明可调用的 API 地址。')
    let baseURL: URL
    try {
      baseURL = new URL(rawURL)
    } catch {
      throw unavailable(response.status, 'Gateway 声明的 API 地址无效。')
    }
    if (
      (baseURL.protocol !== 'http:' && baseURL.protocol !== 'https:') ||
      baseURL.pathname !== '/v1' ||
      baseURL.search ||
      baseURL.hash
    ) {
      throw unavailable(
        response.status,
        'Gateway 声明的 API 地址不是有效的 OpenAI-compatible Base URL。',
      )
    }
    return {
      baseURL: baseURL.toString().replace(/\/$/, ''),
      agentIndexURL: new URL('/llms.txt', baseURL.origin).toString(),
      openAPIURL: new URL('/openapi.json', baseURL.origin).toString(),
    }
  },
}

function unavailable(status: number, message: string): ApiProblem {
  return new ApiProblem({
    status,
    code: 'gateway_documentation_unavailable',
    message,
    retryable: true,
  })
}
