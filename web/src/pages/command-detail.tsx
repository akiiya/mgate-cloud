import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Ban, ChevronDown, Loader2, RefreshCw } from 'lucide-react'
import { commandApi, isTerminalStatus, type CommandDetail } from '@/api/commands'
import { ApiError } from '@/api/client'
import { commandStatusMeta } from '@/lib/command-status'
import { getAction } from '@/lib/action-catalog'
import { presentCommand } from '@/lib/command-presenter'
import { formatDateTime } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'

/** 命令详情页：人话结果 + 概要 + 折叠的工程详情。 */
export function CommandDetailPage() {
  const { commandId = '' } = useParams()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<CommandDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setError(null)
    try {
      setDetail(await commandApi.get(commandId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载命令失败')
    } finally {
      setLoading(false)
    }
  }, [commandId])

  useEffect(() => {
    setLoading(true)
    void load()
  }, [load])

  async function handleCancel() {
    setActionError(null)
    setBusy(true)
    try {
      await commandApi.cancel(commandId)
      await load()
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : '取消失败')
    } finally {
      setBusy(false)
    }
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
        <BackButton onClick={() => navigate('/commands')} />
        <Callout tone="error" title="无法加载命令">{error ?? '命令不存在'}</Callout>
      </div>
    )
  }

  const { command, result } = detail
  const meta = commandStatusMeta(command.status)
  const entry = getAction(command.action)
  const presentation = presentCommand(command, result)
  const cancelable = !isTerminalStatus(command.status)

  return (
    <div className="flex flex-col gap-6">
      <BackButton onClick={() => navigate('/commands')} />

      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">{entry.title}</h1>
            <Badge tone={meta.tone}>{meta.label}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{entry.description}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCw className="h-4 w-4" aria-hidden />
            刷新
          </Button>
          {cancelable && (
            <Button variant="outline" loading={busy} onClick={handleCancel}>
              <Ban className="h-4 w-4" aria-hidden />
              取消命令
            </Button>
          )}
        </div>
      </div>

      {actionError && <Callout tone="error">{actionError}</Callout>}

      {/* 人话结果。severity 直接就是 Callout 的语义色（success/warning/error/info）。 */}
      <Callout tone={presentation.severity} title={presentation.title}>
        <p>{presentation.summary}</p>
        {presentation.suggestedAction && <p className="mt-1">建议：{presentation.suggestedAction}</p>}
      </Callout>

      {/* 概要 */}
      <Card>
        <CardHeader>
          <CardTitle>概要</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
            <InfoRow label="设备">{command.device_name || command.device_id}</InfoRow>
            <InfoRow label="创建时间">{formatDateTime(command.created_at)}</InfoRow>
            <InfoRow label="完成时间">{formatDateTime(command.finished_at)}</InfoRow>
          </dl>
        </CardContent>
      </Card>

      {/* 工程详情（折叠） */}
      <AdvancedCommand detail={detail} />
    </div>
  )
}

/** 折叠的工程详情：原始 action、参数、stdout/stderr/result、完整时间线。 */
function AdvancedCommand({ detail }: { detail: CommandDetail }) {
  const [open, setOpen] = useState(false)
  const { command, result } = detail
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
        <div className="space-y-5 border-t border-border px-6 py-5">
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
            <InfoRow label="action" mono>{command.action}</InfoRow>
            <InfoRow label="command_id" mono>{command.id}</InfoRow>
            <InfoRow label="超时(秒)">{command.timeout_sec}</InfoRow>
            <InfoRow label="尝试次数">{command.attempts} / {command.max_attempts}</InfoRow>
            <InfoRow label="投递通道">{command.leased_by || '—'}</InfoRow>
            <InfoRow label="租约至">{formatDateTime(command.lease_until)}</InfoRow>
            <InfoRow label="过期时间">{formatDateTime(command.expires_at)}</InfoRow>
            <InfoRow label="投递时间">{formatDateTime(command.sent_at)}</InfoRow>
            <InfoRow label="确认时间">{formatDateTime(command.acked_at)}</InfoRow>
            <InfoRow label="开始时间">{formatDateTime(command.started_at)}</InfoRow>
            <InfoRow label="完成时间">{formatDateTime(command.finished_at)}</InfoRow>
          </dl>
          {command.last_error && <p className="text-xs text-muted-foreground">最近错误：{command.last_error}</p>}

          <div>
            <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">参数</p>
            <JsonBlock value={command.params} />
          </div>

          {result == null ? (
            <p className="text-sm text-muted-foreground">暂无执行结果。</p>
          ) : (
            <div className="space-y-4">
              <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
                <InfoRow label="结果状态" mono>{result.status}</InfoRow>
                <InfoRow label="退出码">{result.exit_code ?? '—'}</InfoRow>
                <InfoRow label="接收时间">{formatDateTime(result.received_at)}</InfoRow>
                <InfoRow label="是否截断">{result.truncated ? '是' : '否'}</InfoRow>
              </dl>
              {result.error_message && <Callout tone="error">{result.error_message}</Callout>}
              <OutputBlock label="stdout" text={result.stdout} />
              <OutputBlock label="stderr" text={result.stderr} />
              <div>
                <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">result</p>
                <JsonBlock value={result.result} />
              </div>
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
      返回命令记录
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

function JsonBlock({ value }: { value: unknown }) {
  const text = value == null ? 'null' : JSON.stringify(value, null, 2)
  return (
    <pre className="max-h-72 overflow-auto scrollbar-thin rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
      <code>{text}</code>
    </pre>
  )
}

function OutputBlock({ label, text }: { label: string; text: string }) {
  return (
    <div>
      <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      {text ? (
        <pre className="max-h-72 overflow-auto scrollbar-thin rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
          <code>{text}</code>
        </pre>
      ) : (
        <p className="text-sm text-muted-foreground">（空）</p>
      )}
    </div>
  )
}
