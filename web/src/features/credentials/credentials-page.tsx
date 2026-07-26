import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, CloudDownload, Pencil, Play, Plus, Power, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'

import { catalogApi, type Credential, type CredentialModelProbeBatchResult } from '@/api'
import { DataTable, type ColumnDef } from '@/components/data-table/data-table'
import {
  RowActionItem,
  RowActionMenu,
  RowActionSeparator,
  TableAction,
} from '@/components/data-table/row-actions'
import { Page, PageHeader, PageSection } from '@/components/layout'
import { StatusBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/field'
import { FormProblem } from '@/features/auth/form-problem'
import { formatDateTime, formatNumber } from '@/lib/format'

import { CredentialBatchForm } from './credential-batch-form'
import { CredentialForm } from './credential-form'
import { CredentialOperationDialog, type CredentialOperation } from './credential-operation-dialog'
import { probeErrorLabel } from './credential-probe-copy'
import { CredentialProbeDialog } from './credential-probe-dialog'

export function CredentialsPage() {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<Credential | null>(null)
  const [probing, setProbing] = useState<Credential | null>(null)
  const [probeAllResult, setProbeAllResult] = useState<CredentialModelProbeBatchResult | null>(null)
  const [operation, setOperation] = useState<CredentialOperation | null>(null)
  const [upstreamStatusResult, setUpstreamStatusResult] = useState<{
    credentialName: string
    state: NonNullable<Credential['upstreamStatus']>['state']
  } | null>(null)
  const [resourcePoolId, setResourcePoolId] = useState('')
  const query = useQuery({
    queryKey: ['credentials'],
    queryFn: ({ signal }) => catalogApi.credentials(true, signal),
  })
  const probeAllMutation = useMutation({
    mutationFn: () => catalogApi.probeAllCredentials(crypto.randomUUID()),
    async onSuccess(result) {
      setProbeAllResult(result)
      await queryClient.invalidateQueries({ queryKey: ['credentials'] })
    },
  })
  const upstreamStatusMutation = useMutation({
    mutationFn: (credential: Credential) => catalogApi.fetchCredentialUpstreamStatus(credential.id),
    async onSuccess(result) {
      setUpstreamStatusResult({
        credentialName: result.credential.name,
        state: result.observation.state,
      })
      await queryClient.invalidateQueries({ queryKey: ['credentials'] })
    },
  })
  const resourcePools = useMemo(() => {
    const unique = new Map<string, string>()
    for (const credential of query.data ?? []) {
      unique.set(credential.resourcePoolId, credential.resourcePoolName)
    }
    return Array.from(unique, ([id, name]) => ({ id, name })).sort((left, right) =>
      left.name.localeCompare(right.name),
    )
  }, [query.data])
  const visibleCredentials = useMemo(
    () =>
      resourcePoolId
        ? (query.data ?? []).filter((credential) => credential.resourcePoolId === resourcePoolId)
        : (query.data ?? []),
    [query.data, resourcePoolId],
  )
  const columns = useMemo<ColumnDef<Credential, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: '上游 API Key',
        meta: { align: 'center' },
        cell: ({ row }) => (
          <div>
            <strong>{row.original.name}</strong>
            <small className="table-subline">{row.original.providerName}</small>
          </div>
        ),
      },
      { accessorKey: 'resourcePoolName', header: '资源池', meta: { align: 'center' } },
      {
        id: 'models',
        header: '模型',
        meta: { align: 'center' },
        cell: ({ row }) => row.original.modelBindings.map((item) => item.modelName).join('、'),
      },
      {
        id: 'limits',
        header: 'RPM / TPM / 并发',
        cell: ({ row }) =>
          `${limit(row.original.rpmLimit)} / ${limit(row.original.tpmLimit)} / ${limit(row.original.concurrencyLimit)}`,
        meta: { align: 'center' },
      },
      {
        id: 'capacity',
        header: '网关余量',
        meta: { align: 'center' },
        cell: ({ row }) => <CredentialCapacityCell capacity={row.original.capacity} />,
      },
      {
        id: 'upstream-status',
        header: '上游观测',
        meta: { align: 'center' },
        cell: ({ row }) => <CredentialUpstreamStatusCell status={row.original.upstreamStatus} />,
      },
      {
        id: 'probe',
        header: '最近探测',
        meta: { align: 'center' },
        cell: ({ row }) =>
          row.original.lastProbeAt ? (
            <div className="table-status-cell">
              <StatusBadge status={row.original.lastProbeStatus ?? 'unknown'} />
              <small className="table-subline">
                {row.original.lastProbeStatus === 'succeeded' &&
                row.original.lastProbeLatencyMs !== undefined
                  ? `${formatNumber(row.original.lastProbeLatencyMs)} ms`
                  : probeErrorLabel(row.original.lastProbeErrorKind)}
              </small>
              <small className="table-subline">{formatDateTime(row.original.lastProbeAt)}</small>
            </div>
          ) : (
            '未探测'
          ),
      },
      {
        accessorKey: 'status',
        header: '状态',
        cell: ({ row }) => (
          <div className="table-status-cell">
            <StatusBadge status={row.original.status} />
            <StatusBadge status={row.original.healthStatus} />
            {row.original.cooldownUntil ? (
              <small className="table-subline">
                冷却至 {formatDateTime(row.original.cooldownUntil)}
              </small>
            ) : row.original.lastErrorKind ? (
              <small className="table-subline">上游：{row.original.lastErrorKind}</small>
            ) : null}
          </div>
        ),
        meta: { align: 'center' },
      },
      {
        id: 'actions',
        header: '操作',
        meta: { align: 'center' },
        cell: ({ row }) =>
          row.original.status !== 'retired' ? (
            <div className="row-actions row-actions--center">
              <TableAction
                label="探测"
                icon={<RefreshCw size={16} />}
                onClick={() => setProbing(row.original)}
              />
              <TableAction
                label="获取上游状态"
                icon={<CloudDownload size={16} />}
                disabled={upstreamStatusMutation.isPending}
                onClick={() => {
                  setUpstreamStatusResult(null)
                  upstreamStatusMutation.mutate(row.original)
                }}
              />
              <TableAction
                label="编辑"
                icon={<Pencil size={16} />}
                onClick={() => setEditing(row.original)}
              />
              <RowActionMenu>
                {row.original.status === 'disabled' ? (
                  <RowActionItem
                    icon={<Play size={15} />}
                    onSelect={() =>
                      setOperation({
                        kind: 'set-status',
                        credential: row.original,
                        status: 'active',
                      })
                    }
                  >
                    启用 Key
                  </RowActionItem>
                ) : (
                  <RowActionItem
                    icon={<Power size={15} />}
                    onSelect={() =>
                      setOperation({
                        kind: 'set-status',
                        credential: row.original,
                        status: 'disabled',
                      })
                    }
                  >
                    停用 Key
                  </RowActionItem>
                )}
                {row.original.status === 'disabled' ? (
                  <>
                    <RowActionSeparator />
                    <RowActionItem
                      icon={<Archive size={15} />}
                      danger
                      onSelect={() => setOperation({ kind: 'retire', credential: row.original })}
                    >
                      退役 Key
                    </RowActionItem>
                  </>
                ) : null}
              </RowActionMenu>
            </div>
          ) : null,
      },
    ],
    [upstreamStatusMutation],
  )

  return (
    <Page>
      <PageHeader
        title="上游 API Key"
        actions={
          <>
            <Button
              variant="secondary"
              icon={<RefreshCw size={16} />}
              disabled={
                probeAllMutation.isPending ||
                !(query.data ?? []).some((item) => item.status === 'active')
              }
              onClick={() => {
                setProbeAllResult(null)
                probeAllMutation.mutate()
              }}
            >
              {probeAllMutation.isPending ? '探测全部 Key 中' : '探测全部 Key'}
            </Button>
            <Button
              icon={<Plus size={16} />}
              data-onboarding="create-provider-key"
              onClick={() => setAdding(true)}
            >
              添加上游 API Key
            </Button>
          </>
        }
      />
      <PageSection>
        <FormProblem error={probeAllMutation.error ?? upstreamStatusMutation.error} />
        {probeAllResult ? (
          <p
            className={
              probeAllResult.failed === 0 &&
              probeAllResult.unavailable === 0 &&
              probeAllResult.uncertain === 0
                ? 'batch-probe-result'
                : 'batch-probe-result batch-probe-result--warning'
            }
            role="status"
          >
            已完成模型探测：成功 {probeAllResult.succeeded} 把，失败 {probeAllResult.failed} 把，
            暂不可用 {probeAllResult.unavailable} 把，结果未确认 {probeAllResult.uncertain} 把。
          </p>
        ) : null}
        {upstreamStatusResult ? (
          <p className="batch-probe-result" role="status">
            已获取 {upstreamStatusResult.credentialName} 的上游观测：
            {upstreamStatusLabel(upstreamStatusResult.state)}。
          </p>
        ) : null}
        <div className="table-toolbar">
          <div className="table-toolbar__filters">
            <NativeSelect
              aria-label="资源池筛选"
              value={resourcePoolId}
              onChange={(event) => setResourcePoolId(event.target.value)}
            >
              <option value="">全部资源池</option>
              {resourcePools.map((pool) => (
                <option key={pool.id} value={pool.id}>
                  {pool.name}
                </option>
              ))}
            </NativeSelect>
          </div>
        </div>
        <DataTable
          ariaLabel="上游 API Key 列表"
          data={visibleCredentials}
          columns={columns}
          getRowId={(item) => item.id}
          loading={query.isLoading}
          fetching={query.isFetching}
          error={query.error}
          onRetry={() => void query.refetch()}
          emptyLabel="还没有上游 API Key"
          page={1}
          pageSize={Math.max(visibleCredentials.length, 1)}
          total={visibleCredentials.length}
          onPageChange={() => undefined}
        />
      </PageSection>
      {editing ? (
        <CredentialForm
          credential={editing}
          open
          onOpenChange={(open) => !open && setEditing(null)}
        />
      ) : null}
      <CredentialBatchForm open={adding} onOpenChange={setAdding} />
      {probing ? (
        <CredentialProbeDialog
          credential={probing}
          onOpenChange={(open) => !open && setProbing(null)}
        />
      ) : null}
      <CredentialOperationDialog
        operation={operation}
        onOpenChange={(open) => !open && setOperation(null)}
      />
    </Page>
  )
}

