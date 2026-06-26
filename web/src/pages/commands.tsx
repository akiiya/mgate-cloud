import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Terminal } from 'lucide-react'
import { commandApi, type Command } from '@/api/commands'
import { ApiError } from '@/api/client'
import { commandStatusMeta } from '@/lib/command-status'
import { actionTitle } from '@/lib/action-catalog'
import { formatDateTime } from '@/lib/format'
import { PageHeader } from '@/components/layout/page-header'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'
import { EmptyState } from '@/components/ui/empty-state'
import { SkeletonRows } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'

/** 可筛选的状态选项。 */
const statusOptions: { value: string; label: string }[] = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '进行中' },
  { value: 'succeeded', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'timeout', label: '超时' },
  { value: 'canceled', label: '已取消' },
  { value: 'expired', label: '已过期' },
]

/** 命令记录列表页。 */
export function CommandsPage() {
  const navigate = useNavigate()
  const [commands, setCommands] = useState<Command[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { items } = await commandApi.list({ status: status || undefined, limit: 200 })
      setCommands(items)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载命令失败')
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="命令记录"
        description="查看下发给设备的操作及其执行结果。"
        actions={
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="h-9 rounded-md border border-input bg-card px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {statusOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        }
      />

      {error && <Callout tone="error" title="加载失败" action={<Button size="sm" variant="outline" onClick={() => void load()}>重试</Button>}>{error}</Callout>}

      <Card className="overflow-hidden">
        {loading ? (
          <SkeletonRows rows={6} cols={5} />
        ) : commands.length === 0 ? (
          <EmptyState
            icon={Terminal}
            title={status ? '没有该状态的命令' : '暂无命令记录'}
            description={status ? '换个筛选条件试试。' : '在设备详情页下发操作后，这里会显示命令与执行结果。'}
          />
        ) : (
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="px-5 py-3 font-medium">操作</th>
                  <th className="px-5 py-3 font-medium">设备</th>
                  <th className="px-5 py-3 font-medium">状态</th>
                  <th className="px-5 py-3 font-medium">创建时间</th>
                  <th className="px-5 py-3 font-medium">完成时间</th>
                </tr>
              </thead>
              <tbody>
                {commands.map((c) => {
                  const meta = commandStatusMeta(c.status)
                  return (
                    <tr
                      key={c.id}
                      onClick={() => navigate(`/commands/${c.id}`)}
                      className="cursor-pointer border-b border-border/60 transition-colors last:border-0 hover:bg-muted/50"
                    >
                      <td className="px-5 py-3 font-medium">{actionTitle(c.action)}</td>
                      <td className="px-5 py-3 text-muted-foreground">{c.device_name || c.device_id}</td>
                      <td className="px-5 py-3">
                        <Badge tone={meta.tone}>{meta.label}</Badge>
                      </td>
                      <td className="px-5 py-3 text-muted-foreground">{formatDateTime(c.created_at)}</td>
                      <td className="px-5 py-3 text-muted-foreground">{formatDateTime(c.finished_at)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}
