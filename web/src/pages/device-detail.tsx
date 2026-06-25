import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Ban, Check, Copy, KeyRound, Loader2, ShieldAlert } from 'lucide-react'
import { deviceApi, type DeviceDetail, type PairingCodeResult } from '@/api/devices'
import { commandApi } from '@/api/commands'
import { ApiError } from '@/api/client'
import { deviceStatusMeta, connectionMeta } from '@/lib/device-status'
import { formatDateTime } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { ErrorAlert } from '@/components/ui/alert'

/**
 * 设备详情页。
 * 展示设备完整信息，并依据状态提供"生成设备码 / 禁用 / 启用"操作。
 * 设备码以一次性弹窗展示，离开后无法再次看到明文。
 */
export function DeviceDetailPage() {
  const { deviceId = '' } = useParams()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<DeviceDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [pairing, setPairing] = useState<PairingCodeResult | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setDetail(await deviceApi.get(deviceId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载设备失败')
    } finally {
      setLoading(false)
    }
  }, [deviceId])

  useEffect(() => {
    void load()
  }, [load])

  // 统一的操作执行器：执行异步动作，处理 busy 与错误，完成后刷新详情。
  async function runAction(fn: () => Promise<void>) {
    setActionError(null)
    setBusy(true)
    try {
      await fn()
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  async function handleGenerate() {
    await runAction(async () => {
      const result = await deviceApi.generatePairingCode(deviceId)
      setPairing(result) // 打开一次性展示弹窗
      await load()
    })
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin" aria-hidden />
      </div>
    )
  }

  if (error || !detail) {
    return (
      <div className="flex flex-col gap-4">
        <BackButton onClick={() => navigate('/devices')} />
        <ErrorAlert message={error ?? '设备不存在'} />
      </div>
    )
  }

  const { device } = detail
  const meta = deviceStatusMeta(device.status)
  const conn = connectionMeta(device)

  return (
    <div className="flex flex-col gap-6">
      <BackButton onClick={() => navigate('/devices')} />

      {/* 头部：名称 + 状态 + 在线 + 操作 */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">{device.name}</h1>
            <Badge tone={meta.tone}>{meta.label}</Badge>
            <span className="inline-flex items-center gap-1.5">
              <span className={'h-2 w-2 rounded-full ' + conn.dotClass} aria-hidden />
              <Badge tone={conn.tone}>{conn.label}</Badge>
            </span>
          </div>
          {device.remark && <p className="mt-1 text-sm text-muted-foreground">{device.remark}</p>}
        </div>

        <div className="flex items-center gap-2">
          {device.status === 'pending' && (
            <Button onClick={handleGenerate} loading={busy}>
              <KeyRound className="h-4 w-4" aria-hidden />
              生成设备码
            </Button>
          )}
          {(device.status === 'pending' || device.status === 'enabled') && (
            <Button variant="outline" loading={busy} onClick={() => runAction(async () => {
              await deviceApi.disable(deviceId)
              await load()
            })}>
              <Ban className="h-4 w-4" aria-hidden />
              禁用
            </Button>
          )}
          {device.status === 'disabled' && (
            <Button loading={busy} onClick={() => runAction(async () => {
              await deviceApi.enable(deviceId)
              await load()
            })}>
              <Check className="h-4 w-4" aria-hidden />
              启用
            </Button>
          )}
        </div>
      </div>

      {actionError && <ErrorAlert message={actionError} />}

      {/* 设备信息 */}
      <Card>
        <CardHeader>
          <CardTitle>设备信息</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <InfoRow label="device_id" mono>{device.id}</InfoRow>
            <InfoRow label="状态">{meta.label}</InfoRow>
            <InfoRow label="hostname">{device.hostname || '—'}</InfoRow>
            <InfoRow label="设备型号">{device.device_model || '—'}</InfoRow>
            <InfoRow label="agent 版本">{device.agent_version || '—'}</InfoRow>
            <InfoRow label="mgate 版本">{device.mgate_version || '—'}</InfoRow>
            <InfoRow label="固件信息">{device.firmware_info || '—'}</InfoRow>
            <InfoRow label="有效凭证数">{detail.credential.active_count}</InfoRow>
            <InfoRow label="最近活跃">{formatDateTime(device.last_seen_at)}</InfoRow>
            <InfoRow label="最近 WS 连接">{formatDateTime(device.last_ws_connected_at)}</InfoRow>
            <InfoRow label="最近 WS 断开">{formatDateTime(device.last_ws_disconnected_at)}</InfoRow>
            <InfoRow label="最近 Pull">{formatDateTime(device.last_pull_at)}</InfoRow>
            <InfoRow label="最近 Pull 状态">{formatDateTime(device.last_pull_status_at)}</InfoRow>
            <InfoRow label="最近绑定">{formatDateTime(device.last_enrolled_at)}</InfoRow>
            <InfoRow label="创建时间">{formatDateTime(device.created_at)}</InfoRow>
            <InfoRow label="更新时间">{formatDateTime(device.updated_at)}</InfoRow>
          </dl>
        </CardContent>
      </Card>

      {/* 远程操作（Phase 5：enabled 设备无论在线与否均可下发，离线进入队列等待 Pull/重连） */}
      <RemoteActions
        deviceId={device.id}
        enabled={device.status === 'enabled'}
        online={detail.online}
        onView={(id) => navigate(`/commands/${id}`)}
      />

      {/* 最新状态 */}
      <Card>
        <CardHeader>
          <CardTitle>最新状态</CardTitle>
        </CardHeader>
        <CardContent>
          <LatestStatus detail={detail} />
        </CardContent>
      </Card>

      <PairingCodeDialog result={pairing} onClose={() => setPairing(null)} />
    </div>
  )
}

/** 返回列表按钮。 */
/** 无参数白名单操作（一键下发）。 */
const simpleActions = [
  'ap.status', 'ap.start', 'ap.stop',
  'wlan.scan', 'wlan.list',
  'tproxy.enable', 'tproxy.disable',
  'gateway.status', 'gateway.start', 'gateway.stop',
  'doctor.full',
]

/** 投递提示到中文文案。 */
const hintLabels: Record<string, string> = {
  delivered_via_ws: '已通过 WebSocket 投递',
  queued_for_retry: '已入队，等待重试投递',
  device_offline_waiting_for_pull: '设备离线，已入队，等待设备 Pull 或重新连接',
}

/**
 * 远程操作区域：对 enabled 设备下发白名单命令（Phase 5：离线也可创建，进入队列）。
 * 仅提供固定 action 与受限表单（wlan.connect / tproxy.use），杜绝任意命令输入。
 */
function RemoteActions({
  deviceId,
  enabled,
  online,
  onView,
}: {
  deviceId: string
  enabled: boolean
  online: boolean
  onView: (commandId: string) => void
}) {
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [created, setCreated] = useState<{ id: string; hint: string } | null>(null)
  const [ssid, setSsid] = useState('')
  const [node, setNode] = useState('')

  async function run(action: string, params: Record<string, unknown>) {
    setError(null)
    setBusy(action)
    try {
      const { command, delivery_hint } = await commandApi.create(deviceId, action, params)
      setCreated({ id: command.id, hint: delivery_hint })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '命令下发失败')
    } finally {
      setBusy(null)
    }
  }

  const disabled = !enabled || busy !== null

  return (
    <Card>
      <CardHeader>
        <CardTitle>远程操作</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {!enabled && <p className="text-sm text-muted-foreground">设备未启用，不可下发命令。</p>}
        {enabled && !online && (
          <p className="rounded-md border border-amber-500/20 bg-amber-500/5 px-3.5 py-2.5 text-sm text-amber-700">
            设备当前未保持 WebSocket 在线，命令会进入队列，等待设备 Pull 或重新连接后投递。
          </p>
        )}
        {error && <ErrorAlert message={error} />}
        {created && (
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/50 px-3.5 py-2.5 text-sm">
            <span>命令已创建：{hintLabels[created.hint] ?? created.hint}</span>
            <Button size="sm" variant="outline" onClick={() => onView(created.id)}>
              查看详情
            </Button>
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          {simpleActions.map((a) => (
            <Button
              key={a}
              size="sm"
              variant="outline"
              disabled={disabled}
              loading={busy === a}
              onClick={() => run(a, {})}
              className="font-mono"
            >
              {a}
            </Button>
          ))}
        </div>

        {/* wlan.connect 表单 */}
        <div className="flex flex-wrap items-end gap-2 border-t border-border pt-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ra-ssid">wlan.connect · SSID</Label>
            <Input
              id="ra-ssid"
              value={ssid}
              onChange={(e) => setSsid(e.target.value)}
              placeholder="HomeWiFi"
              className="w-56"
            />
          </div>
          <Button
            size="sm"
            disabled={disabled || ssid.trim() === ''}
            loading={busy === 'wlan.connect'}
            onClick={() => run('wlan.connect', { ssid: ssid.trim() })}
          >
            连接
          </Button>
        </div>

        {/* tproxy.use 表单 */}
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ra-node">tproxy.use · 节点</Label>
            <Input
              id="ra-node"
              value={node}
              onChange={(e) => setNode(e.target.value)}
              placeholder="US"
              className="w-56"
            />
          </div>
          <Button
            size="sm"
            disabled={disabled || node.trim() === ''}
            loading={busy === 'tproxy.use'}
            onClick={() => run('tproxy.use', { node: node.trim() })}
          >
            切换节点
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="inline-flex w-fit items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="h-4 w-4" aria-hidden />
      返回设备列表
    </button>
  )
}

/**
 * 最新状态展示：以格式化 JSON 呈现，限制高度并允许滚动，避免超长内容撑爆页面。
 * 无状态上报时显示占位提示。
 */
function LatestStatus({ detail }: { detail: DeviceDetail }) {
  if (detail.latest_status == null) {
    return <p className="text-sm text-muted-foreground">暂无状态上报</p>
  }
  const pretty = JSON.stringify(detail.latest_status, null, 2)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
        <span>来源：{detail.latest_status_source === 'pull' ? 'HTTPS Pull' : 'WebSocket'}</span>
        <span>上报时间：{formatDateTime(detail.latest_status_reported_at)}</span>
        <span>接收时间：{formatDateTime(detail.latest_status_received_at)}</span>
      </div>
      <pre className="max-h-96 overflow-auto rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
        <code>{pretty}</code>
      </pre>
    </div>
  )
}

/** 信息行：标签 + 值，值可选等宽字体。 */
function InfoRow({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className={mono ? 'mt-1 break-all font-mono text-sm' : 'mt-1 text-sm'}>{children}</dd>
    </div>
  )
}

/**
 * 设备码一次性展示弹窗。
 *
 * 安全约定：设备码明文只在内存（组件 state）中存在，弹窗关闭即清除；
 * 不写入 URL、不写入 localStorage。刷新页面后无法再看到明文。
 */
function PairingCodeDialog({ result, onClose }: { result: PairingCodeResult | null; onClose: () => void }) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    if (!result) return
    try {
      await navigator.clipboard.writeText(result.device_code)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // 复制失败（如非安全上下文）时静默：用户仍可手动选择文本复制。
    }
  }

  return (
    <Dialog
      open={result !== null}
      onClose={onClose}
      title="一次性设备码"
      description="请立即复制并交给 agent。关闭后将无法再次查看明文。"
    >
      {result && (
        <div className="flex flex-col gap-4">
          <div className="flex items-start gap-2.5 rounded-md border border-amber-500/20 bg-amber-500/5 px-3.5 py-3 text-sm text-amber-700">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <span>设备码只展示这一次，且包含一次性 pairing token（不含永久密钥）。绑定成功后立即失效。</span>
          </div>

          <div className="rounded-md border border-border bg-muted/50 p-3">
            <code className="block break-all font-mono text-xs leading-relaxed">{result.device_code}</code>
          </div>

          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">过期时间：{formatDateTime(result.expires_at)}</span>
            <Button size="sm" variant="outline" onClick={handleCopy}>
              <Copy className="h-4 w-4" aria-hidden />
              {copied ? '已复制' : '复制'}
            </Button>
          </div>
        </div>
      )}
    </Dialog>
  )
}
