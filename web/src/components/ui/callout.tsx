import { type ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Info, XCircle, type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/cn'

export type CalloutTone = 'success' | 'warning' | 'error' | 'info'

const toneMap: Record<CalloutTone, { icon: LucideIcon; cls: string; iconCls: string }> = {
  success: { icon: CheckCircle2, cls: 'border-success/25 bg-success/5 text-foreground', iconCls: 'text-success' },
  warning: { icon: AlertTriangle, cls: 'border-warning/25 bg-warning/5 text-foreground', iconCls: 'text-warning' },
  error: { icon: XCircle, cls: 'border-destructive/25 bg-destructive/5 text-foreground', iconCls: 'text-destructive' },
  info: { icon: Info, cls: 'border-info/25 bg-info/5 text-foreground', iconCls: 'text-info' },
}

/**
 * 统一信息条：成功 / 警告 / 错误 / 提示四种语义，全站共用。
 * 比裸文本更醒目，又不像整块卡片那么重。
 */
export function Callout({
  tone = 'info',
  title,
  children,
  action,
  className,
}: {
  tone?: CalloutTone
  title?: string
  children?: ReactNode
  action?: ReactNode
  className?: string
}) {
  const { icon: Icon, cls, iconCls } = toneMap[tone]
  return (
    <div className={cn('flex items-start gap-3 rounded-lg border px-4 py-3 text-sm', cls, className)}>
      <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', iconCls)} aria-hidden />
      <div className="min-w-0 flex-1">
        {title && <p className="font-medium">{title}</p>}
        {children && <div className={cn(title ? 'mt-0.5' : '', 'text-muted-foreground')}>{children}</div>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}
