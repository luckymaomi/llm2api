import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  Check,
  KeyRound,
  PackageCheck,
  ServerCog,
  UsersRound,
  WalletCards,
} from 'lucide-react'

import { operationsApi, type AdministratorOverview } from '@/api'
import { Page, PageHeader, PageSection } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { ErrorState, LoadingState } from '@/components/ui/state'

import {
  useOnboardingTour,
  type OnboardingRoute,
  type OnboardingTarget,
} from './onboarding-tour-context'

interface SetupStep {
  label: string
  description: string
  fields: Array<{
    name: string
    purpose: string
  }>
  to:
    | '/resource-pools'
    | '/provider-keys'
    | '/plans'
    | '/members'
    | '/subscriptions'
    | '/api-keys'
    | '/api-logs'
  ready: boolean
  icon: typeof ServerCog
  target: OnboardingTarget
}

export function GettingStartedPage() {
  const { startTour } = useOnboardingTour()
  const query = useQuery({
    queryKey: ['operations-overview'],
    queryFn: ({ signal }) => operationsApi.overview(signal),
  })
  if (query.isLoading)
    return (
      <Page>
        <LoadingState label="正在读取服务状态" />
      </Page>
    )
  if (query.error || !query.data || query.data.scope !== 'administrator')
    return (
      <Page>
        <ErrorState
          error={query.error ?? new Error('当前账号不能访问管理员新手指引')}
          onRetry={() => void query.refetch()}
        />
      </Page>
    )

  const steps = setupSteps(query.data)
  const next = steps.find((step) => !step.ready)
  const completed = steps.filter((step) => step.ready).length
  return (
    <Page className="onboarding-home">
      <PageHeader
        eyebrow={`${completed} / ${steps.length} 项已完成`}
        title="新手指引"
        description={next ? `当前任务：${next.label}` : '基础服务已经可以使用。'}
        actions={
          next ? (
            <Button icon={<next.icon size={16} />} onClick={() => startTour(next.target)}>
              引导我完成
            </Button>
          ) : (
            <Button asChild variant="secondary" icon={<Activity size={16} />}>
              <Link to="/operations">查看运行状态</Link>
            </Button>
          )
        }
      />
      <PageSection title="这个网关如何工作">
        <div className="onboarding-overview">
          <p>
            每一把上游 API Key 都像一个小水池，拥有自己的模型、限流和可用额度。LLM2API
            把同一资源池内支持相同模型的小水池汇成一个大水池，在池内选择健康且有容量的
            Key，让调用入口更稳定，并充分利用每把 Key 的可用额度。
          </p>
          <dl className="onboarding-terms">
            <Concept
              term="Provider"
              description="提供模型 API 的上游平台，例如智谱 GLM 或 Agnes。"
            />
            <Concept
              term="上游 API Key"
              description="网关调用 Provider 的凭据，也就是一个小水池。"
            />
            <Concept
              term="资源池"
              description="同一 Provider 的多把上游 Key 组成的大水池，也是调度边界。"
            />
            <Concept term="套餐" description="准备提供给成员的“模型 + 资源池”路由集合。" />
            <Concept
              term="订阅"
              description="成员在指定有效期内使用某个套餐的资格；到期留空表示永久有效。"
            />
            <Concept
              term="API 密钥"
              description="管理员或成员交给应用调用本网关的密钥，不是上游 API Key。"
            />
          </dl>
        </div>
      </PageSection>
      <PageSection title="按顺序完成配置">
        <div className="onboarding-path" aria-label="首次配置进度">
          {steps.map((step, index) => (
            <div
              className="onboarding-phase"
              data-current={next === step}
              data-ready={step.ready}
              key={`${step.to}-${step.label}`}
            >
              <span className="onboarding-phase__index">{String(index + 1).padStart(2, '0')}</span>
              <span className="onboarding-phase__icon" aria-hidden="true">
                <step.icon size={21} />
              </span>
              <div>
                <strong>{step.label}</strong>
                <span>{step.description}</span>
                <dl className="onboarding-phase__fields">
                  {step.fields.map((field) => (
                    <div key={field.name}>
                      <dt>{field.name}</dt>
                      <dd>{field.purpose}</dd>
                    </div>
                  ))}
                </dl>
              </div>
              <span className="onboarding-phase__status">
                {step.ready ? (
                  <>
                    <Check size={14} /> 已完成
                  </>
                ) : next === step ? (
                  '当前任务'
                ) : (
                  '待完成'
                )}
              </span>
            </div>
          ))}
        </div>
      </PageSection>
    </Page>
  )
}

function Concept({ term, description }: { term: string; description: string }) {
  return (
    <div>
      <dt>{term}</dt>
      <dd>{description}</dd>
    </div>
  )
}

