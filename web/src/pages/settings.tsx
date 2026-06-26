import { useEffect, useState, type ReactNode } from 'react'
import { Monitor, Palette, ServerCog, ShieldCheck, UserRound } from 'lucide-react'
import { systemApi, type SystemInfo } from '@/api/system'
import { ApiError } from '@/api/client'
import { useAuth } from '@/lib/auth-context'
import { PageHeader } from '@/components/layout/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Callout } from '@/components/ui/callout'
import { Skeleton } from '@/components/ui/skeleton'
import { ThemeToggle } from '@/components/ui/theme-toggle'

const modeLabels: Record<string, { label: string; tone: 'success' | 'warning' | 'neutral' }> = {
  prod: { label: '生产', tone: 'success' },
  dev: { label: '开发', tone: 'warning' },
  test: { label: '测试', tone: 'neutral' },
}

/** 设置 / 系统页：外观、系统信息、账户、安全边界。 */
export function SettingsPage() {
  const { user } = useAuth()
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    systemApi
      .get()
      .then((s) => {
        if (active) setInfo(s)
      })
      .catch((e) => {
        if (active) setError(e instanceof ApiError ? e.message : '加载系统信息失败')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const mode = info ? modeLabels[info.mode] ?? { label: info.mode, tone: 'neutral' as const } : null

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="设置" description="外观偏好、系统信息与安全说明。" />

      {/* 外观 */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Palette className="h-5 w-5 text-primary" aria-hidden />
            <CardTitle>外观</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">主题</p>
              <p className="text-sm text-muted-foreground">浅色 / 深色 / 跟随系统，选择会自动保存。</p>
            </div>
            <ThemeToggle />
          </div>
        </CardContent>
      </Card>

      {/* 系统信息 */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <ServerCog className="h-5 w-5 text-primary" aria-hidden />
            <CardTitle>系统信息</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : error ? (
            <Callout tone="error">{error}</Callout>
          ) : info ? (
            <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
              <Field label="版本">{info.version || 'dev'}</Field>
              <Field label="运行模式">{mode && <Badge tone={mode.tone}>{mode.label}</Badge>}</Field>
              <Field label="更新通道">{info.update_channel}</Field>
              <Field label="检查更新">
                <Badge tone={info.update_enabled ? 'success' : 'neutral'}>{info.update_enabled ? '已启用' : '已关闭'}</Badge>
              </Field>
              <Field label="Cookie Secure">
                <Badge tone={info.cookie_secure ? 'success' : 'warning'}>{info.cookie_secure ? '开启' : '关闭'}</Badge>
              </Field>
            </dl>
          ) : null}
        </CardContent>
      </Card>

      {/* 账户 */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <UserRound className="h-5 w-5 text-primary" aria-hidden />
            <CardTitle>账户</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <Field label="当前管理员">{user?.username ?? '—'}</Field>
        </CardContent>
      </Card>

      {/* 安全边界 */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5 text-success" aria-hidden />
            <CardTitle>安全边界</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <ul className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
            {[
              '不通过 SSH 登录设备',
              '不执行任何远程 shell',
              '不提供 raw command 能力',
              '仅经 agent 下发白名单操作',
            ].map((t) => (
              <li key={t} className="flex items-start gap-2 text-sm">
                <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-success" aria-hidden />
                {t}
              </li>
            ))}
          </ul>
          <p className="mt-4 flex items-start gap-1.5 text-xs text-muted-foreground">
            <Monitor className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
            控制面是公网管理端，并非设备本地执行器；所有设备操作都由 mgate-agent 主动连接后执行。
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className="mt-1 flex items-center text-sm">{children}</dd>
    </div>
  )
}
