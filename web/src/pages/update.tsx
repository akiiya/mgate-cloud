import { useState, type ReactNode } from 'react'
import { Download, RefreshCw, ShieldAlert } from 'lucide-react'
import { updateApi, type UpdateCheck } from '@/api/update'
import { ApiError } from '@/api/client'
import { formatDateTime } from '@/lib/format'
import { PageHeader } from '@/components/layout/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Callout } from '@/components/ui/callout'

/**
 * 更新页：检查 GitHub 最新版本，提示是否有更新，并可下载安装。
 * 自更新只替换程序二进制，安装后需重启生效；更新前提醒备份数据库。
 */
export function UpdatePage() {
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
    if (!window.confirm('更新前请先备份数据库。\n确认下载并安装最新版本？（更新只替换程序二进制，安装后需重启服务）')) {
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
    <div className="flex flex-col gap-6">
      <PageHeader
        title="更新"
        description="检查并安装新版本。控制面为单二进制，更新只替换程序本体。"
        actions={
          <Button variant="outline" loading={checking} onClick={check}>
            <RefreshCw className="h-4 w-4" aria-hidden />
            检查更新
          </Button>
        }
      />

      {error && <Callout tone="error" title="出错了">{error}</Callout>}
      {applyMsg && <Callout tone="success" title="更新已下载">{applyMsg}</Callout>}

      <Callout tone="warning" title="更新前请备份数据库">
        <span className="inline-flex items-start gap-1.5">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
          自更新仅替换程序二进制，且需重启服务后才能完全生效。
        </span>
      </Callout>

      <Card>
        <CardHeader>
          <CardTitle>版本信息</CardTitle>
        </CardHeader>
        <CardContent>
          {info ? (
            <div className="flex flex-col gap-5">
              <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
                <Field label="当前版本">{info.current_version || 'dev'}</Field>
                <Field label="最新版本">
                  <span className="inline-flex items-center gap-2">
                    {info.latest_version}
                    {info.has_update ? <Badge tone="primary">有新版本</Badge> : <Badge tone="success">已是最新</Badge>}
                  </span>
                </Field>
                <Field label="发布时间">{formatDateTime(info.published_at)}</Field>
                <Field label="通道">{info.channel}</Field>
                {info.has_update && (
                  <Field label="下载资产">{info.asset_available ? info.asset_name : '该平台暂无对应资产'}</Field>
                )}
              </dl>

              {info.has_update && info.asset_available && (
                <div>
                  <Button loading={applying} onClick={apply}>
                    <Download className="h-4 w-4" aria-hidden />
                    下载并更新
                  </Button>
                </div>
              )}
              {info.has_update && !info.asset_available && (
                <Callout tone="info">当前平台暂无可直接安装的资产，请前往发布页手动下载。</Callout>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">点击右上角「检查更新」查询是否有可用的新版本。</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-sm">{children}</dd>
    </div>
  )
}
