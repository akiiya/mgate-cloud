import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  Ban,
  Check,
  ChevronDown,
  Copy,
  KeyRound,
  ShieldAlert,
  Loader2,
} from 'lucide-react'
import { deviceApi, type DeviceDetail, type PairingCodeResult } from '@/api/devices'
import { commandApi, type DeliveryHint } from '@/api/commands'
import { ApiError } from '@/api/client'
import { deviceStatusMeta, connectionMeta } from '@/lib/device-status'
import { formatDateTime } from '@/lib/format'
import { actionsByCategory, type ActionCatalogEntry } from '@/lib/action-catalog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog } from '@/components/ui/dialog'
import { Callout } from '@/components/ui/callout'

/**
 * 设备详情页。
 * 展示设备核心信息、产品化的「设备操作」（不暴露裸 action 名）、最新状态，
 * 以及折叠的「高级详情」（ID、时间线、原始状态 JSON）。
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
      setPairing(result)
      await load()
    })
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin" aria-hidden />
      </div>
    )
  }

  if (error || !detail) {
    return (
      <div className="flex flex-col gap-4">
        <BackButton onClick={() => navigate('/devices')} />
        <Callout tone="error" title="无法加载设备">{error ?? '设备不存在'}</Callout>
      </div>
    )
  }

  const { device } = detail
  const meta = deviceStatusMeta(device.status)
  const conn = connectionMeta(device)

  return (
    <div className="flex flex-col gap-6">
      <BackButton onClick={() => navigate('/devices')} />

      {/* 头部 */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
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
            <Button
              variant="outline"
              loading={busy}
              onClick={() => runAction(async () => {
                await deviceApi.disable(deviceId)
                await load()
              })}
            >
              <Ban className="h-4 w-4" aria-hidden />
              禁用
            </Button>
          )}
          {device.status === 'disabled' && (
            <Button
              loading={busy}
              onClick={() => runAction(async () => {
                await deviceApi.enable(deviceId)
                await load()
              })}
            >
              <Check className="h-4 w-4" aria-hidden />
              启用
            </Button>
          )}
        </div>
      </div>

      {actionError && <Callout tone="error">{actionError}</Callout>}

      {/* 概要信息（产品化字段） */}
      <Card>
        <CardHeader>
          <CardTitle>设备信息</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
            <InfoRow label="设备型号">{device.device_model || '—'}</InfoRow>
            <InfoRow label="主机名">{device.hostname || '—'}</InfoRow>
            <InfoRow label="agent 版本">{device.agent_version || '—'}</InfoRow>
            <InfoRow label="mgate 版本">{device.mgate_version || '—'}</InfoRow>
            <InfoRow label="最近活跃">{formatDateTime(device.last_seen_at)}</InfoRow>
            <InfoRow label="最近 Pull">{formatDateTime(device.last_pull_at)}</InfoRow>
          </dl>
        </CardContent>
      </Card>

      {/* 设备操作 */}
      <DeviceOperations
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

      {/* 高级详情 */}
      <AdvancedDetails detail={detail} />

      <PairingCodeDialog result={pairing} onClose={() => setPairing(null)} />
    </div>
  )
}

/** 投递提示到人话。 */
const hintLabels: Record<DeliveryHint, string> = {
  delivered_via_ws: '已实时下发到设备',
  queued_for_retry: '已入队，稍后自动重试投递',
  device_offline_waiting_for_pull: '设备离线，已入队，待设备 Pull 或重连后执行',
}

interface RunResult {
  commandId: string
  hint: DeliveryHint
  title: string
}

/**
 * 设备操作面板：按分类陈列产品化操作。
 * 无参操作一键下发；带参操作（连接 WiFi / 切换节点）弹出表单。
 * 不展示任何裸 action 名（裸名仅在命令详情的高级信息里出现）。
 */
