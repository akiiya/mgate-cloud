import { useEffect, useState, type ReactNode } from 'react'
import { Activity, Database, KeyRound, Radar, ShieldX, ArrowRight, Wifi, WifiOff, Router, CheckCircle2, XCircle, Terminal, Clock, AlertTriangle, DownloadCloud, RefreshCw, Download } from 'lucide-react'
import { useAuth } from '@/lib/auth-context'
import { deviceApi } from '@/api/devices'
import { commandApi } from '@/api/commands'
import { updateApi, type UpdateCheck } from '@/api/update'
import { ApiError } from '@/api/client'
import { formatDateTime } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ErrorAlert } from '@/components/ui/alert'

/** 统计卡片：图标 + 标签 + 大数字，用于设备在线概览。 */
function StatCard({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: typeof Activity
  label: string
  value: number
  tone: 'success' | 'neutral' | 'primary'
}) {
  const toneText =
    tone === 'success' ? 'text-success' : tone === 'primary' ? 'text-primary' : 'text-muted-foreground'
  return (
    <Card>
      <CardContent className="flex items-center gap-4 pt-6">
        <span className={`flex h-11 w-11 items-center justify-center rounded-lg bg-muted ${toneText}`}>
          <Icon className="h-5 w-5" aria-hidden />
        </span>
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold">{value}</p>
        </div>
      </CardContent>
    </Card>
  )
}

/**
 * 检查更新卡片：查询 GitHub 最新版本，提示是否有更新，并可下载安装。
 * 更新前提醒备份数据库；自更新只替换二进制，需重启生效。
 */
