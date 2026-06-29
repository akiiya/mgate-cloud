import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, Router, Search } from 'lucide-react'
import { deviceApi, type Device } from '@/api/devices'
import { ApiError } from '@/api/client'
import { deviceStatusMeta, connectionMeta } from '@/lib/device-status'
import { formatDateTime } from '@/lib/format'
import { PageHeader } from '@/components/layout/page-header'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Callout } from '@/components/ui/callout'
import { EmptyState } from '@/components/ui/empty-state'
import { SkeletonRows } from '@/components/ui/skeleton'

/** 设备列表页：展示设备与连接态，支持创建设备并进入详情。 */
export function DevicesPage() {
  const navigate = useNavigate()

  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [query, setQuery] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { items } = await deviceApi.list()
      setDevices(items)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载设备失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const filtered = devices.filter((d) => {
    if (!query.trim()) return true
    const q = query.trim().toLowerCase()
    return d.name.toLowerCase().includes(q) || (d.remark ?? '').toLowerCase().includes(q) || d.id.toLowerCase().includes(q)
  })

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="设备"
        description="创建设备并生成一次性设备码，供 agent 绑定。"
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="h-4 w-4" aria-hidden />
            创建设备
          </Button>
        }
      />

      {error && <Callout tone="error" title="加载失败" action={<Button size="sm" variant="outline" onClick={() => void load()}>重试</Button>}>{error}</Callout>}

      {/* 搜索：仅在有设备时显示 */}
      {!loading && devices.length > 0 && (
        <div className="relative max-w-xs">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索名称 / 备注 / ID" className="pl-9" />
        </div>
      )}

      <Card className="overflow-hidden">
        {loading ? (
          <SkeletonRows rows={5} cols={5} />
        ) : devices.length === 0 ? (
          <EmptyState
            icon={Router}
            title="还没有任何设备"
            description="创建设备后会生成一次性设备码，交给 mgate-agent 完成绑定。"
            action={
              <Button variant="outline" size="sm" onClick={() => setCreateOpen(true)}>
                <Plus className="h-4 w-4" aria-hidden />
                创建设备
              </Button>
            }
          />
        ) : filtered.length === 0 ? (
          <EmptyState icon={Search} title="没有匹配的设备" description="换个关键词试试。" />
        ) : (
          <DeviceTable devices={filtered} onRowClick={(id) => navigate(`/devices/${id}`)} />
        )}
      </Card>

      <CreateDeviceDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={() => {
          setCreateOpen(false)
          void load()
        }}
      />
    </div>
  )
}

/** 设备表格。横向可滚动以适配窄屏。 */
function DeviceTable({ devices, onRowClick }: { devices: Device[]; onRowClick: (id: string) => void }) {
  return (
    <div className="overflow-x-auto scrollbar-thin">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
            <th className="px-5 py-3 font-medium">名称</th>
            <th className="px-5 py-3 font-medium">状态</th>
            <th className="px-5 py-3 font-medium">连接</th>
            <th className="px-5 py-3 font-medium">最近活跃</th>
            <th className="px-5 py-3 font-medium">最近 Pull</th>
          </tr>
        </thead>
        <tbody>
          {devices.map((d) => {
            const meta = deviceStatusMeta(d.status)
            const conn = connectionMeta(d)
            return (
              <tr
                key={d.id}
                onClick={() => onRowClick(d.id)}
                className="cursor-pointer border-b border-border/60 transition-colors last:border-0 hover:bg-muted/50"
              >
                <td className="px-5 py-3">
                  <div className="font-medium text-foreground">{d.name}</div>
                  {d.remark && <div className="text-xs text-muted-foreground">{d.remark}</div>}
                </td>
                <td className="px-5 py-3">
                  <Badge tone={meta.tone}>{meta.label}</Badge>
                </td>
                <td className="px-5 py-3">
                  <span className="inline-flex items-center gap-1.5">
                    <span className={'h-1.5 w-1.5 rounded-full ' + conn.dotClass} aria-hidden />
                    <Badge tone={conn.tone}>{conn.label}</Badge>
                  </span>
                </td>
                <td className="px-5 py-3 text-muted-foreground">{formatDateTime(d.last_seen_at)}</td>
                <td className="px-5 py-3 text-muted-foreground">{formatDateTime(d.last_pull_at)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/** 创建设备弹窗。 */
function CreateDeviceDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated: () => void
}) {
  const [name, setName] = useState('')
  const [remark, setRemark] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (open) {
      setName('')
      setRemark('')
      setError(null)
    }
  }, [open])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await deviceApi.create(name.trim(), remark.trim())
      onCreated()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '创建失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title="创建设备" description="创建后设备处于待绑定状态，需生成设备码并由 agent 绑定。">
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="device-name">设备名称</Label>
          <Input
            id="device-name"
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例如：办公室随身 WiFi"
            required
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="device-remark">备注（可选）</Label>
          <Input id="device-remark" value={remark} onChange={(e) => setRemark(e.target.value)} placeholder="例如：测试设备" />
        </div>

        {error && <Callout tone="error">{error}</Callout>}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button type="submit" loading={submitting} disabled={name.trim() === ''}>
            创建
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