function DeviceOperations({
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
  const groups = useMemo(() => actionsByCategory(), [])
  const [busyAction, setBusyAction] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<RunResult | null>(null)
  const [formEntry, setFormEntry] = useState<ActionCatalogEntry | null>(null)

  async function run(entry: ActionCatalogEntry, params: Record<string, unknown>) {
    setError(null)
    setResult(null)
    setBusyAction(entry.action)
    try {
      const { command, delivery_hint } = await commandApi.create(deviceId, entry.action, params)
      setResult({ commandId: command.id, hint: delivery_hint, title: entry.title })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : entry.failureHint)
    } finally {
      setBusyAction(null)
    }
  }

  function onClick(entry: ActionCatalogEntry) {
    if (entry.fields && entry.fields.length > 0) {
      setFormEntry(entry)
    } else {
      void run(entry, {})
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>设备操作</CardTitle>
        <p className="text-sm text-muted-foreground">对设备下发受控操作；操作经 agent 执行，控制面不直连设备。</p>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {!enabled ? (
          <Callout tone="warning" title="设备未启用">设备需处于「已启用」状态才能下发操作。</Callout>
        ) : !online ? (
          <Callout tone="info" title="设备当前离线">操作仍可下发，会进入队列，待设备重新连接或 Pull 后执行。</Callout>
        ) : null}

        {error && <Callout tone="error">{error}</Callout>}
        {result && (
          <Callout
            tone="success"
            title={`已下发：${result.title}`}
            action={
              <Button size="sm" variant="outline" onClick={() => onView(result.commandId)}>
                查看
              </Button>
            }
          >
            {hintLabels[result.hint] ?? '命令已创建。'}
          </Callout>
        )}

        {groups.map(({ category, entries }) => (
          <div key={category.key} className="space-y-2.5">
            <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <category.icon className="h-4 w-4" aria-hidden />
              {category.label}
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {entries.map((entry) => (
                <OperationButton
                  key={entry.action}
                  entry={entry}
                  disabled={!enabled || busyAction !== null}
                  loading={busyAction === entry.action}
                  onClick={() => onClick(entry)}
                />
              ))}
            </div>
          </div>
        ))}
      </CardContent>

      <OperationFormDialog
        entry={formEntry}
        onClose={() => setFormEntry(null)}
        onSubmit={async (params) => {
          const entry = formEntry
          setFormEntry(null)
          if (entry) await run(entry, params)
        }}
      />
    </Card>
  )
}

/** 单个操作按钮：图标 + 标题 + 说明 + 风险标记。 */
function OperationButton({
  entry,
  disabled,
  loading,
  onClick,
}: {
  entry: ActionCatalogEntry
  disabled: boolean
  loading: boolean
  onClick: () => void
}) {
  const Icon = entry.icon
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="group flex items-start gap-3 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-muted/50 disabled:pointer-events-none disabled:opacity-50"
    >
      <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground group-hover:text-primary">
        {loading ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> : <Icon className="h-4 w-4" aria-hidden />}
      </span>
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="text-sm font-medium">{entry.title}</span>
          {entry.riskLevel === 'safe' && <Badge tone="neutral" className="px-1.5 py-0 text-[10px]">只读</Badge>}
        </div>
        <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{entry.description}</p>
      </div>
    </button>
  )
}

/** 带参操作的表单弹窗（连接 WiFi / 切换节点）。 */
function OperationFormDialog({
  entry,
  onClose,
  onSubmit,
}: {
  entry: ActionCatalogEntry | null
  onClose: () => void
  onSubmit: (params: Record<string, unknown>) => void
}) {
  const [values, setValues] = useState<Record<string, string>>({})

  useEffect(() => {
    setValues({})
  }, [entry])

  if (!entry) return null
  const fields = entry.fields ?? []

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const params: Record<string, unknown> = {}
    for (const f of fields) {
      const v = (values[f.name] ?? '').trim()
      if (v) params[f.name] = v
    }
    onSubmit(params)
  }

  const canSubmit = fields.every((f) => !f.required || (values[f.name] ?? '').trim() !== '')

  return (
    <Dialog open={entry !== null} onClose={onClose} title={entry.title} description={entry.description}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        {fields.map((f) => (
          <div key={f.name} className="flex flex-col gap-1.5">
            <Label htmlFor={`op-${f.name}`}>{f.label}</Label>
            <Input
              id={`op-${f.name}`}
              autoFocus
              value={values[f.name] ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, [f.name]: e.target.value }))}
              placeholder={f.placeholder}
              required={f.required}
            />
            {f.help && <p className="text-xs text-muted-foreground">{f.help}</p>}
          </div>
        ))}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            下发
          </Button>
        </div>
      </form>
    </Dialog>
  )
}