function limit(value: number | undefined) {
  return value === undefined ? '不限' : formatNumber(value)
}

function CredentialCapacityCell({ capacity }: { capacity: Credential['capacity'] }) {
  if (capacity.state !== 'observed') {
    return <span className="table-subline">暂不可观测</span>
  }
  return (
    <div className="table-status-cell">
      <span>
        {capacityRow(capacity.requestsPerMinuteRemaining, capacity.requestsPerMinuteLimit, 'RPM')}
      </span>
      <small className="table-subline">
        {capacityRow(capacity.tokensPerMinuteRemaining, capacity.tokensPerMinuteLimit, 'TPM')}
      </small>
      <small className="table-subline">
        {capacityRow(capacity.concurrencyInUse, capacity.concurrencyLimit, '并发')}
      </small>
    </div>
  )
}

function CredentialUpstreamStatusCell({ status }: { status: Credential['upstreamStatus'] }) {
  if (!status) return <span className="table-subline">尚未人工获取</span>
  return (
    <div className="table-status-cell">
      <StatusBadge status={status.state} />
      <small className="table-subline">{upstreamScopeLabel(status.scope)}</small>
      <small className="table-subline">证据：{upstreamSourceLabel(status.source)}</small>
      {status.balance ? (
        <small className="table-subline">
          可用 {status.balance.currency} {status.balance.available}
        </small>
      ) : status.reason ? (
        <small className="table-subline">{status.reason}</small>
      ) : null}
      <small className="table-subline">{formatDateTime(status.observedAt)}</small>
    </div>
  )
}

function capacityRow(current: number | undefined, limit: number | undefined, unit: string) {
  if (limit === undefined) return `未配置 ${unit}`
  return `${formatNumber(current ?? 0)} / ${formatNumber(limit)} ${unit}`
}

function upstreamStatusLabel(state: NonNullable<Credential['upstreamStatus']>['state']) {
  return state === 'observed' ? '已获得官方数据' : state === 'unknown' ? '官方数据未知' : '暂不可用'
}

function upstreamScopeLabel(scope: NonNullable<Credential['upstreamStatus']>['scope']) {
  switch (scope) {
    case 'account':
      return '上游账户共享'
    case 'project':
      return '上游项目共享'
    case 'credential':
      return '上游 API Key'
    default:
      return '上游范围未知'
  }
}

function upstreamSourceLabel(source: string) {
  return source === 'official_balance_endpoint' ? '官方余额接口' : source
}
