import { apiClient, listQuery } from './client'
import type { ListQuery, Page, RequestLog, RequestLogDetail } from './types'

const base = '/api/control'

export const usageApi = {
  requestLogs: (query: ListQuery, signal?: AbortSignal) =>
    apiClient.request<Page<RequestLog>>(`${base}/requests`, {
      query: listQuery(query),
      ...(signal ? { signal } : {}),
    }),
  requestLog: (requestId: string, signal?: AbortSignal) =>
    apiClient.request<RequestLogDetail>(`${base}/requests/${encodeURIComponent(requestId)}`, {
      ...(signal ? { signal } : {}),
    }),
}