function UpdateCard() {
  const [info, setInfo] = useState<UpdateCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [applyMsg, setApplyMsg] = useState<string | null>(null)

  async function check() {
    setError(null)
    setApplyMsg(null)
    setChecking(true)
    try {
      setInfo(await updateApi.check())
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '检查更新失败')
    } finally {
      setChecking(false)
    }
  }

  async function apply() {
    if (!window.confirm('⚠️ 更新前请先备份数据库。\n确认下载并安装最新版本？（更新只替换程序二进制，安装后需重启服务）')) {
      return
    }
    setError(null)
    setApplying(true)
    try {
      const r = await updateApi.apply()
      setApplyMsg(r.message)
      setInfo((prev) => (prev ? { ...prev, has_update: false } : prev))
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '更新失败')
    } finally {
      setApplying(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <RefreshCw className="h-5 w-5 text-primary" aria-hidden />
          <CardTitle>检查更新</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && <ErrorAlert message={error} />}
        {applyMsg && (
          <div className="rounded-md border border-success/20 bg-success/5 px-3.5 py-2.5 text-sm text-success">{applyMsg}</div>
        )}

        {info ? (
          <dl className="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
            <Field label="当前版本">{info.current_version || 'dev'}</Field>
            <Field label="最新版本">
              {info.latest_version}
              {info.has_update ? (
                <Badge tone="primary" className="ml-2">有新版本</Badge>
              ) : (
                <Badge tone="success" className="ml-2">已是最新</Badge>
              )}
            </Field>
            <Field label="发布时间">{formatDateTime(info.published_at)}</Field>
            <Field label="通道">{info.channel}</Field>
            {info.has_update && <Field label="下载资产">{info.asset_available ? info.asset_name : '该平台暂无对应资产'}</Field>}
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">点击「检查更新」查询是否有可用的新版本。</p>
        )}

        <p className="text-xs text-muted-foreground">⚠️ 自更新只替换程序二进制并需重启生效；执行前请务必备份数据库。</p>

        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" loading={checking} onClick={check}>
            <RefreshCw className="h-4 w-4" aria-hidden />
            检查更新
          </Button>
          {info?.has_update && info.asset_available && (
            <Button size="sm" loading={applying} onClick={apply}>
              <Download className="h-4 w-4" aria-hidden />
              下载并更新
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

/** 信息字段（更新卡片内）。 */
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 flex items-center text-sm">{children}</dd>
    </div>
  )
}

/** 状态卡片的数据模型，便于以数据驱动渲染、避免重复 JSX。 */
interface StatusItem {
  icon: typeof Activity
  label: string
  value: string
  badge?: { text: string; tone: 'success' | 'primary' | 'neutral' }
}

const statusItems: StatusItem[] = [
  {
    icon: Activity,
    label: '服务状态',
    value: '运行中',
    badge: { text: 'Healthy', tone: 'success' },
  },
  {
    icon: Database,
    label: '数据库',
    value: 'SQLite + WAL',
    badge: { text: 'Embedded', tone: 'neutral' },
  },
  {
    icon: KeyRound,
    label: '认证方式',
    value: 'Session Cookie',
    badge: { text: 'CSRF 防护', tone: 'primary' },
  },
  {
    icon: Radar,
    label: '设备身份',
    value: '注册 / 设备码',
    badge: { text: 'Phase 2', tone: 'primary' },
  },
]

/** 安全边界条目：这些是产品的硬性约束，会持续展示以提醒边界不可逾越。 */
const securityBoundaries = [
  '不通过 SSH 登录设备',
  '不执行任何远程 shell / bash -c',
  '不提供 raw command 能力',
  '后续仅通过 agent 下发白名单 action',
]

/**
 * Dashboard 概览页。
 * 以卡片网格展示 Phase 1 的运行状态，并以独立卡片强调安全边界，
 * 让管理员一眼掌握系统现状与产品的安全承诺。
 */
export function DashboardPage() {
  const { user } = useAuth()

  // 设备在线统计：直接复用设备列表接口在前端聚合，避免为 Dashboard 引入额外统计系统。
  const [counts, setCounts] = useState({ total: 0, online: 0, offline: 0, recentPull: 0 })
  const [cmdCounts, setCmdCounts] = useState({ total: 0, succeeded: 0, failed: 0, pending: 0, timeout: 0 })
  useEffect(() => {
    let active = true
    const recentPullMs = 2 * 60 * 1000
    deviceApi
      .list()
      .then(({ items }) => {
        if (!active) return
        const online = items.filter((d) => d.online).length
        const recentPull = items.filter(
          (d) => !d.online && d.last_pull_at && Date.now() - new Date(d.last_pull_at).getTime() < recentPullMs,
        ).length
        setCounts({ total: items.length, online, offline: items.length - online, recentPull })
      })
      .catch(() => {
        // Dashboard 统计为非关键信息，失败时保持 0 即可，不打断页面。
      })
    commandApi
      .list({ limit: 200 })
      .then(({ items }) => {
        if (!active) return
        const succeeded = items.filter((c) => c.status === 'succeeded').length
        const failed = items.filter((c) => ['failed', 'expired'].includes(c.status)).length
        const pending = items.filter((c) => ['pending', 'leased', 'sent', 'acked', 'running'].includes(c.status)).length
        const timeout = items.filter((c) => c.status === 'timeout').length
        setCmdCounts({ total: items.length, succeeded, failed, pending, timeout })
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [])

  return (
    <div className="flex flex-col gap-8">
      {/* 欢迎区 */}
      <section>
        <h1 className="text-2xl font-semibold tracking-tight">概览</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          欢迎回来，<span className="font-medium text-foreground">{user?.username}</span>。
          以下是当前控制面的运行状态。
        </p>
      </section>

      {/* 设备在线统计 */}
      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard icon={Router} label="设备总数" value={counts.total} tone="primary" />
        <StatCard icon={Wifi} label="在线设备" value={counts.online} tone="success" />
        <StatCard icon={WifiOff} label="离线设备" value={counts.offline} tone="neutral" />
      </section>

      {/* 命令统计 */}
      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard icon={Terminal} label="命令总数" value={cmdCounts.total} tone="primary" />
        <StatCard icon={CheckCircle2} label="成功命令" value={cmdCounts.succeeded} tone="success" />
        <StatCard icon={XCircle} label="失败命令" value={cmdCounts.failed} tone="neutral" />
      </section>

      {/* 队列与 Pull 统计 */}
      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard icon={Clock} label="待处理命令" value={cmdCounts.pending} tone="primary" />
        <StatCard icon={AlertTriangle} label="超时命令" value={cmdCounts.timeout} tone="neutral" />
        <StatCard icon={DownloadCloud} label="最近 Pull 设备" value={counts.recentPull} tone="primary" />
      </section>

      {/* 检查更新 */}
      <UpdateCard />

      {/* 状态卡片网格，响应式：手机 1 列、平板 2 列、桌面 4 列 */}
      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {statusItems.map((item) => {
          const Icon = item.icon
          return (
            <Card key={item.label}>
              <CardContent className="pt-6">
                <div className="flex items-center justify-between">
                  <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted text-foreground">
                    <Icon className="h-5 w-5" aria-hidden />
                  </span>
                  {item.badge && <Badge tone={item.badge.tone}>{item.badge.text}</Badge>}
                </div>
                <p className="mt-4 text-sm text-muted-foreground">{item.label}</p>
                <p className="mt-0.5 text-lg font-semibold">{item.value}</p>
              </CardContent>
            </Card>
          )
        })}
      </section>

      {/* 安全边界与下一步并排 */}
      <section className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2 border-destructive/20">
          <CardHeader>
            <div className="flex items-center gap-2">
              <ShieldX className="h-5 w-5 text-destructive" aria-hidden />
              <CardTitle>安全边界</CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            <ul className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
              {securityBoundaries.map((text) => (
                <li key={text} className="flex items-start gap-2 text-sm text-foreground">
                  <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-destructive" aria-hidden />
                  {text}
                </li>
              ))}
            </ul>
            <p className="mt-4 text-xs text-muted-foreground">
              cloud 是公网控制面，并非设备本地执行器。所有设备操作都将由 mgate-agent
              主动连接 cloud 后，经白名单 action 调用设备本地的 mgate.sh 完成。
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>下一阶段</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col gap-2.5 text-sm">
              {['HTTPS Pull 兜底通道', '离线命令队列与重试', '命令超时/过期清理', '操作审计可视化'].map(
                (text) => (
                  <li key={text} className="flex items-center gap-2 text-muted-foreground">
                    <ArrowRight className="h-4 w-4 shrink-0 text-primary" aria-hidden />
                    {text}
                  </li>
                ),
              )}
            </ul>
          </CardContent>
        </Card>
      </section>
    </div>
  )
}
