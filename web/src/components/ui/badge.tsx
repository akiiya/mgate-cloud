import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export type BadgeTone = 'success' | 'neutral' | 'primary' | 'warning' | 'danger' | 'info'

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone
}

// 全部走语义色变量（globals.css 中 light/dark 两套），不再写死 amber 等字面色，
// 这样暗色模式下徽标对比度也正确。
const toneClasses: Record<BadgeTone, string> = {
  success: 'bg-success/10 text-success ring-1 ring-inset ring-success/25',
  neutral: 'bg-muted text-muted-foreground ring-1 ring-inset ring-border',
  primary: 'bg-primary/10 text-primary ring-1 ring-inset ring-primary/25',
  warning: 'bg-warning/10 text-warning ring-1 ring-inset ring-warning/25',
  danger: 'bg-destructive/10 text-destructive ring-1 ring-inset ring-destructive/25',
  info: 'bg-info/10 text-info ring-1 ring-inset ring-info/25',
}

/** 状态徽标：用小色块表达“运行中”“已启用”等状态。 */
export function Badge({ className, tone = 'neutral', ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
        toneClasses[tone],
        className,
      )}
      {...props}
    />
  )
}