function setupSteps(overview: AdministratorOverview): SetupStep[] {
  const resources = overview.resources
  return [
    {
      label: '创建资源池',
      description: '选择上游平台，建立不会跨 Provider 的调度边界。',
      fields: [
        { name: '上游平台', purpose: '决定调用哪家 Provider，以及采用哪套上游协议。' },
        {
          name: '资源池名称',
          purpose: '方便人在页面中识别这个大水池；资源池可同时服务多个套餐和成员。',
        },
      ],
      to: '/resource-pools',
      ready: resources.activeResourcePoolCount > 0,
      icon: ServerCog,
      target: target('/resource-pools', '创建资源池', 'create-resource-pool'),
    },
    {
      label: '添加并探测上游 API Key',
      description: '添加一把或多把 Key，并读取每把 Key 当前支持的模型。',
      fields: [
        { name: '资源池', purpose: '决定这些 Key 加入哪个大水池，只在该池内参与调度。' },
        { name: '名称', purpose: '用于区分同一资源池里的多把 Key，可以留空。' },
        { name: '上游 API Key', purpose: 'Provider 签发的真实调用凭据；系统不会再次回显。' },
        { name: '支持模型', purpose: '无需手填，系统探测每把 Key 后自动保存并汇总。' },
      ],
      to: '/provider-keys',
      ready: resources.activeCredentialCount > 0 && resources.successfulCredentialProbeCount > 0,
      icon: KeyRound,
      target: target('/provider-keys', '添加并探测上游 API Key', 'create-provider-key'),
    },
    {
      label: '发布套餐',
      description: '把可以提供给成员的模型和资源池路由发布为套餐版本。',
      fields: [
        { name: '套餐名称', purpose: '让管理员和成员识别这组服务权限。' },
        { name: '说明', purpose: '说明套餐用途或适用对象，可以留空。' },
        { name: '模型与资源池', purpose: '固定每个模型使用哪个资源池，调用时不会跨池。' },
      ],
      to: '/plans',
      ready: resources.activeServicePlanCount > 0,
      icon: PackageCheck,
      target: target('/plans', '发布套餐', 'create-plan'),
    },
    {
      label: '创建成员',
      description: '创建成员账号，并保存只显示一次的初始密码。',
      fields: [
        { name: '显示名称', purpose: '用于控制台、订阅和用量记录中识别成员。' },
        { name: '邮箱', purpose: '成员登录控制台时使用的唯一账号。' },
        { name: '初始密码', purpose: '由系统自动生成且只显示一次，需要交给成员保存。' },
      ],
      to: '/members',
      ready: resources.activeMemberCount > 0,
      icon: UsersRound,
      target: target('/members', '创建成员', 'create-member'),
    },
    {
      label: '分配订阅',
      description: '给成员分配套餐，可设置到期时间或永久有效。',
      fields: [
        { name: '成员', purpose: '决定把调用资格分配给谁。' },
        { name: '套餐', purpose: '决定该成员可以使用哪些模型和资源池。' },
        { name: '开始与到期时间', purpose: '决定资格何时生效；到期留空表示永久有效。' },
        { name: '内部备注', purpose: '仅供管理员记录分配原因，成员不可见。' },
      ],
      to: '/subscriptions',
      ready: resources.activeSubscriptionCount > 0,
      icon: WalletCards,
      target: target('/subscriptions', '分配订阅', 'create-subscription'),
    },
    {
      label: '创建 API 密钥',
      description: '为管理员或成员创建用于调用网关的下游 API 密钥。',
      fields: [
        { name: '所属账号', purpose: '决定密钥归谁使用，以及请求和用量记录到谁名下。' },
        { name: '名称', purpose: '用于区分成员的不同应用或设备。' },
        { name: '授权路由', purpose: '限定这把密钥允许调用的模型及其资源池。' },
        { name: '到期时间', purpose: '控制密钥有效期；留空表示不单独到期。' },
      ],
      to: '/api-keys',
      ready: resources.activeApiKeyCount > 0,
      icon: KeyRound,
      target: target('/api-keys', '创建 API 密钥', 'create-api-key'),
    },
    {
      label: '验证统一 API',
      description: '选择已授权模型发起一次测试，确认完整调用链可用。',
      fields: [
        { name: '模型', purpose: '从这把 API 密钥已经获得授权的模型中选择。' },
        { name: '测试消息', purpose: '发送给上游的最小验证内容，请求会留下真实用量记录。' },
      ],
      to: '/api-keys',
      ready: resources.hasCompletedRequest,
      icon: Activity,
      target: target('/api-keys', '验证统一 API', 'test-api-key', '测试请求'),
    },
  ]
}

function target(
  to: OnboardingRoute,
  label: string,
  action: string,
  actionLabel = label,
): OnboardingTarget {
  return { to, label, action, actionLabel }
}
