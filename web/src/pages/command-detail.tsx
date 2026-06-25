import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Ban, Loader2 } from 'lucide-react'
import { commandApi, isTerminalStatus, type CommandDetail, type CommandStatus } from '@/api/commands'
import { ApiError } from '@/api/client'
import { commandStatusMeta } from '@/lib/command-status'
import { formatDateTime } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ErrorAlert } from '@/components/ui/alert'

/** 命令详情页：基础信息、时间线、参数与执行结果回放。 */
export function CommandDetailPage() {
  const { commandId = '' } = useParams()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<CommandDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
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
      <div className="flex items-center justify-center py-20 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin" aria-hidden />
      </div>
    )
  }
  if (error || !detail) {
    return (
      <div className="flex flex-col gap-4">
        <BackButton onClick={() => navigate('/commands')} />
        <ErrorAlert message={error ?? '命令不存在'} />
      </div>
    )
  }

  const { command, result } = detail
  const meta = commandStatusMeta(command.status as CommandStatus)
  const cancelable = !isTerminalStatus(command.status)

  return (
    <div className="flex flex-col gap-6">
      <BackButton onClick={() => navigate('/commands')} />

      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="font-mono text-xl font-semibold tracking-tight">{command.action}</h1>
            <Badge tone={meta.tone}>{meta.label}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">命令 {command.id}</p>
        </div>
        {cancelable && (
          <Button variant="outline" loading={busy} onClick={handleCancel}>
            <Ban className="h-4 w-4" aria-hidden />
            取消命令
          </Button>
        )}
      </div>

      {actionError && <ErrorAlert message={actionError} />}

      {/* 基础信息 + 时间线 */}
      <Card>
        <CardHeader>
          <CardTitle>命令信息</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <InfoRow label="设备">{command.device_name || command.device_id}</InfoRow>
            <InfoRow label="超时(秒)">{command.timeout_sec}</InfoRow>
            <InfoRow label="尝试次数">{command.attempts} / {command.max_attempts}</InfoRow>
            <InfoRow label="投递通道">{command.leased_by || '—'}</InfoRow>
            <InfoRow label="租约至">{formatDateTime(command.lease_until)}</InfoRow>
            <InfoRow label="过期时间">{formatDateTime(command.expires_at)}</InfoRow>
            <InfoRow label="创建时间">{formatDateTime(command.created_at)}</InfoRow>
            <InfoRow label="投递时间">{formatDateTime(command.sent_at)}</InfoRow>
            <InfoRow label="确认时间">{formatDateTime(command.acked_at)}</InfoRow>
            <InfoRow label="开始时间">{formatDateTime(command.started_at)}</InfoRow>
            <InfoRow label="完成时间">{formatDateTime(command.finished_at)}</InfoRow>
          </dl>
          {command.last_error && (
            <p className="mt-3 text-xs text-muted-foreground">最近错误：{command.last_error}</p>
          )}
          <div className="mt-4">
            <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">参数</p>
            <JsonBlock value={command.params} />
          </div>
        </CardContent>
      </Card>

      {/* 执行结果 */}
      <Card>
        <CardHeader>
          <CardTitle>执行结果</CardTitle>
        </CardHeader>
        <CardContent>
          {result == null ? (
            <p className="text-sm text-muted-foreground">暂无结果</p>
          ) : (
            <div className="flex flex-col gap-4">
              <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
                <InfoRow label="结果状态">{result.status}</InfoRow>
                <InfoRow label="退出码">{result.exit_code ?? '—'}</InfoRow>
                <InfoRow label="接收时间">{formatDateTime(result.received_at)}</InfoRow>
                <InfoRow label="是否截断">{result.truncated ? '是' : '否'}</InfoRow>
              </dl>
              {result.error_message && <ErrorAlert message={result.error_message} />}
              <OutputBlock label="stdout" text={result.stdout} />
              <OutputBlock label="stderr" text={result.stderr} />
              <div>
                <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">result</p>
                <JsonBlock value={result.result} />
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
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

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-sm">{children}</dd>
    </div>
  )
}

/**
 * JSON 展示块。用 JSON.stringify 渲染为文本并放入 <pre>，天然避免 XSS
 * （React 文本节点不会执行 HTML）。限高滚动避免撑爆页面。
 */
function JsonBlock({ value }: { value: unknown }) {
  const text = value == null ? 'null' : JSON.stringify(value, null, 2)
  return (
    <pre className="max-h-72 overflow-auto rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
      <code>{text}</code>
    </pre>
  )
}

/** 文本输出块（stdout/stderr）。空内容显示占位。 */
function OutputBlock({ label, text }: { label: string; text: string }) {
  return (
    <div>
      <p className="mb-1 text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      {text ? (
        <pre className="max-h-72 overflow-auto rounded-md border border-border bg-muted/50 p-3 text-xs leading-relaxed">
          <code>{text}</code>
        </pre>
      ) : (
        <p className="text-sm text-muted-foreground">（空）</p>
      )}
    </div>
  )
}
