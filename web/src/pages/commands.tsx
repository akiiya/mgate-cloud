import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Loader2, Terminal } from 'lucide-react'
import { commandApi, type Command, type CommandStatus } from '@/api/commands'
import { ApiError } from '@/api/client'
import { commandStatusMeta } from '@/lib/command-status'
import { formatDateTime } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { ErrorAlert } from '@/components/ui/alert'

/** 可筛选的状态选项。 */
const statusOptions: { value: string; label: string }[] = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '待投递' },
  { value: 'sent', label: '已投递' },
  { value: 'acked', label: '已确认' },
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">命令记录</h1>
          <p className="mt-1 text-sm text-muted-foreground">查看下发给设备的白名单命令及其执行结果。</p>
        </div>
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
      </div>

      {error && <ErrorAlert message={error} />}

      <Card className="overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" aria-hidden />
          </div>
        ) : commands.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
            <span className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
              <Terminal className="h-6 w-6" aria-hidden />
            </span>
            <p className="text-sm text-muted-foreground">暂无命令记录</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="px-4 py-3 font-medium">设备</th>
                  <th className="px-4 py-3 font-medium">操作</th>
                  <th className="px-4 py-3 font-medium">状态</th>
                  <th className="px-4 py-3 font-medium">创建时间</th>
                  <th className="px-4 py-3 font-medium">完成时间</th>
                </tr>
              </thead>
              <tbody>
                {commands.map((c) => {
                  const meta = commandStatusMeta(c.status as CommandStatus)
                  return (
                    <tr
                      key={c.id}
                      onClick={() => navigate(`/commands/${c.id}`)}
                      className="cursor-pointer border-b border-border/60 last:border-0 transition-colors hover:bg-muted/50"
                    >
                      <td className="px-4 py-3">{c.device_name || c.device_id}</td>
                      <td className="px-4 py-3 font-mono text-xs">{c.action}</td>
                      <td className="px-4 py-3">
                        <Badge tone={meta.tone}>{meta.label}</Badge>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{formatDateTime(c.created_at)}</td>
                      <td className="px-4 py-3 text-muted-foreground">{formatDateTime(c.finished_at)}</td>
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
