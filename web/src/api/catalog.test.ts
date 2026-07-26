import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { server } from '@/test/server'

import { catalogApi } from './catalog'

describe('Provider capability catalog contract', () => {
  it('maps the control API snake_case Provider contract into the page model', async () => {
    server.use(
      http.get('http://llm2api.test/api/control/providers', () =>
        HttpResponse.json({
          data: [
            {
              id: 'provider-kimi',
              catalog_id: 'kimi',
              slug: 'kimi',
              name: 'Kimi',
              kind: 'kimi',
              base_url: 'https://api.moonshot.cn/v1',
              source_url: 'https://platform.kimi.com/docs/api/overview',
              verified_at: '2026-07-26T00:00:00Z',
              contract: {
                reference_url: 'https://platform.kimi.com/docs/api/chat',
                contract_snapshot: '2026-07-26',
                verified_at: '2026-07-26',
                reference_provider: 'Moonshot AI',
                verified_models: ['kimi-k3', 'kimi-k2.6'],
                live_capabilities: ['chat', 'tools'],
                status: 'verified',
              },
              models: [
                {
                  upstream_name: 'kimi-k2.6',
                  display_name: 'Kimi K2.6',
                  capabilities: {
                    chat: true,
                    streaming: true,
                    tools: true,
                    reasoning: true,
                    structured_output: true,
                    context_tokens: 256000,
                    output_tokens: 32768,
                    parameters: {
                      max_completion_tokens: { supported: true, minimum: 1 },
                      temperature: { supported: true, exact_values: [0.6, 1] },
                      top_p: { supported: true, exact_values: [0.95] },
                      presence_penalty: { supported: true, exact_values: [0] },
                      frequency_penalty: { supported: true, exact_values: [0] },
                      n: { supported: true, exact_values: [1] },
                      top_k: { supported: false },
                      thinking_budget: { supported: false },
                    },
                  },
                },
              ],
              resource_pool_count: 1,
              active_credential_count: 2,
              created_at: '2026-07-26T00:00:00Z',
              updated_at: '2026-07-26T00:00:00Z',
            },
          ],
        }),
      ),
    )

    const providers = await catalogApi.providers()
    expect(providers).toHaveLength(1)
    const provider = providers[0]
    if (!provider) throw new Error('Provider catalog response did not contain Kimi.')
    expect(provider).toMatchObject({
      catalogId: 'kimi',
      contract: {
        referenceUrl: 'https://platform.kimi.com/docs/api/chat',
        contractSnapshot: '2026-07-26',
        verifiedAt: '2026-07-26',
        referenceProvider: 'Moonshot AI',
        verifiedModels: ['kimi-k3', 'kimi-k2.6'],
        liveCapabilities: ['chat', 'tools'],
        status: 'verified',
      },
    })
    expect(provider.models[0]?.capabilities.parameters.temperature.exactValues).toEqual([0.6, 1])
  })

  it('maps the required credential capacity projection', async () => {
    server.use(
      http.get('http://llm2api.test/api/control/credentials', () =>
        HttpResponse.json({
          data: [
            {
              id: 'credential-kimi-a',
              resource_pool_id: 'pool-kimi',
              resource_pool_name: 'Kimi shared account',
              resource_pool_slug: 'kimi-shared-account',
              provider_id: 'provider-kimi',
              provider_name: 'Kimi',
              provider_kind: 'kimi',
              provider_base_url: 'https://api.moonshot.cn/v1',
              name: 'Kimi account A',
              status: 'active',
              health_status: 'healthy',
              capacity: { state: 'unavailable', scope: 'gateway_credential' },
              consecutive_failures: 0,
              created_at: '2026-07-26T00:00:00Z',
              updated_at: '2026-07-26T00:00:00Z',
              model_bindings: [],
            },
          ],
        }),
      ),
    )

    const credentials = await catalogApi.credentials()
    expect(credentials).toHaveLength(1)
    expect(credentials[0]?.capacity).toEqual({ state: 'unavailable', scope: 'gateway_credential' })
  })

  it('decodes the created credential returned by the full batch-import action', async () => {
    server.use(
      http.post('http://llm2api.test/api/control/credentials/batch', () =>
        HttpResponse.json({
          data: [
            {
              line: 1,
              name: 'Agnes primary',
              status: 'created',
              credential: {
                id: 'credential-agnes-primary',
                resource_pool_id: 'pool-agnes',
                resource_pool_name: 'Agnes shared account',
                resource_pool_slug: 'agnes-shared-account',
                provider_id: 'provider-agnes',
                provider_name: 'Agnes',
                provider_kind: 'agnes',
                provider_base_url: 'https://apihub.agnes-ai.com/v1',
                name: 'Agnes primary',
                status: 'active',
                health_status: 'healthy',
                capacity: { state: 'observed', scope: 'gateway_credential' },
                consecutive_failures: 0,
                created_at: '2026-07-26T00:00:00Z',
                updated_at: '2026-07-26T00:00:00Z',
                model_bindings: [{ model_id: 'model-agnes-flash', model_name: 'agnes-2.0-flash' }],
              },
            },
          ],
        }),
      ),
    )

    const results = await catalogApi.importCredentials(
      {
        resourcePoolId: 'pool-agnes',
        items: [{ name: 'Agnes primary', secret: 'fixture-secret' }],
      },
      '00000000-0000-4000-8000-000000000001',
    )
    expect(results[0]).toMatchObject({
      line: 1,
      status: 'created',
      credential: {
        name: 'Agnes primary',
        capacity: { state: 'observed', scope: 'gateway_credential' },
        modelBindings: [{ modelName: 'agnes-2.0-flash' }],
      },
    })
  })
})
