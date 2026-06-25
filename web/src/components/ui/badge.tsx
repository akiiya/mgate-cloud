import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

type Tone = 'success' | 'neutral' | 'primary' | 'warning' | 'danger'

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: Tone
}

const toneClasses: Record<Tone, string> = {
  success: 'bg-success/10 text-success ring-1 ring-inset ring-success/20',
  neutral: 'bg-muted text-muted-foreground ring-1 ring-inset ring-border',
  primary: 'bg-primary/10 text-primary ring-1 ring-inset ring-primary/20',
  warning: 'bg-amber-500/10 text-amber-600 ring-1 ring-inset ring-amber-500/20',
  danger: 'bg-destructive/10 text-destructive ring-1 ring-inset ring-destructive/20',
}

/** 状态徽标：用小色块表达"运行中""已启用"等状态。 */
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
