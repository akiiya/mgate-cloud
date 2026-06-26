import { useEffect, useState, type ComponentType } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Router,
  Wifi,
  WifiOff,
  DownloadCloud,
  Clock,
  CheckCircle2,
  XCircle,
  ArrowRight,
  Terminal,
  type LucideProps,
} from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { deviceApi, type Device } from '@/api/devices'
import { commandApi, type Command } from '@/api/commands'
import { ApiError } from '@/api/client'
import { formatDateTime } from '@/lib/format'
import { presentCommand, severityTone } from '@/lib/command-presenter'
import { connectionMeta } from '@/lib/device-status'
import { PageHeader } from '@/components/layout/page-header'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { Callout } from '@/components/ui/callout'

const RECENT_PULL_MS = 2 * 60 * 1000

/** 概览页：设备与命令的运行态总览 + 最近活动。 */
export function DashboardPage() {
  const { user } = useAuth()
  const navigate = useNavigate()

  const [devices, setDevices] = useState<Device[] | null>(null)
  const [commands, setCommands] = useState<Command[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    setLoading(true)
    Promise.all([deviceApi.list(), commandApi.list({ limit: 200 })])
      .then(([d, c]) => {
        if (!active) return
        setDevices(d.items)
        setCommands(c.items)
        setError(null)
      })
      .catch((e) => {
        if (!active) return
        setError(e instanceof ApiError ? e.message : '加载概览数据失败')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const online = devices?.filter((d) => d.online).length ?? 0
  const recentPull =
    devices?.filter(
      (d) => !d.online && d.last_pull_at && Date.now() - new Date(d.last_pull_at).getTime() < RECENT_PULL_MS,
    ).length ?? 0
  const total = devices?.length ?? 0
  const offline = total - online

  const cmdPending = commands?.filter((c) => ['pending', 'leased', 'sent', 'acked', 'running'].includes(c.status)).length ?? 0
  const cmdSucceeded = commands?.filter((c) => c.status === 'succeeded').length ?? 0
  const cmdFailed = commands?.filter((c) => ['failed', 'timeout', 'expired'].includes(c.status)).length ?? 0

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="概览"
        description={user ? `欢迎回来，${user.username}。这是当前控制面的运行状态。` : '控制面运行状态总览。'}
      />

      {error && <Callout tone="error" title="加载失败">{error}</Callout>}

      {/* 设备状态 */}
      <section className="space-y-3">
        <h2 className="text-sm font-medium text-muted-foreground">设备</h2>
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard loading={loading} icon={Router} label="设备总数" value={total} tone="primary" />
          <StatCard loading={loading} icon={Wifi} label="在线" value={online} tone="success" />
          <StatCard loading={loading} icon={WifiOff} label="离线" value={offline} tone="neutral" />
          <StatCard loading={loading} icon={DownloadCloud} label="最近 Pull" value={recentPull} tone="info" />
        </div>
      </section>

      {/* 命令状态 */}
      <section className="space-y-3">
        <h2 className="text-sm font-medium text-muted-foreground">命令</h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <StatCard loading={loading} icon={Clock} label="进行中" value={cmdPending} tone="info" />
          <StatCard loading={loading} icon={CheckCircle2} label="成功" value={cmdSucceeded} tone="success" />
          <StatCard loading={loading} icon={XCircle} label="失败 / 超时" value={cmdFailed} tone="neutral" />
        </div>
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <RecentCommands loading={loading} commands={commands ?? []} onOpen={(id) => navigate(`/commands/${id}`)} onOpenAll={() => navigate('/commands')} />
        <RecentDevices loading={loading} devices={devices ?? []} onOpen={(id) => navigate(`/devices/${id}`)} onOpenAll={() => navigate('/devices')} />
      </div>
    </div>
  )
}

type Tone = 'primary' | 'success' | 'neutral' | 'info'
const toneText: Record<Tone, string> = {
  primary: 'text-primary',
  success: 'text-success',
  neutral: 'text-muted-foreground',
  info: 'text-info',
}

function StatCard({
  icon: Icon,
  label,
  value,
  tone,
  loading,
}: {
  icon: ComponentType<LucideProps>
  label: string
  value: number
  tone: Tone
  loading: boolean
}) {
  return (
    <Card className="p-5">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{label}</p>
        <span className={`flex h-9 w-9 items-center justify-center rounded-lg bg-muted ${toneText[tone]}`}>
          <Icon className="h-[18px] w-[18px]" aria-hidden />
        </span>
      </div>
      {loading ? (
        <Skeleton className="mt-3 h-8 w-14" />
      ) : (
        <p className="mt-2 text-3xl font-semibold tabular-nums">{value}</p>
      )}
    </Card>
  )
}

/** 最近命令：人话呈现 + 状态徽标。 */
function RecentCommands({
  loading,
  commands,
  onOpen,
  onOpenAll,
}: {
  loading: boolean
  commands: Command[]
  onOpen: (id: string) => void
  onOpenAll: () => void
}) {
  const recent = commands.slice(0, 6)
  return (
    <Card className="flex flex-col">
      <SectionHead title="最近命令" onMore={onOpenAll} />
      {loading ? (
        <div className="space-y-3 p-5">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : recent.length === 0 ? (
        <EmptyState icon={Terminal} title="暂无命令记录" description="在设备详情页下发操作后，这里会显示最近的命令。" />
      ) : (
        <ul className="divide-y divide-border">
          {recent.map((c) => {
            const p = presentCommand(c)
            return (
              <li key={c.id}>
                <button
                  onClick={() => onOpen(c.id)}
                  className="flex w-full items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-muted/50"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{p.title}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {c.device_name || c.device_id} · {formatDateTime(c.created_at)}
                    </p>
                  </div>
                  <Badge tone={severityTone(p.severity)}>{statusShort(c.status)}</Badge>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

/** 最近设备：名称 + 连接态。 */
function RecentDevices({
  loading,
  devices,
  onOpen,
  onOpenAll,
}: {
  loading: boolean
  devices: Device[]
  onOpen: (id: string) => void
  onOpenAll: () => void
}) {
  const recent = [...devices]
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    .slice(0, 6)
  return (
    <Card className="flex flex-col">
      <SectionHead title="设备" onMore={onOpenAll} />
      {loading ? (
        <div className="space-y-3 p-5">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : recent.length === 0 ? (
        <EmptyState icon={Router} title="还没有设备" description="到「设备」页创建第一台设备并生成设备码。" />
      ) : (
        <ul className="divide-y divide-border">
          {recent.map((d) => {
            const conn = connectionMeta(d)
            return (
              <li key={d.id}>
                <button
                  onClick={() => onOpen(d.id)}
                  className="flex w-full items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-muted/50"
                >
                  <span className={'h-2 w-2 shrink-0 rounded-full ' + conn.dotClass} aria-hidden />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{d.name}</p>
                    <p className="truncate text-xs text-muted-foreground">{d.remark || d.id}</p>
                  </div>
                  <Badge tone={conn.tone}>{conn.label}</Badge>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Card>
  )
}

function SectionHead({ title, onMore }: { title: string; onMore: () => void }) {
  return (
    <div className="flex items-center justify-between border-b border-border px-5 py-4">
      <h3 className="text-sm font-semibold">{title}</h3>
      <button onClick={onMore} className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline">
        查看全部 <ArrowRight className="h-3.5 w-3.5" aria-hidden />
      </button>
    </div>
  )
}

/** 列表里用的极简状态短词。 */
function statusShort(status: Command['status']): string {
  const m: Record<string, string> = {
    pending: '进行中',
    leased: '进行中',
    sent: '进行中',
    acked: '进行中',
    running: '进行中',
    succeeded: '成功',
    failed: '失败',
    timeout: '超时',
    canceled: '已取消',
    expired: '已过期',
  }
  return m[status] ?? status
}