/**
 * 最新状态：把顶层标量字段以「键 : 值」陈列（比裸 JSON 更易读），
 * 完整 JSON 收进高级详情。无上报时显示占位。
 */
function LatestStatus({ detail }: { detail: DeviceDetail }) {
  if (detail.latest_status == null) {
    return <p className="text-sm text-muted-foreground">设备尚未上报状态。</p>
  }
  const source = detail.latest_status_source === 'pull' ? 'HTTPS Pull' : 'WebSocket'
  const scalars = topLevelScalars(detail.latest_status)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
        <span>来源：{source}</span>
        <span>上报：{formatDateTime(detail.latest_status_reported_at)}</span>
        <span>接收：{formatDateTime(detail.latest_status_received_at)}</span>
      </div>
      {scalars.length > 0 ? (
        <dl className="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
          {scalars.map(([k, v]) => (
            <InfoRow key={k} label={k}>{v}</InfoRow>
          ))}
        </dl>
      ) : (
        <p className="text-sm text-muted-foreground">状态已上报，详情见下方「高级详情」。</p>
      )}
    </div>
  )
}

/** 提取对象顶层的标量字段（string/number/boolean）以友好展示。 */
function topLevelScalars(value: unknown): [string, string][] {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return []
  const out: [string, string][] = []
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    if (v == null) continue
    const t = typeof v
    if (t === 'string' || t === 'number' || t === 'boolean') {
      out.push([k, String(v)])
    }
  }
  return out.slice(0, 12)
}

/** 高级详情：折叠的工程信息（ID、凭证、时间线、原始状态 JSON）。 */
function AdvancedDetails({ detail }: { detail: DeviceDetail }) {
  const [open, setOpen] = useState(false)
  const { device } = detail
  return (
    <Card>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between px-6 py-4 text-left"
      >
        <span className="text-base font-semibold">高级详情</span>
        <ChevronDown className={'h-4 w-4 text-muted-foreground transition-transform ' + (open ? 'rotate-180' : '')} aria-hidden />
      </button>
      {open && (
        <div className="border-t border-border px-6 py-5">
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <InfoRow label="device_id" mono>{device.id}</InfoRow>
            <InfoRow label="有效凭证数">{detail.credential.active_count}</InfoRow>
            <InfoRow label="设备码状态">{device.status}</InfoRow>
            <InfoRow label="固件信息">{device.firmware_info || '—'}</InfoRow>
            <InfoRow label="最近 WS 连接">{formatDateTime(device.last_ws_connected_at)}</InfoRow>
            <InfoRow label="最近 WS 断开">{formatDateTime(device.last_ws_disconnected_at)}</InfoRow>
            <InfoRow label="最近 Pull 状态">{formatDateTime(device.last_pull_status_at)}</InfoRow>
            <InfoRow label="最近绑定">{formatDateTime(device.last_enrolled_at)}</InfoRow>
            <InfoRow label="创建时间">{formatDateTime(device.created_at)}</InfoRow>
            <InfoRow label="更新时间">{formatDateTime(device.updated_at)}</InfoRow>
          </dl>
          {detail.latest_status != null && (
            <div className="mt-5">
              <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">原始状态 JSON</p>
              <pre className="max-h-96 overflow-auto scrollbar-thin rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
                <code>{JSON.stringify(detail.latest_status, null, 2)}</code>
              </pre>
            </div>
          )}
        </div>
      )}
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

function InfoRow({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className={mono ? 'mt-1 break-all font-mono text-sm' : 'mt-1 text-sm'}>{children}</dd>
    </div>
  )
}

/** 设备码一次性展示弹窗（明文仅在内存，关闭即清除）。 */
function PairingCodeDialog({ result, onClose }: { result: PairingCodeResult | null; onClose: () => void }) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    if (!result) return
    try {
      await navigator.clipboard.writeText(result.device_code)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* 复制失败静默：用户仍可手动选择文本复制。 */
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
          <Callout tone="warning">
            <span className="inline-flex items-start gap-1.5">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
              设备码只展示这一次，包含一次性 pairing token（不含永久密钥），绑定成功后立即失效。
            </span>
          </Callout>

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
