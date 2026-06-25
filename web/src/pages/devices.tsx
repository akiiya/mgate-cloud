import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Loader2, Plus, Router } from 'lucide-react'
import { deviceApi, type Device } from '@/api/devices'
import { ApiError } from '@/api/client'
import { deviceStatusMeta, connectionMeta } from '@/lib/device-status'
import { formatDateTime } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { ErrorAlert } from '@/components/ui/alert'

/**
 * 设备列表页。
 * 展示设备及状态，支持通过弹窗创建设备；点击行进入设备详情。
 */
export function DevicesPage() {
  const navigate = useNavigate()

  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)

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

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">设备管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">创建设备并生成一次性设备码，供 agent 绑定。</p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4" aria-hidden />
          创建设备
        </Button>
      </div>

      {error && <ErrorAlert message={error} />}

      <Card className="overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center py-16 text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" aria-hidden />
          </div>
        ) : devices.length === 0 ? (
          <EmptyState onCreate={() => setCreateOpen(true)} />
        ) : (
          <DeviceTable devices={devices} onRowClick={(id) => navigate(`/devices/${id}`)} />
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

/** 空状态：引导创建第一台设备。 */
function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
      <span className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
        <Router className="h-6 w-6" aria-hidden />
      </span>
      <p className="text-sm text-muted-foreground">还没有任何设备</p>
      <Button variant="outline" size="sm" onClick={onCreate}>
        <Plus className="h-4 w-4" aria-hidden />
        创建设备
      </Button>
    </div>
  )
}

/** 设备表格。横向可滚动以适配窄屏。 */
function DeviceTable({ devices, onRowClick }: { devices: Device[]; onRowClick: (id: string) => void }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
            <th className="px-4 py-3 font-medium">名称</th>
            <th className="px-4 py-3 font-medium">状态</th>
            <th className="px-4 py-3 font-medium">连接</th>
            <th className="px-4 py-3 font-medium">最近活跃</th>
            <th className="px-4 py-3 font-medium">最近 Pull</th>
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
                className="cursor-pointer border-b border-border/60 last:border-0 transition-colors hover:bg-muted/50"
              >
                <td className="px-4 py-3">
                  <div className="font-medium text-foreground">{d.name}</div>
                  {d.remark && <div className="text-xs text-muted-foreground">{d.remark}</div>}
                </td>
                <td className="px-4 py-3">
                  <Badge tone={meta.tone}>{meta.label}</Badge>
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex items-center gap-1.5">
                    <span className={'h-1.5 w-1.5 rounded-full ' + conn.dotClass} aria-hidden />
                    <Badge tone={conn.tone}>{conn.label}</Badge>
                  </span>
                </td>
                <td className="px-4 py-3 text-muted-foreground">{formatDateTime(d.last_seen_at)}</td>
                <td className="px-4 py-3 text-muted-foreground">{formatDateTime(d.last_pull_at)}</td>
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

  // 每次打开重置表单状态。
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
    <Dialog open={open} onClose={onClose} title="创建设备" description="创建后设备处于待绑定（pending）状态。">
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
          <Input
            id="device-remark"
            value={remark}
            onChange={(e) => setRemark(e.target.value)}
            placeholder="例如：测试设备"
          />
        </div>

        {error && <ErrorAlert message={error} />}

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
